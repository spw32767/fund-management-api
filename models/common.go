// สร้างไฟล์ models/common.go
package models

import "time"

// FileUpload represents the file_uploads table
type FileUpload struct {
	FileID       int        `gorm:"primaryKey;column:file_id" json:"file_id"`
	OriginalName string     `gorm:"column:original_name" json:"original_name"`
	StoredPath   string     `gorm:"column:stored_path" json:"stored_path"`
	FileSize     int64      `gorm:"column:file_size" json:"file_size"`
	MimeType     string     `gorm:"column:mime_type" json:"mime_type"`
	FileHash     string     `gorm:"column:file_hash" json:"file_hash"`
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
	CreateAt         time.Time  `gorm:"column:create_at" json:"create_at"`
	UpdateAt         time.Time  `gorm:"column:update_at" json:"update_at"`
	DeleteAt         *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

// TableName overrides
func (FileUpload) TableName() string {
	return "file_uploads"
}

func (DocumentType) TableName() string {
	return "document_types"
}

// Helper methods for file validation
func (f *FileUpload) IsValidImageType() bool {
	validTypes := []string{"image/jpeg", "image/jpg", "image/png", "image/gif"}
	for _, validType := range validTypes {
		if f.MimeType == validType {
			return true
		}
	}
	return false
}

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

func (f *FileUpload) GetFileSizeInMB() float64 {
	return float64(f.FileSize) / (1024 * 1024)
}
