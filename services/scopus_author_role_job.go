package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"fund-management-api/models"

	"gorm.io/gorm"
)

const authorRoleJobTimeout = 2 * time.Hour

type ScopusAuthorRoleCoverage struct {
	Eligible             int `json:"eligible"`
	Complete             int `json:"complete"`
	NoCorrespondence     int `json:"no_correspondence"`
	NeedsReview          int `json:"needs_review"`
	FetchError           int `json:"fetch_error"`
	Pending              int `json:"pending"`
	NextBackfillRequests int `json:"next_backfill_requests"`
	RetryReviewRequests  int `json:"retry_review_requests"`
	RefreshRequests      int `json:"refresh_requests"`
}

// Coverage counts distinct faculty-linked documents, including documents
// whose authors outside the faculty also have stored roles.
func (s *ScopusAuthorRoleService) Coverage(ctx context.Context) (*ScopusAuthorRoleCoverage, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("scopus author role service has no database")
	}
	var rows []struct {
		Status string `gorm:"column:status"`
		Count  int    `gorm:"column:count"`
	}
	err := facultyDocumentQuery(s.db.WithContext(ctx)).
		Select("COALESCE(author_role_status, '') AS status, COUNT(*) AS count").
		Group("author_role_status").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	coverage := &ScopusAuthorRoleCoverage{}
	for _, row := range rows {
		coverage.Eligible += row.Count
		switch row.Status {
		case authorRoleComplete:
			coverage.Complete += row.Count
		case authorRoleNoCorrespondence:
			coverage.NoCorrespondence += row.Count
		case authorRoleNeedsReview:
			coverage.NeedsReview += row.Count
		case authorRoleFetchError:
			coverage.FetchError += row.Count
		default:
			coverage.Pending += row.Count
		}
	}
	var unchecked int64
	if err := facultyDocumentQuery(s.db.WithContext(ctx)).
		Where("author_role_checked_at IS NULL").Count(&unchecked).Error; err != nil {
		return nil, err
	}
	coverage.NextBackfillRequests = int(unchecked)
	coverage.RetryReviewRequests = coverage.NeedsReview
	coverage.RefreshRequests = coverage.Eligible
	return coverage, nil
}

// Recover a web run left running after the API process stopped. Progress
// refreshes updated_at after every document; one HTTP request has a 35s limit.
func (s *ScopusAuthorRoleService) recoverStaleRoleRuns(ctx context.Context) error {
	return s.db.WithContext(ctx).Model(&models.ScopusAuthorRoleRun{}).
		Where("status = ? AND updated_at < ?", "running", time.Now().Add(-authorRoleJobTimeout)).
		Updates(map[string]interface{}{
			"status":        "failed",
			"active_slot":   nil,
			"finished_at":   time.Now(),
			"error_message": "API process stopped before the run finished",
		}).Error
}

func (s *ScopusAuthorRoleService) ActiveRun(ctx context.Context) (*models.ScopusAuthorRoleRun, error) {
	if err := s.recoverStaleRoleRuns(ctx); err != nil {
		return nil, err
	}
	var run models.ScopusAuthorRoleRun
	err := s.db.WithContext(ctx).Where("active_slot = ?", 1).First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *ScopusAuthorRoleService) ListRuns(ctx context.Context, page, perPage int) ([]models.ScopusAuthorRoleRun, int64, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&models.ScopusAuthorRoleRun{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var runs []models.ScopusAuthorRoleRun
	err := s.db.WithContext(ctx).Order("started_at DESC, id DESC").
		Offset((page - 1) * perPage).Limit(perPage).Find(&runs).Error
	return runs, total, err
}

// StartRun reserves the only active slot before the HTTP request returns 202.
// The unique index also guards simultaneous requests on different API servers.
func (s *ScopusAuthorRoleService) StartRun(ctx context.Context, runType string) (*models.ScopusAuthorRoleRun, error) {
	if runType != "backfill" && runType != "retry_review" && runType != "refresh" {
		return nil, fmt.Errorf("invalid author role run type %q", runType)
	}
	if err := s.recoverStaleRoleRuns(ctx); err != nil {
		return nil, err
	}
	slot := 1
	run := &models.ScopusAuthorRoleRun{RunType: runType, Status: "running", ActiveSlot: &slot, StartedAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		if active, checkErr := s.ActiveRun(ctx); checkErr == nil && active != nil {
			return nil, ErrScopusAuthorRoleRunActive
		}
		return nil, err
	}
	return run, nil
}

// ExecuteRun runs independently of the HTTP request and persists progress.
func (s *ScopusAuthorRoleService) ExecuteRun(ctx context.Context, run *models.ScopusAuthorRoleRun) {
	if run == nil {
		return
	}
	refresh := run.RunType == "refresh"
	reviewOnly := run.RunType == "retry_review"
	progress := func(summary ScopusAuthorRoleSummary) {
		if err := s.db.WithContext(ctx).Model(run).Updates(roleRunCounts(summary)).Error; err != nil {
			log.Printf("scopus author roles: run %d progress update failed: %v", run.ID, err)
		}
	}
	summary, runErr := s.BackfillWithProgress(ctx, 0, refresh, reviewOnly, progress)
	status := "success"
	if runErr != nil {
		status = "failed"
	} else if summary != nil && (summary.Failed > 0 || summary.NeedsReview > 0) {
		status = "partial"
	}
	updates := map[string]interface{}{
		"status": status, "active_slot": nil, "finished_at": time.Now(),
	}
	if summary != nil {
		for key, value := range roleRunCounts(*summary) {
			updates[key] = value
		}
	}
	if runErr != nil {
		updates["error_message"] = runErr.Error()
		log.Printf("scopus author roles: run %d failed: %v", run.ID, runErr)
	}
	finalCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.db.WithContext(finalCtx).Model(run).Updates(updates).Error; err != nil {
		log.Printf("scopus author roles: run %d final update failed: %v", run.ID, err)
	}
}

func roleRunCounts(summary ScopusAuthorRoleSummary) map[string]interface{} {
	return map[string]interface{}{
		"eligible": summary.Eligible, "selected": summary.Selected,
		"skipped_existing": summary.SkippedExisting, "fetched": summary.Fetched,
		"complete": summary.Complete, "no_correspondence": summary.NoCorrespondence,
		"needs_review": summary.NeedsReview, "failed": summary.Failed,
	}
}
