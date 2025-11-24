package models

import (
	"encoding/json"
	"time"
)

type NotificationMessage struct {
	ID            uint            `gorm:"primaryKey;column:id" json:"id"`
	EventKey      string          `gorm:"column:event_key" json:"event_key"`
	SendTo        string          `gorm:"column:send_to" json:"send_to"`
	TitleTemplate string          `gorm:"column:title_template" json:"title_template"`
	BodyTemplate  string          `gorm:"column:body_template" json:"body_template"`
	Description   *string         `gorm:"column:description" json:"description,omitempty"`
	Variables     json.RawMessage `gorm:"column:variables" json:"variables"`
	IsActive      bool            `gorm:"column:is_active" json:"is_active"`
	UpdatedBy     *uint           `gorm:"column:updated_by" json:"updated_by,omitempty"`
	CreatedAt     time.Time       `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time       `gorm:"column:updated_at" json:"updated_at"`
}

func (NotificationMessage) TableName() string { return "notification_message" }