// controllers/submission.go
package controllers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const publicationRewardFormDocumentCode = "publication_reward_form_docx"
const publicationRewardFormPdfDocumentCode = "publication_reward_form_pdf"

// ===================== SUBMISSION MANAGEMENT =====================

// GetSubmissions returns user's submissions
func GetSubmissions(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	submissionType := c.Query("submission_type")
	status := c.Query("status")
	yearID := c.Query("year_id")

	var submissions []models.Submission
	query := config.DB.Preload("User").
		Preload("Year").
		Preload("Status").
		Preload("Category").
		Preload("Subcategory").
		Preload("Documents.File").
		Preload("Documents.DocumentType").
		Preload("SubmissionUsers.User").
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

	// เพิ่มการโหลด type-specific details สำหรับแต่ละ submission
	for i := range submissions {
		switch submissions[i].SubmissionType {
		case "fund_application":
			fundDetail := &models.FundApplicationDetail{}
			if err := config.DB.Preload("Subcategory.Category").Where("submission_id = ?", submissions[i].SubmissionID).First(fundDetail).Error; err == nil {
				if submissions[i].StatusID != 2 {
					fundDetail.AnnounceReferenceNumber = ""
				}
				submissions[i].FundApplicationDetail = fundDetail
				submissions[i].AnnounceReferenceNumber = fundDetail.AnnounceReferenceNumber
			}
		case "publication_reward":
			pubDetail := &models.PublicationRewardDetail{}
			if err := config.DB.Where("submission_id = ?", submissions[i].SubmissionID).First(pubDetail).Error; err == nil {
				if submissions[i].StatusID != 2 {
					pubDetail.AnnounceReferenceNumber = ""
				}
				submissions[i].PublicationRewardDetail = pubDetail
				submissions[i].AnnounceReferenceNumber = pubDetail.AnnounceReferenceNumber
			}
		}
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
	query := config.DB.Model(&models.Submission{}).
		Joins("LEFT JOIN fund_categories ON submissions.category_id = fund_categories.category_id").
		Joins("LEFT JOIN fund_subcategories ON fund_subcategories.subcategory_id = submissions.subcategory_id AND fund_subcategories.category_id = submissions.category_id").
		Select("submissions.*, fund_categories.category_name AS category_name, CASE WHEN fund_subcategories.subcategory_id IS NULL THEN NULL ELSE submissions.subcategory_id END AS subcategory_id, fund_subcategories.subcategory_name AS subcategory_name").
		Preload("User").
		Preload("Year").
		Preload("Status").
		Preload("Documents", func(db *gorm.DB) *gorm.DB {
			return db.Joins("LEFT JOIN document_types dt ON dt.document_type_id = submission_documents.document_type_id").
				Joins("LEFT JOIN publication_reward_external_funds pref ON pref.document_id = submission_documents.document_id AND (pref.deleted_at IS NULL OR pref.deleted_at = '0000-00-00 00:00:00')").
				Select("submission_documents.*, dt.document_type_name, pref.external_fund_id AS external_funding_id").
				Order("submission_documents.display_order, submission_documents.created_at")
		}).
		Preload("Documents.File").
		Preload("Documents.DocumentType").
		Preload("SubmissionUsers.User")

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.Where("submission_id = ? AND deleted_at IS NULL", submissionID).First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Ensure applicant user data is loaded
	if submission.User == nil && submission.UserID != 0 {
		var applicant models.User
		if err := config.DB.
			Select("user_id", "user_fname", "user_lname", "email").
			Where("user_id = ?", submission.UserID).
			First(&applicant).Error; err == nil {
			submission.User = &applicant
		}
	}

	// เพิ่มการโหลด submission_users พร้อม User data
	var submissionUsers []models.SubmissionUser
	if err := config.DB.
		Where("submission_id = ?", submissionID).
		Preload("User").
		Order("display_order ASC").
		Find(&submissionUsers).Error; err == nil {
		applicantIncluded := false
		for i := range submissionUsers {
			if submissionUsers[i].UserID == submission.UserID {
				submissionUsers[i].IsApplicant = true
				applicantIncluded = true
			}
		}

		if !applicantIncluded && submission.User != nil {
			applicant := models.SubmissionUser{
				SubmissionID: submission.SubmissionID,
				UserID:       submission.UserID,
				User:         submission.User,
				Role:         "owner",
				IsPrimary:    true,
				DisplayOrder: 1,
				IsApplicant:  true,
			}
			submissionUsers = append([]models.SubmissionUser{applicant}, submissionUsers...)
		}

		submission.SubmissionUsers = submissionUsers
	}

	// Load type-specific details
	switch submission.SubmissionType {
	case "fund_application":
		fundDetail := &models.FundApplicationDetail{}
		if err := config.DB.Preload("Subcategory.Category").Where("submission_id = ?", submission.SubmissionID).First(fundDetail).Error; err == nil {
			submission.FundApplicationDetail = fundDetail
		}
	case "publication_reward":
		pubDetail := &models.PublicationRewardDetail{}
		if err := config.DB.Preload("ExternalFunds", func(db *gorm.DB) *gorm.DB {
			return db.Where("publication_reward_external_funds.deleted_at IS NULL OR publication_reward_external_funds.deleted_at = '0000-00-00 00:00:00'").
				Order("external_fund_id ASC").
				Preload("Document").
				Preload("Document.File")
		}).Where("submission_id = ?", submission.SubmissionID).First(pubDetail).Error; err == nil {
			if submission.StatusID != 2 {
				pubDetail.ApprovedAmount = nil
				pubDetail.AnnounceReferenceNumber = ""
			}
			submission.PublicationRewardDetail = pubDetail
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"submission":        submission,
		"applicant_user":    submission.User,
		"applicant_user_id": submission.UserID,
		"submission_users":  submission.SubmissionUsers,
	})
}

// CreateSubmission creates a new submission
func CreateSubmission(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID := 0
	if roleVal, exists := c.Get("roleID"); exists {
		if cast, ok := roleVal.(int); ok {
			roleID = cast
		}
	}

	type CreateSubmissionRequest struct {
		SubmissionType      string `json:"submission_type" binding:"required"` // 'fund_application', 'publication_reward', ...
		YearID              int    `json:"year_id" binding:"required"`
		CategoryID          *int   `json:"category_id"`           // <-- ใหม่
		SubcategoryID       *int   `json:"subcategory_id"`        // <-- ใหม่
		SubcategoryBudgetID *int   `json:"subcategory_budget_id"` // <-- ใหม่
		StatusID            *int   `json:"status_id"`
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

	statusID, err := determineInitialStatusID(req.SubmissionType, req.StatusID, roleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create submission
	now := time.Now()
	submission := models.Submission{
		SubmissionType:   req.SubmissionType,
		SubmissionNumber: generateSubmissionNumber(req.SubmissionType),
		UserID:           userID.(int),
		YearID:           req.YearID,
		StatusID:         statusID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// เซ็ตฟิลด์หมวดหมู่ถ้ามีส่งมา
	if req.CategoryID != nil {
		submission.CategoryID = req.CategoryID
	}
	if req.SubcategoryID != nil {
		submission.SubcategoryID = req.SubcategoryID
	}
	if req.SubcategoryBudgetID != nil {
		submission.SubcategoryBudgetID = req.SubcategoryBudgetID
	}

	if err := config.DB.Create(&submission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create submission"})
		return
	}

	config.DB.Preload("User").Preload("Year").Preload("Status").First(&submission, submission.SubmissionID)
	c.JSON(http.StatusCreated, gin.H{
		"success":    true,
		"message":    "Submission created successfully",
		"submission": submission,
	})
}

func determineInitialStatusID(submissionType string, requestedStatusID *int, roleID int) (int, error) {
	if requestedStatusID != nil && roleID == 3 {
		status, err := utils.GetApplicationStatusByID(*requestedStatusID)
		if err != nil {
			return 0, err
		}
		return status.ApplicationStatusID, nil
	}

	switch submissionType {
	case "fund_application":
		return utils.GetStatusIDByCode(utils.StatusCodeDeptHeadPending)
	case "publication_reward":
		return utils.GetStatusIDByCode(utils.StatusCodeDeptHeadPending)
	default:
		return utils.GetStatusIDByCode(utils.StatusCodePending)
	}
}

// UpdateSubmission updates a submission (only if editable)
func UpdateSubmission(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type UpdateSubmissionRequest struct {
		CategoryID                *int `json:"category_id"`
		SubcategoryID             *int `json:"subcategory_id"`
		SubcategoryBudgetID       *int `json:"subcategory_budget_id"`
		InstallmentNumberAtSubmit *int `json:"installment_number_at_submit"`
		// อนาคตจะมีฟิลด์อื่นก็ใส่เพิ่มได้
	}

	var req UpdateSubmissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)
	if roleID.(int) != 3 { // ถ้าไม่ใช่ admin ต้องเป็นเจ้าของรายการเท่านั้น
		query = query.Where("user_id = ?", userID)
	}
	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	updates := map[string]interface{}{"updated_at": time.Now()}

	if req.CategoryID != nil {
		updates["category_id"] = *req.CategoryID
	}
	if req.SubcategoryID != nil {
		updates["subcategory_id"] = *req.SubcategoryID
	}
	if req.SubcategoryBudgetID != nil {
		updates["subcategory_budget_id"] = *req.SubcategoryBudgetID
	}
	if req.InstallmentNumberAtSubmit != nil {
		updates["installment_number_at_submit"] = *req.InstallmentNumberAtSubmit
	}

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
	userIDValue, _ := c.Get("userID")
	userID, ok := userIDValue.(int)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user context"})
		return
	}

	var submission models.Submission
	if err := config.DB.
		Preload("User").
		Preload("User.Position").
		Where("submission_id = ? AND user_id = ? AND deleted_at IS NULL", submissionID, userID).
		First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	if !submission.CanBeSubmitted() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission cannot be submitted"})
		return
	}

	now := time.Now()

	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		resolvedInstallment, resolveErr := determineSubmissionInstallmentNumber(tx, submission.YearID, now)
		if resolveErr != nil {
			log.Printf("failed to resolve installment number for submission %d: %v", submission.SubmissionID, resolveErr)
		}

		updates := map[string]interface{}{
			"submitted_at": &now,
			"updated_at":   now,
		}

		if submission.InstallmentNumberAtSubmit != nil {
			updates["installment_number_at_submit"] = *submission.InstallmentNumberAtSubmit
		}

		if resolvedInstallment != nil {
			updates["installment_number_at_submit"] = *resolvedInstallment
		}

		if err := tx.Model(&models.Submission{}).
			Where("submission_id = ?", submission.SubmissionID).
			Updates(updates).Error; err != nil {
			return err
		}

		submission.SubmittedAt = &now
		submission.UpdatedAt = now
		if resolvedInstallment != nil {
			submission.InstallmentNumberAtSubmit = resolvedInstallment
		}

		if submission.SubmissionType != "publication_reward" {
			return nil
		}

		applicant := submission.User
		if applicant == nil {
			applicant = &models.User{}
		}
		if err := tx.Preload("Position").Where("user_id = ?", submission.UserID).First(applicant).Error; err != nil {
			return fmt.Errorf("failed to load applicant: %w", err)
		}
		submission.User = applicant

		var detail models.PublicationRewardDetail
		if err := tx.Where("submission_id = ? AND (delete_at IS NULL OR delete_at = '0000-00-00 00:00:00')", submission.SubmissionID).
			First(&detail).Error; err != nil {
			return fmt.Errorf("failed to load publication reward detail: %w", err)
		}

		sysConfig, err := fetchLatestSystemConfig()
		if err != nil {
			return fmt.Errorf("failed to load system configuration: %w", err)
		}

		if err := resequenceSubmissionDocumentsByDocumentType(tx, submission.SubmissionID); err != nil {
			return fmt.Errorf("failed to resequence submission documents: %w", err)
		}

		documents, err := fetchSubmissionDocuments(tx, submission.SubmissionID)
		if err != nil {
			return fmt.Errorf("failed to load submission documents: %w", err)
		}

		replacements, err := buildSubmissionPreviewReplacements(&submission, &detail, sysConfig, documents)
		if err != nil {
			return err
		}

		docType, err := ensurePublicationRewardFormDocumentType(tx)
		if err != nil {
			return fmt.Errorf("failed to prepare document type: %w", err)
		}

		pdfDocType, err := ensurePublicationRewardFormPdfDocumentType(tx)
		if err != nil {
			return fmt.Errorf("failed to prepare pdf document type: %w", err)
		}

		uploadPath := os.Getenv("UPLOAD_PATH")
		if uploadPath == "" {
			uploadPath = "./uploads"
		}

		userFolderPath, err := utils.CreateUserFolderIfNotExists(*submission.User, uploadPath)
		if err != nil {
			return fmt.Errorf("failed to prepare user directory: %w", err)
		}

		submissionFolderPath, err := utils.CreateSubmissionFolder(userFolderPath, submission.SubmissionType, submission.SubmissionID, submission.SubmissionNumber, submission.CreatedAt)
		if err != nil {
			return fmt.Errorf("failed to prepare submission folder: %w", err)
		}

		baseFilename := "publication_reward_form.docx"
		if submission.SubmissionNumber != "" {
			baseFilename = fmt.Sprintf("%s_publication_reward_form.docx", submission.SubmissionNumber)
		}
		uniqueFilename := utils.GenerateUniqueFilename(submissionFolderPath, baseFilename)
		outputPath := filepath.Join(submissionFolderPath, uniqueFilename)

		if err := renderPublicationRewardDocx(outputPath, replacements); err != nil {
			return err
		}

		stat, err := os.Stat(outputPath)
		if err != nil {
			os.Remove(outputPath)
			return fmt.Errorf("failed to stat generated docx: %w", err)
		}

		fileUpload := models.FileUpload{
			OriginalName: uniqueFilename,
			StoredPath:   outputPath,
			FolderType:   "submission",
			FileSize:     stat.Size(),
			MimeType:     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			FileHash:     "",
			IsPublic:     false,
			UploadedBy:   submission.UserID,
			UploadedAt:   now,
			CreateAt:     now,
			UpdateAt:     now,
		}

		if err := createFileUploadRecord(tx, &fileUpload); err != nil {
			os.Remove(outputPath)
			return fmt.Errorf("failed to persist generated docx: %w", err)
		}

		displayOrder := nextDocumentDisplayOrder(documents)
		submissionDocument := models.SubmissionDocument{
			SubmissionID:   submission.SubmissionID,
			FileID:         fileUpload.FileID,
			OriginalName:   fileUpload.OriginalName,
			DocumentTypeID: docType.DocumentTypeID,
			DisplayOrder:   displayOrder,
			IsRequired:     false,
			IsVerified:     false,
			CreatedAt:      now,
		}

		if err := createSubmissionDocumentRecord(tx, &submissionDocument); err != nil {
			os.Remove(outputPath)
			return fmt.Errorf("failed to register generated docx: %w", err)
		}

		documents = append(documents, submissionDocument)

		pdfData, err := convertDocxToPDFBytes(outputPath)
		if err != nil {
			return fmt.Errorf("failed to generate pdf: %w", err)
		}

		pdfBaseFilename := strings.TrimSuffix(uniqueFilename, filepath.Ext(uniqueFilename)) + ".pdf"
		pdfFilename := utils.GenerateUniqueFilename(submissionFolderPath, pdfBaseFilename)
		pdfOutputPath := filepath.Join(submissionFolderPath, pdfFilename)

		if err := os.WriteFile(pdfOutputPath, pdfData, 0o644); err != nil {
			return fmt.Errorf("failed to write generated pdf: %w", err)
		}

		pdfStat, err := os.Stat(pdfOutputPath)
		if err != nil {
			os.Remove(pdfOutputPath)
			return fmt.Errorf("failed to stat generated pdf: %w", err)
		}

		pdfFileUpload := models.FileUpload{
			OriginalName: pdfFilename,
			StoredPath:   pdfOutputPath,
			FolderType:   "submission",
			FileSize:     pdfStat.Size(),
			MimeType:     "application/pdf",
			FileHash:     "",
			IsPublic:     false,
			UploadedBy:   submission.UserID,
			UploadedAt:   now,
			CreateAt:     now,
			UpdateAt:     now,
		}

		if err := createFileUploadRecord(tx, &pdfFileUpload); err != nil {
			os.Remove(pdfOutputPath)
			return fmt.Errorf("failed to persist generated pdf: %w", err)
		}

		pdfDisplayOrder := nextDocumentDisplayOrder(documents)
		pdfSubmissionDocument := models.SubmissionDocument{
			SubmissionID:   submission.SubmissionID,
			FileID:         pdfFileUpload.FileID,
			OriginalName:   pdfFileUpload.OriginalName,
			DocumentTypeID: pdfDocType.DocumentTypeID,
			DisplayOrder:   pdfDisplayOrder,
			IsRequired:     false,
			IsVerified:     false,
			CreatedAt:      now,
		}

		if err := createSubmissionDocumentRecord(tx, &pdfSubmissionDocument); err != nil {
			os.Remove(pdfOutputPath)
			return fmt.Errorf("failed to register generated pdf: %w", err)
		}

		documents = append(documents, pdfSubmissionDocument)

		if err := resequenceSubmissionDocumentsByDocumentType(tx, submission.SubmissionID); err != nil {
			return fmt.Errorf("failed to resequence submission documents: %w", err)
		}

		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Submission submitted successfully",
	})
}

