package models

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ImportTemplate represents the import_templates table
// Structure mirrors fund_forms conventions and SQL in scripts/add_import_templates_table.sql.
type ImportTemplate struct {
	TemplateID   int        `gorm:"primaryKey;column:template_id" json:"template_id"`
	Title        string     `gorm:"column:title" json:"title"`
	Description  *string    `gorm:"column:description" json:"description"`
	FileName     string     `gorm:"column:file_name" json:"file_name"`
	FilePath     string     `gorm:"column:file_path" json:"file_path"`
	FileSize     *int64     `gorm:"column:file_size" json:"file_size"`
	MimeType     *string    `gorm:"column:mime_type" json:"mime_type"`
	TemplateType string     `gorm:"column:template_type;type:enum('user_import','legacy_submission','other');default:'other'" json:"template_type"`
	IsRequired   *bool      `gorm:"column:is_required" json:"is_required"`
	DisplayOrder *int       `gorm:"column:display_order" json:"display_order"`
	Status       string     `gorm:"column:status;type:enum('active','inactive','archived');default:'active'" json:"status"`
	YearID       *int       `gorm:"column:year_id" json:"year_id"`
	CreatedBy    int        `gorm:"column:created_by" json:"created_by"`
	CreateAt     time.Time  `gorm:"column:create_at" json:"create_at"`
	UpdateAt     time.Time  `gorm:"column:update_at" json:"update_at"`
	DeleteAt     *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	Creator User `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Year    Year `gorm:"foreignKey:YearID;references:YearID" json:"year_info,omitempty"`
}

func (ImportTemplate) TableName() string {
	return "import_templates"
}

func (t *ImportTemplate) GetFileSizeReadable() string {
	if t.FileSize == nil {
		return ""
	}
	kb := float64(*t.FileSize) / 1024.0
	if kb >= 1024 {
		return formatFloat(kb/1024.0) + " MB"
	}
	return formatFloat(kb) + " KB"
}

func (t *ImportTemplate) IsActive() bool {
	return t.Status == "active" && t.DeleteAt == nil
}

func (t *ImportTemplate) GetStatusName() string {
	if t.Status == "active" {
		return "เปิดใช้งาน"
	}
	if t.Status == "inactive" {
		return "ปิดใช้งาน"
	}
	return "เก็บถาวร"
}

func (t *ImportTemplate) GetTemplateTypeName() string {
	switch t.TemplateType {
	case "user_import":
		return "นำเข้าผู้ใช้"
	case "legacy_submission":
		return "นำเข้าประวัติทุนย้อนหลัง"
	default:
		return "อื่นๆ"
	}
}

func (t *ImportTemplate) normalizedFilePath() string {
	path := strings.ReplaceAll(t.FilePath, "\\", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// collapse duplicate slashes
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return path
}

func (t *ImportTemplate) ToResponse() ImportTemplateResponse {
	resp := ImportTemplateResponse{
		TemplateID:       t.TemplateID,
		Title:            t.Title,
		Description:      t.Description,
		FileName:         t.FileName,
		FilePath:         t.normalizedFilePath(),
		FileSize:         t.FileSize,
		FileSizeReadable: t.GetFileSizeReadable(),
		MimeType:         t.MimeType,
		TemplateType:     t.TemplateType,
		TemplateTypeName: t.GetTemplateTypeName(),
		DisplayOrder:     t.DisplayOrder,
		IsRequired:       t.IsRequired != nil && *t.IsRequired,
		Status:           t.Status,
		StatusName:       t.GetStatusName(),
		IsActive:         t.IsActive(),
		YearID:           t.YearID,
		CreatedBy:        t.CreatedBy,
		CreateAt:         t.CreateAt,
		UpdateAt:         t.UpdateAt,
	}

	if t.Creator.UserID != 0 {
		resp.CreatorName = strings.TrimSpace(t.Creator.UserFname + " " + t.Creator.UserLname)
		if resp.CreatorName == "" {
			resp.CreatorName = t.Creator.Email
		}
	}
	if t.Year.Year != "" {
		y := t.Year.Year
		resp.Year = &y
	}
	if resp.FileSizeReadable == "" && t.FileName != "" {
		if ext := filepath.Ext(t.FileName); ext != "" {
			resp.FileSizeReadable = strings.TrimPrefix(ext, ".")
		}
	}
	return resp
}

type ImportTemplateResponse struct {
	TemplateID       int       `json:"template_id"`
	Title            string    `json:"title"`
	Description      *string   `json:"description"`
	FileName         string    `json:"file_name"`
	FilePath         string    `json:"file_path"`
	FileSize         *int64    `json:"file_size"`
	FileSizeReadable string    `json:"file_size_readable"`
	MimeType         *string   `json:"mime_type"`
	TemplateType     string    `json:"template_type"`
	TemplateTypeName string    `json:"template_type_name"`
	DisplayOrder     *int      `json:"display_order"`
	IsRequired       bool      `json:"is_required"`
	Status           string    `json:"status"`
	StatusName       string    `json:"status_name"`
	IsActive         bool      `json:"is_active"`
	YearID           *int      `json:"year_id"`
	Year             *string   `json:"year"`
	CreatedBy        int       `json:"created_by"`
	CreatorName      string    `json:"creator_name,omitempty"`
	CreateAt         time.Time `json:"create_at"`
	UpdateAt         time.Time `json:"update_at"`
}

type ImportTemplateCreateRequest struct {
	Title        string  `json:"title" binding:"required"`
	Description  *string `json:"description"`
	TemplateType string  `json:"template_type" binding:"omitempty,oneof=user_import legacy_submission other"`
	IsRequired   *bool   `json:"is_required"`
	DisplayOrder *int    `json:"display_order"`
	Status       string  `json:"status" binding:"omitempty,oneof=active inactive archived"`
	YearID       *int    `json:"year_id"`
}

type ImportTemplateUpdateRequest struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	TemplateType *string `json:"template_type" binding:"omitempty,oneof=user_import legacy_submission other"`
	IsRequired   *bool   `json:"is_required"`
	DisplayOrder *int    `json:"display_order"`
	Status       *string `json:"status" binding:"omitempty,oneof=active inactive archived"`
	YearID       *int    `json:"year_id"`
}

func formatFloat(val float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", val), "0"), ".")
}
