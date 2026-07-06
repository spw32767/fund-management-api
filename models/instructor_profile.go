package models 

import (
    "time"  
    "gorm.io/gorm"
)

type InstructorProfileHeader struct {
    // ใส่ gorm:"primaryKey;column:user_id" เพื่อบอกว่านี่คือ PK และชื่อคอลัมน์คือ user_id
    UserID           int    `json:"user_id" gorm:"primaryKey;column:user_id"`
    InstructorPrefix string `json:"prefix" gorm:"column:prefix"`
    ThaiFirstName    string `json:"user_fname" gorm:"column:user_fname"`
    ThaiLastName     string `json:"user_lname" gorm:"column:user_lname"`
    EngName          string `json:"Name_en" gorm:"column:Name_en"`
	Email			string `json:"email" gorm:"column:email"`
	Tel				string `json:"tel" gorm:"column:tel"`
    Position         string `json:"position" gorm:"column:position"`
    LinkScopus       string `json:"scopus_id" gorm:"column:scopus_id"`
    LinkGoogleScholar string `json:"scholar_author_id" gorm:"column:scholar_author_id"`
    LinkThaiJo         string `json:"thaijo_author_id" gorm:"column:thaijo_author_id"`
    DateOfEmployment    string `json:"date_of_employment" gorm:"column:date_of_employment"`
    InstructorCourseResponsibility []InstructorCourseResponsibility `json:"courses_data" gorm:"foreignKey:UserID;references:UserID"`
}

type InstructorEducationTab struct {
	ID            uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	DegreeID      int            `gorm:"column:degree_id;not null" json:"degree_id"`
	UserID        int            `gorm:"column:user_id;not null" json:"user_id"`
	DegreeTitleTh string         `gorm:"column:degree_title_th;type:varchar(255);not null" json:"degree_title_th"`
	UniversityTh  string         `gorm:"column:university_th;type:varchar(255);not null" json:"university_th"`
	Country       string         `gorm:"column:country;type:varchar(255);not null" json:"country"`
	GradYear      string         `gorm:"column:grad_year;type:char(4);not null" json:"grad_year"` // เก็บเป็น string เพื่อรองรับ CHAR(4) เช่น "2569"
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

type InstructorExpertiseTab struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int            `gorm:"column:user_id;not null" json:"user_id"`
	Expertise string         `gorm:"column:expertise;type:text;not null" json:"expertise"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}
// 1. ตัวอย่างสำหรับโมเดล Header ให้ไปดึงจากตาราง users
func (InstructorProfileHeader) TableName() string {
    return "users"
}

// 2. ตัวอย่างสำหรับโมเดล Education ให้ไปดึงจากตาราง instructor_educations
func (InstructorEducationTab) TableName() string {
    return "instructor_educations"
}

// 3. ตัวอย่างสำหรับโมเดล Expertise
func (InstructorExpertiseTab) TableName() string {
    return "instructor_expertises"
}