func determineSubmissionInstallmentNumber(db *gorm.DB, yearID int, submissionTime time.Time) (*int, error) {
	number, err := resolveInstallmentNumberFromPeriods(db, yearID, submissionTime)
	if err != nil {
		return nil, err
	}
	return number, nil
}

func resolveInstallmentNumberFromPeriods(db *gorm.DB, yearID int, submissionTime time.Time) (*int, error) {
	if db == nil {
		db = config.DB
	}

	var periods []models.FundInstallmentPeriod
	query := db.Model(&models.FundInstallmentPeriod{}).
		Where("deleted_at IS NULL").
		Order("cutoff_date ASC, installment_number ASC")

	if yearID > 0 {
		query = query.Where("year_id = ?", yearID)
	}

	if err := query.Find(&periods).Error; err != nil {
		return nil, err
	}
	if len(periods) == 0 {
		return nil, nil
	}

	active := make([]models.FundInstallmentPeriod, 0, len(periods))
	for _, period := range periods {
		if isInstallmentPeriodActive(period.Status) {
			active = append(active, period)
		}
	}

	if len(active) == 0 {
		return nil, nil
	}

	candidates := active

	submissionUTC := submissionTime.UTC()

	for _, period := range candidates {
		if period.CutoffDate.IsZero() {
			continue
		}
		cutoff := endOfDayUTC(period.CutoffDate)
		if !submissionUTC.After(cutoff) {
			value := period.InstallmentNumber
			return &value, nil
		}
	}

	last := candidates[len(candidates)-1].InstallmentNumber
	return &last, nil
}

