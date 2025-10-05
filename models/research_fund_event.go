package models

import "time"

const (
	ResearchFundEventTypeNote    = "note"
	ResearchFundEventTypePayment = "payment"
	ResearchFundEventTypeClosure = "closure"

	FileFolderTypeAdminEvent = "admin_event"
)

// ResearchFundAdminEvent captures administrative actions taken against a research fund submission.
type ResearchFundAdminEvent struct {
	EventID      int        `gorm:"primaryKey;column:event_id" json:"event_id"`
	SubmissionID int        `gorm:"column:submission_id" json:"submission_id"`
	EventType    string     `gorm:"column:event_type" json:"event_type"`
	Comment      string     `gorm:"column:comment" json:"comment"`
	Amount       *float64   `gorm:"column:amount" json:"amount,omitempty"`
	CreatedBy    int        `gorm:"column:created_by" json:"created_by"`
	CreatedAt    time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt    *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`

	Creator *User                   `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Files   []ResearchFundEventFile `gorm:"foreignKey:EventID" json:"files,omitempty"`
}

// ResearchFundEventFile associates uploaded files with a research fund admin event.
type ResearchFundEventFile struct {
	ID        int        `gorm:"primaryKey;column:id" json:"id"`
	EventID   int        `gorm:"column:event_id" json:"event_id"`
	FileID    int        `gorm:"column:file_id" json:"file_id"`
	CreatedAt time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`

	Event ResearchFundAdminEvent `gorm:"foreignKey:EventID" json:"event,omitempty"`
	File  FileUpload             `gorm:"foreignKey:FileID" json:"file"`
}

func (ResearchFundAdminEvent) TableName() string {
	return "research_fund_admin_events"
}

func (ResearchFundEventFile) TableName() string {
	return "research_fund_event_files"
}

// IsPayment reports whether the admin event records a payment.
func (e ResearchFundAdminEvent) IsPayment() bool {
	return e.EventType == ResearchFundEventTypePayment
}
