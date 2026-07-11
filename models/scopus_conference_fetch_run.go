package models

import "time"

// ScopusConferenceFetchRun tracks a manual run that fetches conference event
// details (from the Scopus Abstract Retrieval API) into scopus_documents.
type ScopusConferenceFetchRun struct {
	ID               uint64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	RunType          string     `json:"run_type" gorm:"column:run_type;type:varchar(32);not null"`
	Status           string     `json:"status" gorm:"column:status;type:varchar(32);not null;default:'running'"`
	ErrorMessage     *string    `json:"error_message,omitempty" gorm:"column:error_message;type:text"`
	DocumentsScanned int        `json:"documents_scanned" gorm:"column:documents_scanned;not null;default:0"`
	DocumentsFetched int        `json:"documents_fetched" gorm:"column:documents_fetched;not null;default:0"`
	SkippedExisting  int        `json:"skipped_existing" gorm:"column:skipped_existing;not null;default:0"`
	DocumentsFailed  int        `json:"documents_failed" gorm:"column:documents_failed;not null;default:0"`
	StartedAt        time.Time  `json:"started_at" gorm:"column:started_at;autoCreateTime"`
	FinishedAt       *time.Time `json:"finished_at,omitempty" gorm:"column:finished_at"`
	DurationSeconds  *float64   `json:"duration_seconds,omitempty" gorm:"column:duration_seconds"`
	CreatedAt        time.Time  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt        time.Time  `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (ScopusConferenceFetchRun) TableName() string { return "scopus_conference_fetch_runs" }
