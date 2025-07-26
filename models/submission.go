// models/submission.go
package models

import (
	"time"
)

// Submission represents the main submission table
type Submission struct {
	SubmissionID     int        `gorm:"primaryKey;column:submission_id" json:"submission_id"`
	SubmissionType   string     `gorm:"column:submission_type" json:"submission_type"` // 'fund_application', 'publication_reward', 'conference_grant', 'training_request'
	SubmissionNumber string     `gorm:"column:submission_number" json:"submission_number"`
	UserID           int        `gorm:"column:user_id" json:"user_id"`
	YearID           int        `gorm:"column:year_id" json:"year_id"`
	StatusID         int        `gorm:"column:status_id" json:"status_id"`
	Priority         string     `gorm:"column:priority" json:"priority"` // 'low', 'normal', 'high', 'urgent'
	SubmittedAt      *time.Time `gorm:"column:submitted_at" json:"submitted_at"`
	ReviewedAt       *time.Time `gorm:"column:reviewed_at" json:"reviewed_at"`
	ApprovedAt       *time.Time `gorm:"column:approved_at" json:"approved_at"`
	ApprovedBy       *int       `gorm:"column:approved_by" json:"approved_by"`
	CompletedAt      *time.Time `gorm:"column:completed_at" json:"completed_at"`
	CreatedAt        time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`

	// Relations
	User     User              `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Year     Year              `gorm:"foreignKey:YearID" json:"year,omitempty"`
	Status   ApplicationStatus `gorm:"foreignKey:StatusID" json:"status,omitempty"`
	Approver *User             `gorm:"foreignKey:ApprovedBy" json:"approver,omitempty"`

	// Related details (will be loaded separately based on submission_type)
	FundApplicationDetail   *FundApplicationDetail   `json:"fund_application_detail,omitempty"`
	PublicationRewardDetail *PublicationRewardDetail `json:"publication_reward_detail,omitempty"`
	Documents               []SubmissionDocument     `json:"documents,omitempty"`
}

// FundApplicationDetail represents fund application specific details
type FundApplicationDetail struct {
	DetailID           int        `gorm:"primaryKey;column:detail_id" json:"detail_id"`
	SubmissionID       int        `gorm:"column:submission_id" json:"submission_id"`
	SubcategoryID      int        `gorm:"column:subcategory_id" json:"subcategory_id"`
	ProjectTitle       string     `gorm:"column:project_title" json:"project_title"`
	ProjectDescription string     `gorm:"column:project_description" json:"project_description"`
	RequestedAmount    float64    `gorm:"column:requested_amount" json:"requested_amount"`
	ApprovedAmount     float64    `gorm:"column:approved_amount" json:"approved_amount"`
	ClosedAt           *time.Time `gorm:"column:closed_at" json:"closed_at"`
	Comment            string     `gorm:"column:comment" json:"comment"`

	// Relations
	Submission  Submission      `gorm:"foreignKey:SubmissionID" json:"submission,omitempty"`
	Subcategory FundSubcategory `gorm:"foreignKey:SubcategoryID" json:"subcategory,omitempty"`
}

// PublicationRewardDetail represents publication reward specific details
type PublicationRewardDetail struct {
	DetailID              int       `gorm:"primaryKey;column:detail_id" json:"detail_id"`
	SubmissionID          int       `gorm:"column:submission_id" json:"submission_id"`
	PaperTitle            string    `gorm:"column:paper_title" json:"paper_title"`
	JournalName           string    `gorm:"column:journal_name" json:"journal_name"`
	PublicationDate       time.Time `gorm:"column:publication_date" json:"publication_date"`
	PublicationType       string    `gorm:"column:publication_type;type:enum('journal','conference','book_chapter','other')" json:"publication_type"`
	Quartile              string    `gorm:"column:quartile;type:enum('Q1','Q2','Q3','Q4','N/A')" json:"quartile"`
	ImpactFactor          float64   `gorm:"column:impact_factor" json:"impact_factor"`
	DOI                   string    `gorm:"column:doi" json:"doi"`
	URL                   string    `gorm:"column:url" json:"url"`
	PageNumbers           string    `gorm:"column:page_numbers" json:"page_numbers"`
	VolumeIssue           string    `gorm:"column:volume_issue" json:"volume_issue"`
	Indexing              string    `gorm:"column:indexing" json:"indexing"`
	RewardAmount          float64   `gorm:"column:reward_amount" json:"reward_amount"`
	AuthorCount           int       `gorm:"column:author_count" json:"author_count"`
	IsCorrespondingAuthor bool      `gorm:"column:is_corresponding_author" json:"is_corresponding_author"`
	AuthorStatus          string    `gorm:"column:author_status;type:enum('first_author','corresponding_author','coauthor')" json:"author_status"`

	// Relations
	Submission Submission `gorm:"foreignKey:SubmissionID" json:"submission,omitempty"`
}

// SubmissionDocument represents the submission_documents table (junction table)
type SubmissionDocument struct {
	DocumentID     int        `gorm:"primaryKey;column:document_id" json:"document_id"`
	SubmissionID   int        `gorm:"column:submission_id" json:"submission_id"`
	FileID         int        `gorm:"column:file_id" json:"file_id"`
	DocumentTypeID int        `gorm:"column:document_type_id" json:"document_type_id"`
	Description    string     `gorm:"column:description" json:"description"`
	DisplayOrder   int        `gorm:"column:display_order" json:"display_order"`
	IsRequired     bool       `gorm:"column:is_required" json:"is_required"`
	IsVerified     bool       `gorm:"column:is_verified" json:"is_verified"`
	VerifiedBy     *int       `gorm:"column:verified_by" json:"verified_by"`
	VerifiedAt     *time.Time `gorm:"column:verified_at" json:"verified_at"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`

	// Relations
	Submission   Submission   `gorm:"foreignKey:SubmissionID" json:"submission,omitempty"`
	File         FileUpload   `gorm:"foreignKey:FileID" json:"file,omitempty"`
	DocumentType DocumentType `gorm:"foreignKey:DocumentTypeID" json:"document_type,omitempty"`
	Verifier     *User        `gorm:"foreignKey:VerifiedBy" json:"verifier,omitempty"`
}

// TableName overrides
func (Submission) TableName() string {
	return "submissions"
}

func (FundApplicationDetail) TableName() string {
	return "fund_application_details"
}

func (PublicationRewardDetail) TableName() string {
	return "publication_reward_details"
}

func (SubmissionDocument) TableName() string {
	return "submission_documents"
}

// Helper methods for Submission
func (s *Submission) IsEditable() bool {
	return s.StatusID == 1 && s.SubmittedAt == nil // Only draft status and not submitted
}

func (s *Submission) CanBeSubmitted() bool {
	return s.StatusID == 1 && s.SubmittedAt == nil
}

func (s *Submission) IsSubmitted() bool {
	return s.SubmittedAt != nil
}

func (s *Submission) IsApproved() bool {
	return s.StatusID == 2 // Assuming 2 is approved status
}

func (s *Submission) IsRejected() bool {
	return s.StatusID == 3 // Assuming 3 is rejected status
}
