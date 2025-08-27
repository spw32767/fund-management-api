package models

import (
	"fund-management-api/config"
	"strconv"
)

// SystemConfig represents key-value configuration settings
// such as the current fiscal year.
type SystemConfig struct {
	Key   string `gorm:"primaryKey;column:key" json:"key"`
	Value string `gorm:"column:value" json:"value"`
}

// TableName specifies the table name for GORM
func (SystemConfig) TableName() string {
	return "system_config"
}

// GetCurrentYear fetches the current fiscal year from system_config
func GetCurrentYear() (int, error) {
	var cfg SystemConfig
	if err := config.DB.Where("`key` = ?", "current_year").First(&cfg).Error; err != nil {
		return 0, err
	}
	year, err := strconv.Atoi(cfg.Value)
	if err != nil {
		return 0, err
	}
	return year, nil
}
