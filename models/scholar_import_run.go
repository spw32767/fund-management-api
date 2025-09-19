package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	ScholarImportRunStatusRunning = "running"
	ScholarImportRunStatusSuccess = "success"
	ScholarImportRunStatusFailed  = "failed"
)

type ScholarImportRun struct {
	ID uint `json:"id" gorm:"primaryKey;autoIncrement"`

	TriggerSource string     `json:"trigger_source" gorm:"type:varchar(64);not null"`
	Status        string     `json:"status" gorm:"type:enum('running','success','failed');not null;default:'running'"`
	ErrorMessage  *string    `json:"error_message" gorm:"type:text"`
	StartedAt     time.Time  `json:"started_at" gorm:"column:started_at;autoCreateTime"`
	FinishedAt    *time.Time `json:"finished_at" gorm:"column:finished_at"`

	UsersProcessed      uint `json:"users_processed" gorm:"column:users_processed;not null;default:0"`
	UsersWithErrors     uint `json:"users_with_errors" gorm:"column:users_with_errors;not null;default:0"`
	PublicationsFetched uint `json:"publications_fetched" gorm:"column:publications_fetched;not null;default:0"`
	PublicationsCreated uint `json:"publications_created" gorm:"column:publications_created;not null;default:0"`
	PublicationsUpdated uint `json:"publications_updated" gorm:"column:publications_updated;not null;default:0"`
	PublicationsFailed  uint `json:"publications_failed" gorm:"column:publications_failed;not null;default:0"`

	CreatedAt time.Time      `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"column:deleted_at;index"`
}

func (ScholarImportRun) TableName() string { return "scholar_import_runs" }
