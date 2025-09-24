package models

import "time"

// SubmissionReview represents an audit record for reviews such as dept head decisions.
type SubmissionReview struct {
	ReviewID      int       `gorm:"primaryKey;column:review_id" json:"review_id"`
	SubmissionID  int       `gorm:"column:submission_id" json:"submission_id"`
	ReviewerID    int       `gorm:"column:reviewer_id" json:"reviewer_id"`
	ReviewRound   int       `gorm:"column:review_round" json:"review_round"`
	ReviewStatus  string    `gorm:"column:review_status" json:"review_status"`
	Comments      *string   `gorm:"column:comments" json:"comments"`
	InternalNotes *string   `gorm:"column:internal_notes" json:"internal_notes"`
	ReviewedAt    time.Time `gorm:"column:reviewed_at" json:"reviewed_at"`

	Reviewer *User `gorm:"foreignKey:ReviewerID" json:"reviewer,omitempty"`
}

// TableName specifies the table name for SubmissionReview.
func (SubmissionReview) TableName() string {
	return "submission_reviews"
}