func isInstallmentPeriodActive(status *string) bool {
	if status == nil {
		return true
	}

	normalized := strings.TrimSpace(strings.ToLower(*status))
	if normalized == "" {
		return true
	}

	switch normalized {
	case "active", "enabled", "open", "current":
		return true
	default:
		return false
	}
}

func endOfDayUTC(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}

	year, month, day := t.In(time.UTC).Date()
	return time.Date(year, month, day, 23, 59, 59, 999999999, time.UTC)
}

// MergeSubmissionDocuments collects every PDF document attached to a submission, merges them
// into a single file and stores the result under uploads/merge_submissions/{current_year}.
func MergeSubmissionDocuments(c *gin.Context) {
	submissionIDParam := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDParam)
	if err != nil || submissionID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission id"})
		return
	}

	userIDValue, _ := c.Get("userID")
	userID, ok := userIDValue.(int)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user context"})
		return
	}

	log.Printf("[MergeSubmissionDocuments] user %d requested merge for submission %d", userID, submissionID)

	roleIDValue, _ := c.Get("roleID")
	roleID, _ := roleIDValue.(int)

	query := config.DB.
		Preload("Documents.File").
		Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	if roleID != 3 { // allow admin (role id 3) to access every submission
		query = query.Where("user_id = ?", userID)
	}

	var submission models.Submission
	if err := query.First(&submission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[MergeSubmissionDocuments] submission %d not found", submissionID)
			c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
			return
		}
		log.Printf("[MergeSubmissionDocuments] failed to load submission %d: %v", submissionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load submission"})
		return
	}

	if submission.SubmittedAt == nil {
		log.Printf("[MergeSubmissionDocuments] submission %d has not been submitted yet", submission.SubmissionID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission must be submitted before merging documents"})
		return
	}

	documents, err := fetchSubmissionDocuments(config.DB, submission.SubmissionID)
	if err != nil {
		log.Printf("[MergeSubmissionDocuments] failed to load documents for submission %d: %v", submission.SubmissionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load submission documents"})
		return
	}

	uploadRoot := os.Getenv("UPLOAD_PATH")
	if uploadRoot == "" {
		uploadRoot = "./uploads"
	}

	pdfPaths := make([]string, 0, len(documents))
	for _, doc := range documents {
		file := doc.File
		log.Printf("[MergeSubmissionDocuments] inspecting document %d (file_id=%d) for submission %d", doc.DocumentID, file.FileID, submission.SubmissionID)
		if file.FileID == 0 {
			log.Printf("[MergeSubmissionDocuments] skipping document %d: missing file record", doc.DocumentID)
			continue
		}

		storedPath := strings.TrimSpace(file.StoredPath)
		if storedPath == "" {
			log.Printf("[MergeSubmissionDocuments] skipping document %d: empty stored path", doc.DocumentID)
			continue
		}

		mimeType := strings.ToLower(strings.TrimSpace(file.MimeType))
		ext := strings.ToLower(filepath.Ext(storedPath))
		originalExt := strings.ToLower(filepath.Ext(file.OriginalName))
		log.Printf("[MergeSubmissionDocuments] document %d mime=%q stored_ext=%q original_ext=%q", doc.DocumentID, mimeType, ext, originalExt)

		if mimeType != "application/pdf" && ext != ".pdf" && originalExt != ".pdf" {
			log.Printf("[MergeSubmissionDocuments] skipping document %d: not a pdf", doc.DocumentID)
			continue
		}

		resolvedPath := resolveStoredFilePath(storedPath, uploadRoot)
		if resolvedPath == "" {
			log.Printf("[MergeSubmissionDocuments] skipping document %d: could not resolve stored path %q", doc.DocumentID, storedPath)
			continue
		}

		log.Printf("[MergeSubmissionDocuments] submission %d resolved pdf path %s", submission.SubmissionID, resolvedPath)
		pdfPaths = append(pdfPaths, resolvedPath)
	}

	if len(pdfPaths) == 0 {
		log.Printf("[MergeSubmissionDocuments] submission %d has no PDF documents", submission.SubmissionID)
		c.JSON(http.StatusOK, gin.H{
			"success":       true,
			"merged_file":   nil,
			"message":       "No PDF documents available to merge",
			"pdf_documents": 0,
		})
		return
	}

	currentYear := getCurrentBEYearStr()
	mergeDir := filepath.Join(uploadRoot, "merge_submissions", currentYear)
	log.Printf("[MergeSubmissionDocuments] preparing merge output directory %s", mergeDir)
	if err := os.MkdirAll(mergeDir, 0o755); err != nil {
		log.Printf("[MergeSubmissionDocuments] failed to create merge directory %s: %v", mergeDir, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare merge directory"})
		return
	}

	baseName := strings.TrimSpace(submission.SubmissionNumber)
	if baseName == "" {
		baseName = fmt.Sprintf("%s-%d", strings.ToUpper(strings.TrimSpace(submission.SubmissionType)), submission.SubmissionID)
	}
	baseName = utils.SanitizeForFilename(baseName)
	if baseName == "" {
		baseName = fmt.Sprintf("submission-%d", submission.SubmissionID)
	}

	desiredFilename := fmt.Sprintf("%s_merged_document.pdf", baseName)
	safeFilename := utils.GenerateUniqueFilename(mergeDir, desiredFilename)
	outputPath := filepath.Join(mergeDir, safeFilename)

	log.Printf("[MergeSubmissionDocuments] merging %d pdf(s) into %s", len(pdfPaths), outputPath)
	if err := mergePDFs(pdfPaths, outputPath); err != nil {
		log.Printf("[MergeSubmissionDocuments] merge failed for submission %d: %v", submission.SubmissionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to merge PDF documents: %v", err)})
		return
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		os.Remove(outputPath)
		log.Printf("[MergeSubmissionDocuments] failed to stat merged file %s: %v", outputPath, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to access merged file"})
		return
	}

	now := time.Now()
	fileRecord := models.FileUpload{
		OriginalName: safeFilename,
		StoredPath:   outputPath,
		FolderType:   "submission",
		FileSize:     info.Size(),
		MimeType:     "application/pdf",
		UploadedBy:   submission.UserID,
		UploadedAt:   now,
		CreateAt:     now,
		UpdateAt:     now,
	}

	if err := createFileUploadRecord(config.DB, &fileRecord); err != nil {
		os.Remove(outputPath)
		log.Printf("[MergeSubmissionDocuments] failed to persist file record for submission %d: %v", submission.SubmissionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist merged document"})
		return
	}

	cleanedRoot := filepath.Clean(uploadRoot)
	relativePath := filepath.ToSlash(outputPath)
	if trimmed := strings.TrimPrefix(relativePath, cleanedRoot+"/"); trimmed != relativePath {
		relativePath = filepath.ToSlash(filepath.Join(filepath.Base(cleanedRoot), trimmed))
	}

	log.Printf("[MergeSubmissionDocuments] submission %d merged file stored as %s (file_id=%d)", submission.SubmissionID, relativePath, fileRecord.FileID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"merged_file": gin.H{
			"file_id":       fileRecord.FileID,
			"filename":      fileRecord.OriginalName,
			"stored_path":   fileRecord.StoredPath,
			"relative_path": relativePath,
			"size":          fileRecord.FileSize,
		},
	})
}

func resolveStoredFilePath(storedPath, uploadRoot string) string {
	trimmed := strings.TrimSpace(storedPath)
	if trimmed == "" {
		return ""
	}

	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	fromSlash := filepath.FromSlash(normalized)

	candidates := make([]string, 0, 6)
	seen := make(map[string]struct{})
	addCandidate := func(path string) {
		cleaned := strings.TrimSpace(path)
		if cleaned == "" {
			return
		}
		cleaned = filepath.Clean(filepath.FromSlash(strings.ReplaceAll(cleaned, "\\", "/")))
		if cleaned == "." {
			return
		}
		if _, exists := seen[cleaned]; exists {
			return
		}
		seen[cleaned] = struct{}{}
		candidates = append(candidates, cleaned)
	}

	addCandidate(trimmed)
	addCandidate(normalized)
	addCandidate(fromSlash)

	rootCandidate := strings.TrimSpace(uploadRoot)
	if rootCandidate != "" {
		rootCandidate = filepath.Clean(filepath.FromSlash(strings.ReplaceAll(rootCandidate, "\\", "/")))
		if rootCandidate != "" && rootCandidate != "." {
			if !filepath.IsAbs(fromSlash) && filepath.VolumeName(fromSlash) == "" {
				addCandidate(filepath.Join(rootCandidate, normalized))
				addCandidate(filepath.Join(rootCandidate, fromSlash))
			}
		}
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func nextDocumentDisplayOrder(existing []models.SubmissionDocument) int {
	maxOrder := 0
	for _, doc := range existing {
		if doc.DisplayOrder > maxOrder {
			maxOrder = doc.DisplayOrder
		}
	}
	if maxOrder == 0 {
		return len(existing) + 1
	}
	return maxOrder + 1
}

func ensurePublicationRewardFormDocumentType(tx *gorm.DB) (*models.DocumentType, error) {
	var docType models.DocumentType
	if err := tx.Where("code = ? AND (delete_at IS NULL OR delete_at = '0000-00-00 00:00:00')", publicationRewardFormDocumentCode).
		First(&docType).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}

		now := time.Now()
		fundTypes := "[\"publication_reward\"]"
		docType = models.DocumentType{
			DocumentTypeName: "แบบฟอร์มคำขอรับเงินรางวัล (DOCX)",
			Code:             publicationRewardFormDocumentCode,
			Required:         false,
			Multiple:         false,
			DocumentOrder:    0,
			CreateAt:         now,
			UpdateAt:         now,
			FundTypes:        &fundTypes,
		}

		if err := tx.Create(&docType).Error; err != nil {
			return nil, err
		}
	}

	return &docType, nil
}

func ensurePublicationRewardFormPdfDocumentType(tx *gorm.DB) (*models.DocumentType, error) {
	var docType models.DocumentType
	if err := tx.Where("code = ? AND (delete_at IS NULL OR delete_at = '0000-00-00 00:00:00')", publicationRewardFormPdfDocumentCode).
		First(&docType).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}

		now := time.Now()
		fundTypes := "[\"publication_reward\"]"
		docType = models.DocumentType{
			DocumentTypeName: "แบบฟอร์มคำขอรับเงินรางวัล (PDF)",
			Code:             publicationRewardFormPdfDocumentCode,
			Required:         false,
			Multiple:         false,
			DocumentOrder:    0,
			CreateAt:         now,
			UpdateAt:         now,
			FundTypes:        &fundTypes,
		}

		if err := tx.Create(&docType).Error; err != nil {
			return nil, err
		}
	}

	return &docType, nil
}

func convertDocxToPDFBytes(docxPath string) ([]byte, error) {
	trimmed := strings.TrimSpace(docxPath)
	if trimmed == "" {
		return nil, fmt.Errorf("docx path is required")
	}

	info, err := os.Stat(trimmed)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("docx file not found")
		}
		return nil, fmt.Errorf("failed to access docx: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("docx path points to a directory")
	}

	tmpDir, err := os.MkdirTemp("", "publication-form-pdf-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fontEnv, err := configureLibreOfficeFonts(tmpDir)
	if err != nil {
		return nil, err
	}

	converter, err := lookupLibreOfficeBinary()
	if err != nil {
		return nil, err
	}

	profileDir := filepath.Join(tmpDir, "lo-profile")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to prepare libreoffice profile: %w", err)
	}

	profileURL, err := fileURLFromPath(profileDir)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare libreoffice profile: %w", err)
	}

	profileArg := fmt.Sprintf("-env:UserInstallation=%s", profileURL)
	filterArg := "pdf:writer_pdf_Export:EmbedStandardFonts=true;EmbedFonts=true"
	args := []string{profileArg, "--headless", "--convert-to", filterArg, "--outdir", tmpDir, trimmed}
	cmd := exec.Command(converter, args...)
	env := append([]string{}, os.Environ()...)
	if len(fontEnv) > 0 {
		env = append(env, fontEnv...)
	}
	cmd.Env = env

	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to convert docx to pdf: %v", strings.TrimSpace(string(output)))
	}

	pdfName := strings.TrimSuffix(filepath.Base(trimmed), filepath.Ext(trimmed)) + ".pdf"
	outputPDF := filepath.Join(tmpDir, pdfName)
	data, err := os.ReadFile(outputPDF)
	if err != nil {
		return nil, fmt.Errorf("failed to read generated pdf: %w", err)
	}

	return data, nil
}

