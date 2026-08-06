package services

import (
	"context"
	"encoding/json"
	"fmt"
	"fund-management-api/models"

	"gorm.io/gorm"
)

type CourseService interface {
	GetAllCourses(ctx context.Context) ([]models.InstructorCourse, error)
	CreateCourse(ctx context.Context, editorID int, course models.InstructorCourse) (*models.InstructorCourse, error)
	UpdateCourse(ctx context.Context, editorID int, id uint, changes models.InstructorCourse) (*models.InstructorCourse, error)
	DeleteCourse(ctx context.Context, editorID int, id uint) error
}

type courseService struct {
	db *gorm.DB
}

func NewCourseService(db *gorm.DB) CourseService {
	return &courseService{db: db}
}

// GetAllCourses ดึงข้อมูลรายวิชา/หลักสูตรทั้งหมด เรียงตามระดับปริญญาและ ID วิชา
func (s *courseService) GetAllCourses(ctx context.Context) ([]models.InstructorCourse, error) {
	var courses []models.InstructorCourse
	err := s.db.WithContext(ctx).
		Order("degree_id ASC, course_id ASC").
		Find(&courses).Error
	return courses, err
}

// CreateCourse เพิ่มรายวิชาเข้าสู่ระบบ พร้อมบันทึก audit log
func (s *courseService) CreateCourse(ctx context.Context, editorID int, course models.InstructorCourse) (*models.InstructorCourse, error) {
	return &course, s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		course.CourseID = 0 // ให้ DB ทำ Auto-Increment เสมอ
		if err := tx.Create(&course).Error; err != nil {
			return err
		}
		newJSON, _ := json.Marshal(course)
		return tx.Create(&models.InstructorEditLog{
			UserEditID:   editorID,
			TargetUserID: nil,
			Action:       "INSERT",
			TargetTable:  "instructor_courses",
			FieldName:    "course_item",
			RecordID:     int(course.CourseID),
			OldValue:     "",
			NewValue:     string(newJSON),
		}).Error
	})
}

// UpdateCourse แก้ไขข้อมูลรายวิชา พร้อมบันทึก audit log เปรียบเทียบ old/new
func (s *courseService) UpdateCourse(ctx context.Context, editorID int, id uint, changes models.InstructorCourse) (*models.InstructorCourse, error) {
	var updated models.InstructorCourse
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old models.InstructorCourse
		if err := tx.First(&old, id).Error; err != nil {
			return fmt.Errorf("ไม่พบหลักสูตร ID %d", id)
		}

		if err := tx.Model(&old).Updates(changes).Error; err != nil {
			return err
		}

		// ดึงค่าล่าสุดหลัง update เพื่อ log ที่ถูกต้อง
		if err := tx.First(&updated, id).Error; err != nil {
			return err
		}

		oldJSON, _ := json.Marshal(old)
		newJSON, _ := json.Marshal(updated)

		// log เฉพาะเมื่อข้อมูลเปลี่ยนจริง
		if string(oldJSON) == string(newJSON) {
			return nil
		}

		return tx.Create(&models.InstructorEditLog{
			UserEditID:   editorID,
			TargetUserID: nil,
			Action:       "UPDATE",
			TargetTable:  "instructor_courses",
			FieldName:    "course_item",
			RecordID:     int(id),
			OldValue:     string(oldJSON),
			NewValue:     string(newJSON),
		}).Error
	})
	return &updated, err
}

// DeleteCourse ลบข้อมูลหลักสูตร (Soft Delete) พร้อมบันทึก audit log
func (s *courseService) DeleteCourse(ctx context.Context, editorID int, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old models.InstructorCourse
		if err := tx.First(&old, id).Error; err != nil {
			return fmt.Errorf("ไม่พบหลักสูตร ID %d", id)
		}
		if err := tx.Delete(&models.InstructorCourse{}, id).Error; err != nil {
			return err
		}
		oldJSON, _ := json.Marshal(old)
		return tx.Create(&models.InstructorEditLog{
			UserEditID:   editorID,
			TargetUserID: nil,
			Action:       "DELETE",
			TargetTable:  "instructor_courses",
			FieldName:    "course_item",
			RecordID:     int(id),
			OldValue:     string(oldJSON),
			NewValue:     "",
		}).Error
	})
}