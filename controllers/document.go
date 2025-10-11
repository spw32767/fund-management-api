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

func fetchSubcategoryMaps() (map[int]string, map[string]int, error) {
	var subcategories []models.FundSubcategory
	if err := config.DB.Where("delete_at IS NULL").Find(&subcategories).Error; err != nil {
		return nil, nil, err
	}

	idToName := make(map[int]string, len(subcategories))
	nameToID := make(map[string]int, len(subcategories))

	for _, sub := range subcategories {
		trimmed := strings.TrimSpace(sub.SubcategoryName)
		idToName[sub.SubcategoryID] = trimmed
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if _, exists := nameToID[lower]; !exists {
			nameToID[lower] = sub.SubcategoryID
		}
	}

	return idToName, nameToID, nil
}

func normalizeSubcategoryNames(names []string) []string {
	seen := make(map[string]bool)
	normalized := make([]string, 0, len(names))

	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		normalized = append(normalized, trimmed)
	}

	return normalized
}

func resolveSubcategoryNamesFromIDs(ids []int) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}

	var subcategories []models.FundSubcategory
	if err := config.DB.Where("subcategory_id IN ?", ids).Where("delete_at IS NULL").Find(&subcategories).Error; err != nil {
		return nil, err
	}

	idToName := make(map[int]string, len(subcategories))
	for _, sub := range subcategories {
		idToName[sub.SubcategoryID] = strings.TrimSpace(sub.SubcategoryName)
	}

	names := make([]string, 0, len(ids))
	for _, id := range ids {
		name, ok := idToName[id]
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("invalid subcategory id %d", id)
		}
		names = append(names, name)
	}

	return normalizeSubcategoryNames(names), nil
}

func resolveSubcategoryNamesFromRequest(names []string, ids []int) ([]string, error) {
	normalized := normalizeSubcategoryNames(names)
	if len(normalized) > 0 {
		return normalized, nil
	}

	if len(ids) == 0 {
		return []string{}, nil
	}

	return resolveSubcategoryNamesFromIDs(ids)
}

