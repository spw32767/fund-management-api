package models

import (
    "time" 
    "gorm.io/gorm"
)

type RankingTierWeight struct {
	TierWeightID int       `json:"tier_weight_id" gorm:"column:tier_weight_id;primaryKey;autoIncrement"`
	SourceID     int       `json:"source_id" gorm:"column:source_id"`
	TierCode     string    `json:"tier_code" gorm:"column:tier_code"`
	TierName     string    `json:"tier_name" gorm:"column:tier_name"`
	Description  string    `json:"description" gorm:"column:description"`
	ThaiDescription string `json:"thai_description" gorm:"column:thai_description"`
	Weight       float64   `json:"weight" gorm:"column:weight"` // decimal(5,2) ใน Go ใช้ float64
	SortOrder    int       `json:"sort_order" gorm:"column:sort_order"`
	IsActive     bool      `json:"is_active" gorm:"column:is_active"`
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
	RankingSource   *RankingSource `gorm:"foreignKey:SourceId" json:"ranking_source,omitempty"` 

}

func (RankingTierWeight) TableName() string {
	return "ranking_tier_weights"
}