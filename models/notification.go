package models

import "time"

type Notification struct {
	NotificationID      uint       `gorm:"primaryKey;column:notification_id" json:"notification_id"`
	UserID              uint       `gorm:"column:user_id" json:"user_id"`
	Title               string     `gorm:"column:title" json:"title"`
	Message             string     `gorm:"column:message" json:"message"`
	Type                string     `gorm:"column:type" json:"type"` // info|success|warning|error
	RelatedSubmissionID *uint      `gorm:"column:related_submission_id" json:"related_submission_id,omitempty"`
	IsRead              bool       `gorm:"column:is_read" json:"is_read"`
	CreateAt            time.Time  `gorm:"column:create_at" json:"created_at"`
	UpdateAt            *time.Time `gorm:"column:update_at" json:"-"`
}

func (Notification) TableName() string { return "notifications" }
