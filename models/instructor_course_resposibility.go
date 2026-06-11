package models

import (
	"time"  
	"gorm.io/gorm"
)

type InstructorCourseResponsibility struct {
	//เพิ่มคอลัมน์ ID ตัวใหม่เข้ามาเป็น primaryKey และตั้งค่าเป็น autoIncrement
	ID        uint           `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	
	//ปลดคำว่า primaryKey ออกจาก UserID และ CourseID ให้เหลือแค่คุณสมบัติธรรมดา
	UserID    int            `gorm:"column:user_id" json:"user_id"`
	CourseID  int            `gorm:"column:course_id" json:"course_id"`
	
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	// การเชื่อมโยง (Relation) ตัวเดิม ยังคงใช้ได้เหมือนเดิม ไม่ต้องแก้ไขครับ 
	// เพราะยังอ้างอิงผ่านคอลัมน์ CourseID เพื่อทำ Preload เหมือนเดิม
	Course InstructorCourse `gorm:"foreignKey:CourseID;references:CourseID" json:"course"`
}

func (InstructorCourseResponsibility) TableName() string {
	return "instructor_course_responsibility"
}