func buildSubmissionPreviewReplacements(submission *models.Submission, detail *models.PublicationRewardDetail, sysConfig *systemConfigSnapshot, documents []models.SubmissionDocument) (map[string]string, error) {
	if submission == nil {
		return nil, fmt.Errorf("submission is required")
	}
	if submission.User == nil {
		return nil, fmt.Errorf("submission missing applicant information")
	}
	if detail == nil {
		return nil, fmt.Errorf("publication reward detail is required")
	}
	if sysConfig == nil {
		sysConfig = &systemConfigSnapshot{}
	}

	var documentDate time.Time
	switch {
	case submission.SubmittedAt != nil:
		documentDate = *submission.SubmittedAt
	case !submission.CreatedAt.IsZero():
		documentDate = submission.CreatedAt
	default:
		documentDate = time.Now()
	}

	positionName := ""
	if submission.User != nil {
		positionName = strings.TrimSpace(submission.User.Position.PositionName)
	}

	replacements := map[string]string{
		"{{date_th}}":            utils.FormatThaiDate(documentDate),
		"{{applicant_name}}":     buildApplicantName(submission.User),
		"{{date_of_employment}}": resolveApplicantEmploymentDate(submission.User),
		"{{position}}":           positionName,
		"{{installment}}":        formatNullableInt(sysConfig.Installment),
		"{{total_amount}}":       formatAmount(detail.TotalAmount),
		"{{total_amount_text}}":  utils.BahtText(detail.TotalAmount),
		"{{author_name_list}}":   strings.TrimSpace(detail.AuthorNameList),
		"{{paper_title}}":        strings.TrimSpace(detail.PaperTitle),
		"{{journal_name}}":       strings.TrimSpace(detail.JournalName),
		"{{publication_year}}":   formatThaiYear(detail.PublicationDate),
		"{{volume_issue}}":       strings.TrimSpace(detail.VolumeIssue),
		"{{page_number}}":        strings.TrimSpace(detail.PageNumbers),
		"{{author_role}}":        buildAuthorRole(detail.AuthorType),
		"{{quartile_line}}":      buildQuartileLine(detail.Quartile),
		"{{document_line}}":      buildDocumentLine(documents),
		"{{kku_report_year}}":    formatNullableString(sysConfig.KkuReportYear),
		"{{signature}}":          strings.TrimSpace(detail.Signature),
	}

	return replacements, nil
}

