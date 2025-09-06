// controllers/admin_submission.go
package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ==============================
// Keep: GetSubmissionDetails (as in your latest file)
// ==============================

// GetSubmissionDetails - ดึงข้อมูล submission แบบละเอียด
func GetSubmissionDetails(c *gin.Context) {
	submissionIDStr := c.Param("id")

	// Validate submissionID
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var submission models.Submission

	// Query หลักพร้อม preload associations
	query := config.DB.
		Preload("User").
		Preload("Year").
		Preload("Status").
		Preload("FundApplicationDetail").
		Preload("FundApplicationDetail.Subcategory").
		Preload("FundApplicationDetail.Subcategory.Category").
		Preload("PublicationRewardDetail")

	// ดึงข้อมูล submission
	if err := query.First(&submission, submissionID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// ดึง submission users (co-authors)
	var submissionUsers []models.SubmissionUser
	if err := config.DB.Where("submission_id = ?", submissionID).
		Preload("User").
		Order("display_order ASC").
		Find(&submissionUsers).Error; err != nil {
		submissionUsers = []models.SubmissionUser{}
	}

	// ดึง documents
	var documents []models.SubmissionDocument
	config.DB.Where("submission_id = ?", submissionID).
		Preload("DocumentType").
		Preload("File").
		Find(&documents)

	// สร้าง response structure
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

	// เพิ่มรายละเอียดตาม submission type
	if submission.SubmissionType == "publication_reward" && submission.PublicationRewardDetail != nil {
		if submission.StatusID != 2 {
			submission.PublicationRewardDetail.AnnounceReferenceNumber = ""
		}
		response["details"] = gin.H{
			"type": "publication_reward",
			"data": submission.PublicationRewardDetail,
		}
	} else if submission.SubmissionType == "fund_application" && submission.FundApplicationDetail != nil {
		if submission.StatusID != 2 {
			submission.FundApplicationDetail.AnnounceReferenceNumber = ""
		}
		response["details"] = gin.H{
			"type": "fund_application",
			"data": submission.FundApplicationDetail,
		}
	}

	// Format submission users (with nil check)
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

	// Format documents
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
				"required":           doc.DocumentType.Required,
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

	c.JSON(http.StatusOK, response)
}

// ==============================
// NEW: PATCH approval amounts (publication_reward only)
// ==============================

// UpdatePublicationRewardApprovalAmounts updates *_approve_amount fields for a publication_reward submission
func UpdatePublicationRewardApprovalAmounts(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	type AmountsReq struct {
		RewardApproveAmount         *float64 `json:"reward_approve_amount" binding:"required"`
		RevisionFeeApproveAmount    *float64 `json:"revision_fee_approve_amount" binding:"required"`
		PublicationFeeApproveAmount *float64 `json:"publication_fee_approve_amount" binding:"required"`
		TotalApproveAmount          *float64 `json:"total_approve_amount" binding:"required"`
	}

	var req AmountsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	// Load submission + detail
	var submission models.Submission
	if err := config.DB.Preload("PublicationRewardDetail").First(&submission, submissionID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}
	if submission.SubmissionType != "publication_reward" || submission.PublicationRewardDetail == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This endpoint only supports publication_reward submissions"})
		return
	}

	// Validate non-negative
	if *req.RewardApproveAmount < 0 || *req.RevisionFeeApproveAmount < 0 || *req.PublicationFeeApproveAmount < 0 || *req.TotalApproveAmount < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amounts must be non-negative"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"reward_approve_amount":          *req.RewardApproveAmount,
		"revision_fee_approve_amount":    *req.RevisionFeeApproveAmount,
		"publication_fee_approve_amount": *req.PublicationFeeApproveAmount,
		"total_approve_amount":           *req.TotalApproveAmount,
		"update_at":                      now,
	}

	if err := config.DB.Model(&models.PublicationRewardDetail{}).
		Where("submission_id = ?", submissionID).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update approval amounts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Approval amounts updated",
		"amounts": req,
	})
}

// ==============================
// REPLACED: ApproveSubmission (single source of truth)
// ==============================

