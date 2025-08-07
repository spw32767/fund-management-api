// controllers/submission.go
package controllers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ===================== SUBMISSION MANAGEMENT =====================

// GetSubmissions returns user's submissions
func GetSubmissions(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	submissionType := c.Query("submission_type")
	status := c.Query("status")
	yearID := c.Query("year_id")

	var submissions []models.Submission
	query := config.DB.Preload("User").Preload("Year").Preload("Status").
		Where("deleted_at IS NULL")

	// Filter by user if not admin
	if roleID.(int) != 3 { // 3 = admin role
		query = query.Where("user_id = ?", userID)
	}

	// Apply filters
	if submissionType != "" {
		query = query.Where("submission_type = ?", submissionType)
	}
	if status != "" {
		query = query.Where("status_id = ?", status)
	}
	if yearID != "" {
		query = query.Where("year_id = ?", yearID)
	}

	if err := query.Order("created_at DESC").Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"submissions": submissions,
		"total":       len(submissions),
	})
}

// GetSubmission returns a specific submission
func GetSubmission(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var submission models.Submission
	query := config.DB.Preload("User").Preload("Year").Preload("Status").
		Preload("Documents.File").Preload("Documents.DocumentType")

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.Where("submission_id = ? AND deleted_at IS NULL", submissionID).First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Load type-specific details
	switch submission.SubmissionType {
	case "fund_application":
		var fundDetail models.FundApplicationDetail
		if err := config.DB.Preload("Subcategory").Where("submission_id = ?", submission.SubmissionID).First(&fundDetail).Error; err == nil {
			submission.FundApplicationDetail = &fundDetail
		}
	case "publication_reward":
		var pubDetail models.PublicationRewardDetail
		if err := config.DB.Where("submission_id = ?", submission.SubmissionID).First(&pubDetail).Error; err == nil {
			submission.PublicationRewardDetail = &pubDetail
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"submission": submission,
	})
}

// CreateSubmission creates a new submission
func CreateSubmission(c *gin.Context) {
	userID, _ := c.Get("userID")

	type CreateSubmissionRequest struct {
		SubmissionType string `json:"submission_type" binding:"required"` // 'fund_application', 'publication_reward'
		YearID         int    `json:"year_id" binding:"required"`
		//Priority       string `json:"priority"`
	}

	var req CreateSubmissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate submission type
	validTypes := []string{"fund_application", "publication_reward", "conference_grant", "training_request"}
	isValidType := false
	for _, validType := range validTypes {
		if req.SubmissionType == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission type"})
		return
	}

	// Validate year exists
	var year models.Year
	if err := config.DB.Where("year_id = ? AND delete_at IS NULL", req.YearID).First(&year).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year"})
		return
	}

	// Generate submission number
	submissionNumber := generateSubmissionNumber(req.SubmissionType)

	// Set default priority
	// priority := req.Priority
	// if priority == "" {
	// 	priority = "normal"
	// }

	// Create submission
	now := time.Now()
	submission := models.Submission{
		SubmissionType:   req.SubmissionType,
		SubmissionNumber: submissionNumber,
		UserID:           userID.(int),
		YearID:           req.YearID,
		StatusID:         1, // Draft status
		//Priority:         priority,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := config.DB.Create(&submission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create submission"})
		return
	}

	// Load relations for response
	config.DB.Preload("User").Preload("Year").Preload("Status").First(&submission, submission.SubmissionID)

	c.JSON(http.StatusCreated, gin.H{
		"success":    true,
		"message":    "Submission created successfully",
		"submission": submission,
	})
}

// UpdateSubmission updates a submission (only if editable)
func UpdateSubmission(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// type UpdateSubmissionRequest struct {
	// 	Priority string `json:"priority"`
	// }

	// var req UpdateSubmissionRequest
	// if err := c.ShouldBindJSON(&req); err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	return
	// }

	// Find submission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Check if editable
	if !submission.IsEditable() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission cannot be edited"})
		return
	}

	// Update submission
	now := time.Now()
	updates := map[string]interface{}{
		"updated_at": now,
	}

	// if req.Priority != "" {
	// 	updates["priority"] = req.Priority
	// }

	if err := config.DB.Model(&submission).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Submission updated successfully",
	})
}

