package models

import "time"

// SubmissionApprovalAttachment stores an administrator-managed PDF evidence
// file for a submission. It intentionally does not reuse file_uploads so this
// feature remains independent from the legacy upload/document workflow.
type SubmissionApprovalAttachment struct {
	AttachmentID     int        `gorm:"primaryKey;column:attachment_id;autoIncrement" json:"attachment_id"`
	SubmissionID     int        `gorm:"column:submission_id" json:"submission_id"`
	Label            string     `gorm:"column:label" json:"label"`
	OriginalFilename string     `gorm:"column:original_filename" json:"original_filename"`
	StoredFilename   string     `gorm:"column:stored_filename" json:"stored_filename"`
	StoredPath       string     `gorm:"column:stored_path" json:"-"`
	MimeType         string     `gorm:"column:mime_type" json:"mime_type"`
	FileSize         int64      `gorm:"column:file_size" json:"file_size"`
	FileHash         string     `gorm:"column:file_hash" json:"file_hash,omitempty"`
	DisplayOrder     int        `gorm:"column:display_order" json:"display_order"`
	UploadedBy       int        `gorm:"column:uploaded_by" json:"uploaded_by"`
	UploadedAt       time.Time  `gorm:"column:uploaded_at" json:"uploaded_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at" json:"-"`

	Submission Submission `gorm:"foreignKey:SubmissionID;references:SubmissionID" json:"-"`
	Uploader   User       `gorm:"foreignKey:UploadedBy;references:UserID" json:"uploader,omitempty"`
}

func (SubmissionApprovalAttachment) TableName() string {
	return "submission_approval_attachments"
}
