package models

import (
    "time"  
    "gorm.io/gorm"
)

type RankingSource struct {
	SourceID    uint      `gorm:"primaryKey;autoIncrement;column:source_id" json:"source_id"`
	SourceCode  string    `json:"source_code" gorm:"column:source_code"`
	SourceName  string    `json:"source_name" gorm:"column:source_name"`
	Description string    `json:"description" gorm:"column:description"`
	IsActive    bool      `json:"is_active" gorm:"column:is_active"` // tinyint(1) ใน Go ใช้ bool ได้เลย
	CreatedAt   time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (RankingSource) TableName() string {
	return "ranking_sources"
}	

