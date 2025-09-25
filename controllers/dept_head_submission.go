package controllers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetDeptHeadSubmissions(c *gin.Context) {
	statusCode := c.DefaultQuery("status_code", utils.StatusCodeDeptHeadPending)

	status, err := utils.GetApplicationStatusByCode(statusCode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	var submissions []models.Submission
	query := config.DB.Preload("Status").
		Preload("User").
		Preload("Category").
		Preload("Subcategory").
		Preload("SubmissionUsers.User").
		Where("submissions.deleted_at IS NULL").
		Where("submissions.status_id = ?", status.ApplicationStatusID)

	if err := query.Order("COALESCE(submissions.submitted_at, submissions.created_at) DESC").Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to fetch submissions",
		})
		return
	}

	responseItems := make([]gin.H, 0, len(submissions))
	for _, submission := range submissions {
		applicantUser := extractApplicantUser(&submission)
		applicantName := formatUserFullName(applicantUser)
		applicantPayload := gin.H{}
		if applicantUser != nil {
			applicantPayload = gin.H{
				"user_id":    applicantUser.UserID,
				"user_fname": applicantUser.UserFname,
				"user_lname": applicantUser.UserLname,
				"email":      applicantUser.Email,
			}
		}
		responseItems = append(responseItems, gin.H{
			"submission_id":     submission.SubmissionID,
			"submission_number": submission.SubmissionNumber,
			"submission_type":   submission.SubmissionType,
			"status_id":         submission.StatusID,
			"status": gin.H{
				"application_status_id": submission.Status.ApplicationStatusID,
				"status_code":           submission.Status.StatusCode,
				"status_name":           submission.Status.StatusName,
			},
			"category_id":      submission.CategoryID,
			"category_name":    submission.CategoryName,
			"subcategory_id":   submission.SubcategoryID,
			"subcategory_name": submission.SubcategoryName,
			"submitted_at":     submission.SubmittedAt,
			"created_at":       submission.CreatedAt,
			"applicant":        applicantPayload,
			"applicant_name":   applicantName,
			"user":             submission.User,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"filters": gin.H{
			"status_code": statusCode,
			"status": gin.H{
				"application_status_id": status.ApplicationStatusID,
				"status_code":           status.StatusCode,
				"status_name":           status.StatusName,
			},
		},
		"submissions": responseItems,
	})
}

func GetDeptHeadSubmissionDetails(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	payload, err := buildSubmissionDetailPayload(submissionID)
	if err != nil {
		status := http.StatusInternalServerError
		if err == gorm.ErrRecordNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, payload)
}

func DeptHeadRecommendSubmission(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var req struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !strings.Contains(err.Error(), "EOF") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	userIDVal, _ := c.Get("userID")
	userID := userIDVal.(int)

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	var submission models.Submission
	if err := tx.Preload("Status").First(&submission, submissionID).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load submission"})
		}
		return
	}

	allowed, err := utils.StatusMatchesCodes(submission.StatusID, utils.StatusCodeDeptHeadPending)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify submission status"})
		return
	}
	if !allowed {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission is not awaiting department review"})
		return
	}

	targetStatus, err := utils.GetApplicationStatusByCode(utils.StatusCodePending)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve target status"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status_id":        targetStatus.ApplicationStatusID,
		"updated_at":       now,
		"reviewed_at":      now,
		"head_approved_at": now,
		"head_approved_by": userID,
	}

	comment := strings.TrimSpace(req.Comment)
	if comment != "" {
		updates["comment"] = comment
	} else {
		updates["comment"] = gorm.Expr("NULL")
	}

	if err := tx.Model(&models.Submission{}).
		Where("submission_id = ?", submissionID).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	logDescription := comment
	if logDescription == "" {
		logDescription = "Department head recommended submission"
	}
	auditLog := models.AuditLog{
		UserID:       userID,
		Action:       "review",
		EntityType:   "submission",
		EntityID:     &submission.SubmissionID,
		EntityNumber: &submission.SubmissionNumber,
		Description:  &logDescription,
		IPAddress:    c.ClientIP(),
		CreatedAt:    now,
	}
	if err := tx.Create(&auditLog).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record audit log"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save recommendation"})
		return
	}

	payload, err := buildSubmissionDetailPayload(submissionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	payload["success"] = true
	c.JSON(http.StatusOK, payload)
}

func DeptHeadRejectSubmission(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var req struct {
		RejectionReason string `json:"rejection_reason" binding:"required"`
		Comment         string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rejection reason is required"})
		return
	}

	reason := strings.TrimSpace(req.RejectionReason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rejection reason is required"})
		return
	}
	comment := strings.TrimSpace(req.Comment)
	if comment == "" {
		comment = reason
	}

	userIDVal, _ := c.Get("userID")
	userID := userIDVal.(int)

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	var submission models.Submission
	if err := tx.Preload("Status").First(&submission, submissionID).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load submission"})
		}
		return
	}

	allowed, err := utils.StatusMatchesCodes(submission.StatusID, utils.StatusCodeDeptHeadPending)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify submission status"})
		return
	}
	if !allowed {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission is not awaiting department review"})
		return
	}

	rejectStatus, err := utils.GetApplicationStatusByCode(utils.StatusCodeRejected)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve rejection status"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status_id":        rejectStatus.ApplicationStatusID,
		"updated_at":       now,
		"reviewed_at":      now,
		"head_approved_at": now,
		"head_approved_by": userID,
		"closed_at":        now,
		"comment":          comment,
	}

	if err := tx.Model(&models.Submission{}).
		Where("submission_id = ?", submissionID).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	switch submission.SubmissionType {
	case "publication_reward":
		detailUpdates := map[string]interface{}{
			"rejection_reason": reason,
			"rejected_by":      userID,
			"rejected_at":      now,
			"update_at":        now,
		}
		if err := tx.Model(&models.PublicationRewardDetail{}).
			Where("submission_id = ?", submissionID).
			Updates(detailUpdates).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record rejection details"})
			return
		}
	case "fund_application":
		detailUpdates := map[string]interface{}{
			"comment":     comment,
			"rejected_by": userID,
			"rejected_at": now,
			"closed_at":   now,
		}
		if err := tx.Model(&models.FundApplicationDetail{}).
			Where("submission_id = ?", submissionID).
			Updates(detailUpdates).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record rejection details"})
			return
		}
	}

	desc := reason
	auditLog := models.AuditLog{
		UserID:       userID,
		Action:       "reject",
		EntityType:   "submission",
		EntityID:     &submission.SubmissionID,
		EntityNumber: &submission.SubmissionNumber,
		Description:  &desc,
		IPAddress:    c.ClientIP(),
		CreatedAt:    now,
	}
	if err := tx.Create(&auditLog).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record audit log"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rejection"})
		return
	}

	payload, err := buildSubmissionDetailPayload(submissionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	payload["success"] = true
	c.JSON(http.StatusOK, payload)
}