// ApproveSubmission - อนุมัติ submission พร้อมระบุจำนวนเงิน และบันทึกเลขประกาศ
func ApproveSubmission(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}
	userID, _ := c.Get("userID")

	// Accept both legacy and new payloads
	var req struct {
		// New granular fields (admin page)
		RewardApproveAmount         *float64 `json:"reward_approve_amount"`
		RevisionFeeApproveAmount    *float64 `json:"revision_fee_approve_amount"`
		PublicationFeeApproveAmount *float64 `json:"publication_fee_approve_amount"`
		TotalApproveAmount          *float64 `json:"total_approve_amount"`
		AnnounceReferenceNumber     string   `json:"announce_reference_number"`

		// Legacy fields (keep compatibility)
		ApprovedAmount  *float64 `json:"approved_amount"`
		ApprovalComment string   `json:"approval_comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Load submission with publication detail
	var submission models.Submission
	if err := tx.Preload("PublicationRewardDetail").
		Where("submission_id = ? AND deleted_at IS NULL", submissionID).
		First(&submission).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Validate status (1=pending, 4=revision requested)
	if submission.StatusID != 1 && submission.StatusID != 4 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only pending or revision-requested submissions can be approved"})
		return
	}

	// Update submission status → approved
	now := time.Now()
	submission.StatusID = 2
	submission.UpdatedAt = now
	if uid, ok := userID.(int); ok {
		submission.ApprovedBy = &uid
	}
	submission.ApprovedAt = &now
	if err := tx.Save(&submission).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission status"})
		return
	}

	// Update detail for publication_reward
	if submission.SubmissionType == "publication_reward" {
		var detail models.PublicationRewardDetail
		if submission.PublicationRewardDetail != nil {
			detail = *submission.PublicationRewardDetail
		} else {
			// try to find; or prepare new
			if err := tx.Where("submission_id = ?", submissionID).First(&detail).Error; err != nil {
				detail.SubmissionID = submissionID
			}
		}

		// Apply amounts if provided
		if req.RewardApproveAmount != nil {
			detail.RewardApproveAmount = *req.RewardApproveAmount
		}
		if req.RevisionFeeApproveAmount != nil {
			detail.RevisionFeeApproveAmount = *req.RevisionFeeApproveAmount
		}
		if req.PublicationFeeApproveAmount != nil {
			detail.PublicationFeeApproveAmount = *req.PublicationFeeApproveAmount
		}

		// Total priority: explicit total → legacy approved_amount → sum of parts
		if req.TotalApproveAmount != nil {
			detail.TotalApproveAmount = *req.TotalApproveAmount
		} else if req.ApprovedAmount != nil {
			detail.TotalApproveAmount = *req.ApprovedAmount
		} else {
			detail.TotalApproveAmount = detail.RewardApproveAmount +
				detail.RevisionFeeApproveAmount +
				detail.PublicationFeeApproveAmount
		}

		// Save announce reference number
		detail.AnnounceReferenceNumber = strings.TrimSpace(req.AnnounceReferenceNumber)

		if detail.DetailID == 0 {
			if err := tx.Create(&detail).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create detail"})
				return
			}
		} else {
			if err := tx.Save(&detail).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update detail"})
				return
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction failed"})
		return
	}

	// Return latest (with detail)
	var out models.Submission
	if err := config.DB.Preload("PublicationRewardDetail").
		Where("submission_id = ?", submissionID).
		First(&out).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Submission approved"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Submission approved successfully",
		"submission": out,
		"details": gin.H{
			"type": "publication_reward",
			"data": out.PublicationRewardDetail,
		},
	})
}

// ==============================
// REPLACED: RejectSubmission (single source of truth)
// ==============================

// RejectSubmission - ปฏิเสธ submission พร้อมเหตุผล
func RejectSubmission(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}
	userID, _ := c.Get("userID")

	var req struct {
		RejectionReason string `json:"rejection_reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.RejectionReason) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rejection reason is required"})
		return
	}

	tx := config.DB.Begin()

	var submission models.Submission
	if err := tx.First(&submission, submissionID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Only pending (1) or revision-requested (4) can be rejected
	if submission.StatusID != 1 && submission.StatusID != 4 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only pending or revision-requested submissions can be rejected"})
		return
	}

	now := time.Now()
	submission.StatusID = 3
	submission.UpdatedAt = now
	if err := tx.Save(&submission).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission status"})
		return
	}

	// Save rejection info
	switch submission.SubmissionType {
	case "publication_reward":
		if err := tx.Model(&models.PublicationRewardDetail{}).
			Where("submission_id = ?", submissionID).
			Updates(map[string]interface{}{
				"rejection_reason": req.RejectionReason,
				"rejected_by":      userID,
				"rejected_at":      now,
				"update_at":        now,
			}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rejection info"})
			return
		}
	case "fund_application":
		var detail models.FundApplicationDetail
		if err := tx.Where("submission_id = ?", submissionID).First(&detail).Error; err == nil {
			detail.Comment = req.RejectionReason
			rejectedByID := userID.(int)
			detail.RejectedBy = &rejectedByID
			detail.RejectedAt = &now
			detail.ClosedAt = &now
			if err := tx.Save(&detail).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rejection info"})
				return
			}
		}
	}

	// Audit log
	desc := req.RejectionReason
	auditLog := models.AuditLog{
		UserID:       userID.(int),
		Action:       "reject",
		EntityType:   "submission",
		EntityID:     &submission.SubmissionID,
		EntityNumber: &submission.SubmissionNumber,
		Description:  &desc,
		IPAddress:    c.ClientIP(),
		CreatedAt:    now,
	}
	tx.Create(&auditLog)

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject submission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Submission rejected successfully",
		"submission": gin.H{
			"submission_id":     submission.SubmissionID,
			"submission_number": submission.SubmissionNumber,
			"status_id":         submission.StatusID,
		},
	})
}