// DeleteSubmission soft deletes a submission (only if not submitted)
func DeleteSubmission(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Find submission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Check if can be deleted
	if submission.IsSubmitted() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete submitted submission"})
		return
	}

	// Soft delete
	now := time.Now()
	submission.DeletedAt = &now

	if err := config.DB.Save(&submission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete submission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Submission deleted successfully",
	})
}

// SubmitSubmission submits a submission (changes status)
func SubmitSubmission(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")

	// Find submission
	var submission models.Submission
	if err := config.DB.Where("submission_id = ? AND user_id = ? AND deleted_at IS NULL",
		submissionID, userID).First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Check if can be submitted
	if !submission.CanBeSubmitted() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission cannot be submitted"})
		return
	}

	// Update submission
	now := time.Now()
	submission.SubmittedAt = &now
	submission.UpdatedAt = now

	if err := config.DB.Save(&submission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit submission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Submission submitted successfully",
	})
}

// ===================== FILE UPLOAD SYSTEM =====================

// UploadFile handles file upload (User-Based Folders)
func UploadFile(c *gin.Context) {
	userID, _ := c.Get("userID")

	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	// Validate file size (10MB limit)
	maxSize := int64(10 * 1024 * 1024)
	if file.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
		return
	}

	// Validate file type
	if !isValidFileType(file) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
		return
	}

	// Get user info for folder creation
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}

	// Create storage path
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

	// เก็บไฟล์ในโฟลเดอร์ temp ก่อน (รอแนบกับ submission)
	tempFolderPath := filepath.Join(userFolderPath, "temp")

	// Generate unique filename in temp directory
	safeFilename := utils.GenerateUniqueFilename(tempFolderPath, file.Filename)
	storedPath := filepath.Join(tempFolderPath, safeFilename)

	// Save file
	if err := c.SaveUploadedFile(file, storedPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Save to database
	now := time.Now()
	fileUpload := models.FileUpload{
		OriginalName: file.Filename,
		StoredPath:   storedPath,
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
		os.Remove(storedPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file info"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File uploaded successfully",
		"file":    fileUpload,
	})
}

// MoveFileToSubmissionFolder ย้ายไฟล์จาก temp ไปยัง submission folder
func MoveFileToSubmissionFolder(fileID int, submissionID int, submissionType string) error {
	var fileUpload models.FileUpload
	if err := config.DB.First(&fileUpload, fileID).Error; err != nil {
		return err
	}

	var user models.User
	if err := config.DB.First(&user, fileUpload.UploadedBy).Error; err != nil {
		return err
	}

	var submission models.Submission
	if err := config.DB.First(&submission, submissionID).Error; err != nil {
		return err
	}

	// Generate new path
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	userFolderPath, err := utils.CreateUserFolderIfNotExists(user, uploadPath)
	if err != nil {
		return err
	}

	submissionFolderPath, err := utils.CreateSubmissionFolder(
		userFolderPath, submissionType, submissionID, submission.CreatedAt)
	if err != nil {
		return err
	}

	// Generate new filename
	newFilename := utils.GenerateUniqueFilename(submissionFolderPath, fileUpload.OriginalName)
	newPath := filepath.Join(submissionFolderPath, newFilename)

	// Move file
	if err := utils.MoveFileToSubmissionFolder(fileUpload.StoredPath, newPath); err != nil {
		return err
	}

	// Update database
	fileUpload.StoredPath = newPath
	fileUpload.UpdateAt = time.Now()
	return config.DB.Save(&fileUpload).Error
}

