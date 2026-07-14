package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"
	"fund-management-api/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	approvalAttachmentMaxSize = int64(20 * 1024 * 1024)
	approvalAttachmentFolder  = "approval-evidence"
)

func approvalAttachmentSubmission(c *gin.Context, requireMutation bool) (*models.Submission, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission id"})
		return nil, false
	}

	var submission models.Submission
	if err := config.DB.Preload("User").First(&submission, "submission_id = ? AND deleted_at IS NULL", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load submission"})
		}
		return nil, false
	}
	// GORM may leave the pointer relation empty when the legacy user record is
	// filtered or the association cannot be preloaded. The filesystem path is
	// owned by the applicant, so resolve it explicitly before any dereference.
	if submission.User == nil && submission.UserID > 0 {
		var owner models.User
		if err := config.DB.Where("user_id = ?", submission.UserID).First(&owner).Error; err == nil {
			submission.User = &owner
		}
	}

	if requireMutation {
		eligible, err := utils.StatusMatchesCodes(submission.StatusID, utils.StatusCodeApproved, utils.StatusCodeAdminClosed)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve submission status"})
			return nil, false
		}
		if !eligible {
			c.JSON(http.StatusBadRequest, gin.H{"error": "approval evidence can only be managed after approval"})
			return nil, false
		}
	}

	return &submission, true
}

func canManageApprovalAttachment(c *gin.Context) bool {
	userID, userOK := c.Get("userID")
	roleID, roleOK := c.Get("roleID")
	if !userOK || !roleOK {
		return false
	}
	uid, okUID := userID.(int)
	rid, okRID := roleID.(int)
	if !okUID || !okRID {
		return false
	}
	return services.GetAuthorizationService().HasPermission(uid, rid, "submission.approval_attachment.manage")
}

func approvalAttachmentOwnerOrManager(c *gin.Context, submission *models.Submission) bool {
	if canManageApprovalAttachment(c) {
		return true
	}
	uid, ok := c.Get("userID")
	userID, valid := uid.(int)
	return ok && valid && submission != nil && submission.UserID == userID
}

func ListSubmissionApprovalAttachments(c *gin.Context) {
	submission, ok := approvalAttachmentSubmission(c, false)
	if !ok || !approvalAttachmentOwnerOrManager(c, submission) {
		c.JSON(http.StatusForbidden, gin.H{"error": "you do not have access to this submission"})
		return
	}

	var attachments []models.SubmissionApprovalAttachment
	err := config.DB.Preload("Uploader").
		Where("submission_id = ? AND deleted_at IS NULL", submission.SubmissionID).
		Order("display_order ASC, uploaded_at ASC, attachment_id ASC").Find(&attachments).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load approval attachments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"submission_id": submission.SubmissionID, "attachments": attachments})
}

func CreateSubmissionApprovalAttachment(c *gin.Context) {
	submission, ok := approvalAttachmentSubmission(c, true)
	if !ok {
		return
	}
	userID, _ := c.Get("userID")
	uploaderID, ok := userID.(int)
	if !ok || !canManageApprovalAttachment(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "approval attachment permission required"})
		return
	}

	label := strings.TrimSpace(c.PostForm("label"))
	if label == "" || len([]rune(label)) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "label is required and must not exceed 255 characters"})
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "PDF file is required"})
		return
	}
	if file.Size <= 0 || file.Size > approvalAttachmentMaxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "PDF file must not exceed 20MB"})
		return
	}
	if !isPDFUpload(file) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only valid PDF files are allowed"})
		return
	}
	if submission.User == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "submission applicant could not be resolved"})
		return
	}

	uploadRoot := os.Getenv("UPLOAD_PATH")
	if uploadRoot == "" {
		uploadRoot = "./uploads"
	}
	userFolder, err := utils.CreateUserFolderIfNotExists(*submission.User, uploadRoot)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload directory"})
		return
	}
	submissionFolder, err := utils.CreateSubmissionFolder(userFolder, submission.SubmissionType, submission.SubmissionID, submission.SubmissionNumber, submission.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create submission directory"})
		return
	}
	evidenceFolder := filepath.Join(submissionFolder, approvalAttachmentFolder)
	if err := os.MkdirAll(evidenceFolder, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create evidence directory"})
		return
	}

	storedFilename := utils.GenerateUniqueFilename(evidenceFolder, file.Filename)
	storedPath := filepath.Join(evidenceFolder, storedFilename)
	if err := c.SaveUploadedFile(file, storedPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save PDF file"})
		return
	}
	removeOnFailure := true
	defer func() {
		if removeOnFailure {
			_ = os.Remove(storedPath)
		}
	}()

	hash, err := sha256File(storedPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash PDF file"})
		return
	}
	now := time.Now()
	attachment := models.SubmissionApprovalAttachment{
		SubmissionID: submission.SubmissionID, Label: label,
		OriginalFilename: file.Filename, StoredFilename: storedFilename,
		StoredPath: storedPath, MimeType: "application/pdf", FileSize: file.Size,
		FileHash: hash, UploadedBy: uploaderID, UploadedAt: now, UpdatedAt: now,
	}

	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&attachment).Error; err != nil {
			return err
		}
		return createApprovalAttachmentAudit(tx, c, uploaderID, "create", &attachment, nil)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save approval attachment"})
		return
	}
	removeOnFailure = false
	config.DB.Preload("Uploader").First(&attachment, attachment.AttachmentID)
	c.JSON(http.StatusCreated, gin.H{"success": true, "attachment": attachment})
}

