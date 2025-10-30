package controllers

import (
	"errors"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adminLegacySubmissionCore struct {
	SubmissionNumber          *string `json:"submission_number"`
	SubmissionType            string  `json:"submission_type" binding:"required"`
	UserID                    int     `json:"user_id" binding:"required"`
	YearID                    int     `json:"year_id" binding:"required"`
	StatusID                  int     `json:"status_id" binding:"required"`
	CategoryID                *int    `json:"category_id"`
	SubcategoryID             *int    `json:"subcategory_id"`
	SubcategoryBudgetID       *int    `json:"subcategory_budget_id"`
	ApprovedBy                *int    `json:"approved_by"`
	ApprovedAt                *string `json:"approved_at"`
	SubmittedAt               *string `json:"submitted_at"`
	InstallmentNumberAtSubmit *int    `json:"installment_number_at_submit"`
	AdminApprovedBy           *int    `json:"admin_approved_by"`
	AdminApprovedAt           *string `json:"admin_approved_at"`
	HeadApprovedBy            *int    `json:"head_approved_by"`
	HeadApprovedAt            *string `json:"head_approved_at"`
	HeadRejectedBy            *int    `json:"head_rejected_by"`
	HeadRejectedAt            *string `json:"head_rejected_at"`
	HeadRejectionReason       *string `json:"head_rejection_reason"`
	HeadComment               *string `json:"head_comment"`
	HeadSignature             *string `json:"head_signature"`
	AdminRejectedBy           *int    `json:"admin_rejected_by"`
	AdminRejectedAt           *string `json:"admin_rejected_at"`
	AdminRejectionReason      *string `json:"admin_rejection_reason"`
	AdminComment              *string `json:"admin_comment"`
	RejectedBy                *int    `json:"rejected_by"`
	RejectedAt                *string `json:"rejected_at"`
	RejectionReason           *string `json:"rejection_reason"`
	Comment                   *string `json:"comment"`
	ReviewedAt                *string `json:"reviewed_at"`
	ClosedAt                  *string `json:"closed_at"`
	CreatedAt                 *string `json:"created_at"`
	UpdatedAt                 *string `json:"updated_at"`
	DeletedAt                 *string `json:"deleted_at"`
}

type adminLegacyDocumentInput struct {
	DocumentID        *int    `json:"document_id"`
	FileID            int     `json:"file_id" binding:"required"`
	DocumentTypeID    int     `json:"document_type_id" binding:"required"`
	StoredPath        *string `json:"stored_path"`
	OriginalName      *string `json:"original_name"`
	Description       *string `json:"description"`
	DisplayOrder      *int    `json:"display_order"`
	IsRequired        *bool   `json:"is_required"`
	IsVerified        *bool   `json:"is_verified"`
	VerifiedBy        *int    `json:"verified_by"`
	VerifiedAt        *string `json:"verified_at"`
	ExternalFundingID *int    `json:"external_funding_id"`
	CreatedAt         *string `json:"created_at"`
}

type adminLegacyUserInput struct {
	ID           *int    `json:"id"`
	UserID       int     `json:"user_id" binding:"required"`
	Role         *string `json:"role"`
	IsPrimary    *bool   `json:"is_primary"`
	DisplayOrder *int    `json:"display_order"`
	CreatedAt    *string `json:"created_at"`
}

type adminLegacySubmissionPayload struct {
	Submission  adminLegacySubmissionCore  `json:"submission" binding:"required"`
	Documents   []adminLegacyDocumentInput `json:"documents"`
	Users       []adminLegacyUserInput     `json:"users"`
	ClearFields []string                   `json:"clear_fields"`
}

type adminLegacySubmissionRecord struct {
	Submission      models.Submission           `json:"submission"`
	Documents       []models.SubmissionDocument `json:"documents,omitempty"`
	SubmissionUsers []models.SubmissionUser     `json:"submission_users,omitempty"`
}

func AdminLegacyListSubmissions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit < 1 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}

	yearID := c.Query("year_id")
	submissionType := c.Query("submission_type")
	statusID := c.Query("status_id")
	userID := c.Query("user_id")
	categoryID := c.Query("category_id")
	search := c.Query("search")

	query := baseLegacySubmissionQuery(config.DB).
		Where("submissions.deleted_at IS NULL")

	if yearID != "" {
		query = query.Where("submissions.year_id = ?", yearID)
	}
	if submissionType != "" {
		query = query.Where("submissions.submission_type = ?", submissionType)
	}
	if statusID != "" {
		query = query.Where("submissions.status_id = ?", statusID)
	}
	if userID != "" {
		query = query.Where("submissions.user_id = ?", userID)
	}
	if categoryID != "" {
		query = query.Where("submissions.category_id = ?", categoryID)
	}
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("submissions.submission_number LIKE ?", like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count submissions"})
		return
	}

	var submissions []models.Submission
	if err := query.Order("submissions.created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	records := make([]adminLegacySubmissionRecord, len(submissions))
	for i := range submissions {
		records[i] = adminLegacySubmissionRecord{Submission: submissions[i]}
	}

	totalPages := (total + int64(limit) - 1) / int64(limit)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"items":   records,
		"pagination": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total_count":  total,
			"total_pages":  totalPages,
			"has_next":     page < int(totalPages),
			"has_prev":     page > 1,
		},
	})
}

