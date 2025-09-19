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
	ErrScholarImportRunNotFound = errors.New("scholar import run not found")
)

type ScholarImportRunService struct {
	db *gorm.DB
}

func NewScholarImportRunService(db *gorm.DB) *ScholarImportRunService {
	if db == nil {
		db = config.DB
	}
	return &ScholarImportRunService{db: db}
}

func (s *ScholarImportRunService) Start(trigger string) (*models.ScholarImportRun, error) {
	if trigger == "" {
		trigger = "unknown"
	}
	run := &models.ScholarImportRun{
		TriggerSource: trigger,
		Status:        models.ScholarImportRunStatusRunning,
	}
	if err := s.db.Create(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (s *ScholarImportRunService) MarkSuccess(runID uint, summary *ScholarImportSummary) error {
	return s.finish(runID, models.ScholarImportRunStatusSuccess, summary, nil)
}

func (s *ScholarImportRunService) MarkFailure(runID uint, summary *ScholarImportSummary, err error) error {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return s.finish(runID, models.ScholarImportRunStatusFailed, summary, &msg)
}

func (s *ScholarImportRunService) finish(runID uint, status string, summary *ScholarImportSummary, errMsg *string) error {
	updates := map[string]interface{}{
		"status":      status,
		"finished_at": time.Now(),
	}
	if summary != nil {
		updates["users_processed"] = summary.UsersProcessed
		updates["users_with_errors"] = summary.UsersWithErrors
		updates["publications_fetched"] = summary.PublicationsFetched
		updates["publications_created"] = summary.PublicationsCreated
		updates["publications_updated"] = summary.PublicationsUpdated
		updates["publications_failed"] = summary.PublicationsFailed
	}
	if errMsg != nil {
		if len(*errMsg) > 1000 {
			truncated := fmt.Sprintf("%s...", (*errMsg)[:997])
			updates["error_message"] = truncated
		} else {
			updates["error_message"] = *errMsg
		}
	}
	res := s.db.Model(&models.ScholarImportRun{}).Where("id = ?", runID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrScholarImportRunNotFound
	}
	return nil
}
