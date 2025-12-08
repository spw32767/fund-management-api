package models

import "time"

type ScopusAPIImportJob struct {
	ID             uint64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	Service        string     `json:"service" gorm:"column:service;type:varchar(64);not null"`
	JobType        string     `json:"job_type" gorm:"column:job_type;type:varchar(64);not null"`
	ScopusAuthorID *string    `json:"scopus_author_id,omitempty" gorm:"column:scopus_author_id;type:varchar(100)"`
	QueryString    string     `json:"query_string" gorm:"column:query_string;type:text;not null"`
	TotalResults   *int       `json:"total_results,omitempty" gorm:"column:total_results"`
	Status         string     `json:"status" gorm:"column:status;type:varchar(32);not null"`
	ErrorMessage   *string    `json:"error_message,omitempty" gorm:"column:error_message;type:text"`
	StartedAt      time.Time  `json:"started_at" gorm:"column:started_at;autoCreateTime"`
	FinishedAt     *time.Time `json:"finished_at,omitempty" gorm:"column:finished_at"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (ScopusAPIImportJob) TableName() string { return "scopus_api_import_jobs" }
