package models

import "time"

// ScopusAuthorRoleRun records a manual Abstract XML backfill started in the admin UI.
type ScopusAuthorRoleRun struct {
	ID               uint64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	RunType          string     `json:"run_type" gorm:"column:run_type"`
	Status           string     `json:"status" gorm:"column:status"`
	ActiveSlot       *int       `json:"-" gorm:"column:active_slot"`
	Eligible         int        `json:"eligible" gorm:"column:eligible"`
	Selected         int        `json:"selected" gorm:"column:selected"`
	SkippedExisting  int        `json:"skipped_existing" gorm:"column:skipped_existing"`
	Fetched          int        `json:"fetched" gorm:"column:fetched"`
	Complete         int        `json:"complete" gorm:"column:complete"`
	NoCorrespondence int        `json:"no_correspondence" gorm:"column:no_correspondence"`
	NeedsReview      int        `json:"needs_review" gorm:"column:needs_review"`
	Failed           int        `json:"failed" gorm:"column:failed"`
	ErrorMessage     *string    `json:"error_message,omitempty" gorm:"column:error_message"`
	StartedAt        time.Time  `json:"started_at" gorm:"column:started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty" gorm:"column:finished_at"`
	CreatedAt        time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt        time.Time  `json:"updated_at" gorm:"column:updated_at"`
}

func (ScopusAuthorRoleRun) TableName() string { return "scopus_author_role_runs" }
