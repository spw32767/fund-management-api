package controllers

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UploadDocument handles document upload for application
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

	// Generate unique filename
	uniqueID := uuid.New().String()
	storedFilename := fmt.Sprintf("%s_%s%s", applicationID, uniqueID, ext)

	// Create upload directory if not exists
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	// Create subdirectory for year/month
	now := time.Now()
	subDir := filepath.Join(uploadPath, now.Format("2006/01"))
	if err := os.MkdirAll(subDir, os.ModePerm); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	// Save file
	fullPath := filepath.Join(subDir, storedFilename)
	if err := c.SaveUploadedFile(file, fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Save to database
	document := models.ApplicationDocument{
		ApplicationID:    application.ApplicationID,
		DocumentTypeID:   documentTypeID,
		UploadedBy:       userID.(int),
		OriginalFilename: file.Filename,
		StoredFilename:   storedFilename,
		FileType:         ext,
		UploadedAt:       &now,
		CreateAt:         &now,
		UpdateAt:         &now,
	}

	if err := config.DB.Create(&document).Error; err != nil {
		// Delete uploaded file if database save fails
		os.Remove(fullPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save document info"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Document uploaded successfully",
		"document": document,
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

// GetDocumentTypes returns all document types
func GetDocumentTypes(c *gin.Context) {
	var documentTypes []models.DocumentType
	if err := config.DB.Where("delete_at IS NULL").Find(&documentTypes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch document types"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"document_types": documentTypes,
	})
}
