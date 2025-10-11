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
		return []string{}, []int{}
	}

	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return []string{}, []int{}
	}

	appendName := func(names *[]string, seen map[string]bool, value string) {
		trimmedName := strings.TrimSpace(value)
		if trimmedName == "" {
			return
		}
		lower := strings.ToLower(trimmedName)
		if seen[lower] {
			return
		}
		seen[lower] = true
		*names = append(*names, trimmedName)
	}

	appendID := func(ids *[]int, seen map[int]bool, id int) {
		if seen[id] {
			return
		}
		seen[id] = true
		*ids = append(*ids, id)
	}

	processStringEntry := func(value string, names *[]string, ids *[]int, seenNames map[string]bool, seenIDs map[int]bool) {
		appendName(names, seenNames, value)
		lower := strings.ToLower(strings.TrimSpace(value))
		if id, ok := nameToID[lower]; ok {
			appendID(ids, seenIDs, id)
		}
	}

	names := make([]string, 0)
	ids := make([]int, 0)
	seenNames := make(map[string]bool)
	seenIDs := make(map[int]bool)

	var rawItems []interface{}
	if err := json.Unmarshal([]byte(trimmed), &rawItems); err != nil {
		// Fallback for legacy snapshot strings stored as comma separated text
		parts := strings.Split(trimmed, ",")
		for _, part := range parts {
			processStringEntry(part, &names, &ids, seenNames, seenIDs)
		}
		return names, ids
	}

	for _, item := range rawItems {
		switch value := item.(type) {
		case string:
			processStringEntry(value, &names, &ids, seenNames, seenIDs)
		case float64:
			id := int(value)
			appendID(&ids, seenIDs, id)
			if name, ok := idToName[id]; ok {
				processStringEntry(name, &names, &ids, seenNames, seenIDs)
			}
		default:
			continue
		}
	}

	return names, ids
}

type documentTypeMetadata struct {
	FundTypes          []string
	FundTypeMode       string
	SubcategoryNames   []string
	SubcategoryIDs     []int
	SubcategoryMode    string
	PrimarySubcategory *string
}