func UpdateSubmissionApprovalAttachment(c *gin.Context) {
	submission, ok := approvalAttachmentSubmission(c, true)
	if !ok {
		return
	}
	if !canManageApprovalAttachment(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "approval attachment permission required"})
		return
	}
	attachmentID, err := strconv.Atoi(c.Param("attachment_id"))
	if err != nil || attachmentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid attachment id"})
		return
	}
	var attachment models.SubmissionApprovalAttachment
	if err := config.DB.Where("attachment_id = ? AND submission_id = ? AND deleted_at IS NULL", attachmentID, submission.SubmissionID).First(&attachment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval attachment not found"})
		return
	}
	var req struct {
		Label        *string `json:"label"`
		DisplayOrder *int    `json:"display_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	old := attachment
	updates := map[string]any{"updated_at": time.Now()}
	if req.Label != nil {
		label := strings.TrimSpace(*req.Label)
		if label == "" || len([]rune(label)) > 255 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "label must not be empty or exceed 255 characters"})
			return
		}
		updates["label"] = label
	}
	if req.DisplayOrder != nil {
		if *req.DisplayOrder < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "display_order must be non-negative"})
			return
		}
		updates["display_order"] = *req.DisplayOrder
	}
	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&attachment).Updates(updates).Error; err != nil {
			return err
		}
		return createApprovalAttachmentAudit(tx, c, c.GetInt("userID"), "update", &attachment, &old)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update approval attachment"})
		return
	}
	config.DB.Preload("Uploader").First(&attachment, attachment.AttachmentID)
	c.JSON(http.StatusOK, gin.H{"success": true, "attachment": attachment})
}

func DeleteSubmissionApprovalAttachment(c *gin.Context) {
	submission, ok := approvalAttachmentSubmission(c, true)
	if !ok {
		return
	}
	if !canManageApprovalAttachment(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "approval attachment permission required"})
		return
	}
	attachmentID, err := strconv.Atoi(c.Param("attachment_id"))
	if err != nil || attachmentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid attachment id"})
		return
	}
	var attachment models.SubmissionApprovalAttachment
	if err := config.DB.Where("attachment_id = ? AND submission_id = ? AND deleted_at IS NULL", attachmentID, submission.SubmissionID).First(&attachment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval attachment not found"})
		return
	}
	now := time.Now()
	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&attachment).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return createApprovalAttachmentAudit(tx, c, c.GetInt("userID"), "delete", &attachment, nil)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete approval attachment"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "approval attachment deleted"})
}

func DownloadSubmissionApprovalAttachment(c *gin.Context) {
	attachmentID, err := strconv.Atoi(c.Param("attachment_id"))
	if err != nil || attachmentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid attachment id"})
		return
	}
	var attachment models.SubmissionApprovalAttachment
	if err := config.DB.Preload("Submission").Where("attachment_id = ? AND deleted_at IS NULL", attachmentID).First(&attachment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval attachment not found"})
		return
	}
	if !approvalAttachmentOwnerOrManager(c, &attachment.Submission) {
		c.JSON(http.StatusForbidden, gin.H{"error": "you do not have access to this attachment"})
		return
	}
	if _, err := os.Stat(attachment.StoredPath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found on disk"})
		return
	}
	if err := createApprovalAttachmentAudit(config.DB, c, c.GetInt("userID"), "download", &attachment, nil); err != nil {
		// A download should remain available even if audit storage is temporarily
		// unavailable; the failure is still visible in server logs.
		fmt.Printf("approval attachment download audit failed: %v\n", err)
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", attachment.OriginalFilename))
	c.Header("Content-Type", "application/pdf")
	c.File(attachment.StoredPath)
}

func isPDFUpload(file *multipart.FileHeader) bool {
	if file == nil || !strings.EqualFold(filepath.Ext(file.Filename), ".pdf") {
		return false
	}
	opened, err := file.Open()
	if err != nil {
		return false
	}
	defer opened.Close()
	header := make([]byte, 5)
	if _, err := io.ReadFull(opened, header); err != nil {
		return false
	}
	return string(header) == "%PDF-"
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func createApprovalAttachmentAudit(tx *gorm.DB, c *gin.Context, userID int, action string, attachment *models.SubmissionApprovalAttachment, old *models.SubmissionApprovalAttachment) error {
	description := fmt.Sprintf("Approval evidence %s for submission %d", action, attachment.SubmissionID)
	newJSON, _ := json.Marshal(map[string]any{"attachment_id": attachment.AttachmentID, "label": attachment.Label, "original_filename": attachment.OriginalFilename})
	var oldJSON *string
	if old != nil {
		b, _ := json.Marshal(map[string]any{"attachment_id": old.AttachmentID, "label": old.Label, "original_filename": old.OriginalFilename})
		value := string(b)
		oldJSON = &value
	}
	newValue := string(newJSON)
	entityID := attachment.AttachmentID
	entityNumber := strconv.Itoa(attachment.SubmissionID)
	return tx.Create(&models.AuditLog{UserID: userID, Action: action, EntityType: "submission_approval_attachment", EntityID: &entityID, EntityNumber: &entityNumber, OldValues: oldJSON, NewValues: &newValue, Description: &description, IPAddress: c.ClientIP(), UserAgent: func() *string { v := c.GetHeader("User-Agent"); return &v }(), CreatedAt: time.Now()}).Error
}