// AttachDocumentToSubmission แนบไฟล์กับ submission และย้ายไฟล์
func AttachDocumentToSubmission(c *gin.Context) {
	submissionID, _ := strconv.Atoi(c.Param("id"))
	userID, _ := c.Get("userID")

	type AttachDocumentRequest struct {
		FileID         int    `json:"file_id" binding:"required"`
		DocumentTypeID int    `json:"document_type_id" binding:"required"`
		Description    string `json:"description"`
		DisplayOrder   int    `json:"display_order"`
	}

	var req AttachDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify submission exists and user has permission
	var submission models.Submission
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, userID).
		First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Verify file exists and belongs to user
	var fileUpload models.FileUpload
	if err := config.DB.Where("file_id = ? AND uploaded_by = ?", req.FileID, userID).
		First(&fileUpload).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Move file from temp to submission folder
	if err := MoveFileToSubmissionFolder(req.FileID, submissionID, submission.SubmissionType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to move file to submission folder"})
		return
	}

	// Create submission document record
	document := models.SubmissionDocument{
		SubmissionID:   submissionID,
		FileID:         req.FileID,
		DocumentTypeID: req.DocumentTypeID,
		Description:    req.Description,
		DisplayOrder:   req.DisplayOrder,
		CreatedAt:      time.Now(),
	}

	if err := config.DB.Create(&document).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to attach document"})
		return
	}

	// Preload relations
	config.DB.Preload("File").Preload("DocumentType").First(&document, document.DocumentID)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Document attached successfully",
		"document": document,
	})
}

// GetFile returns file info
func GetFile(c *gin.Context) {
	fileID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var file models.FileUpload
	query := config.DB.Where("file_id = ? AND delete_at IS NULL", fileID)

	// Check permission (user can see own files, admin can see all)
	if roleID.(int) != 3 {
		query = query.Where("uploaded_by = ? OR is_public = ?", userID, true)
	}

	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"file":    file,
	})
}

// DownloadFile serves file for download
func DownloadFile(c *gin.Context) {
	fileID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var file models.FileUpload
	query := config.DB.Where("file_id = ? AND delete_at IS NULL", fileID)

	// Check permission
	if roleID.(int) != 3 {
		query = query.Where("uploaded_by = ? OR is_public = ?", userID, true)
	}

	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Check if file exists
	if _, err := os.Stat(file.StoredPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found on disk"})
		return
	}

	// Serve file
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.OriginalName))
	c.Header("Content-Type", file.MimeType)
	c.File(file.StoredPath)
}

// DeleteFile soft deletes a file
func DeleteFile(c *gin.Context) {
	fileID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var file models.FileUpload
	query := config.DB.Where("file_id = ? AND delete_at IS NULL", fileID)

	// Check permission (user can delete own files, admin can delete all)
	if roleID.(int) != 3 {
		query = query.Where("uploaded_by = ?", userID)
	}

	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Check if file is used in any submissions
	var docCount int64
	config.DB.Model(&models.SubmissionDocument{}).Where("file_id = ?", fileID).Count(&docCount)

	if docCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete file that is used in submissions"})
		return
	}

	// Soft delete
	now := time.Now()
	file.DeleteAt = &now

	if err := config.DB.Save(&file).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File deleted successfully",
	})
}

// ===================== SUBMISSION DOCUMENT MANAGEMENT =====================

// AttachDocument attaches a file to a submission
func AttachDocument(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type AttachDocumentRequest struct {
		FileID         int    `json:"file_id" binding:"required"`
		DocumentTypeID int    `json:"document_type_id" binding:"required"`
		Description    string `json:"description"`
		DisplayOrder   int    `json:"display_order"`
	}

	var req AttachDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find submission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Check if submission is editable
	if !submission.IsEditable() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify submitted submission"})
		return
	}

	// Validate file exists and user has access
	var file models.FileUpload
	fileQuery := config.DB.Where("file_id = ? AND delete_at IS NULL", req.FileID)
	if roleID.(int) != 3 {
		fileQuery = fileQuery.Where("uploaded_by = ?", userID)
	}

	if err := fileQuery.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Validate document type exists
	var docType models.DocumentType
	if err := config.DB.Where("document_type_id = ? AND delete_at IS NULL", req.DocumentTypeID).First(&docType).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document type"})
		return
	}

	// Check if document already attached
	var existingDoc models.SubmissionDocument
	if err := config.DB.Where("submission_id = ? AND file_id = ?", submissionID, req.FileID).First(&existingDoc).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "File already attached to this submission"})
		return
	}

	// Create submission document
	now := time.Now()
	submissionDoc := models.SubmissionDocument{
		SubmissionID:   submission.SubmissionID,
		FileID:         req.FileID,
		DocumentTypeID: req.DocumentTypeID,
		Description:    req.Description,
		DisplayOrder:   req.DisplayOrder,
		IsRequired:     docType.Required,
		CreatedAt:      now,
	}

	if err := config.DB.Create(&submissionDoc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to attach document"})
		return
	}

	// Load relations for response
	config.DB.Preload("File").Preload("DocumentType").First(&submissionDoc, submissionDoc.DocumentID)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Document attached successfully",
		"document": submissionDoc,
	})
}

