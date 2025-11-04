package services

import (
	"context"

	"fund-management-api/config"

	"gorm.io/gorm"
)

type CpProfileImportSummary struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Failed  int `json:"failed"`
}

type CpProfileService struct {
	db  *gorm.DB
	job *KkuPeopleImportJobService
}

func NewCpProfileService(db *gorm.DB) *CpProfileService {
	if db == nil {
		db = config.DB
	}
	return &CpProfileService{db: db, job: NewKkuPeopleImportJobService(db)}
}

// Import runs the KKU people scraper and upserts results into cp_profile.
func (s *CpProfileService) Import(ctx context.Context, debug bool) (*CpProfileImportSummary, error) {
	job := s.job
	if job == nil {
		job = NewKkuPeopleImportJobService(s.db)
	}

	summary, _, err := job.Run(ctx, &KkuPeopleImportInput{
		DryRun:        false,
		Debug:         debug,
		TriggerSource: "cp_profile_service",
		LockName:      "kku_people_import_job",
		RecordRun:     true,
	})
	if err != nil {
		return nil, err
	}

	return &CpProfileImportSummary{
		Total:   summary.FetchedCount,
		Created: summary.CreatedCount,
		Updated: summary.UpdatedCount,
		Failed:  summary.FailedCount,
	}, nil
}
