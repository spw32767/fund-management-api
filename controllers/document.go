// controllers/document.go
package controllers

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// UploadDocument handles document upload for application (User-Based Folders)
func UploadDocument(c *gin.Context) {
	applicationID := c.Param("id")
	userID, _ := c.Get("userID")

	// Check if application exists and belongs to user
	var application models.FundApplication
	if err := config.DB.Where("application_id = ? AND user_id = ? AND delete_at IS NULL",
		applicationID, userID).First(&application).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
		return
	}

	// Check if application is still pending
	if application.ApplicationStatusID != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot upload documents to processed applications"})
		return
	}

	// Get document type
	documentTypeID, err := strconv.Atoi(c.PostForm("document_type_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document type"})
		return
	}

	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	// Validate file size
	maxSize := int64(10 * 1024 * 1024) // 10MB
	if file.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
		return
	}

	// Validate file type
	allowedTypes := map[string]bool{
		".pdf":  true,
		".doc":  true,
		".docx": true,
		".xls":  true,
		".xlsx": true,
		".png":  true,
		".jpg":  true,
		".jpeg": true,
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedTypes[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
		return
	}

	// Get user info
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}

	// Create user-based path
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	// Create user folder if not exists
	userFolderPath, err := utils.CreateUserFolderIfNotExists(user, uploadPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user directory"})
		return
	}

	// Convert applicationID to int
	appID, _ := strconv.Atoi(applicationID)

	// Create submission folder for this application
	submissionFolderPath, err := utils.CreateSubmissionFolder(
		userFolderPath, "fund", appID, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create submission directory"})
		return
	}

	// Use original filename with safety checks
	safeFilename := utils.GenerateUniqueFilename(submissionFolderPath, file.Filename)
	fullPath := filepath.Join(submissionFolderPath, safeFilename)

	// Save file
	if err := c.SaveUploadedFile(file, fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Save to database - Create FileUpload record first
	now := time.Now()
	fileUpload := models.FileUpload{
		OriginalName: file.Filename,
		StoredPath:   fullPath,
		FileSize:     file.Size,
		MimeType:     file.Header.Get("Content-Type"),
		FileHash:     "", // ไม่ใช้ hash ในระบบ user-based
		IsPublic:     false,
		UploadedBy:   userID.(int),
		UploadedAt:   now,
		CreateAt:     now,
		UpdateAt:     now,
	}

	if err := config.DB.Create(&fileUpload).Error; err != nil {
		// Delete uploaded file if database save fails
		os.Remove(fullPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file info"})
		return
	}

	// Create application document record ใช้ field ที่มีจริงใน models.ApplicationDocument
	document := models.ApplicationDocument{
		ApplicationID:    application.ApplicationID,
		DocumentTypeID:   documentTypeID,
		UploadedBy:       userID.(int),
		OriginalFilename: file.Filename,                // ใช้ field ที่มีจริง
		StoredFilename:   safeFilename,                 // ใช้ field ที่มีจริง
		FileType:         strings.TrimPrefix(ext, "."), // ใช้ field ที่มีจริง
		UploadedAt:       &now,
		CreateAt:         &now,
		UpdateAt:         &now,
	}

	if err := config.DB.Create(&document).Error; err != nil {
		// Delete uploaded file and FileUpload record if document creation fails
		os.Remove(fullPath)
		config.DB.Delete(&fileUpload)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save document record"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "File uploaded successfully",
		"document": document,
		"file":     fileUpload,
	})
}

// GetDocuments returns all documents for an application
func GetDocuments(c *gin.Context) {
	applicationID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Check permissions
	query := config.DB.Where("application_id = ?", applicationID)
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	var application models.FundApplication
	if err := query.First(&application).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
		return
	}

	// Get documents
	var documents []models.ApplicationDocument
	if err := config.DB.Preload("DocumentType").
		Where("application_id = ? AND delete_at IS NULL", applicationID).
		Find(&documents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch documents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"documents": documents,
		"total":     len(documents),
	})
}