func AdminLegacyGetSubmission(c *gin.Context) {
	idParam := c.Param("id")
	submissionID, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	record, err := loadLegacySubmissionRecord(config.DB, submissionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load submission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"item":    record,
	})
}

func AdminLegacyCreateSubmission(c *gin.Context) {
	var payload adminLegacySubmissionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	clearSet := buildClearFieldSet(payload.ClearFields)

	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		submission := models.Submission{}
		if err := applyLegacySubmissionFields(&submission, payload.Submission, true, clearSet); err != nil {
			return err
		}
		if err := validateLegacySubmission(tx, &submission); err != nil {
			return err
		}

		if err := tx.Create(&submission).Error; err != nil {
			return err
		}

		if payload.Documents != nil {
			if err := replaceSubmissionDocuments(tx, submission.SubmissionID, payload.Documents); err != nil {
				return err
			}
		}
		if payload.Users != nil {
			if err := replaceSubmissionUsers(tx, submission.SubmissionID, payload.Users); err != nil {
				return err
			}
		}

		record, err := loadLegacySubmissionRecord(tx, submission.SubmissionID)
		if err != nil {
			return err
		}

		c.JSON(http.StatusCreated, gin.H{
			"success": true,
			"item":    record,
		})
		return nil
	}); err != nil {
		if _, ok := err.(*legacyValidationError); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func AdminLegacyUpdateSubmission(c *gin.Context) {
	idParam := c.Param("id")
	submissionID, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var payload adminLegacySubmissionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	clearSet := buildClearFieldSet(payload.ClearFields)

	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		var submission models.Submission
		if err := baseLegacySubmissionQuery(tx).
			Where("submissions.submission_id = ?", submissionID).
			First(&submission).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			return err
		}

		if err := applyLegacySubmissionFields(&submission, payload.Submission, false, clearSet); err != nil {
			return err
		}
		if err := validateLegacySubmission(tx, &submission); err != nil {
			return err
		}

		if err := tx.Model(&models.Submission{}).
			Where("submission_id = ?", submission.SubmissionID).
			Save(&submission).Error; err != nil {
			return err
		}

		if payload.Documents != nil {
			if err := replaceSubmissionDocuments(tx, submission.SubmissionID, payload.Documents); err != nil {
				return err
			}
		}
		if payload.Users != nil {
			if err := replaceSubmissionUsers(tx, submission.SubmissionID, payload.Users); err != nil {
				return err
			}
		}

		record, err := loadLegacySubmissionRecord(tx, submission.SubmissionID)
		if err != nil {
			return err
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"item":    record,
		})
		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
			return
		}
		if _, ok := err.(*legacyValidationError); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func AdminLegacyDeleteSubmission(c *gin.Context) {
	idParam := c.Param("id")
	submissionID, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		cleanup := []interface{}{
			&models.SubmissionDocument{},
			&models.SubmissionUser{},
			&models.PublicationRewardExternalFund{},
			&models.PublicationRewardDetail{},
			&models.FundApplicationDetail{},
			&models.ResearchFundAdminEvent{},
		}

		for _, model := range cleanup {
			if err := tx.Unscoped().Where("submission_id = ?", submissionID).Delete(model).Error; err != nil {
				return err
			}
		}

		if err := tx.Unscoped().Where("submission_id = ?", submissionID).Delete(&models.Submission{}).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Submission removed",
	})
}

