package services

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

const (
	thaiJOBatchImportRunLockName = "thaijo_batch_import_job_lock"
	thaiJOBatchRunFinalizeTimeout = 10 * time.Second
	thaiJOBatchRunStaleAfter = 2 * time.Hour
)

var ErrThaiJOBatchImportAlreadyRunning = errors.New("thaijo batch import already running")

type ThaiJOIngestJobService struct {
	db     *gorm.DB
	ingest *ThaiJOIngestService
}

type ThaiJOIngestAllInput struct {
	UserIDs []uint
	Limit   int
}

type ThaiJOIngestJobSummary struct {
	UsersProcessed      int `json:"users_processed"`
	UsersWithErrors     int `json:"users_with_errors"`
	DocumentsFetched    int `json:"documents_fetched"`
	DocumentsCreated    int `json:"documents_created"`
	DocumentsUpdated    int `json:"documents_updated"`
	DocumentsFailed     int `json:"documents_failed"`
	AuthorsCreated      int `json:"authors_created"`
	AuthorsUpdated      int `json:"authors_updated"`
	JournalsCreated     int `json:"journals_created"`
	JournalsUpdated     int `json:"journals_updated"`
	LinksInserted       int `json:"links_inserted"`
	LinksUpdated        int `json:"links_updated"`
	RejectedHits        int `json:"rejected_hits"`
}

func NewThaiJOIngestJobService(db *gorm.DB) *ThaiJOIngestJobService {
	if db == nil {
		db = config.DB
	}
	return &ThaiJOIngestJobService{db: db, ingest: NewThaiJOIngestService(db, nil)}
}

func (s *ThaiJOIngestJobService) GetActiveBatchRun(ctx context.Context) (*models.ThaiJOBatchImportRun, error) {
	var run models.ThaiJOBatchImportRun
	err := s.db.WithContext(ctx).
		Where("status = ?", "running").
		Order("started_at DESC").
		First(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if time.Since(run.StartedAt) > thaiJOBatchRunStaleAfter {
		errMsg := "stale timeout: marked as failed after exceeding running time limit"
		updates := map[string]interface{}{
			"status":        "failed",
			"error_message": &errMsg,
			"finished_at":   time.Now(),
		}
		if err := s.db.WithContext(ctx).Model(&run).Updates(updates).Error; err != nil {
			return nil, err
		}
		return nil, nil
	}

	return &run, nil
}

func (s *ThaiJOIngestJobService) RunForUser(ctx context.Context, userID uint) (*ThaiJOIngestResult, error) {
	if userID == 0 {
		return nil, errors.New("user id is required")
	}
	return s.ingest.RunForUser(ctx, &ThaiJOIngestUserInput{UserID: userID})
}

func (s *ThaiJOIngestJobService) RunForAll(ctx context.Context, input *ThaiJOIngestAllInput) (*ThaiJOIngestJobSummary, error) {
	if input == nil {
		input = &ThaiJOIngestAllInput{}
	}

	release, err := s.acquireBatchRunLock(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := release(); err != nil {
			log.Printf("failed to release thaijo batch import lock: %v", err)
		}
	}()

	summary := &ThaiJOIngestJobSummary{}
	run := &models.ThaiJOBatchImportRun{Status: "running", StartedAt: time.Now()}
	if len(input.UserIDs) > 0 {
		parts := make([]string, 0, len(input.UserIDs))
		for _, id := range input.UserIDs {
			parts = append(parts, strconv.FormatUint(uint64(id), 10))
		}
		csv := strings.Join(parts, ",")
		run.RequestedUserIDs = &csv
	}
	if input.Limit > 0 {
		limit := input.Limit
		run.LimitCount = &limit
	}

	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return nil, err
	}

	startedAt := time.Now()
	var runErr error
	defer func() {
		status := "success"
		if runErr != nil {
			status = "failed"
		}

		updates := map[string]interface{}{
			"status":            status,
			"finished_at":       time.Now(),
			"duration_seconds":  time.Since(startedAt).Seconds(),
			"users_processed":   summary.UsersProcessed,
			"users_with_errors": summary.UsersWithErrors,
			"documents_fetched": summary.DocumentsFetched,
			"documents_created": summary.DocumentsCreated,
			"documents_updated": summary.DocumentsUpdated,
			"documents_failed":  summary.DocumentsFailed,
			"authors_created":   summary.AuthorsCreated,
			"authors_updated":   summary.AuthorsUpdated,
			"journals_created":  summary.JournalsCreated,
			"journals_updated":  summary.JournalsUpdated,
			"links_inserted":    summary.LinksInserted,
			"links_updated":     summary.LinksUpdated,
			"rejected_hits":     summary.RejectedHits,
		}
		if runErr != nil {
			errMsg := runErr.Error()
			updates["error_message"] = &errMsg
		}

		finalizeCtx, cancel := context.WithTimeout(context.Background(), thaiJOBatchRunFinalizeTimeout)
		defer cancel()
		if err := s.db.WithContext(finalizeCtx).Model(run).Updates(updates).Error; err != nil {
			log.Printf("failed to update thaijo batch run %d: %v", run.ID, err)
		}
	}()

	type userRow struct {
		UserID uint
	}

	query := s.db.WithContext(ctx).Table("users").
		Select("user_id").
		Where("thaijo_sync_enabled = 1")
	if len(input.UserIDs) > 0 {
		query = query.Where("user_id IN ?", input.UserIDs)
	}
	if input.Limit > 0 {
		query = query.Limit(input.Limit)
	}

	var users []userRow
	if err := query.Order("user_id ASC").Find(&users).Error; err != nil {
		runErr = err
		return nil, err
	}

	for _, u := range users {
		res, err := s.ingest.RunForUser(ctx, &ThaiJOIngestUserInput{UserID: u.UserID})
		if err != nil {
			summary.UsersWithErrors++
			log.Printf("thaijo ingest failed for user %d: %v", u.UserID, err)
			continue
		}
		summary.UsersProcessed++
		summary.DocumentsFetched += res.DocumentsFetched
		summary.DocumentsCreated += res.DocumentsCreated
		summary.DocumentsUpdated += res.DocumentsUpdated
		summary.DocumentsFailed += res.DocumentsFailed
		summary.AuthorsCreated += res.AuthorsCreated
		summary.AuthorsUpdated += res.AuthorsUpdated
		summary.JournalsCreated += res.JournalsCreated
		summary.JournalsUpdated += res.JournalsUpdated
		summary.LinksInserted += res.DocumentAuthorsInserted
		summary.LinksUpdated += res.DocumentAuthorsUpdated
		summary.RejectedHits += res.RejectedHits
	}

	return summary, nil
}

func (s *ThaiJOIngestJobService) acquireBatchRunLock(ctx context.Context) (func() error, error) {
	lockCtx := persistentContext(ctx)
	var ok int
	if err := s.db.WithContext(lockCtx).Raw("SELECT GET_LOCK(?, 0)", thaiJOBatchImportRunLockName).Scan(&ok).Error; err != nil {
		return nil, err
	}
	if ok != 1 {
		return nil, ErrThaiJOBatchImportAlreadyRunning
	}
	return func() error {
		var released int
		if err := s.db.WithContext(lockCtx).Raw("SELECT RELEASE_LOCK(?)", thaiJOBatchImportRunLockName).Scan(&released).Error; err != nil {
			return err
		}
		if released != 1 {
			return errors.New("release thaijo batch lock failed")
		}
		return nil
	}, nil
}
