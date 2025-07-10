package models

import (
	"time"
)

type ApplicationDocument struct {
	DocumentID       int        `gorm:"primaryKey;column:document_id" json:"document_id"`
	ApplicationID    int        `gorm:"column:application_id" json:"application_id"`
	DocumentTypeID   int        `gorm:"column:document_type_id" json:"document_type_id"`
	UploadedBy       int        `gorm:"column:uploaded_by" json:"uploaded_by"`
	OriginalFilename string     `gorm:"column:original_filename" json:"original_filename"`
	StoredFilename   string     `gorm:"column:stored_filename" json:"stored_filename"`
	FileType         string     `gorm:"column:file_type" json:"file_type"`
	UploadedAt       *time.Time `gorm:"column:uploaded_at" json:"uploaded_at"`
	CreateAt         *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt         *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt         *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	// Relations
	Application  FundApplication `gorm:"foreignKey:ApplicationID" json:"application,omitempty"`
	DocumentType DocumentType    `gorm:"foreignKey:DocumentTypeID" json:"document_type,omitempty"`
}

type DocumentType struct {
	DocumentTypeID   int        `gorm:"primaryKey;column:document_type_id" json:"document_type_id"`
	DocumentTypeName string     `gorm:"column:document_type_name" json:"document_type_name"`
	Code             string     `gorm:"column:code" json:"code"`
	Category         string     `gorm:"column:category" json:"category"`
	Required         bool       `gorm:"column:required" json:"required"`
	Multiple         bool       `gorm:"column:multiple" json:"multiple"`
	IsRequired       string     `gorm:"column:is_required" json:"is_required"`
	CreateAt         *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt         *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt         *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`
}

// TableName overrides
func (ApplicationDocument) TableName() string {
	return "application_documents"
}

func (DocumentType) TableName() string {
	return "document_types"
}
