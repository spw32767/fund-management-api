package models

import (
	"time"

	"gorm.io/gorm"
)

type InstructorIntellectualProperty struct {
	ID                 uint           `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserID             int            `gorm:"column:user_id" json:"user_id"`
	Type               string         `gorm:"column:type" json:"type"`
	Title              string         `gorm:"column:title" json:"title"`
	RegistrationNumber *string        `gorm:"column:registration_number" json:"registration_number"`
	GrantedYear        *int           `gorm:"column:granted_year" json:"granted_year"`
	CreatedAt          time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index;column:deleted_at" json:"deleted_at"`
	TierDetails        *RankingTierWeight `json:"tier_details,omitempty" gorm:"-"`
	User               User           `gorm:"foreignKey:UserID;references:UserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"-"`
}

func (InstructorIntellectualProperty) TableName() string {
	return "instructor_intellectual_properties"
}