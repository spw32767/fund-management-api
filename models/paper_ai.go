package models

import "time"

type PaperCategory struct {
	CategoryID   uint64    `gorm:"primaryKey;column:category_id" json:"category_id"`
	Code         string    `gorm:"column:code" json:"code"`
	Name         string    `gorm:"column:name" json:"name"`
	Description  *string   `gorm:"column:description" json:"description,omitempty"`
	IsActive     bool      `gorm:"column:is_active" json:"is_active"`
	DisplayOrder int       `gorm:"column:display_order" json:"display_order"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (PaperCategory) TableName() string { return "paper_categories" }

type PaperAIJob struct {
	JobID              string     `gorm:"primaryKey;column:job_id" json:"job_id"`
	UserID             int        `gorm:"column:user_id" json:"user_id"`
	SubmissionID       *int       `gorm:"column:submission_id" json:"submission_id,omitempty"`
	JobType            string     `gorm:"column:job_type" json:"job_type"`
	Status             string     `gorm:"column:status" json:"status"`
	RequestFingerprint *string    `gorm:"column:request_fingerprint" json:"request_fingerprint,omitempty"`
	ResultJSON         *string    `gorm:"column:result_json" json:"result_json,omitempty"`
	ErrorMessage       *string    `gorm:"column:error_message" json:"error_message,omitempty"`
	CreatedAt          time.Time  `gorm:"column:created_at" json:"created_at"`
	StartedAt          *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	CompletedAt        *time.Time `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (PaperAIJob) TableName() string { return "paper_ai_jobs" }