func lookupSubcategoryIDsByNames(names []string, nameToID map[string]int) []int {
	ids := make([]int, 0, len(names))
	seen := make(map[int]bool)

	for _, name := range names {
		lower := strings.ToLower(strings.TrimSpace(name))
		if lower == "" {
			continue
		}
		if id, ok := nameToID[lower]; ok {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}

	return ids
}

func parseStoredSubcategories(raw *string, idToName map[int]string, nameToID map[string]int) ([]string, []int) {
	if raw == nil || *raw == "" {
		return nil, nil
	}

	var rawItems []interface{}
	if err := json.Unmarshal([]byte(*raw), &rawItems); err != nil {
		return nil, nil
	}

	names := make([]string, 0, len(rawItems))
	ids := make([]int, 0, len(rawItems))
	seenNames := make(map[string]bool)
	seenIDs := make(map[int]bool)

	for _, item := range rawItems {
		switch value := item.(type) {
		case string:
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			lower := strings.ToLower(trimmed)
			if !seenNames[lower] {
				names = append(names, trimmed)
				seenNames[lower] = true
			}
			if id, ok := nameToID[lower]; ok {
				if !seenIDs[id] {
					ids = append(ids, id)
					seenIDs[id] = true
				}
			}
		case float64:
			id := int(value)
			if !seenIDs[id] {
				ids = append(ids, id)
				seenIDs[id] = true
			}
			if name, ok := idToName[id]; ok {
				trimmed := strings.TrimSpace(name)
				if trimmed != "" {
					lower := strings.ToLower(trimmed)
					if !seenNames[lower] {
						names = append(names, trimmed)
						seenNames[lower] = true
					}
				}
			}
		default:
			continue
		}
	}

	return names, ids
}

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
		FolderType:   "submission",
		FileSize:     file.Size,
		MimeType:     file.Header.Get("Content-Type"),
		FileHash:     "", // ไม่ใช้ hash ในระบบ user-based
		IsPublic:     false,
		UploadedBy:   userID.(int),
		UploadedAt:   now,
		CreateAt:     now,
		UpdateAt:     now,
	}

	if err := createFileUploadRecord(config.DB, &fileUpload); err != nil {
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
	subcategoryNameQuery := strings.TrimSpace(c.Query("subcategory_name"))

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

	idToName, nameToID, err := fetchSubcategoryMaps()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subcategories"})
		return
	}

	subcategoryNameFilter := strings.ToLower(subcategoryNameQuery)
	if subcategoryNameFilter == "" && subcategoryIdStr != "" {
		subcategoryId, err := strconv.Atoi(subcategoryIdStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_id format"})
			return
		}

		if name, ok := idToName[subcategoryId]; ok && strings.TrimSpace(name) != "" {
			subcategoryNameFilter = strings.ToLower(strings.TrimSpace(name))
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success":        true,
				"document_types": []map[string]interface{}{},
			})
			return
		}
	}

	type docWithMetadata struct {
		Document models.DocumentType
		Names    []string
		IDs      []int
	}

	// Apply fund_type and subcategory filtering
	var filteredTypes []docWithMetadata
	for _, dt := range documentTypes {
		shouldInclude := true
		var parsedNames []string
		var parsedIDs []int

		if dt.SubcategoryIds != nil {
			parsedNames, parsedIDs = parseStoredSubcategories(dt.SubcategoryIds, idToName, nameToID)
		}

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

		// Filter by subcategory name (converted from id if necessary)
		if subcategoryNameFilter != "" && shouldInclude {
			if len(parsedNames) == 0 {
				shouldInclude = false
			} else {
				match := false
				for _, name := range parsedNames {
					if strings.ToLower(name) == subcategoryNameFilter {
						match = true
						break
					}
				}
				if !match {
					shouldInclude = false
				}
			}
		}

		if shouldInclude {
			filteredTypes = append(filteredTypes, docWithMetadata{
				Document: dt,
				Names:    parsedNames,
				IDs:      parsedIDs,
			})
		}
	}

	// Transform for frontend - maintain backward compatibility
	var result []map[string]interface{}
	for _, item := range filteredTypes {
		dt := item.Document
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

		if len(item.Names) > 0 {
			documentTypeMap["subcategory_names"] = item.Names
		} else {
			documentTypeMap["subcategory_names"] = []string{}
		}

		if len(item.IDs) > 0 {
			documentTypeMap["subcategory_ids"] = item.IDs
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

	idToName, nameToID, err := fetchSubcategoryMaps()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subcategories"})
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
		names, ids := parseStoredSubcategories(dt.SubcategoryIds, idToName, nameToID)
		if len(names) > 0 {
			documentTypeMap["subcategory_names"] = names
		} else {
			documentTypeMap["subcategory_names"] = []string{}
		}

		if len(ids) > 0 {
			documentTypeMap["subcategory_ids"] = ids
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
		IsRequired       *string   `json:"is_required"` // enum('yes','no')
		FundTypes        *[]string `json:"fund_types"`  // Array of fund types
		SubcategoryNames *[]string `json:"subcategory_names"`
		SubcategoryIds   *[]int    `json:"subcategory_ids"` // Array of subcategory IDs (deprecated)
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
	if req.SubcategoryNames != nil {
		normalized := normalizeSubcategoryNames(*req.SubcategoryNames)
		if len(normalized) == 0 {
			updates["subcategory_ids"] = nil
		} else {
			namesJSON, err := json.Marshal(normalized)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_names format"})
				return
			}
			updates["subcategory_ids"] = string(namesJSON)
		}
	} else if req.SubcategoryIds != nil {
		if len(*req.SubcategoryIds) == 0 {
			updates["subcategory_ids"] = nil
		} else {
			names, err := resolveSubcategoryNamesFromIDs(*req.SubcategoryIds)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			namesJSON, err := json.Marshal(names)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_ids format"})
				return
			}
			updates["subcategory_ids"] = string(namesJSON)
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
		SubcategoryNames []string `json:"subcategory_names"`
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

	// Handle subcategory assignments (store as names)
	subcategoryNames, err := resolveSubcategoryNamesFromRequest(req.SubcategoryNames, req.SubcategoryIds)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(subcategoryNames) > 0 {
		namesJSON, err := json.Marshal(subcategoryNames)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_names format"})
			return
		}
		namesStr := string(namesJSON)
		documentType.SubcategoryIds = &namesStr
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
