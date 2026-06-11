package models

import (
    "time"  
    "gorm.io/gorm"
)
type InstructorCourseResponsibility struct {
    // กำหนดให้ทั้งคู่เป็น PrimaryKey
    UserID   int `gorm:"primaryKey;column:user_id" json:"user_id"`
    CourseID int `gorm:"primaryKey;column:course_id" json:"course_id"`
    CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`

    // เพิ่มการเชื่อมโยง (Relation) เพื่อให้ Preload("Course") ใน Service ไม่ระเบิด
    // foreignKey: ชี้ไปที่ CourseID ใน Struct นี้
    // references: ชี้ไปที่ CourseID ใน Struct InstructorCourse
    Course InstructorCourse `gorm:"foreignKey:CourseID;references:CourseID" json:"course"`
}

func (InstructorCourseResponsibility) TableName() string {
    return "instructor_course_responsibility"
}