type legacyValidationError struct {
	msg string
}

func (e *legacyValidationError) Error() string {
	return e.msg
}

func newLegacyValidationError(message string) error {
	return &legacyValidationError{msg: message}
}

func baseLegacySubmissionQuery(db *gorm.DB) *gorm.DB {
	if db == nil {
		db = config.DB
	}
	return db.Model(&models.Submission{}).
		Joins("LEFT JOIN fund_categories ON fund_categories.category_id = submissions.category_id").
		Joins("LEFT JOIN fund_subcategories ON fund_subcategories.subcategory_id = submissions.subcategory_id AND fund_subcategories.category_id = submissions.category_id").
		Joins("LEFT JOIN publication_reward_details ON publication_reward_details.submission_id = submissions.submission_id").
		Select("submissions.*, fund_categories.category_name AS category_name, CASE WHEN fund_subcategories.subcategory_id IS NULL THEN NULL ELSE submissions.subcategory_id END AS subcategory_id, fund_subcategories.subcategory_name AS subcategory_name, publication_reward_details.journal_name AS publication_reward_journal_name").
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("user_id", "user_fname", "user_lname", "email")
		}).
		Preload("Year").
		Preload("Status").
		Preload("Category").
		Preload("Subcategory", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Category").Preload("SubcategoryBudget")
		})
}

func loadLegacySubmissionRecord(db *gorm.DB, submissionID int) (adminLegacySubmissionRecord, error) {
	record := adminLegacySubmissionRecord{}

	var submission models.Submission
	if err := baseLegacySubmissionQuery(db).
		Where("submissions.submission_id = ?", submissionID).
		First(&submission).Error; err != nil {
		return record, err
	}

	docs, err := fetchSubmissionDocuments(db, submission.SubmissionID)
	if err != nil {
		return record, err
	}
	for i := range docs {
		docs[i].Submission = models.Submission{}
	}

	var users []models.SubmissionUser
	if err := db.Preload("User", func(db *gorm.DB) *gorm.DB {
		return db.Select("user_id", "user_fname", "user_lname", "email")
	}).
		Where("submission_id = ?", submission.SubmissionID).
		Order("display_order ASC, id ASC").
		Find(&users).Error; err != nil {
		return record, err
	}
	for i := range users {
		users[i].Submission = models.Submission{}
	}

	record.Submission = submission
	record.Documents = docs
	record.SubmissionUsers = users
	return record, nil
}

