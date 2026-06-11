package services

import (
	"context"
	"fund-management-api/models"

	"gorm.io/gorm"
)

// ─── Filter options ───────────────────────────────────────────────────────────

type AuditLogFilter struct {
	Action       string // "INSERT" | "UPDATE" | "DELETE" | "" = ทั้งหมด
	TargetTable  string // ชื่อตาราง | "" = ทั้งหมด
	UserEditID   int    // กรอง editor | 0 = ทั้งหมด
	TargetUserID int    // กรองอาจารย์ที่ถูกแก้ | 0 = ทั้งหมด
}

// ─── Interface ────────────────────────────────────────────────────────────────

type AuditLogService interface {
	GetLogs(ctx context.Context, filter AuditLogFilter) ([]models.InstructorEditLog, error)
	GetLogByID(ctx context.Context, id int) (*models.InstructorEditLog, error)
	GetDistinctTables(ctx context.Context) ([]string, error)
}

type auditLogService struct{ db *gorm.DB }

func NewAuditLogService(db *gorm.DB) AuditLogService {
	return &auditLogService{db: db}
}

// ─── GetLogs ──────────────────────────────────────────────────────────────────

func (s *auditLogService) GetLogs(ctx context.Context, filter AuditLogFilter) ([]models.InstructorEditLog, error) {
	var logs []models.InstructorEditLog

	q := s.db.WithContext(ctx).Order("id DESC")

	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if filter.TargetTable != "" {
		q = q.Where("table_name = ?", filter.TargetTable)
	}
	if filter.UserEditID != 0 {
		q = q.Where("user_edit_id = ?", filter.UserEditID)
	}
	if filter.TargetUserID != 0 {
		q = q.Where("target_user_id = ?", filter.TargetUserID)
	}

	// แก้ไขตรงนี้: ต่อ Chain ไปที่ q ตัวเดียวให้จบ พร้อม Preload ข้อมูลผู้แก้ไข
	err := q.Preload("UserEdit").Find(&logs).Error
	return logs, err
}

// ─── GetLogByID ───────────────────────────────────────────────────────────────

func (s *auditLogService) GetLogByID(ctx context.Context, id int) (*models.InstructorEditLog, error) {
	var log models.InstructorEditLog
	
	// แก้ไขตรงนี้: ลบคำสั่งซ้ำซ้อนออก เหลือเฉพาะตัวที่มี Preload บรรทัดเดียว
	if err := s.db.WithContext(ctx).Preload("UserEdit").First(&log, id).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

// ─── GetDistinctTables ────────────────────────────────────────────────────────

func (s *auditLogService) GetDistinctTables(ctx context.Context) ([]string, error) {
	var tables []string
	err := s.db.WithContext(ctx).
		Model(&models.InstructorEditLog{}).
		Distinct("table_name").
		Where("table_name IS NOT NULL AND table_name != ''").
		Order("table_name ASC").
		Pluck("table_name", &tables).Error
	return tables, err
}