func renderPublicationRewardDocx(outputPath string, replacements map[string]string) error {
	if strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("output path is required")
	}
	if replacements == nil {
		return fmt.Errorf("replacement data is required")
	}

	templatePath := filepath.Join("templates", "publication_reward_template.docx")
	if _, err := os.Stat(templatePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("template file not found")
		}
		return fmt.Errorf("failed to access template: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to prepare output directory: %w", err)
	}

	if err := fillDocxTemplate(templatePath, outputPath, replacements); err != nil {
		return err
	}
	return nil
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
		FolderType:   "temp",
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

// REPLACE: MoveFileToSubmissionFolder ย้ายไฟล์จาก temp → submission folder และตั้งชื่อไฟล์ให้มีเลขคำร้อง
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

	// Generate base paths
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	userFolderPath, err := utils.CreateUserFolderIfNotExists(user, uploadPath)
	if err != nil {
		return err
	}

	submissionFolderPath, err := utils.CreateSubmissionFolder(
		userFolderPath, submissionType, submissionID, submission.SubmissionNumber, submission.CreatedAt)
	if err != nil {
		return err
	}

	// ===== ตั้งชื่อไฟล์ใหม่: <original-name>_<submission-number><ext>
	orig := fileUpload.OriginalName
	ext := filepath.Ext(orig)
	base := strings.TrimSuffix(orig, ext)

	desiredName := fmt.Sprintf("%s_%s%s", base, submission.SubmissionNumber, ext)

	// ให้ utils.GenerateUniqueFilename ช่วยกันชื่อซ้ำ (ส่ง desiredName เข้าไปให้เป็น "ต้นฉบับ")
	newFilename := utils.GenerateUniqueFilename(submissionFolderPath, desiredName)
	newPath := filepath.Join(submissionFolderPath, newFilename)

	// Move file on disk
	if err := utils.MoveFileToSubmissionFolder(fileUpload.StoredPath, newPath); err != nil {
		return err
	}

	// Update DB path (เก็บ OriginalName ตามเดิมไว้ เพื่อแสดงชื่อไฟล์เดิมใน UI ได้ถ้าต้องการ)
	fileUpload.StoredPath = newPath
	fileUpload.FolderType = "submission"
	fileUpload.UpdateAt = time.Now()
	return saveFileUploadRecord(config.DB, &fileUpload)
}

