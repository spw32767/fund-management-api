// models/submission.go
package models

import (
	"encoding/json"
	"time"
)

// Submission represents the main submission table
type Submission struct {
	SubmissionID     int    `gorm:"primaryKey;column:submission_id" json:"submission_id"`
	SubmissionNumber string `gorm:"column:submission_number" json:"submission_number"`
	SubmissionType   string `gorm:"column:submission_type" json:"submission_type"`
	UserID           int    `gorm:"column:user_id" json:"user_id"`
	YearID           int    `gorm:"column:year_id" json:"year_id"`
	CategoryID       *int   `gorm:"column:category_id" json:"category_id"`
	CategoryName     string `gorm:"column:category_name;->" json:"category_name,omitempty"`
	StatusID         int    `gorm:"column:status_id" json:"status_id"`
	//Priority         string     `gorm:"column:priority" json:"priority"`
	ApprovedBy  *int       `gorm:"column:approved_by" json:"approved_by"`
	ApprovedAt  *time.Time `gorm:"column:approved_at" json:"approved_at"`
	SubmittedAt *time.Time `gorm:"column:submitted_at" json:"submitted_at"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at" json:"deleted_at"`

	// (nullable, because passed &categoryID, &subcategoryID, &subcategoryBudgetID)
	SubcategoryID       *int `gorm:"column:subcategory_id" json:"subcategory_id,omitempty"`
	SubcategoryBudgetID *int `gorm:"column:subcategory_budget_id" json:"subcategory_budget_id,omitempty"`

	// Relations
	User                    *User                    `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Year                    Year                     `gorm:"foreignKey:YearID" json:"year,omitempty"`
	Status                  ApplicationStatus        `gorm:"foreignKey:StatusID" json:"status,omitempty"`
	Category                *FundCategory            `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	Documents               []SubmissionDocument     `gorm:"foreignKey:SubmissionID" json:"documents,omitempty"`
	SubmissionUsers         []SubmissionUser         `gorm:"foreignKey:SubmissionID" json:"submission_users,omitempty"` // เพิ่มใหม่
	FundApplicationDetail   *FundApplicationDetail   `json:"fund_application_detail,omitempty"`
	PublicationRewardDetail *PublicationRewardDetail `json:"publication_reward_detail,omitempty"`
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

	// ========== เพิ่ม fields ใหม่สำหรับ tracking ==========
	ApprovedBy *int       `json:"approved_by" gorm:"column:approved_by"`
	ApprovedAt *time.Time `json:"approved_at" gorm:"column:approved_at"`
	RejectedBy *int       `json:"rejected_by" gorm:"column:rejected_by"`
	RejectedAt *time.Time `json:"rejected_at" gorm:"column:rejected_at"`

	CreateAt time.Time  `json:"create_at" gorm:"column:create_at;autoCreateTime"`
	UpdateAt time.Time  `json:"update_at" gorm:"column:update_at;autoUpdateTime"`
	DeleteAt *time.Time `json:"delete_at" gorm:"column:delete_at"`

	// Relations
	Submission  Submission      `gorm:"foreignKey:SubmissionID" json:"submission,omitempty"`
	Subcategory FundSubcategory `gorm:"foreignKey:SubcategoryID" json:"subcategory,omitempty"`
}

