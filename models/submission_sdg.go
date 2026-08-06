package models

import "time"

// SubmissionSDG stores an SDG selected for a submission, including a snapshot
// of the master data so historical submissions remain self-contained.
type SubmissionSDG struct {
	SubmissionSDGID       int       `gorm:"primaryKey;column:submission_sdg_id" json:"submission_sdg_id"`
	SubmissionID          int       `gorm:"column:submission_id" json:"submission_id"`
	SDGID                 int       `gorm:"column:sdg_id" json:"sdg_id"`
	SDGNumberSnapshot     int       `gorm:"column:sdg_number_snapshot" json:"sdg_number"`
	NameTHSnapshot        string    `gorm:"column:name_th_snapshot" json:"name_th"`
	NameENSnapshot        string    `gorm:"column:name_en_snapshot" json:"name_en"`
	DescriptionTHSnapshot *string   `gorm:"column:description_th_snapshot" json:"description_th,omitempty"`
	DescriptionENSnapshot *string   `gorm:"column:description_en_snapshot" json:"description_en,omitempty"`
	CreatedAt             time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at" json:"updated_at"`

	SDG SDG `gorm:"foreignKey:SDGID;references:SDGID" json:"-"`
}

func (SubmissionSDG) TableName() string { return "submission_sdgs" }