// GetSubmissionDocuments returns documents attached to a submission
func GetSubmissionDocuments(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Find submission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Get documents
	var documents []models.SubmissionDocument
	if err := config.DB.Preload("File").Preload("DocumentType").
		Where("submission_id = ?", submissionID).
		Order("display_order, created_at").
		Find(&documents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch documents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"documents": documents,
		"total":     len(documents),
	})
}

// DetachDocument removes a document from submission
func DetachDocument(c *gin.Context) {
	submissionID := c.Param("id")
	documentID := c.Param("doc_id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Find submission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Check if submission is editable
	if !submission.IsEditable() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify submitted submission"})
		return
	}

	// Find and delete document
	var submissionDoc models.SubmissionDocument
	if err := config.DB.Where("document_id = ? AND submission_id = ?", documentID, submissionID).First(&submissionDoc).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	if err := config.DB.Delete(&submissionDoc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to detach document"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Document detached successfully",
	})
}

// ===================== HELPER FUNCTIONS =====================

// Global mutex for submission number generation
var submissionNumberMutex sync.Mutex

// generateSubmissionNumber creates a unique submission number with hybrid approach
func generateSubmissionNumber(submissionType string) string {
	// Use mutex to prevent concurrent access
	submissionNumberMutex.Lock()
	defer submissionNumberMutex.Unlock()

	now := time.Now()
	dateStr := now.Format("20060102")

	var prefix string
	switch submissionType {
	case "fund_application":
		prefix = "FA"
	case "publication_reward":
		prefix = "PR"
	case "conference_grant":
		prefix = "CG"
	case "training_request":
		prefix = "TR"
	default:
		prefix = "SUB"
	}

	// Try sequential number first (user-friendly)
	var count int64
	config.DB.Model(&models.Submission{}).
		Where("submission_type = ? AND DATE(created_at) = DATE(NOW())", submissionType).
		Count(&count)

	// Try up to 10 sequential numbers
	for i := int64(1); i <= 10; i++ {
		potentialNumber := fmt.Sprintf("%s-%s-%04d", prefix, dateStr, count+i)

		var existingCount int64
		config.DB.Model(&models.Submission{}).
			Where("submission_number = ?", potentialNumber).
			Count(&existingCount)

		if existingCount == 0 {
			return potentialNumber
		}
	}

	// Fallback to random suffix if sequential fails
	bytes := make([]byte, 3) // 6 characters hex
	rand.Read(bytes)
	randomSuffix := hex.EncodeToString(bytes)

	// Format: PR-20250730-R-A1B2C3 (R indicates random)
	return fmt.Sprintf("%s-%s-R-%s", prefix, dateStr, strings.ToUpper(randomSuffix))
}

// isValidFileType checks if file type is allowed
func isValidFileType(file *multipart.FileHeader) bool {
	allowedTypes := map[string]bool{
		"application/pdf":    true,
		"image/jpeg":         true,
		"image/jpg":          true,
		"image/png":          true,
		"image/gif":          true,
		"application/msword": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/vnd.ms-excel": true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
	}

	contentType := file.Header.Get("Content-Type")
	return allowedTypes[contentType]
}