func buildSubmissionDetailPayload(submissionID int) (gin.H, error) {
	var submission models.Submission
	query := config.DB.Preload("User").
		Preload("Year").
		Preload("Status").
		Preload("FundApplicationDetail.Subcategory.Category").
		Preload("PublicationRewardDetail")

	if err := query.First(&submission, submissionID).Error; err != nil {
		return nil, err
	}

	var submissionUsers []models.SubmissionUser
	if err := config.DB.Where("submission_id = ?", submissionID).
		Preload("User").
		Order("display_order ASC").
		Find(&submissionUsers).Error; err != nil {
		submissionUsers = []models.SubmissionUser{}
	}

	var documents []models.SubmissionDocument
	config.DB.Where("submission_id = ?", submissionID).
		Preload("DocumentType").
		Preload("File").
		Find(&documents)

	response := gin.H{
		"submission": gin.H{
			"submission_id":         submission.SubmissionID,
			"submission_number":     submission.SubmissionNumber,
			"submission_type":       submission.SubmissionType,
			"user_id":               submission.UserID,
			"year_id":               submission.YearID,
			"category_id":           submission.CategoryID,
			"subcategory_id":        submission.SubcategoryID,
			"subcategory_budget_id": submission.SubcategoryBudgetID,
			"status_id":             submission.StatusID,
			"submitted_at":          submission.SubmittedAt,
			"created_at":            submission.CreatedAt,
			"updated_at":            submission.UpdatedAt,
			"user":                  submission.User,
			"year":                  submission.Year,
			"status":                submission.Status,
		},
		"details":          nil,
		"submission_users": []gin.H{},
		"documents":        []gin.H{},
	}

	if submission.SubmissionType == "publication_reward" && submission.PublicationRewardDetail != nil {
		response["details"] = gin.H{
			"type": "publication_reward",
			"data": submission.PublicationRewardDetail,
		}
	} else if submission.SubmissionType == "fund_application" && submission.FundApplicationDetail != nil {
		response["details"] = gin.H{
			"type": "fund_application",
			"data": submission.FundApplicationDetail,
		}
	}

	for _, su := range submissionUsers {
		if su.User == nil {
			var user models.User
			if err := config.DB.Where("user_id = ?", su.UserID).First(&user).Error; err == nil {
				su.User = &user
			} else {
				continue
			}
		}
		response["submission_users"] = append(response["submission_users"].([]gin.H), gin.H{
			"user_id":       su.UserID,
			"role":          su.Role,
			"display_order": su.DisplayOrder,
			"is_primary":    su.IsPrimary,
			"created_at":    su.CreatedAt,
			"user": gin.H{
				"user_id":    su.User.UserID,
				"user_fname": su.User.UserFname,
				"user_lname": su.User.UserLname,
				"email":      su.User.Email,
			},
		})
	}

	for _, doc := range documents {
		docInfo := gin.H{
			"document_id":      doc.DocumentID,
			"submission_id":    doc.SubmissionID,
			"file_id":          doc.FileID,
			"document_type_id": doc.DocumentTypeID,
			"description":      doc.Description,
			"display_order":    doc.DisplayOrder,
			"is_required":      doc.IsRequired,
			"created_at":       doc.CreatedAt,
		}
		if doc.DocumentType.DocumentTypeID != 0 {
			docInfo["document_type"] = gin.H{
				"document_type_id":   doc.DocumentType.DocumentTypeID,
				"document_type_name": doc.DocumentType.DocumentTypeName,
			}
		}
		if doc.File.FileID != 0 {
			docInfo["file"] = gin.H{
				"file_id":       doc.File.FileID,
				"original_name": doc.File.OriginalName,
				"file_size":     doc.File.FileSize,
				"mime_type":     doc.File.MimeType,
				"uploaded_at":   doc.File.UploadedAt,
			}
		}
		response["documents"] = append(response["documents"].([]gin.H), docInfo)
	}

	return response, nil
}

func extractApplicantUser(submission *models.Submission) *models.User {
	if submission.User != nil {
		return submission.User
	}

	for _, su := range submission.SubmissionUsers {
		if su.IsPrimary || strings.EqualFold(su.Role, "owner") {
			if su.User != nil {
				return su.User
			}
		}
	}

	return nil
}

func formatUserFullName(user *models.User) string {
	if user == nil {
		return ""
	}
	first := strings.TrimSpace(user.UserFname)
	last := strings.TrimSpace(user.UserLname)
	full := strings.TrimSpace(strings.TrimSpace(first + " " + last))
	if full != "" {
		return full
	}
	return strings.TrimSpace(user.Email)
}
