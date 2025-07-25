package models

import (
	"time"
)

type Notification struct {
	NotificationID       int        `gorm:"primaryKey;column:notification_id" json:"notification_id"`
	UserID               int        `gorm:"column:user_id" json:"user_id"`
	Title                string     `gorm:"column:title" json:"title"`
	Message              string     `gorm:"column:message" json:"message"`
	Type                 string     `gorm:"column:type" json:"type"` // info, success, warning, error
	IsRead               bool       `gorm:"column:is_read" json:"is_read"`
	RelatedApplicationID *int       `gorm:"column:related_application_id" json:"related_application_id,omitempty"`
	CreateAt             time.Time  `gorm:"column:create_at" json:"create_at"`
	UpdateAt             time.Time  `gorm:"column:update_at" json:"update_at"`
	DeleteAt             *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	// Relations
	User               User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	RelatedApplication *FundApplication `gorm:"foreignKey:RelatedApplicationID" json:"related_application,omitempty"`
}

// TableName overrides
func (Notification) TableName() string {
	return "notifications"
}