package models

import (
    "time"  
    "gorm.io/gorm"
)

type InstructorCourse struct {
	CourseID      uint           `gorm:"primaryKey;autoIncrement;column:course_id" json:"course_id"`
	DegreeID      int            `gorm:"column:degree_id;not null" json:"degree_id"`
	CourseNameTh  string         `gorm:"column:course_nameTH;type:varchar(255);not null" json:"course_name_th"`
	CourseNameEn  string         `gorm:"column:course_nameEN;type:varchar(255);not null" json:"course_name_en"`
	DegreeFullTh  string         `gorm:"column:degree_fullTH;type:varchar(255);not null" json:"degree_full_th"`
	DegreeShortTh string         `gorm:"column:degree_shortTH;type:varchar(100);not null" json:"degree_short_th"`
	DegreeFullEn  string         `gorm:"column:degree_fullEN;type:varchar(255);not null" json:"degree_full_en"`
	DegreeShortEn string         `gorm:"column:degree_shortEN;type:varchar(100);not null" json:"degree_short_en"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (InstructorCourse) TableName() string {
	return "instructor_courses"
}