// models/reward_config.go
package models

import (
	"time"
)

// RewardConfig represents reward configuration for manuscript and page charge fees
type RewardConfig struct {
	ConfigID int    `gorm:"primaryKey;column:config_id" json:"config_id"`
	Year     string `gorm:"column:year" json:"year"`
	//ConfigType           string     `gorm:"column:config_type" json:"config_type"`
	JournalQuartile      string     `gorm:"column:journal_quartile" json:"journal_quartile"`
	MaxAmount            float64    `gorm:"column:max_amount" json:"max_amount"`
	ConditionDescription string     `gorm:"column:condition_description" json:"condition_description,omitempty"`
	IsActive             bool       `gorm:"column:is_active" json:"is_active"`
	CreateAt             *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt             *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt             *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

// TableName overrides the table name
func (RewardConfig) TableName() string {
	return "reward_config"
}