// AttachDocumentToSubmission แนบไฟล์กับ submission และย้ายไฟล์
func AttachDocumentToSubmission(c *gin.Context) {
	submissionID, _ := strconv.Atoi(c.Param("id"))
	userID, _ := c.Get("userID")

	type AttachDocumentRequest struct {
		FileID            int    `json:"file_id" binding:"required"`
		DocumentTypeID    int    `json:"document_type_id" binding:"required"`
		Description       string `json:"description"`
		DisplayOrder      int    `json:"display_order"`
		OriginalName      string `json:"original_name"`
		ExternalFundingID *int   `json:"external_funding_id"`
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

	originalName := strings.TrimSpace(req.OriginalName)
	if originalName == "" {
		originalName = fileUpload.OriginalName
	}

	// Create submission document record
	now := time.Now()
	document := models.SubmissionDocument{
		SubmissionID:   submissionID,
		FileID:         req.FileID,
		OriginalName:   originalName,
		DocumentTypeID: req.DocumentTypeID,
		Description:    req.Description,
		DisplayOrder:   req.DisplayOrder,
		CreatedAt:      now,
	}

	if err := createSubmissionDocumentRecord(config.DB, &document); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to attach document"})
		return
	}

	if err := resequenceSubmissionDocumentsByDocumentType(config.DB, submission.SubmissionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update document order"})
		return
	}

	if req.ExternalFundingID != nil && *req.ExternalFundingID > 0 {
		updates := map[string]interface{}{
			"document_id": document.DocumentID,
			"file_id":     document.FileID,
			"updated_at":  now,
		}
		if err := config.DB.Model(&models.PublicationRewardExternalFund{}).
			Where("external_fund_id = ? AND submission_id = ?", *req.ExternalFundingID, submission.SubmissionID).
			Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link external funding document"})
			return
		}
	}

	// Preload relations
	config.DB.Preload("File").Preload("DocumentType").First(&document, document.DocumentID)

	c.JSON(http.StatusOK, gin.H{
		"success":             true,
		"message":             "Document attached successfully",
		"document":            document,
		"external_funding_id": req.ExternalFundingID,
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

// REPLACE: DownloadFile serves file for download (filename includes submission number)
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

	// ===== ประกอบชื่อไฟล์แนบ: <original-name>_<submission-number><ext> (ถ้าหา submission ได้)
	downloadName := file.OriginalName
	ext := filepath.Ext(file.OriginalName)
	base := strings.TrimSuffix(file.OriginalName, ext)

	// หา submission_id ผ่านตาราง submission_documents (ไฟล์นี้ถูกแนบกับ submission ไหน)
	var doc models.SubmissionDocument
	if err := config.DB.
		Where("file_id = ?", file.FileID).
		Order("created_at ASC").
		First(&doc).Error; err == nil {

		var sub models.Submission
		if err := config.DB.
			Select("submission_id", "submission_number").
			Where("submission_id = ?", doc.SubmissionID).
			First(&sub).Error; err == nil && sub.SubmissionNumber != "" {
			downloadName = fmt.Sprintf("%s_%s%s", base, sub.SubmissionNumber, ext)
		}
	}

	// Serve file
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", downloadName))
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
		FileID            int    `json:"file_id" binding:"required"`
		DocumentTypeID    int    `json:"document_type_id" binding:"required"`
		Description       string `json:"description"`
		DisplayOrder      int    `json:"display_order"`
		OriginalName      string `json:"original_name"`
		ExternalFundingID *int   `json:"external_funding_id"`
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

	originalName := strings.TrimSpace(req.OriginalName)
	if originalName == "" {
		originalName = file.OriginalName
	}

	// Create submission document
	now := time.Now()
	submissionDoc := models.SubmissionDocument{
		SubmissionID:   submission.SubmissionID,
		FileID:         req.FileID,
		OriginalName:   originalName,
		DocumentTypeID: req.DocumentTypeID,
		Description:    req.Description,
		DisplayOrder:   req.DisplayOrder,
		IsRequired:     docType.Required,
		CreatedAt:      now,
	}

	if err := createSubmissionDocumentRecord(config.DB, &submissionDoc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to attach document"})
		return
	}

	if err := resequenceSubmissionDocumentsByDocumentType(config.DB, submission.SubmissionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update document order"})
		return
	}

	if req.ExternalFundingID != nil && *req.ExternalFundingID > 0 {
		updates := map[string]interface{}{
			"document_id": submissionDoc.DocumentID,
			"file_id":     submissionDoc.FileID,
			"updated_at":  now,
		}
		if err := config.DB.Model(&models.PublicationRewardExternalFund{}).
			Where("external_fund_id = ? AND submission_id = ?", *req.ExternalFundingID, submission.SubmissionID).
			Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link external funding document"})
			return
		}
	}

	// Load relations for response
	config.DB.Preload("File").Preload("DocumentType").First(&submissionDoc, submissionDoc.DocumentID)

	c.JSON(http.StatusOK, gin.H{
		"success":             true,
		"message":             "Document attached successfully",
		"document":            submissionDoc,
		"external_funding_id": req.ExternalFundingID,
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
	if roleID.(int) != 3 && roleID.(int) != 4 { // เดิมเช็คแค่ != 3
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Get documents
	var documents []models.SubmissionDocument
	if err := config.DB.Joins("LEFT JOIN document_types dt ON dt.document_type_id = submission_documents.document_type_id").
		Joins("LEFT JOIN publication_reward_external_funds pref ON pref.document_id = submission_documents.document_id AND (pref.deleted_at IS NULL OR pref.deleted_at = '0000-00-00 00:00:00')").
		Select("submission_documents.*, dt.document_type_name, pref.external_fund_id AS external_funding_id").
		Preload("File").
		Preload("DocumentType").
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

func AdminResequenceSubmissionDocuments(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var submission models.Submission
	if err := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID).First(&submission).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": "Submission not found"})
		return
	}

	if err := resequenceSubmissionDocumentsByDocumentType(config.DB, submission.SubmissionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reorder submission documents"})
		return
	}

	documents, err := fetchSubmissionDocuments(config.DB, submission.SubmissionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load submission documents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "Submission documents reordered successfully",
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

// ดึงปี พ.ศ. (string) จาก system_config.current_year ถ้ามี; ถ้าไม่มี fallback เป็น (ปีค.ศ.+543)
func getCurrentBEYearStr() string {
	type row struct {
		CurrentYear *string
	}
	var r row
	// current_year อาจเป็น string รูปแบบต่าง ๆ จึงสแกนเป็น *string
	_ = config.DB.Raw(`
		SELECT current_year
		FROM system_config
		ORDER BY config_id DESC
		LIMIT 1
	`).Scan(&r).Error

	if r.CurrentYear != nil {
		only := onlyDigits(*r.CurrentYear)
		// พยายามใช้ 4 หลักแรก เช่น "2568", "2568/2569", "ปี 2568" -> "2568"
		if len(only) >= 4 {
			return only[:4]
		}
	}

	// fallback: ปีปัจจุบันแบบ พ.ศ.
	return fmt.Sprintf("%04d", time.Now().Year()+543)
}

// คืนเฉพาะตัวเลขจากสตริง (กันเคส "2568/2569" หรือ "ปี 2568")
func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ===================== HELPER FUNCTIONS =====================

// Global mutex for submission number generation
var submissionNumberMutex sync.Mutex

// REPLACE: generateSubmissionNumber creates a unique submission number (prefix-BEYYYY-RUNNING)
// - ปีใช้ พ.ศ. จาก system_config.current_year (ถ้าไม่มีค่อย fallback เป็น ปีปัจจุบัน+543)
// - running number รีเซ็ต "เมื่อปี พ.ศ. เปลี่ยน" (นับรวมทั้งปี ไม่รีเซ็ตรายวัน)
func generateSubmissionNumber(submissionType string) string {
	submissionNumberMutex.Lock()
	defer submissionNumberMutex.Unlock()

	// ปี พ.ศ. จาก system_config (หรือ fallback)
	beYear := getCurrentBEYearStr()

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

	// นับจำนวน submission ภายใน "ปี พ.ศ. ปัจจุบัน" (ตามเลขใน submission_number)
	// ตัวอย่าง prefixYear = "PR-2568%" จะ match ทั้งปีนั้น ไม่ขึ้นกับวัน
	prefixYearLike := fmt.Sprintf("%s-%s%%", prefix, beYear)

	var count int64
	config.DB.Model(&models.Submission{}).
		Where("submission_type = ? AND submission_number LIKE ?", submissionType, prefixYearLike).
		Count(&count)

	// พยายามจองเลขลำดับ + ตรวจซ้ำ
	for i := int64(1); i <= 10; i++ {
		potentialNumber := fmt.Sprintf("%s-%s-%04d", prefix, beYear, count+i)

		var existing int64
		config.DB.Model(&models.Submission{}).
			Where("submission_number = ?", potentialNumber).
			Count(&existing)

		if existing == 0 {
			return potentialNumber
		}
	}

	// กรณีชนพร้อมกันหลายเธรด/หลายเครื่อง (โอกาสน้อย) ค่อย fallback เป็นสุ่ม
	bytes := make([]byte, 3)
	rand.Read(bytes)
	randomSuffix := strings.ToUpper(hex.EncodeToString(bytes))
	return fmt.Sprintf("%s-%s-R-%s", prefix, beYear, randomSuffix)
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
		PaperTitle      string  `json:"article_title"`
		JournalName     string  `json:"journal_name"`
		PublicationDate string  `json:"publication_date"` // "YYYY-MM-DD"
		PublicationType string  `json:"publication_type"`
		Quartile        string  `json:"journal_quartile"`
		ImpactFactor    float64 `json:"impact_factor"`
		DOI             string  `json:"doi"`
		URL             string  `json:"url"`
		PageNumbers     string  `json:"page_numbers"`
		VolumeIssue     string  `json:"volume_issue"`
		Indexing        string  `json:"indexing"`

		// === เงินรางวัลและการคำนวณ ===
		RewardAmount                float64 `json:"publication_reward"`
		RewardApproveAmount         float64 `json:"reward_approve_amount"`
		RevisionFee                 float64 `json:"revision_fee"`
		RevisionFeeApproveAmount    float64 `json:"revision_fee_approve_amount"`
		PublicationFee              float64 `json:"publication_fee"`
		PublicationFeeApproveAmount float64 `json:"publication_fee_approve_amount"`
		ExternalFundingAmount       float64 `json:"external_funding_amount"`
		TotalAmount                 float64 `json:"total_amount"`
		TotalApproveAmount          float64 `json:"total_approve_amount"`

		// === ข้อมูลผู้แต่ง ===
		AuthorCount    int    `json:"author_count"`
		AuthorType     string `json:"author_status"` // FE เดิมส่งเป็น author_status
		AuthorNameList string `json:"author_name_list"`
		Signature      string `json:"signature"`

		// === อื่นๆ ===
		AnnounceReferenceNumber string `json:"announce_reference_number"`

		// === ฟิลด์ใหม่จาก FE (ไม่บังคับใช้ ใช้เป็น fallback) ===
		MainAnnoucement    *int `json:"main_annoucement"`
		RewardAnnouncement *int `json:"reward_announcement"`

		// === ฟิลด์อื่น ===
		HasUniversityFunding string `json:"has_university_funding"` // "yes" | "no"
		FundingReferences    string `json:"funding_references"`
		UniversityRankings   string `json:"university_rankings"`

		ExternalFundings []struct {
			ExternalFundID *int    `json:"external_fund_id"`
			ClientID       string  `json:"client_id"`
			FundName       string  `json:"fund_name"`
			Amount         float64 `json:"amount"`
		} `json:"external_fundings"`
	}

	var req PublicationDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// ตรวจสอบ submission เป็นของ user นี้
	var submission models.Submission
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, userID).
		First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// แปลงวันตีพิมพ์
	pubDate, err := time.Parse("2006-01-02", req.PublicationDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid publication date format"})
		return
	}

	// --- ดึง "เลขประกาศ" ปัจจุบันจาก system_config (snapshot ณ เวลายื่น) ---
	var ann struct {
		MainAnnoucement    *int
		RewardAnnouncement *int
	}
	if err := config.DB.Raw(`
		SELECT main_annoucement, reward_announcement
		FROM system_config
		ORDER BY config_id DESC
		LIMIT 1
	`).Scan(&ann).Error; err != nil {
		// ถ้าดึงไม่ได้ ปล่อยให้ใช้ค่าจาก FE เป็น fallback
		ann.MainAnnoucement = req.MainAnnoucement
		ann.RewardAnnouncement = req.RewardAnnouncement
	}
	// ถ้าดึงได้แต่เป็น NULL ให้ fallbackไปใช้ที่ FE ส่งมา (ถ้ามี)
	if ann.MainAnnoucement == nil && req.MainAnnoucement != nil {
		ann.MainAnnoucement = req.MainAnnoucement
	}
	if ann.RewardAnnouncement == nil && req.RewardAnnouncement != nil {
		ann.RewardAnnouncement = req.RewardAnnouncement
	}

	now := time.Now()

	var externalTotal float64
	if len(req.ExternalFundings) > 0 {
		for _, fund := range req.ExternalFundings {
			externalTotal += fund.Amount
		}
	}

	authorNameList := strings.TrimSpace(req.AuthorNameList)
	signature := strings.TrimSpace(req.Signature)
	if authorNameList == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "author_name_list is required"})
		return
	}
	if signature == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "signature is required"})
		return
	}

	toNullableString := func(value string) *string {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil
		}
		return &trimmed
	}

	hasUniversityFunding := strings.TrimSpace(req.HasUniversityFunding)
	if hasUniversityFunding == "" {
		hasUniversityFunding = "no"
	}

	fundingReferences := toNullableString(req.FundingReferences)
	universityRankings := toNullableString(req.UniversityRankings)
	announceRef := strings.TrimSpace(req.AnnounceReferenceNumber)
	authorType := strings.TrimSpace(req.AuthorType)

	var existing models.PublicationRewardDetail
	if err := config.DB.Where("submission_id = ?", submission.SubmissionID).First(&existing).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch publication details"})
			return
		}
	}

	detail := existing
	if detail.DetailID == 0 {
		detail.SubmissionID = submission.SubmissionID
	}

	detail.SubmissionID = submission.SubmissionID
	detail.PaperTitle = req.PaperTitle
	detail.JournalName = req.JournalName
	detail.PublicationDate = pubDate
	detail.PublicationType = req.PublicationType
	detail.Quartile = req.Quartile
	detail.ImpactFactor = req.ImpactFactor
	detail.DOI = req.DOI
	detail.URL = req.URL
	detail.PageNumbers = req.PageNumbers
	detail.VolumeIssue = req.VolumeIssue
	detail.Indexing = req.Indexing

	detail.RewardAmount = req.RewardAmount
	detail.RewardApproveAmount = req.RewardApproveAmount
	detail.RevisionFee = req.RevisionFee
	detail.RevisionFeeApproveAmount = req.RevisionFeeApproveAmount
	detail.PublicationFee = req.PublicationFee
	detail.PublicationFeeApproveAmount = req.PublicationFeeApproveAmount
	if externalTotal > 0 {
		detail.ExternalFundingAmount = externalTotal
	} else {
		detail.ExternalFundingAmount = req.ExternalFundingAmount
	}
	detail.TotalAmount = req.TotalAmount
	detail.TotalApproveAmount = req.TotalApproveAmount

	detail.AuthorCount = req.AuthorCount
	detail.AuthorType = authorType
	detail.AuthorNameList = authorNameList
	detail.Signature = signature

	detail.AnnounceReferenceNumber = announceRef
	detail.MainAnnoucement = ann.MainAnnoucement
	detail.RewardAnnouncement = ann.RewardAnnouncement

	detail.HasUniversityFunding = hasUniversityFunding
	detail.FundingReferences = fundingReferences
	detail.UniversityRankings = universityRankings

	if detail.CreateAt.IsZero() {
		detail.CreateAt = now
	}
	detail.UpdateAt = now

	var saveErr error
	if detail.DetailID == 0 {
		saveErr = config.DB.Create(&detail).Error
	} else {
		saveErr = config.DB.Save(&detail).Error
	}

	if saveErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save publication details"})
		return
	}

	// Handle external funding breakdown records
	var savedExternalFunds []models.PublicationRewardExternalFund
	responseExternalFunds := make([]gin.H, 0, len(req.ExternalFundings))

	if len(req.ExternalFundings) > 0 {
		keepIDs := make([]int, 0, len(req.ExternalFundings))

		for _, fund := range req.ExternalFundings {
			trimmedName := strings.TrimSpace(fund.FundName)
			record := models.PublicationRewardExternalFund{
				DetailID:     detail.DetailID,
				SubmissionID: submission.SubmissionID,
				FundName:     trimmedName,
				Amount:       fund.Amount,
				UpdatedAt:    now,
			}

			if fund.ExternalFundID != nil && *fund.ExternalFundID > 0 {
				// Try to update existing record
				var existingFund models.PublicationRewardExternalFund
				if err := config.DB.Where("external_fund_id = ? AND detail_id = ?", *fund.ExternalFundID, detail.DetailID).
					First(&existingFund).Error; err != nil {
					if !errors.Is(err, gorm.ErrRecordNotFound) {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load external funding record"})
						return
					}
					record.CreatedAt = now
					if err := config.DB.Create(&record).Error; err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save external funding record"})
						return
					}
				} else {
					existingFund.FundName = trimmedName
					existingFund.Amount = fund.Amount
					existingFund.UpdatedAt = now
					if existingFund.SubmissionID == 0 {
						existingFund.SubmissionID = submission.SubmissionID
					}
					if err := config.DB.Save(&existingFund).Error; err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update external funding record"})
						return
					}
					record = existingFund
				}
			} else {
				record.CreatedAt = now
				if err := config.DB.Create(&record).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save external funding record"})
					return
				}
			}

			keepIDs = append(keepIDs, record.ExternalFundID)
			savedExternalFunds = append(savedExternalFunds, record)
			responseExternalFunds = append(responseExternalFunds, gin.H{
				"external_fund_id": record.ExternalFundID,
				"client_id":        fund.ClientID,
				"fund_name":        record.FundName,
				"amount":           record.Amount,
			})
		}

		// Remove stale records
		if len(keepIDs) > 0 {
			if err := config.DB.Where("detail_id = ? AND external_fund_id NOT IN ?", detail.DetailID, keepIDs).
				Delete(&models.PublicationRewardExternalFund{}).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove outdated external funding records"})
				return
			}
		}
	} else {
		// No external fundings provided, clear existing ones
		if err := config.DB.Where("detail_id = ?", detail.DetailID).
			Delete(&models.PublicationRewardExternalFund{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear external funding records"})
			return
		}
		detail.ExternalFundingAmount = 0
	}

	if len(savedExternalFunds) > 0 {
		var computedTotal float64
		for _, fund := range savedExternalFunds {
			computedTotal += fund.Amount
		}
		detail.ExternalFundingAmount = computedTotal
	}

	detail.ExternalFunds = savedExternalFunds

	if err := config.DB.Model(&models.PublicationRewardDetail{}).
		Where("detail_id = ?", detail.DetailID).
		Update("external_funding_amount", detail.ExternalFundingAmount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update external funding amount"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"message":           "Publication details saved successfully",
		"details":           detail,
		"external_fundings": responseExternalFunds,
	})
}