func applyLegacySubmissionFields(submission *models.Submission, input adminLegacySubmissionCore, isCreate bool, clear map[string]bool) error {
	if submission == nil {
		return errors.New("submission is nil")
	}

	submissionType := strings.TrimSpace(input.SubmissionType)
	if submissionType == "" {
		return newLegacyValidationError("submission_type is required")
	}
	submission.SubmissionType = submissionType

	if input.SubmissionNumber != nil {
		candidate := strings.TrimSpace(*input.SubmissionNumber)
		if candidate == "" {
			if isCreate {
				submission.SubmissionNumber = generateSubmissionNumber(submissionType)
			}
		} else {
			submission.SubmissionNumber = candidate
		}
	} else if isCreate && submission.SubmissionNumber == "" {
		submission.SubmissionNumber = generateSubmissionNumber(submissionType)
	}

	submission.UserID = input.UserID
	submission.YearID = input.YearID
	submission.StatusID = input.StatusID

	assignOptionalInt(&submission.CategoryID, input.CategoryID, clear["category_id"])
	assignOptionalInt(&submission.SubcategoryID, input.SubcategoryID, clear["subcategory_id"])
	assignOptionalInt(&submission.SubcategoryBudgetID, input.SubcategoryBudgetID, clear["subcategory_budget_id"])
	assignOptionalInt(&submission.ApprovedBy, input.ApprovedBy, clear["approved_by"])
	assignOptionalInt(&submission.InstallmentNumberAtSubmit, input.InstallmentNumberAtSubmit, clear["installment_number_at_submit"])
	assignOptionalInt(&submission.AdminApprovedBy, input.AdminApprovedBy, clear["admin_approved_by"])
	assignOptionalInt(&submission.HeadApprovedBy, input.HeadApprovedBy, clear["head_approved_by"])
	assignOptionalInt(&submission.HeadRejectedBy, input.HeadRejectedBy, clear["head_rejected_by"])
	assignOptionalInt(&submission.AdminRejectedBy, input.AdminRejectedBy, clear["admin_rejected_by"])
	assignOptionalInt(&submission.RejectedBy, input.RejectedBy, clear["rejected_by"])

	if err := assignOptionalTime(&submission.ApprovedAt, input.ApprovedAt, clear["approved_at"], "approved_at"); err != nil {
		return err
	}
	if err := assignOptionalTime(&submission.SubmittedAt, input.SubmittedAt, clear["submitted_at"], "submitted_at"); err != nil {
		return err
	}
	if err := assignOptionalTime(&submission.AdminApprovedAt, input.AdminApprovedAt, clear["admin_approved_at"], "admin_approved_at"); err != nil {
		return err
	}
	if err := assignOptionalTime(&submission.HeadApprovedAt, input.HeadApprovedAt, clear["head_approved_at"], "head_approved_at"); err != nil {
		return err
	}
	if err := assignOptionalTime(&submission.HeadRejectedAt, input.HeadRejectedAt, clear["head_rejected_at"], "head_rejected_at"); err != nil {
		return err
	}
	if err := assignOptionalTime(&submission.AdminRejectedAt, input.AdminRejectedAt, clear["admin_rejected_at"], "admin_rejected_at"); err != nil {
		return err
	}
	if err := assignOptionalTime(&submission.RejectedAt, input.RejectedAt, clear["rejected_at"], "rejected_at"); err != nil {
		return err
	}
	if err := assignOptionalTime(&submission.ReviewedAt, input.ReviewedAt, clear["reviewed_at"], "reviewed_at"); err != nil {
		return err
	}
	if err := assignOptionalTime(&submission.ClosedAt, input.ClosedAt, clear["closed_at"], "closed_at"); err != nil {
		return err
	}
	if err := assignOptionalTime(&submission.DeletedAt, input.DeletedAt, clear["deleted_at"], "deleted_at"); err != nil {
		return err
	}

	assignOptionalString(&submission.HeadRejectionReason, input.HeadRejectionReason, clear["head_rejection_reason"])
	assignOptionalString(&submission.HeadComment, input.HeadComment, clear["head_comment"])
	assignOptionalString(&submission.HeadSignature, input.HeadSignature, clear["head_signature"])
	assignOptionalString(&submission.AdminRejectionReason, input.AdminRejectionReason, clear["admin_rejection_reason"])
	assignOptionalString(&submission.AdminComment, input.AdminComment, clear["admin_comment"])
	assignOptionalString(&submission.RejectionReason, input.RejectionReason, clear["rejection_reason"])
	assignOptionalString(&submission.Comment, input.Comment, clear["comment"])

	if input.CreatedAt != nil {
		if t, err := parseOptionalTime(input.CreatedAt); err != nil {
			return fmt.Errorf("created_at: %w", err)
		} else if t != nil {
			submission.CreatedAt = *t
		}
	} else if isCreate {
		submission.CreatedAt = time.Now()
	}

	if input.UpdatedAt != nil {
		if t, err := parseOptionalTime(input.UpdatedAt); err != nil {
			return fmt.Errorf("updated_at: %w", err)
		} else if t != nil {
			submission.UpdatedAt = *t
		}
	} else {
		submission.UpdatedAt = time.Now()
	}

	return nil
}

func validateLegacySubmission(db *gorm.DB, submission *models.Submission) error {
	if submission.UserID <= 0 {
		return newLegacyValidationError("user_id is required")
	}
	if err := ensureActiveUser(db, submission.UserID); err != nil {
		return newLegacyValidationError(err.Error())
	}

	var year models.Year
	if err := db.Where("year_id = ? AND delete_at IS NULL", submission.YearID).First(&year).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return newLegacyValidationError("invalid year_id")
		}
		return err
	}

	if _, err := utils.GetApplicationStatusByID(submission.StatusID); err != nil {
		return newLegacyValidationError(fmt.Sprintf("invalid status_id: %v", err))
	}

	if err := validateCategoryHierarchy(db, submission); err != nil {
		return err
	}

	userPointers := []*int{
		submission.ApprovedBy,
		submission.AdminApprovedBy,
		submission.HeadApprovedBy,
		submission.HeadRejectedBy,
		submission.AdminRejectedBy,
		submission.RejectedBy,
	}

	for _, pointer := range userPointers {
		if pointer != nil {
			if err := ensureActiveUser(db, *pointer); err != nil {
				return newLegacyValidationError(err.Error())
			}
		}
	}

	return nil
}

