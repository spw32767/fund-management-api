package models

import "time"

type Innovation struct {
	ID             uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID         uint       `json:"user_id" gorm:"not null;index"`
	Title          string     `json:"title" gorm:"type:varchar(500);not null"`
	InnovationType *string    `json:"innovation_type,omitempty" gorm:"type:varchar(255)"`
	Description    *string    `json:"description,omitempty" gorm:"type:text"`
	RegisteredDate *time.Time `json:"registered_date,omitempty" gorm:"type:date"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (Innovation) TableName() string { return "innovations" }
