package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	KkuPeopleImportStatusRunning = "running"
	KkuPeopleImportStatusSuccess = "success"
	KkuPeopleImportStatusFailed  = "failed"
)

type KkuPeopleImportRun struct {
	ID            uint           `json:"run_id" gorm:"primaryKey;autoIncrement"`
	TriggerSource string         `json:"trigger_source" gorm:"type:varchar(64);not null"`
	DryRun        bool           `json:"dry_run" gorm:"column:dry_run;not null;default:false"`
	Status        string         `json:"status" gorm:"type:enum('running','success','failed');not null;default:'running'"`
	ErrorMessage  *string        `json:"error_message,omitempty" gorm:"type:text"`
	StartedAt     time.Time      `json:"started_at" gorm:"column:started_at;autoCreateTime"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty" gorm:"column:finished_at"`
	Duration      *float64       `json:"duration_seconds,omitempty" gorm:"column:duration_seconds"`
	FetchedCount  uint           `json:"fetched_count" gorm:"column:fetched_count;not null;default:0"`
	CreatedCount  uint           `json:"created_count" gorm:"column:created_count;not null;default:0"`
	UpdatedCount  uint           `json:"updated_count" gorm:"column:updated_count;not null;default:0"`
	FailedCount   uint           `json:"failed_count" gorm:"column:failed_count;not null;default:0"`
	ExitCode      *int           `json:"exit_code,omitempty" gorm:"column:exit_code"`
	Stdout        string         `json:"-" gorm:"column:stdout;type:longtext"`
	Stderr        string         `json:"-" gorm:"column:stderr;type:longtext"`
	CreatedAt     time.Time      `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time      `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index"`
}

func (KkuPeopleImportRun) TableName() string { return "kku_people_import_runs" }
