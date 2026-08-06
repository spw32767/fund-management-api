package models

import "time"

// SDG represents one of the 17 Sustainable Development Goals.
type SDG struct {
	SDGID         int        `gorm:"primaryKey;column:sdg_id" json:"sdg_id"`
	SDGNumber     int        `gorm:"column:sdg_number" json:"sdg_number"`
	NameTH        string     `gorm:"column:name_th" json:"name_th"`
	NameEN        string     `gorm:"column:name_en" json:"name_en"`
	DescriptionTH *string    `gorm:"column:description_th" json:"description_th,omitempty"`
	DescriptionEN *string    `gorm:"column:description_en" json:"description_en,omitempty"`
	CreateAt      time.Time  `gorm:"column:create_at" json:"create_at"`
	UpdateAt      time.Time  `gorm:"column:update_at" json:"update_at"`
	DeleteAt      *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

func (SDG) TableName() string { return "sdgs" }
