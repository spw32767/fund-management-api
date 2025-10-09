package controllers

import (
	"encoding/json"
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

// GetDocumentTypes returns document types with filtering by fund_type and subcategory_id
func GetDocumentTypes(c *gin.Context) {
	var documentTypes []models.DocumentType

	// Get query parameters
	category := c.Query("category")
	fundType := c.Query("fund_type")              // "publication_reward" หรือ "fund_application"
	subcategoryIdStr := c.Query("subcategory_id") // "1", "2", etc.

	// Build query
	query := config.DB.Where("delete_at IS NULL")

	// Filter by category if provided
	if category != "" {
		query = query.Where("category = ?", category)
	}

	if err := query.Order("document_order").Find(&documentTypes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch document types"})
		return
	}

	// Apply fund_type and subcategory filtering
	var filteredTypes []models.DocumentType
	for _, dt := range documentTypes {
		shouldInclude := true

		// Filter by fund_type
		if fundType != "" && dt.FundTypes != nil {
			var fundTypes []string
			if err := json.Unmarshal([]byte(*dt.FundTypes), &fundTypes); err != nil {
				// ถ้า parse JSON ไม่ได้ ให้รวมไว้ด้วย
				continue
			}

			// ตรวจสอบว่า fund_type ที่ต้องการอยู่ใน array หรือไม่
			found := false
			for _, ft := range fundTypes {
				if ft == fundType {
					found = true
					break
				}
			}
			if !found {
				shouldInclude = false
			}
		}

		// Filter by subcategory_id
		if subcategoryIdStr != "" && shouldInclude && dt.SubcategoryIds != nil {
			subcategoryId, err := strconv.Atoi(subcategoryIdStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_id format"})
				return
			}

			var subcategoryIds []int
			if err := json.Unmarshal([]byte(*dt.SubcategoryIds), &subcategoryIds); err != nil {
				// ถ้า parse JSON ไม่ได้ ให้รวมไว้ด้วย
			} else {
				// ตรวจสอบว่า subcategory_id ที่ต้องการอยู่ใน array หรือไม่
				found := false
				for _, scId := range subcategoryIds {
					if scId == subcategoryId {
						found = true
						break
					}
				}
				if !found {
					shouldInclude = false
				}
			}
		}

		if shouldInclude {
			filteredTypes = append(filteredTypes, dt)
		}
	}

	// Transform for frontend - maintain backward compatibility
	var result []map[string]interface{}
	for _, dt := range filteredTypes {
		documentTypeMap := map[string]interface{}{
			// Original fields for backward compatibility
			"id":       dt.DocumentTypeID,
			"code":     dt.Code,
			"name":     dt.DocumentTypeName,
			"required": dt.Required,
			"multiple": dt.Multiple,
			"category": dt.Category,

			// Additional fields
			"document_order": dt.DocumentOrder,

			// New naming convention (for future use)
			"document_type_id":   dt.DocumentTypeID,
			"document_type_name": dt.DocumentTypeName,
		}

		// Add new fields if they exist
		if dt.FundTypes != nil {
			var fundTypes []string
			if err := json.Unmarshal([]byte(*dt.FundTypes), &fundTypes); err == nil {
				documentTypeMap["fund_types"] = fundTypes
			}
		}

		if dt.SubcategoryIds != nil {
			var subcategoryIds []int
			if err := json.Unmarshal([]byte(*dt.SubcategoryIds), &subcategoryIds); err == nil {
				documentTypeMap["subcategory_ids"] = subcategoryIds
			}
		}

		result = append(result, documentTypeMap)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"document_types": result,
	})
}

// GetDocumentTypesAdmin - Admin endpoint to manage document types (CRUD)
func GetDocumentTypesAdmin(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	var documentTypes []models.DocumentType

	if err := config.DB.Where("delete_at IS NULL").Order("document_order").Find(&documentTypes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch document types"})
		return
	}

	// Transform for admin frontend with all fields
	var result []map[string]interface{}
	for _, dt := range documentTypes {
		documentTypeMap := map[string]interface{}{
			"document_type_id":   dt.DocumentTypeID,
			"document_type_name": dt.DocumentTypeName,
			"code":               dt.Code,
			"category":           dt.Category,
			"required":           dt.Required,
			"multiple":           dt.Multiple,
			"document_order":     dt.DocumentOrder,
			"create_at":          dt.CreateAt,
			"update_at":          dt.UpdateAt,
		}

		// Handle IsRequired (pointer to string for enum)
		if dt.IsRequired != nil {
			documentTypeMap["is_required"] = *dt.IsRequired
		} else {
			documentTypeMap["is_required"] = nil
		}

		// Parse and add fund_types
		if dt.FundTypes != nil {
			var fundTypes []string
			if err := json.Unmarshal([]byte(*dt.FundTypes), &fundTypes); err == nil {
				documentTypeMap["fund_types"] = fundTypes
			} else {
				documentTypeMap["fund_types"] = nil
			}
		} else {
			documentTypeMap["fund_types"] = nil
		}

		// Parse and add subcategory_ids
		if dt.SubcategoryIds != nil {
			var subcategoryIds []int
			if err := json.Unmarshal([]byte(*dt.SubcategoryIds), &subcategoryIds); err == nil {
				documentTypeMap["subcategory_ids"] = subcategoryIds
			} else {
				documentTypeMap["subcategory_ids"] = nil
			}
		} else {
			documentTypeMap["subcategory_ids"] = nil
		}

		result = append(result, documentTypeMap)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"document_types": result,
	})
}

