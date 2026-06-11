package models
import (
    "time"  
    "gorm.io/gorm"
)

type InstructorDegree struct {
	DegreeID     uint           `gorm:"primaryKey;autoIncrement;column:degree_id" json:"degree_id"`
	DegreeNameTh string         `gorm:"column:degree_nameTH;type:varchar(255);not null" json:"degree_name_th"`
	DegreeNameEn string         `gorm:"column:degree_nameEN;type:varchar(255);not null" json:"degree_name_en"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (InstructorDegree) TableName() string {
	return "instructor_degrees"
}