package models

import "time"

type ThaiJOAPIImportJob struct {
	ID             uint64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	Service        string     `json:"service" gorm:"column:service;type:varchar(64);not null"`
	JobType        string     `json:"job_type" gorm:"column:job_type;type:varchar(64);not null"`
	UserID         *uint      `json:"user_id,omitempty" gorm:"column:user_id"`
	ThaiJOAuthorID *string    `json:"thaijo_author_id,omitempty" gorm:"column:thaijo_author_id;type:varchar(100)"`
	SearchName     *string    `json:"search_name,omitempty" gorm:"column:search_name;type:text"`
	QueryString    string     `json:"query_string" gorm:"column:query_string;type:text;not null"`
	TotalResults   *int       `json:"total_results,omitempty" gorm:"column:total_results"`
	AuthorSelectionReason *string `json:"author_selection_reason,omitempty" gorm:"column:author_selection_reason;type:varchar(64)"`
	Status         string     `json:"status" gorm:"column:status;type:varchar(32);not null"`
	ErrorMessage   *string    `json:"error_message,omitempty" gorm:"column:error_message;type:text"`
	StartedAt      time.Time  `json:"started_at" gorm:"column:started_at;autoCreateTime"`
	FinishedAt     *time.Time `json:"finished_at,omitempty" gorm:"column:finished_at"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (ThaiJOAPIImportJob) TableName() string { return "thaijo_api_import_jobs" }
