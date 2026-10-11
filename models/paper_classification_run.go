package models

import "time"

type PaperClassificationRun struct {
	RunID           string     `gorm:"primaryKey;column:run_id" json:"run_id"`
	Source          string     `gorm:"column:source" json:"source"`
	PublicationYear *int       `gorm:"column:publication_year" json:"publication_year"`
	Scope           string     `gorm:"column:scope" json:"scope"`
	TaxonomyVersion string     `gorm:"column:taxonomy_version" json:"taxonomy_version"`
	Status          string     `gorm:"column:status" json:"status"`
	StopRequested   bool       `gorm:"column:stop_requested" json:"stop_requested"`
	UserID          int        `gorm:"column:user_id" json:"user_id"`
	Total           int        `gorm:"column:total" json:"total"`
	Completed       int        `gorm:"column:completed" json:"completed"`
	Preface         int        `gorm:"column:preface" json:"preface"`
	Failed          int        `gorm:"column:failed" json:"failed"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	StartedAt       *time.Time `gorm:"column:started_at" json:"started_at"`
	CompletedAt     *time.Time `gorm:"column:completed_at" json:"completed_at"`
}

func (PaperClassificationRun) TableName() string { return "paper_classification_runs" }

type PaperClassificationRunItem struct {
	ID            uint64     `gorm:"primaryKey;column:id" json:"id"`
	RunID         string     `gorm:"column:run_id" json:"run_id"`
	DocumentID    uint64     `gorm:"column:document_id" json:"document_id"`
	TitleSnapshot *string    `gorm:"column:title_snapshot" json:"title_snapshot"`
	InputHash     string     `gorm:"column:input_hash" json:"-"`
	Status        string     `gorm:"column:status" json:"status"`
	PriorJSON     *string    `gorm:"column:prior_json" json:"prior_json"`
	ResultJSON    *string    `gorm:"column:result_json" json:"result_json"`
	ErrorMessage  *string    `gorm:"column:error_message" json:"error_message"`
	StartedAt     *time.Time `gorm:"column:started_at" json:"started_at"`
	CompletedAt   *time.Time `gorm:"column:completed_at" json:"completed_at"`
}

func (PaperClassificationRunItem) TableName() string { return "paper_classification_run_items" }