func validateCategoryHierarchy(db *gorm.DB, submission *models.Submission) error {
	if submission.CategoryID != nil {
		var count int64
		if err := db.Model(&models.FundCategory{}).
			Where("category_id = ? AND (delete_at IS NULL)", *submission.CategoryID).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return newLegacyValidationError("invalid category_id")
		}
	}

	if submission.SubcategoryID != nil {
		var subcategory models.FundSubcategory
		if err := db.Where("subcategory_id = ? AND (delete_at IS NULL)", *submission.SubcategoryID).
			First(&subcategory).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return newLegacyValidationError("invalid subcategory_id")
			}
			return err
		}
		if submission.CategoryID != nil && subcategory.CategoryID != *submission.CategoryID {
			return newLegacyValidationError("subcategory_id does not belong to the selected category")
		}
		if submission.CategoryID == nil {
			submission.CategoryID = &subcategory.CategoryID
		}
	} else if submission.SubcategoryBudgetID != nil {
		return newLegacyValidationError("subcategory_id is required when specifying subcategory_budget_id")
	}

	if submission.SubcategoryBudgetID != nil {
		var budget models.SubcategoryBudget
		if err := db.Where("subcategory_budget_id = ?", *submission.SubcategoryBudgetID).
			First(&budget).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return newLegacyValidationError("invalid subcategory_budget_id")
			}
			return err
		}
		if submission.SubcategoryID == nil || budget.SubcategoryID != *submission.SubcategoryID {
			return newLegacyValidationError("subcategory_budget_id does not match the selected subcategory")
		}
	}

	return nil
}

func ensureActiveUser(db *gorm.DB, userID int) error {
	var count int64
	if err := db.Model(&models.User{}).
		Where("user_id = ? AND (delete_at IS NULL)", userID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("user %d not found", userID)
	}
	return nil
}

func replaceSubmissionDocuments(db *gorm.DB, submissionID int, inputs []adminLegacyDocumentInput) error {
	if err := db.Unscoped().Where("submission_id = ?", submissionID).Delete(&models.SubmissionDocument{}).Error; err != nil {
		return err
	}
	if len(inputs) == 0 {
		return nil
	}

	for idx, input := range inputs {
		if input.FileID <= 0 {
			return newLegacyValidationError(fmt.Sprintf("documents[%d].file_id is required", idx))
		}
		if input.DocumentTypeID <= 0 {
			return newLegacyValidationError(fmt.Sprintf("documents[%d].document_type_id is required", idx))
		}

		var file models.FileUpload
		if err := db.Where("file_id = ? AND (delete_at IS NULL)", input.FileID).First(&file).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return newLegacyValidationError(fmt.Sprintf("documents[%d]: file not found", idx))
			}
			return err
		}

		if input.StoredPath != nil {
			trimmed := strings.TrimSpace(*input.StoredPath)
			if trimmed == "" {
				return newLegacyValidationError(fmt.Sprintf("documents[%d].stored_path is required", idx))
			}
			if trimmed != file.StoredPath {
				file.StoredPath = trimmed
				file.UpdateAt = time.Now()
				if err := saveFileUploadRecord(db, &file); err != nil {
					return fmt.Errorf("documents[%d]: failed to update file path: %w", idx, err)
				}
			}
		}

		var docType models.DocumentType
		if err := db.Where("document_type_id = ? AND (delete_at IS NULL)", input.DocumentTypeID).
			First(&docType).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return newLegacyValidationError(fmt.Sprintf("documents[%d]: invalid document_type_id", idx))
			}
			return err
		}

		doc := models.SubmissionDocument{
			SubmissionID:   submissionID,
			FileID:         input.FileID,
			DocumentTypeID: input.DocumentTypeID,
			Description:    "",
			DisplayOrder:   idx + 1,
			IsRequired:     docType.Required,
			IsVerified:     false,
			CreatedAt:      time.Now(),
		}

		if input.Description != nil {
			doc.Description = strings.TrimSpace(*input.Description)
		}
		if input.DisplayOrder != nil {
			doc.DisplayOrder = *input.DisplayOrder
		}
		if input.IsRequired != nil {
			doc.IsRequired = *input.IsRequired
		}
		if input.IsVerified != nil {
			doc.IsVerified = *input.IsVerified
		}
		if input.OriginalName != nil {
			trimmed := strings.TrimSpace(*input.OriginalName)
			if trimmed != "" {
				doc.OriginalName = trimmed
			} else {
				doc.OriginalName = file.OriginalName
			}
		} else {
			doc.OriginalName = file.OriginalName
		}

		if input.VerifiedBy != nil {
			if err := ensureActiveUser(db, *input.VerifiedBy); err != nil {
				return newLegacyValidationError(fmt.Sprintf("documents[%d]: %v", idx, err))
			}
			doc.VerifiedBy = input.VerifiedBy
		}
		if err := assignOptionalTime(&doc.VerifiedAt, input.VerifiedAt, false, fmt.Sprintf("documents[%d].verified_at", idx)); err != nil {
			return err
		}
		if input.ExternalFundingID != nil {
			doc.ExternalFundingID = input.ExternalFundingID
		}
		if input.CreatedAt != nil {
			t, err := parseOptionalTime(input.CreatedAt)
			if err != nil {
				return fmt.Errorf("documents[%d].created_at: %w", idx, err)
			}
			if t != nil {
				doc.CreatedAt = *t
			}
		}

		if err := createSubmissionDocumentRecord(db, &doc); err != nil {
			return err
		}
	}

	return resequenceSubmissionDocumentsByDocumentType(db, submissionID)
}