// UpdateDocumentType - Admin updates document type including fund_types and subcategory_ids
func UpdateDocumentType(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	documentTypeID := c.Param("id")

	var req struct {
		DocumentTypeName *string   `json:"document_type_name"`
		Code             *string   `json:"code"`
		Category         *string   `json:"category"`
		Required         *bool     `json:"required"`
		Multiple         *bool     `json:"multiple"`
		DocumentOrder    *int      `json:"document_order"`
		IsRequired       *string   `json:"is_required"`     // enum('yes','no')
		FundTypes        *[]string `json:"fund_types"`      // Array of fund types
		SubcategoryIds   *[]int    `json:"subcategory_ids"` // Array of subcategory IDs
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate is_required enum
	if req.IsRequired != nil {
		if *req.IsRequired != "yes" && *req.IsRequired != "no" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "is_required must be 'yes' or 'no'"})
			return
		}
	}

	// Check if document type exists
	var documentType models.DocumentType
	if err := config.DB.First(&documentType, documentTypeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document type not found"})
		return
	}

	// Build update map
	updates := map[string]interface{}{}

	if req.DocumentTypeName != nil {
		updates["document_type_name"] = *req.DocumentTypeName
	}

	if req.Code != nil {
		updates["code"] = *req.Code
	}

	if req.Category != nil {
		updates["category"] = *req.Category
	}

	if req.Required != nil {
		updates["required"] = *req.Required
	}

	if req.Multiple != nil {
		updates["multiple"] = *req.Multiple
	}

	if req.DocumentOrder != nil {
		updates["document_order"] = *req.DocumentOrder
	}

	if req.IsRequired != nil {
		updates["is_required"] = *req.IsRequired
	}

	// Handle fund_types JSON field
	if req.FundTypes != nil {
		if len(*req.FundTypes) == 0 {
			updates["fund_types"] = nil
		} else {
			fundTypesJSON, err := json.Marshal(*req.FundTypes)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid fund_types format"})
				return
			}
			updates["fund_types"] = string(fundTypesJSON)
		}
	}

	// Handle subcategory_ids JSON field
	if req.SubcategoryIds != nil {
		if len(*req.SubcategoryIds) == 0 {
			updates["subcategory_ids"] = nil
		} else {
			subcategoryIdsJSON, err := json.Marshal(*req.SubcategoryIds)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_ids format"})
				return
			}
			updates["subcategory_ids"] = string(subcategoryIdsJSON)
		}
	}

	// Add update timestamp
	updates["update_at"] = time.Now()

	// Update document type
	if err := config.DB.Model(&documentType).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update document type"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "Document type updated successfully",
		"document_type": documentType,
	})
}

// CreateDocumentType - Admin creates new document type
func CreateDocumentType(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	var req struct {
		DocumentTypeName string   `json:"document_type_name" binding:"required"`
		Code             string   `json:"code" binding:"required"`
		Category         string   `json:"category"`
		Required         bool     `json:"required"`
		Multiple         bool     `json:"multiple"`
		DocumentOrder    int      `json:"document_order"`
		IsRequired       string   `json:"is_required"` // enum('yes','no')
		FundTypes        []string `json:"fund_types"`
		SubcategoryIds   []int    `json:"subcategory_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate is_required enum
	if req.IsRequired != "" {
		if req.IsRequired != "yes" && req.IsRequired != "no" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "is_required must be 'yes' or 'no'"})
			return
		}
	}

	// Create document type
	documentType := models.DocumentType{
		DocumentTypeName: req.DocumentTypeName,
		Code:             req.Code,
		Category:         req.Category,
		Required:         req.Required,
		Multiple:         req.Multiple,
		DocumentOrder:    req.DocumentOrder,
		CreateAt:         time.Now(),
		UpdateAt:         time.Now(),
	}

	// Set IsRequired if provided
	if req.IsRequired != "" {
		documentType.IsRequired = &req.IsRequired
	}

	// Handle fund_types JSON
	if len(req.FundTypes) > 0 {
		fundTypesJSON, err := json.Marshal(req.FundTypes)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid fund_types format"})
			return
		}
		fundTypesStr := string(fundTypesJSON)
		documentType.FundTypes = &fundTypesStr
	}

	// Handle subcategory_ids JSON
	if len(req.SubcategoryIds) > 0 {
		subcategoryIdsJSON, err := json.Marshal(req.SubcategoryIds)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_ids format"})
			return
		}
		subcategoryIdsStr := string(subcategoryIdsJSON)
		documentType.SubcategoryIds = &subcategoryIdsStr
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

// DeleteDocumentType - Admin soft deletes document type
func DeleteDocumentType(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	documentTypeID := c.Param("id")

	// Check if document type exists
	var documentType models.DocumentType
	if err := config.DB.First(&documentType, documentTypeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document type not found"})
		return
	}

	// Soft delete
	now := time.Now()
	if err := config.DB.Model(&documentType).Update("delete_at", now).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete document type"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Document type deleted successfully",
	})
}