// AddFundDetails
func AddFundDetails(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")

	type FundDetailsRequest struct {
		ProjectTitle                string  `json:"project_title"`
		ProjectDescription          string  `json:"project_description"`
		RequestedAmount             float64 `json:"requested_amount"`
		SubcategoryID               int     `json:"subcategory_id"`
		MainAnnoucement             *int    `json:"main_annoucement"`
		ActivitySupportAnnouncement *int    `json:"activity_support_announcement"`
	}

	var req FundDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Resolve announcement snapshot at the time of submission.
	var ann struct {
		MainAnnoucement             *int
		ActivitySupportAnnouncement *int
	}
	if err := config.DB.Raw(`
                SELECT main_annoucement, activity_support_announcement
                FROM system_config
                ORDER BY config_id DESC
                LIMIT 1
        `).Scan(&ann).Error; err != nil {
		ann.MainAnnoucement = req.MainAnnoucement
		ann.ActivitySupportAnnouncement = req.ActivitySupportAnnouncement
	}

	if ann.MainAnnoucement == nil && req.MainAnnoucement != nil {
		ann.MainAnnoucement = req.MainAnnoucement
	}
	if ann.ActivitySupportAnnouncement == nil && req.ActivitySupportAnnouncement != nil {
		ann.ActivitySupportAnnouncement = req.ActivitySupportAnnouncement
	}

	// Validate submission exists and user has permission
	var submission models.Submission
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, userID).First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Fetch subcategory to determine its parent category
	var subcategory models.FundSubcategory
	if err := config.DB.Where("subcategory_id = ?", req.SubcategoryID).First(&subcategory).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_id"})
		return
	}

	// Find active budget for the selected subcategory
	var budget models.SubcategoryBudget
	if err := config.DB.Where("subcategory_id = ? AND status = 'active' AND delete_at IS NULL", req.SubcategoryID).First(&budget).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Active subcategory budget not found"})
		return
	}

	// Create fund application details
	fundDetails := models.FundApplicationDetail{
		SubmissionID:                submission.SubmissionID,
		SubcategoryID:               req.SubcategoryID,
		ProjectTitle:                req.ProjectTitle,
		ProjectDescription:          req.ProjectDescription,
		RequestedAmount:             req.RequestedAmount,
		MainAnnoucement:             ann.MainAnnoucement,
		ActivitySupportAnnouncement: ann.ActivitySupportAnnouncement,
	}

	if err := config.DB.Create(&fundDetails).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save fund details"})
		return
	}

	// Update submission with category, subcategory, and budget references
	if err := config.DB.Model(&submission).Updates(map[string]interface{}{
		"subcategory_id":        req.SubcategoryID,
		"category_id":           subcategory.CategoryID,
		"subcategory_budget_id": budget.SubcategoryBudgetID,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission category"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Fund details saved successfully",
		"details": fundDetails,
	})
}
