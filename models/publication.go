// models/publication.go
package models

import (
	"time"
)

// PublicationReward represents a publication reward request
type PublicationReward struct {
	RewardID                 int        `gorm:"primaryKey;column:reward_id" json:"reward_id"`
	RewardNumber             string     `gorm:"column:reward_number;unique" json:"reward_number"`
	UserID                   int        `gorm:"column:user_id" json:"user_id"`
	AuthorStatus             string     `gorm:"column:author_status" json:"author_status"`
	ArticleTitle             string     `gorm:"column:article_title" json:"article_title"`
	JournalName              string     `gorm:"column:journal_name" json:"journal_name"`
	JournalIssue             string     `gorm:"column:journal_issue" json:"journal_issue,omitempty"`
	JournalPages             string     `gorm:"column:journal_pages" json:"journal_pages,omitempty"`
	JournalMonth             string     `gorm:"column:journal_month" json:"journal_month,omitempty"`
	JournalYear              string     `gorm:"column:journal_year" json:"journal_year,omitempty"`
	JournalURL               string     `gorm:"column:journal_url" json:"journal_url,omitempty"`
	DOI                      string     `gorm:"column:doi" json:"doi,omitempty"`
	ArticleOnlineDB          string     `gorm:"column:article_online_db" json:"article_online_db,omitempty"`
	ArticleOnlineDate        *time.Time `gorm:"column:article_online_date" json:"article_online_date,omitempty"`
	JournalTier              string     `gorm:"column:journal_tier" json:"journal_tier,omitempty"`
	JournalQuartile          string     `gorm:"column:journal_quartile" json:"journal_quartile,omitempty"`
	InISI                    bool       `gorm:"column:in_isi" json:"in_isi"`
	InScopus                 bool       `gorm:"column:in_scopus" json:"in_scopus"`
	ArticleType              string     `gorm:"column:article_type" json:"article_type,omitempty"`
	JournalType              string     `gorm:"column:journal_type" json:"journal_type,omitempty"`
	PublicationReward        float64    `gorm:"column:publication_reward" json:"publication_reward"`
	EditorFee                float64    `gorm:"column:editor_fee" json:"editor_fee"`
	PublicationFeeTotal      float64    `gorm:"column:publication_fee_total" json:"publication_fee_total"`
	PublicationFeeUniversity float64    `gorm:"column:publication_fee_university" json:"publication_fee_university"`
	PublicationFeeCollege    float64    `gorm:"column:publication_fee_college" json:"publication_fee_college"`
	TotalAmount              float64    `gorm:"column:total_amount" json:"total_amount"`
	UniversityRanking        string     `gorm:"column:university_ranking" json:"university_ranking,omitempty"`
	BankAccount              string     `gorm:"column:bank_account" json:"bank_account,omitempty"`
	BankName                 string     `gorm:"column:bank_name" json:"bank_name,omitempty"`
	PhoneNumber              string     `gorm:"column:phone_number" json:"phone_number,omitempty"`
	HasUniversityFund        bool       `gorm:"column:has_university_fund" json:"has_university_fund"`
	UniversityFundRef        string     `gorm:"column:university_fund_ref" json:"university_fund_ref,omitempty"`
	Status                   string     `gorm:"column:status" json:"status"`
	SubmittedAt              *time.Time `gorm:"column:submitted_at" json:"submitted_at,omitempty"`
	ApprovedAt               *time.Time `gorm:"column:approved_at" json:"approved_at,omitempty"`
	PaidAt                   *time.Time `gorm:"column:paid_at" json:"paid_at,omitempty"`
	CreateAt                 *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt                 *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt                 *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	// Relations
	User      User                  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Coauthors []PublicationCoauthor `gorm:"foreignKey:RewardID" json:"coauthors,omitempty"`
	Documents []PublicationDocument `gorm:"foreignKey:RewardID" json:"documents,omitempty"`
	Comments  []PublicationComment  `gorm:"foreignKey:RewardID" json:"comments,omitempty"`
}

// PublicationCoauthor represents co-authors of a publication
type PublicationCoauthor struct {
	CoauthorID  int        `gorm:"primaryKey;column:coauthor_id" json:"coauthor_id"`
	RewardID    int        `gorm:"column:reward_id" json:"reward_id"`
	UserID      int        `gorm:"column:user_id" json:"user_id"`
	AuthorOrder int        `gorm:"column:author_order" json:"author_order,omitempty"`
	CreateAt    *time.Time `gorm:"column:create_at" json:"create_at"`

	// Relations
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// PublicationDocument represents documents for publication reward
type PublicationDocument struct {
	DocumentID       int        `gorm:"primaryKey;column:document_id" json:"document_id"`
	RewardID         int        `gorm:"column:reward_id" json:"reward_id"`
	DocumentType     string     `gorm:"column:document_type" json:"document_type"`
	OriginalFilename string     `gorm:"column:original_filename" json:"original_filename"`
	StoredFilename   string     `gorm:"column:stored_filename" json:"stored_filename"`
	FileType         string     `gorm:"column:file_type" json:"file_type,omitempty"`
	UploadedBy       int        `gorm:"column:uploaded_by" json:"uploaded_by,omitempty"`
	UploadedAt       *time.Time `gorm:"column:uploaded_at" json:"uploaded_at"`
	CreateAt         *time.Time `gorm:"column:create_at" json:"create_at"`
	DeleteAt         *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

// PublicationComment represents comments/reviews on publication reward
type PublicationComment struct {
	CommentID     int        `gorm:"primaryKey;column:comment_id" json:"comment_id"`
	RewardID      int        `gorm:"column:reward_id" json:"reward_id"`
	CommentBy     int        `gorm:"column:comment_by" json:"comment_by"`
	CommentText   string     `gorm:"column:comment_text" json:"comment_text,omitempty"`
	CommentStatus string     `gorm:"column:comment_status" json:"comment_status,omitempty"`
	CreateAt      *time.Time `gorm:"column:create_at" json:"create_at"`

	// Relations
	User User `gorm:"foreignKey:CommentBy" json:"user,omitempty"`
}

// PublicationRewardRate represents reward rates configuration
type PublicationRewardRate struct {
	RateID          int        `gorm:"primaryKey;column:rate_id" json:"rate_id"`
	Year            string     `gorm:"column:year" json:"year"`
	AuthorStatus    string     `gorm:"column:author_status" json:"author_status"`
	JournalQuartile string     `gorm:"column:journal_quartile" json:"journal_quartile"`
	RewardAmount    float64    `gorm:"column:reward_amount" json:"reward_amount"`
	IsActive        bool       `gorm:"column:is_active" json:"is_active"`
	CreateAt        *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt        *time.Time `gorm:"column:update_at" json:"update_at"`
}

// TableName overrides
func (PublicationReward) TableName() string {
	return "publication_rewards"
}

func (PublicationCoauthor) TableName() string {
	return "publication_coauthors"
}

func (PublicationDocument) TableName() string {
	return "publication_documents"
}

func (PublicationComment) TableName() string {
	return "publication_comments"
}

func (PublicationRewardRate) TableName() string {
	return "publication_reward_rates"
}