// PublicationRewardDetail represents publication reward specific details
type PublicationRewardDetail struct {
	DetailID        int       `gorm:"primaryKey;column:detail_id" json:"detail_id"`
	SubmissionID    int       `gorm:"column:submission_id" json:"submission_id"`
	PaperTitle      string    `gorm:"column:paper_title" json:"paper_title"`
	JournalName     string    `gorm:"column:journal_name" json:"journal_name"`
	PublicationDate time.Time `gorm:"column:publication_date" json:"publication_date"`
	PublicationType string    `gorm:"column:publication_type;type:enum('journal','conference','book_chapter','other')" json:"publication_type"`
	Quartile        string    `gorm:"column:quartile;type:enum('Q1','Q2','Q3','Q4','N/A')" json:"quartile"`
	ImpactFactor    float64   `gorm:"column:impact_factor" json:"impact_factor"`
	DOI             string    `gorm:"column:doi" json:"doi"`
	URL             string    `gorm:"column:url" json:"url"`
	PageNumbers     string    `gorm:"column:page_numbers" json:"page_numbers"`
	VolumeIssue     string    `gorm:"column:volume_issue" json:"volume_issue"`
	Indexing        string    `gorm:"column:indexing" json:"indexing"`

	// === เงินรางวัลและการคำนวณ ===
	RewardAmount                float64 `gorm:"column:reward_amount" json:"reward_amount"`                 // เงินรางวัลฐาน (อ้างอิงจาก Author และ Quartile)
	RewardApproveAmount         float64 `gorm:"column:reward_approve_amount" json:"reward_approve_amount"` // จำนวนเงินรางวัลที่อนุมัติ
	RevisionFee                 float64 `gorm:"column:revision_fee" json:"revision_fee"`                   // ค่าปรับปรุง
	RevisionFeeApproveAmount    float64 `gorm:"column:revision_fee_approve_amount" json:"revision_fee_approve_amount"`
	PublicationFee              float64 `gorm:"column:publication_fee" json:"publication_fee"` // ค่าตีพิมพ์
	PublicationFeeApproveAmount float64 `gorm:"column:publication_fee_approve_amount" json:"publication_fee_approve_amount"`
	ExternalFundingAmount       float64 `gorm:"column:external_funding_amount" json:"external_funding_amount"` // รวมจำนวนเงินจากทุนที่ user แนบเข้ามา
	TotalAmount                 float64 `gorm:"column:total_amount" json:"total_amount"`                       // เกิดจากการหักลบค่าปรับปรุง+ค่าตีพิมพ์ ลบกับ รายการที่เบิกจากหน่วยงานนอก
	TotalApproveAmount          float64 `gorm:"column:total_approve_amount" json:"total_approve_amount"`       // จำนวนเงินจริงที่วิทยาลัยจ่ายให้ (หลังจากได้รับการอนุมัติ)

	// === ข้อมูลผู้แต่ง ===
	AuthorCount int    `gorm:"column:author_count" json:"author_count"`
	AuthorType  string `gorm:"column:author_type;type:enum('first_author','corresponding_author','coauthor')" json:"author_type"` // เปลี่ยนจาก author_status

	// === อื่นๆ ===
	AnnounceReferenceNumber string `gorm:"column:announce_reference_number" json:"announce_reference_number,omitempty"`

	HasUniversityFunding string  `json:"has_university_funding" gorm:"column:has_university_funding"`
	FundingReferences    *string `json:"funding_references" gorm:"column:funding_references"`
	UniversityRankings   *string `json:"university_rankings" gorm:"column:university_rankings"`

	// ========== เพิ่ม fields ใหม่สำหรับ Admin Management ==========
	ApprovedAmount  *float64   `json:"approved_amount,omitempty" gorm:"column:approved_amount"`
	ApprovalComment *string    `json:"approval_comment" gorm:"column:approval_comment"`
	ApprovedBy      *int       `json:"approved_by" gorm:"column:approved_by"`
	ApprovedAt      *time.Time `json:"approved_at" gorm:"column:approved_at"`

	RejectionReason *string    `json:"rejection_reason" gorm:"column:rejection_reason"`
	RejectedBy      *int       `json:"rejected_by" gorm:"column:rejected_by"`
	RejectedAt      *time.Time `json:"rejected_at" gorm:"column:rejected_at"`

	RevisionRequest     *string    `json:"revision_request" gorm:"column:revision_request"`
	RevisionRequestedBy *int       `json:"revision_requested_by" gorm:"column:revision_requested_by"`
	RevisionRequestedAt *time.Time `json:"revision_requested_at" gorm:"column:revision_requested_at"`

	CreateAt time.Time  `json:"create_at" gorm:"column:create_at;autoCreateTime"`
	UpdateAt time.Time  `json:"update_at" gorm:"column:update_at;autoUpdateTime"`
	DeleteAt *time.Time `json:"delete_at" gorm:"column:delete_at"`

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

// SubmissionUser represents co-authors and collaborators in submissions
type SubmissionUser struct {
	ID           int       `gorm:"primaryKey;column:id" json:"id"`
	SubmissionID int       `gorm:"column:submission_id" json:"submission_id"`
	UserID       int       `gorm:"column:user_id" json:"user_id"`
	Role         string    `gorm:"column:role;type:enum('owner','coauthor','team_member','advisor','coordinator');default:'coauthor'" json:"role"`
	IsPrimary    bool      `gorm:"column:is_primary;default:false" json:"is_primary"`
	DisplayOrder int       `gorm:"column:display_order;default:0" json:"display_order"`
	CreatedAt    time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`

	// Relations
	Submission Submission `gorm:"foreignKey:SubmissionID" json:"submission,omitempty"`
	User       *User      `gorm:"foreignKey:UserID" json:"user,omitempty"`

	// Computed fields (not stored in DB)
	AuthorRole            string `gorm:"-" json:"author_role,omitempty"`
	IsFirstAuthor         bool   `gorm:"-" json:"is_first_author,omitempty"`
	IsCorrespondingAuthor bool   `gorm:"-" json:"is_corresponding_author,omitempty"`
	IsApplicant           bool   `gorm:"-" json:"is_applicant,omitempty"`
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

// TableName specifies the table name
func (SubmissionUser) TableName() string {
	return "submission_users"
}

// Helper methods สำหรับ SubmissionUser
func (su *SubmissionUser) IsMainAuthor() bool {
	return su.Role == "first_author" || su.Role == "corresponding_author"
}

func (su *SubmissionUser) IsCoauthor() bool {
	return su.Role == "coauthor"
}

func (su *SubmissionUser) IsTeamMember() bool {
	return su.Role == "team_member"
}

func (su *SubmissionUser) IsAdvisor() bool {
	return su.Role == "advisor"
}

func (su *SubmissionUser) IsCoordinator() bool {
	return su.Role == "coordinator"
}

func (su *SubmissionUser) IsOwner() bool {
	return su.Role == "owner"
}

func (su *SubmissionUser) GetAuthorType() string {
	if su.IsOwner() && su.IsPrimary {
		return "main_author"
	} else if su.IsCoauthor() {
		return "co_author"
	}
	return "other"
}

func (su *SubmissionUser) GetAuthorTypeDisplay() string {
	switch su.Role {
	case "first_author":
		return "ผู้แต่งหลัก"
	case "corresponding_author":
		return "Corresponding Author"
	case "co_author":
		return "ผู้แต่งร่วม"
	case "advisor":
		return "ที่ปรึกษา"
	default:
		return "อื่นๆ"
	}
}

// JSON serialization helper - เพิ่ม order_sequence field สำหรับ backward compatibility
func (su *SubmissionUser) MarshalJSON() ([]byte, error) {
	type Alias SubmissionUser

	authorRole := su.Role
	isFirst := false
	isCorresponding := false

	if su.Role == "owner" {
		if su.IsPrimary {
			authorRole = "first_author"
			isFirst = true
		} else {
			authorRole = "corresponding_author"
			isCorresponding = true
		}
	} else if su.Role == "coauthor" || su.Role == "co_author" {
		authorRole = "co_author"
	}
	return json.Marshal(&struct {
		OrderSequence         int    `json:"order_sequence"`
		AuthorRole            string `json:"author_role"`
		IsFirstAuthor         bool   `json:"is_first_author"`
		IsCorrespondingAuthor bool   `json:"is_corresponding_author"`
		*Alias
	}{
		OrderSequence:         su.DisplayOrder,
		AuthorRole:            authorRole,
		IsFirstAuthor:         isFirst,
		IsCorrespondingAuthor: isCorresponding,
		Alias:                 (*Alias)(su),
	})
}
