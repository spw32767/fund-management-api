// models/common.go

package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileUpload represents the file_uploads table
type FileUpload struct {
	FileID       int        `gorm:"primaryKey;column:file_id" json:"file_id"`
	OriginalName string     `gorm:"column:original_name" json:"original_name"`
	StoredPath   string     `gorm:"column:stored_path" json:"stored_path"`
	FileSize     int64      `gorm:"column:file_size" json:"file_size"`
	MimeType     string     `gorm:"column:mime_type" json:"mime_type"`
	FileHash     string     `gorm:"column:file_hash" json:"file_hash"` // เก็บไว้แต่ไม่ใช้
	IsPublic     bool       `gorm:"column:is_public" json:"is_public"`
	UploadedBy   int        `gorm:"column:uploaded_by" json:"uploaded_by"`
	UploadedAt   time.Time  `gorm:"column:uploaded_at" json:"uploaded_at"`
	CreateAt     time.Time  `gorm:"column:create_at" json:"create_at"`
	UpdateAt     time.Time  `gorm:"column:update_at" json:"update_at"`
	DeleteAt     *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	// Relations
	Uploader User `gorm:"foreignKey:UploadedBy" json:"uploader,omitempty"`
}

// DocumentType represents document types for submissions
type DocumentType struct {
	DocumentTypeID   int        `gorm:"primaryKey;column:document_type_id" json:"document_type_id"`
	DocumentTypeName string     `gorm:"column:document_type_name" json:"document_type_name"`
	Code             string     `gorm:"column:code" json:"code"`
	Category         string     `gorm:"column:category" json:"category"`
	Required         bool       `gorm:"column:required" json:"required"`
	Multiple         bool       `gorm:"column:multiple" json:"multiple"`
	DocumentOrder    int        `gorm:"column:document_order" json:"document_order"`
	IsRequired       *string    `gorm:"column:is_required" json:"is_required"` // enum('yes','no')
	CreateAt         time.Time  `gorm:"column:create_at" json:"create_at"`
	UpdateAt         time.Time  `gorm:"column:update_at" json:"update_at"`
	DeleteAt         *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	// เพิ่มฟิลด์ใหม่
	FundTypes      *string `gorm:"column:fund_types" json:"fund_types"`           // JSON field
	SubcategoryIds *string `gorm:"column:subcategory_ids" json:"subcategory_ids"` // JSON field
}

// TableName overrides
func (FileUpload) TableName() string {
	return "file_uploads"
}

func (DocumentType) TableName() string {
	return "document_types"
}

// ===== Helper methods สำหรับ FileUpload =====

// GetReadablePath แปลง path ให้อ่านง่าย
func (f *FileUpload) GetReadablePath() string {
	return strings.ReplaceAll(f.StoredPath, "\\", "/")
}

// GetRelativePath ตัด upload path ออก
func (f *FileUpload) GetRelativePath() string {
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}
	return strings.TrimPrefix(f.StoredPath, uploadPath+string(filepath.Separator))
}

// GetUserFolder ดึงชื่อโฟลเดอร์ user จาก path
func (f *FileUpload) GetUserFolder() string {
	parts := strings.Split(f.GetRelativePath(), string(filepath.Separator))
	if len(parts) >= 2 && parts[0] == "users" {
		return parts[1]
	}
	return ""
}

// GetSubmissionFolder ดึงชื่อโฟลเดอร์ submission จาก path
func (f *FileUpload) GetSubmissionFolder() string {
	parts := strings.Split(f.GetRelativePath(), string(filepath.Separator))
	if len(parts) >= 4 && parts[0] == "users" && parts[2] == "submissions" {
		return parts[3]
	}
	return ""
}

// IsInTempFolder ตรวจสอบว่าไฟล์อยู่ในโฟลเดอร์ temp หรือไม่
func (f *FileUpload) IsInTempFolder() bool {
	return strings.Contains(f.StoredPath, "/temp/") || strings.Contains(f.StoredPath, "\\temp\\")
}

// IsInSubmissionFolder ตรวจสอบว่าไฟล์อยู่ในโฟลเดอร์ submission หรือไม่
func (f *FileUpload) IsInSubmissionFolder() bool {
	return strings.Contains(f.StoredPath, "/submissions/") || strings.Contains(f.StoredPath, "\\submissions\\")
}

// GetFileExtension ดึงนามสกุลไฟล์
func (f *FileUpload) GetFileExtension() string {
	return strings.ToLower(filepath.Ext(f.OriginalName))
}

// IsValidImageType ตรวจสอบว่าเป็นไฟล์รูปภาพหรือไม่
func (f *FileUpload) IsValidImageType() bool {
	validTypes := []string{"image/jpeg", "image/jpg", "image/png", "image/gif"}
	for _, validType := range validTypes {
		if f.MimeType == validType {
			return true
		}
	}
	return false
}

// IsValidDocumentType ตรวจสอบว่าเป็นไฟล์เอกสารหรือไม่
func (f *FileUpload) IsValidDocumentType() bool {
	validTypes := []string{
		"application/pdf",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.ms-excel",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}
	for _, validType := range validTypes {
		if f.MimeType == validType {
			return true
		}
	}
	return false
}

// GetFileSizeInMB คำนวณขนาดไฟล์เป็น MB
func (f *FileUpload) GetFileSizeInMB() float64 {
	return float64(f.FileSize) / (1024 * 1024)
}

// GetFormattedFileSize แสดงขนาดไฟล์ในรูปแบบที่อ่านง่าย
func (f *FileUpload) GetFormattedFileSize() string {
	size := float64(f.FileSize)
	units := []string{"B", "KB", "MB", "GB"}

	for i, unit := range units {
		if size < 1024 || i == len(units)-1 {
			if i == 0 {
				return fmt.Sprintf("%.0f %s", size, unit)
			}
			return fmt.Sprintf("%.2f %s", size, unit)
		}
		size /= 1024
	}
	return ""
}

// FileExists ตรวจสอบว่าไฟล์มีอยู่จริงใน filesystem หรือไม่
func (f *FileUpload) FileExists() bool {
	_, err := os.Stat(f.StoredPath)
	return !os.IsNotExist(err)
}
