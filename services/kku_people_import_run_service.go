package services

import (
	"errors"
	"fmt"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"gorm.io/gorm"
)

var (
	ErrKkuPeopleImportRunNotFound = errors.New("kku people import run not found")
)

type KkuPeopleImportRunService struct {
	db *gorm.DB
}

func NewKkuPeopleImportRunService(db *gorm.DB) *KkuPeopleImportRunService {
	if db == nil {
		db = config.DB
	}
	return &KkuPeopleImportRunService{db: db}
}

func (s *KkuPeopleImportRunService) Start(trigger string, dryRun bool) (*models.KkuPeopleImportRun, error) {
	if trigger == "" {
		trigger = "unknown"
	}
	run := &models.KkuPeopleImportRun{
		TriggerSource: trigger,
		DryRun:        dryRun,
		Status:        models.KkuPeopleImportStatusRunning,
	}
	if err := s.db.Create(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (s *KkuPeopleImportRunService) MarkSuccess(runID uint, summary *KkuPeopleImportSummary, exitCode *int, stdout, stderr string, duration float64) error {
	return s.finish(runID, models.KkuPeopleImportStatusSuccess, summary, exitCode, stdout, stderr, nil, duration)
}

func (s *KkuPeopleImportRunService) MarkFailure(runID uint, summary *KkuPeopleImportSummary, exitCode *int, stdout, stderr string, err error, duration float64) error {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return s.finish(runID, models.KkuPeopleImportStatusFailed, summary, exitCode, stdout, stderr, &msg, duration)
}

func (s *KkuPeopleImportRunService) finish(runID uint, status string, summary *KkuPeopleImportSummary, exitCode *int, stdout, stderr string, errMsg *string, duration float64) error {
	updates := map[string]interface{}{
		"status":           status,
		"finished_at":      time.Now(),
		"duration_seconds": duration,
		"stdout":           truncateForLog(stdout),
		"stderr":           truncateForLog(stderr),
	}
	if exitCode != nil {
		updates["exit_code"] = *exitCode
	}
	if summary != nil {
		updates["fetched_count"] = summary.FetchedCount
		updates["created_count"] = summary.CreatedCount
		updates["updated_count"] = summary.UpdatedCount
		updates["failed_count"] = summary.FailedCount
	}
	if errMsg != nil {
		if len(*errMsg) > 2000 {
			truncated := fmt.Sprintf("%s...", (*errMsg)[:1997])
			updates["error_message"] = truncated
		} else {
			updates["error_message"] = *errMsg
		}
	}
	res := s.db.Model(&models.KkuPeopleImportRun{}).Where("id = ?", runID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrKkuPeopleImportRunNotFound
	}
	return nil
}

func (s *KkuPeopleImportRunService) GetByID(id uint) (*models.KkuPeopleImportRun, error) {
	var run models.KkuPeopleImportRun
	if err := s.db.Where("id = ?", id).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKkuPeopleImportRunNotFound
		}
		return nil, err
	}
	return &run, nil
}

func (s *KkuPeopleImportRunService) GetLatestCompleted() (*models.KkuPeopleImportRun, error) {
	var run models.KkuPeopleImportRun
	err := s.db.Where("status <> ?", models.KkuPeopleImportStatusRunning).
		Order("started_at DESC").
		Select("id, trigger_source, dry_run, status, error_message, started_at, finished_at, duration_seconds, fetched_count, created_count, updated_count, failed_count").
		First(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

func (s *KkuPeopleImportRunService) GetRunning() (*models.KkuPeopleImportRun, error) {
	var run models.KkuPeopleImportRun
	err := s.db.Where("status = ?", models.KkuPeopleImportStatusRunning).
		Order("started_at DESC").
		Select("id, trigger_source, dry_run, status, error_message, started_at, finished_at, duration_seconds, fetched_count, created_count, updated_count, failed_count").
		First(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

func (s *KkuPeopleImportRunService) List(limit, offset int) ([]models.KkuPeopleImportRun, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	if err := s.db.Model(&models.KkuPeopleImportRun{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var runs []models.KkuPeopleImportRun
	err := s.db.Order("started_at DESC").
		Offset(offset).
		Limit(limit).
		Select("id, trigger_source, dry_run, status, error_message, started_at, finished_at, duration_seconds, fetched_count, created_count, updated_count, failed_count").
		Find(&runs).Error
	if err != nil {
		return nil, 0, err
	}
	return runs, total, nil
}

func truncateForLog(s string) string {
	const maxLen = 100000
	if len(s) <= maxLen {
		return s
	}
	return fmt.Sprintf("%s...", s[:maxLen-3])
}
