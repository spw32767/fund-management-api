package models

import "time"

// SubmissionStatusHistory tracks historical status changes for submissions.
type SubmissionStatusHistory struct {
	HistoryID    int       `gorm:"primaryKey;column:history_id" json:"history_id"`
	SubmissionID int       `gorm:"column:submission_id" json:"submission_id"`
	OldStatusID  *int      `gorm:"column:old_status_id" json:"old_status_id"`
	NewStatusID  int       `gorm:"column:new_status_id" json:"new_status_id"`
	ChangedBy    int       `gorm:"column:changed_by" json:"changed_by"`
	Reason       *string   `gorm:"column:reason" json:"reason"`
	Notes        *string   `gorm:"column:notes" json:"notes"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName specifies the table for SubmissionStatusHistory.
func (SubmissionStatusHistory) TableName() string {
	return "submission_status_history"
}