// DownloadDocument handles document download
func DownloadDocument(c *gin.Context) {
	documentID := c.Param("document_id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Get document info
	var document models.ApplicationDocument
	if err := config.DB.Preload("Application").
		Where("document_id = ? AND application_documents.delete_at IS NULL", documentID).
		First(&document).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	// Check permissions
	if roleID.(int) != 3 && document.Application.UserID != userID.(int) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Find file
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	// Try to find file in year/month subdirectory
	uploadTime := document.UploadedAt
	subDir := filepath.Join(uploadPath, uploadTime.Format("2006/01"))
	fullPath := filepath.Join(subDir, document.StoredFilename)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		// Try root upload directory
		fullPath = filepath.Join(uploadPath, document.StoredFilename)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
			return
		}
	}

	// Set headers for download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", document.OriginalFilename))
	c.Header("Content-Type", "application/octet-stream")

	c.File(fullPath)
}

// DeleteDocument soft deletes a document
func DeleteDocument(c *gin.Context) {
	documentID := c.Param("document_id")
	userID, _ := c.Get("userID")

	// Get document
	var document models.ApplicationDocument
	if err := config.DB.Preload("Application").
		Where("document_id = ? AND application_documents.delete_at IS NULL", documentID).
		First(&document).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	// Check ownership
	if document.Application.UserID != userID.(int) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Check if application is still pending
	if document.Application.ApplicationStatusID != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete documents from processed applications"})
		return
	}

	// Soft delete
	now := time.Now()
	document.DeleteAt = &now

	if err := config.DB.Save(&document).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete document"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Document deleted successfully"})
}

func GetDocumentTypes(c *gin.Context) {
	// เรียกใช้ legacy function เพื่อ backward compatibility
	GetDocumentTypesLegacy(c)
}

// GetDocumentTypesAdmin ดึงข้อมูล document types ทั้งหมดสำหรับ Admin
// GET /api/admin/document-types
func GetDocumentTypesAdmin(c *gin.Context) {
	var documentTypes []models.DocumentType

	query := config.DB.Order("document_order ASC")

	// Include soft deleted records for admin
	if c.Query("include_deleted") == "true" {
		query = query.Unscoped()
	} else {
		query = query.Where("delete_at IS NULL OR delete_at = ''")
	}

	if err := query.Find(&documentTypes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch document types"})
		return
	}

	// Transform for frontend
	var result []map[string]interface{}
	for _, dt := range documentTypes {
		result = append(result, map[string]interface{}{
			"document_type_id":   dt.DocumentTypeID,
			"document_type_name": dt.DocumentTypeName,
			"code":               dt.Code,
			"category":           dt.Category,
			"required":           dt.Required,
			"multiple":           dt.Multiple,
			"document_order":     dt.DocumentOrder,
			"is_required":        dt.Required,
			"create_at":          dt.CreateAt,
			"update_at":          dt.UpdateAt,
			"delete_at":          dt.DeleteAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"document_types": result,
		"total":          len(result),
	})
}

// CreateDocumentType สร้าง document type ใหม่ (Admin only)
// POST /api/admin/document-types
func CreateDocumentType(c *gin.Context) {
	var request struct {
		DocumentTypeName string `json:"document_type_name" binding:"required"`
		Code             string `json:"code" binding:"required"`
		Category         string `json:"category"`
		Required         bool   `json:"required"`
		Multiple         bool   `json:"multiple"`
		DocumentOrder    int    `json:"document_order"`
		IsRequired       string `json:"is_required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if code already exists
	var existingCount int64
	config.DB.Model(&models.DocumentType{}).Where("code = ?", request.Code).Count(&existingCount)
	if existingCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Document type code already exists"})
		return
	}

	documentType := models.DocumentType{
		DocumentTypeName: request.DocumentTypeName,
		Code:             request.Code,
		Category:         request.Category,
		Required:         request.Required,
		Multiple:         request.Multiple,
		DocumentOrder:    request.DocumentOrder,
		// IsRequired:       request.IsRequired, // Removed because models.DocumentType does not have this field
	}

	if err := config.DB.Create(&documentType).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create document type"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":       true,
		"message":       "Document type created successfully",
		"document_type": documentType,
	})
}

// UpdateDocumentType อัพเดท document type (Admin only)
// PUT /api/admin/document-types/:id
func UpdateDocumentType(c *gin.Context) {
	id := c.Param("id")

	var request struct {
		DocumentTypeName *string `json:"document_type_name"`
		Code             *string `json:"code"`
		Category         *string `json:"category"`
		Required         *bool   `json:"required"`
		Multiple         *bool   `json:"multiple"`
		DocumentOrder    *int    `json:"document_order"`
		IsRequired       *string `json:"is_required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var documentType models.DocumentType
	if err := config.DB.First(&documentType, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document type not found"})
		return
	}

	// Check if new code conflicts with existing
	if request.Code != nil && *request.Code != documentType.Code {
		var existingCount int64
		config.DB.Model(&models.DocumentType{}).Where("code = ? AND document_type_id != ?", *request.Code, id).Count(&existingCount)
		if existingCount > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "Document type code already exists"})
			return
		}
		documentType.Code = *request.Code
	}

	// Update fields
	if request.DocumentTypeName != nil {
		documentType.DocumentTypeName = *request.DocumentTypeName
	}
	if request.Category != nil {
		documentType.Category = *request.Category
	}
	if request.Required != nil {
		documentType.Required = *request.Required
	}
	if request.Multiple != nil {
		documentType.Multiple = *request.Multiple
	}
	if request.DocumentOrder != nil {
		documentType.DocumentOrder = *request.DocumentOrder
	}
	// Removed assignment to IsRequired because models.DocumentType does not have this field

	if err := config.DB.Save(&documentType).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update document type"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "Document type updated successfully",
		"document_type": documentType,
	})
}