func dedupeFundTypes(values []string) []string {
	seen := make(map[string]bool)
	normalized := make([]string, 0, len(values))

	for _, value := range values {
		trimmed := strings.TrimSpace(value)
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

func computeDocumentTypeMetadata(dt models.DocumentType, idToName map[int]string, nameToID map[string]int) documentTypeMetadata {
	meta := documentTypeMetadata{
		FundTypes:        []string{},
		FundTypeMode:     "inactive",
		SubcategoryNames: []string{},
		SubcategoryIDs:   []int{},
		SubcategoryMode:  "inactive",
	}

	if dt.FundTypes != nil {
		var parsed []string
		if err := json.Unmarshal([]byte(*dt.FundTypes), &parsed); err == nil {
			meta.FundTypes = dedupeFundTypes(parsed)
			if len(meta.FundTypes) == 0 {
				meta.FundTypeMode = "all"
			} else {
				meta.FundTypeMode = "limited"
			}
		} else {
			meta.FundTypes = []string{}
			meta.FundTypeMode = "all"
		}
	}

	names, ids := parseStoredSubcategories(dt.SubcategoryName, idToName, nameToID)
	if len(names) == 0 && len(ids) == 0 && dt.SubcategoryIds != nil {
		fallbackNames, fallbackIDs := parseStoredSubcategories(dt.SubcategoryIds, idToName, nameToID)
		names = fallbackNames
		ids = fallbackIDs
	}

	meta.SubcategoryNames = names
	meta.SubcategoryIDs = ids

	if len(meta.SubcategoryNames) > 0 {
		primary := strings.TrimSpace(meta.SubcategoryNames[0])
		if primary != "" {
			meta.PrimarySubcategory = &primary
		}
	}

	if dt.SubcategoryName == nil {
		if len(meta.SubcategoryNames) > 0 || len(meta.SubcategoryIDs) > 0 {
			meta.SubcategoryMode = "limited"
		} else if meta.FundTypeMode == "inactive" {
			meta.SubcategoryMode = "inactive"
		} else {
			meta.SubcategoryMode = "all"
		}
	} else {
		if len(meta.SubcategoryNames) == 0 {
			meta.SubcategoryMode = "all"
		} else {
			meta.SubcategoryMode = "limited"
		}
	}

	if meta.FundTypeMode == "inactive" {
		meta.SubcategoryMode = "inactive"
	}

	return meta
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
		userFolderPath, "fund", appID, "", time.Now())
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
	fundType := c.Query("fund_type")              // "publication_reward" หรือ "fund_application"
	subcategoryIdStr := c.Query("subcategory_id") // "1", "2", etc.
	subcategoryNameQuery := strings.TrimSpace(c.Query("subcategory_name"))

	// Build query
	query := config.DB.Where("delete_at IS NULL")

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
		Metadata documentTypeMetadata
	}

	// Apply fund_type and subcategory filtering
	var filteredTypes []docWithMetadata
	for _, dt := range documentTypes {
		meta := computeDocumentTypeMetadata(dt, idToName, nameToID)
		shouldInclude := true

		if fundType != "" {
			matched := false
			switch meta.FundTypeMode {
			case "inactive":
				matched = false
			case "all":
				matched = true
			default:
				for _, ft := range meta.FundTypes {
					if ft == fundType {
						matched = true
						break
					}
				}
			}

			if !matched {
				shouldInclude = false
			}
		}

		// Filter by subcategory name (converted from id if necessary)
		if subcategoryNameFilter != "" && shouldInclude {
			if len(meta.SubcategoryNames) == 0 {
				shouldInclude = false
			} else {
				match := false
				for _, name := range meta.SubcategoryNames {
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
				Metadata: meta,
			})
		}
	}

	// Transform for frontend - maintain backward compatibility
	var result []map[string]interface{}
	for _, item := range filteredTypes {
		dt := item.Document
		meta := item.Metadata

		documentTypeMap := map[string]interface{}{
			// Original fields for backward compatibility
			"id":       dt.DocumentTypeID,
			"code":     dt.Code,
			"name":     dt.DocumentTypeName,
			"required": dt.Required,
			"multiple": dt.Multiple,

			// Additional fields
			"document_order": dt.DocumentOrder,

			// New naming convention (for future use)
			"document_type_id":   dt.DocumentTypeID,
			"document_type_name": dt.DocumentTypeName,
		}

		if dt.FundTypes != nil {
			documentTypeMap["fund_types"] = meta.FundTypes
		} else {
			documentTypeMap["fund_types"] = nil
		}
		documentTypeMap["fund_type_mode"] = meta.FundTypeMode

		documentTypeMap["subcategory_names"] = meta.SubcategoryNames
		if meta.PrimarySubcategory != nil {
			documentTypeMap["subcategory_name"] = *meta.PrimarySubcategory
		} else {
			documentTypeMap["subcategory_name"] = nil
		}
		documentTypeMap["subcategory_mode"] = meta.SubcategoryMode

		if len(meta.SubcategoryIDs) > 0 {
			documentTypeMap["subcategory_ids"] = meta.SubcategoryIDs
		} else {
			documentTypeMap["subcategory_ids"] = []int{}
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
		meta := computeDocumentTypeMetadata(dt, idToName, nameToID)

		documentTypeMap := map[string]interface{}{
			"document_type_id":   dt.DocumentTypeID,
			"document_type_name": dt.DocumentTypeName,
			"code":               dt.Code,
			"required":           dt.Required,
			"multiple":           dt.Multiple,
			"document_order":     dt.DocumentOrder,
			"create_at":          dt.CreateAt,
			"update_at":          dt.UpdateAt,
		}

		if dt.FundTypes != nil {
			documentTypeMap["fund_types"] = meta.FundTypes
		} else {
			documentTypeMap["fund_types"] = nil
		}
		documentTypeMap["fund_type_mode"] = meta.FundTypeMode

		documentTypeMap["subcategory_names"] = meta.SubcategoryNames
		if meta.PrimarySubcategory != nil {
			documentTypeMap["subcategory_name"] = *meta.PrimarySubcategory
		} else {
			documentTypeMap["subcategory_name"] = nil
		}
		documentTypeMap["subcategory_mode"] = meta.SubcategoryMode

		if len(meta.SubcategoryIDs) > 0 {
			documentTypeMap["subcategory_ids"] = meta.SubcategoryIDs
		} else {
			documentTypeMap["subcategory_ids"] = []int{}
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
		Required         *bool     `json:"required"`
		Multiple         *bool     `json:"multiple"`
		DocumentOrder    *int      `json:"document_order"`
		FundTypes        *[]string `json:"fund_types"` // Array of fund types
		SubcategoryNames *[]string `json:"subcategory_names"`
		SubcategoryIds   *[]int    `json:"subcategory_ids"` // Array of subcategory IDs (deprecated)
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
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

	if req.Required != nil {
		updates["required"] = *req.Required
	}

	if req.Multiple != nil {
		updates["multiple"] = *req.Multiple
	}

	if req.DocumentOrder != nil {
		updates["document_order"] = *req.DocumentOrder
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

	// Handle subcategory assignments (stored as JSON names array)
	if req.SubcategoryNames != nil {
		normalized := normalizeSubcategoryNames(*req.SubcategoryNames)
		if len(normalized) == 0 {
			updates["subcategory_ids"] = nil
			updates["subcategory_name"] = nil
		} else {
			namesJSON, err := json.Marshal(normalized)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_names format"})
				return
			}
			namesJSONStr := string(namesJSON)
			updates["subcategory_ids"] = namesJSONStr
			updates["subcategory_name"] = namesJSONStr
		}
	} else if req.SubcategoryIds != nil {
		names, err := resolveSubcategoryNamesFromIDs(*req.SubcategoryIds)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if len(names) == 0 {
			updates["subcategory_ids"] = nil
			updates["subcategory_name"] = nil
		} else {
			namesJSON, err := json.Marshal(names)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_ids format"})
				return
			}
			namesJSONStr := string(namesJSON)
			updates["subcategory_ids"] = namesJSONStr
			updates["subcategory_name"] = namesJSONStr
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
		Required         bool     `json:"required"`
		Multiple         bool     `json:"multiple"`
		DocumentOrder    int      `json:"document_order"`
		FundTypes        []string `json:"fund_types"`
		SubcategoryNames []string `json:"subcategory_names"`
		SubcategoryIds   []int    `json:"subcategory_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create document type
	documentType := models.DocumentType{
		DocumentTypeName: req.DocumentTypeName,
		Code:             req.Code,
		Required:         req.Required,
		Multiple:         req.Multiple,
		DocumentOrder:    req.DocumentOrder,
		CreateAt:         time.Now(),
		UpdateAt:         time.Now(),
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
		namesStrCopy := namesStr
		documentType.SubcategoryName = &namesStrCopy
	} else {
		documentType.SubcategoryIds = nil
		documentType.SubcategoryName = nil
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