func replaceSubmissionUsers(db *gorm.DB, submissionID int, inputs []adminLegacyUserInput) error {
	if err := db.Unscoped().Where("submission_id = ?", submissionID).Delete(&models.SubmissionUser{}).Error; err != nil {
		return err
	}
	if len(inputs) == 0 {
		return nil
	}

	for idx, input := range inputs {
		if input.UserID <= 0 {
			return newLegacyValidationError(fmt.Sprintf("users[%d].user_id is required", idx))
		}
		if err := ensureActiveUser(db, input.UserID); err != nil {
			return newLegacyValidationError(fmt.Sprintf("users[%d]: %v", idx, err))
		}

		role := "coauthor"
		if input.Role != nil {
			role = mapFrontendRoleToDatabase(strings.TrimSpace(*input.Role))
		}
		isPrimary := false
		if input.IsPrimary != nil {
			isPrimary = *input.IsPrimary
		}
		displayOrder := idx + 1
		if input.DisplayOrder != nil && *input.DisplayOrder > 0 {
			displayOrder = *input.DisplayOrder
		}

		createdAt := time.Now()
		if input.CreatedAt != nil {
			t, err := parseOptionalTime(input.CreatedAt)
			if err != nil {
				return fmt.Errorf("users[%d].created_at: %w", idx, err)
			}
			if t != nil {
				createdAt = *t
			}
		}

		record := models.SubmissionUser{
			SubmissionID: submissionID,
			UserID:       input.UserID,
			Role:         role,
			IsPrimary:    isPrimary,
			DisplayOrder: displayOrder,
			CreatedAt:    createdAt,
		}

		if err := db.Create(&record).Error; err != nil {
			return err
		}
	}

	return nil
}

func parseOptionalTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, trimmed); err == nil {
			return &t, nil
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("invalid datetime format %q", trimmed)
}

func assignOptionalInt(target **int, source *int, clear bool) {
	if target == nil {
		return
	}
	if clear {
		*target = nil
	}
	if source != nil {
		val := *source
		*target = &val
	}
}

func assignOptionalString(target **string, source *string, clear bool) {
	if target == nil {
		return
	}
	if clear {
		*target = nil
	}
	if source != nil {
		trimmed := strings.TrimSpace(*source)
		if trimmed == "" {
			*target = nil
		} else {
			val := trimmed
			*target = &val
		}
	}
}

func assignOptionalTime(target **time.Time, source *string, clear bool, field string) error {
	if target == nil {
		return nil
	}
	if clear {
		*target = nil
	}
	if source == nil {
		return nil
	}
	t, err := parseOptionalTime(source)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	*target = t
	return nil
}

func buildClearFieldSet(fields []string) map[string]bool {
	set := make(map[string]bool, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(strings.ToLower(field))
		if trimmed != "" {
			set[trimmed] = true
		}
	}
	return set
}