// DeleteDocumentType ลบ document type (Admin only)
// DELETE /api/admin/document-types/:id
func DeleteDocumentType(c *gin.Context) {
	id := c.Param("id")

	var documentType models.DocumentType
	if err := config.DB.First(&documentType, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document type not found"})
		return
	}

	// Check if document type is being used
	var usageCount int64
	config.DB.Model(&models.FundDocumentRequirement{}).Where("document_type_id = ?", id).Count(&usageCount)
	if usageCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":       "Cannot delete document type as it is being used in fund requirements",
			"usage_count": usageCount,
		})
		return
	}

	// Check if used in submissions
	var submissionUsageCount int64
	config.DB.Model(&models.SubmissionDocument{}).Where("document_type_id = ?", id).Count(&submissionUsageCount)
	if submissionUsageCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":       "Cannot delete document type as it is being used in submissions",
			"usage_count": submissionUsageCount,
		})
		return
	}

	// Soft delete
	if err := config.DB.Delete(&documentType).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete document type"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Document type deleted successfully",
	})
}

// RestoreDocumentType กู้คืน document type (Admin only)
// POST /api/admin/document-types/:id/restore
func RestoreDocumentType(c *gin.Context) {
	id := c.Param("id")

	var documentType models.DocumentType
	if err := config.DB.Unscoped().First(&documentType, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document type not found"})
		return
	}

	if documentType.DeleteAt == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type is not deleted"})
		return
	}

	// Restore by setting DeleteAt to nil
	documentType.DeleteAt = nil
	if err := config.DB.Unscoped().Save(&documentType).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore document type"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "Document type restored successfully",
		"document_type": documentType,
	})
}

// GetDocumentTypeUsage ดูการใช้งานของ document type
// GET /api/admin/document-types/:id/usage
func GetDocumentTypeUsage(c *gin.Context) {
	id := c.Param("id")

	var documentType models.DocumentType
	if err := config.DB.First(&documentType, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document type not found"})
		return
	}

	// Get fund requirements using this document type
	var fundRequirements []models.FundDocumentRequirementView
	config.DB.Table("v_fund_document_requirements").
		Where("document_type_id = ?", id).
		Find(&fundRequirements)

	// Get submission documents using this document type
	var submissionCount int64
	config.DB.Model(&models.SubmissionDocument{}).
		Where("document_type_id = ?", id).
		Count(&submissionCount)

	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"document_type":     documentType,
		"fund_requirements": fundRequirements,
		"submission_usage":  submissionCount,
		"total_usage":       len(fundRequirements) + int(submissionCount),
	})
}
