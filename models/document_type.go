// models/document_type.go
// สร้างไฟล์นี้ใหม่เพื่อแก้ปัญหา DocumentType struct ซ้ำกัน

package models

import (
	"time"
)

// DocumentType represents document types for submissions
type DocumentType struct {
	DocumentTypeID   int        `gorm:"primaryKey;column:document_type_id" json:"document_type_id"`
	DocumentTypeName string     `gorm:"column:document_type_name" json:"document_type_name"`
	Code             string     `gorm:"column:code" json:"code"`
	Category         string     `gorm:"column:category" json:"category"`
	Required         bool       `gorm:"column:required" json:"required"`
	Multiple         bool       `gorm:"column:multiple" json:"multiple"`
	DocumentOrder    int        `gorm:"column:document_order" json:"document_order"`
	IsRequired       string     `gorm:"column:is_required" json:"is_required"` // enum: yes, no
	CreateAt         *time.Time `gorm:"column:create_at" json:"create_at"`
	UpdateAt         *time.Time `gorm:"column:update_at" json:"update_at"`
	DeleteAt         *time.Time `gorm:"column:delete_at" json:"delete_at,omitempty"`

	// Relations (เพิ่มเฉพาะเมื่อจำเป็น)
	FundRequirements []FundDocumentRequirement `gorm:"foreignKey:DocumentTypeID" json:"fund_requirements,omitempty"`
}

// TableName for DocumentType
func (DocumentType) TableName() string {
	return "document_types"
}

// Helper Methods

// GetRequirementsForFundType ดึง requirements สำหรับ fund type เฉพาะ
func (dt *DocumentType) GetRequirementsForFundType(fundType string) []FundDocumentRequirement {
	var requirements []FundDocumentRequirement
	for _, req := range dt.FundRequirements {
		if req.FundType == fundType && req.IsActive {
			requirements = append(requirements, req)
		}
	}
	return requirements
}

// IsRequiredForFund ตรวจสอบว่าเอกสารนี้บังคับสำหรับ fund type และ subcategory หรือไม่
func (dt *DocumentType) IsRequiredForFund(fundType string, subcategoryID *int) bool {
	for _, req := range dt.FundRequirements {
		if req.FundType == fundType && req.IsActive {
			// ถ้า subcategoryID เป็น nil แสดงว่าใช้ได้กับทุก subcategory
			if req.SubcategoryID == nil {
				return req.IsRequired
			}
			// ถ้าตรงกับ subcategory ที่ระบุ
			if subcategoryID != nil && req.SubcategoryID != nil && *req.SubcategoryID == *subcategoryID {
				return req.IsRequired
			}
		}
	}
	return false
}