// generateFileHash creates SHA256 hash of file content
func generateFileHash(file *multipart.FileHeader) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, src); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// PublicationDetails
func AddPublicationDetails(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")

	type PublicationDetailsRequest struct {
		// === ข้อมูลพื้นฐาน ===
		PaperTitle      string  `json:"article_title"` // จาก frontend
		JournalName     string  `json:"journal_name"`
		PublicationDate string  `json:"publication_date"` // รับเป็น string แล้วแปลง
		PublicationType string  `json:"publication_type"`
		Quartile        string  `json:"journal_quartile"` // จาก frontend
		ImpactFactor    float64 `json:"impact_factor"`
		DOI             string  `json:"doi"`
		URL             string  `json:"url"`
		PageNumbers     string  `json:"page_numbers"`
		VolumeIssue     string  `json:"volume_issue"`
		Indexing        string  `json:"indexing"`

		// === เงินรางวัลและการคำนวณ (ใหม่) ===
		RewardAmount          float64 `json:"publication_reward"` // จาก frontend
		RewardApproveAmount   float64 `json:"reward_approve_amount"`
		RevisionFee           float64 `json:"revision_fee"`
		PublicationFee        float64 `json:"publication_fee"`
		ExternalFundingAmount float64 `json:"external_funding_amount"`
		TotalAmount           float64 `json:"total_amount"`
		TotalApproveAmount    float64 `json:"total_approve_amount"`

		// === ข้อมูลผู้แต่ง ===
		AuthorCount int    `json:"author_count"`
		AuthorType  string `json:"author_status"` // จาก frontend (ยังใช้ชื่อเดิม)

		// === อื่นๆ ===
		AnnounceReferenceNumber string `json:"announce_reference_number"`

		// === ฟิลด์ใหม่ ===
		HasUniversityFunding string `json:"has_university_funding"` // "yes", "no"
		FundingReferences    string `json:"funding_references"`     // หมายเลขอ้างอิงทุน
		UniversityRankings   string `json:"university_rankings"`    // อันดับมหาวิทยาลัย
	}

	var req PublicationDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate submission exists and user has permission
	var submission models.Submission
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, userID).First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// แปลง publication_date จาก string เป็น time.Time
	pubDate, err := time.Parse("2006-01-02", req.PublicationDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid publication date format"})
		return
	}

	// สร้าง publication details ตรงกับ database schema ใหม่
	publicationDetails := models.PublicationRewardDetail{
		SubmissionID:    submission.SubmissionID,
		PaperTitle:      req.PaperTitle,
		JournalName:     req.JournalName,
		PublicationDate: pubDate,
		PublicationType: req.PublicationType,
		Quartile:        req.Quartile,
		ImpactFactor:    req.ImpactFactor,
		DOI:             req.DOI,
		URL:             req.URL,
		PageNumbers:     req.PageNumbers,
		VolumeIssue:     req.VolumeIssue,
		Indexing:        req.Indexing,

		// === เงินรางวัลและการคำนวณ (ใหม่) ===
		RewardAmount:          req.RewardAmount,
		RewardApproveAmount:   req.RewardApproveAmount,
		RevisionFee:           req.RevisionFee,
		PublicationFee:        req.PublicationFee,
		ExternalFundingAmount: req.ExternalFundingAmount,
		TotalAmount:           req.TotalAmount,
		TotalApproveAmount:    req.TotalApproveAmount,

		// === ข้อมูลผู้แต่ง ===
		AuthorCount: req.AuthorCount,
		AuthorType:  req.AuthorType, // เปลี่ยนจาก author_status เป็น author_type

		// === อื่นๆ ===
		AnnounceReferenceNumber: req.AnnounceReferenceNumber,

		// === ฟิลด์ใหม่ ===
		HasUniversityFunding: req.HasUniversityFunding, // → database: has_university_funding
		FundingReferences:    &req.FundingReferences,   // → database: funding_references
		UniversityRankings:   &req.UniversityRankings,  // → database: university_rankings
	}

	if err := config.DB.Create(&publicationDetails).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save publication details"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Publication details saved successfully",
		"details": publicationDetails,
	})
}

// AddFundDetails
func AddFundDetails(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")

	type FundDetailsRequest struct {
		ProjectTitle       string  `json:"project_title"`
		ProjectDescription string  `json:"project_description"`
		RequestedAmount    float64 `json:"requested_amount"`
		SubcategoryID      int     `json:"subcategory_id"`
	}

	var req FundDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate submission exists and user has permission
	var submission models.Submission
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, userID).First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Create fund application details
	fundDetails := models.FundApplicationDetail{
		SubmissionID:       submission.SubmissionID,
		SubcategoryID:      req.SubcategoryID,
		ProjectTitle:       req.ProjectTitle,
		ProjectDescription: req.ProjectDescription,
		RequestedAmount:    req.RequestedAmount,
	}

	if err := config.DB.Create(&fundDetails).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save fund details"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Fund details saved successfully",
		"details": fundDetails,
	})
}
