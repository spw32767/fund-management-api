// controllers/admin_submission.go
package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"log"
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
// controllers/admin_submission.go

// GET /admin/submissions/:id/details
func GetSubmissionDetails(c *gin.Context) {
	idStr := c.Param("id")
	sid, err := strconv.Atoi(idStr)
	if err != nil || sid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission id"})
		return
	}

	var s models.Submission

	// 1) โหลด submission + ความสัมพันธ์หลักแบบกันพลาด
	q := config.DB.
		Preload("User"). // เจ้าของคำร้อง
		Preload("Year").
		Preload("Status").
		Preload("Category").
		Preload("Subcategory").
		Preload("Subcategory.SubcategoryBudget"). // เผื่อ UI ต้องใช้
		Preload("FundApplicationDetail").         // FA detail
		Preload("FundApplicationDetail.Subcategory").
		Preload("FundApplicationDetail.Subcategory.Category").
		Preload("PublicationRewardDetail") // PR detail

	if err := q.First(&s, "submission_id = ?", sid).Error; err != nil {
		log.Printf("[GetSubmissionDetails] find submission %v error: %v", sid, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}

	// 2) fallback: ถ้า User ไม่ถูก preload แต่มี user_id ให้ดึงซ้ำแบบตรงๆ
	if s.UserID > 0 && (s.User == nil || s.User.UserID == 0) {
		var owner models.User
		if err := config.DB.First(&owner, "user_id = ?", s.UserID).Error; err == nil {
			s.User = &owner
		}
	}

	// 3) co-authors (submission_users)
	var su []models.SubmissionUser
	if err := config.DB.
		Where("submission_id = ?", sid).
		Preload("User").
		Order("display_order ASC").
		Find(&su).Error; err != nil {
		log.Printf("[GetSubmissionDetails] load submission_users error: %v", err)
	}

	// 4) เอกสารแนบ
	var docs []models.SubmissionDocument
	if err := config.DB.
		Where("submission_id = ?", sid).
		Preload("DocumentType").
		Preload("File").
		Find(&docs).Error; err != nil {
		log.Printf("[GetSubmissionDetails] load documents error: %v", err)
	}

	// 5) จัด details ตามประเภท (กัน nil)
	var details gin.H
	switch s.SubmissionType {
	case "publication_reward":
		if s.PublicationRewardDetail != nil {
			details = gin.H{"type": "publication_reward", "data": s.PublicationRewardDetail}
		}
	case "fund_application":
		if s.FundApplicationDetail != nil {
			details = gin.H{"type": "fund_application", "data": s.FundApplicationDetail}
		}
	default:
		// เผื่อมีเคส type ว่าง แต่มี detail ฝั่งใดฝั่งหนึ่ง
		if s.FundApplicationDetail != nil {
			details = gin.H{"type": "fund_application", "data": s.FundApplicationDetail}
		} else if s.PublicationRewardDetail != nil {
			details = gin.H{"type": "publication_reward", "data": s.PublicationRewardDetail}
		}
	}

	// 6) map co-authors
	suOut := make([]gin.H, 0, len(su))
	for _, x := range su {
		var u map[string]any
		if x.User != nil && x.User.UserID > 0 {
			u = map[string]any{
				"user_id":    x.User.UserID,
				"user_fname": x.User.UserFname,
				"user_lname": x.User.UserLname,
				"email":      x.User.Email,
			}
		}
		suOut = append(suOut, gin.H{
			"user_id":       x.UserID,
			"role":          x.Role,
			"display_order": x.DisplayOrder,
			"is_primary":    x.IsPrimary,
			"created_at":    x.CreatedAt,
			"user":          u,
		})
	}

	// 7) map documents
	docOut := make([]gin.H, 0, len(docs))
	for _, d := range docs {
		item := gin.H{
			"document_id":      d.DocumentID,
			"submission_id":    d.SubmissionID,
			"file_id":          d.FileID,
			"document_type_id": d.DocumentTypeID,
			"description":      d.Description,
			"display_order":    d.DisplayOrder,
			"is_required":      d.IsRequired,
			"created_at":       d.CreatedAt,
		}
		if d.DocumentType.DocumentTypeID != 0 {
			item["document_type"] = gin.H{
				"document_type_id":   d.DocumentType.DocumentTypeID,
				"document_type_name": d.DocumentType.DocumentTypeName,
				"required":           d.DocumentType.Required,
			}
		}
		if d.File.FileID != 0 {
			item["file"] = gin.H{
				"file_id":       d.File.FileID,
				"original_name": d.File.OriginalName,
				"file_size":     d.File.FileSize,
				"mime_type":     d.File.MimeType,
				"uploaded_at":   d.File.UploadedAt,
			}
		}
		docOut = append(docOut, item)
	}

	// 8) applicant alias (ให้ FE ใช้ง่าย)
	var applicant map[string]any
	if s.User != nil && s.User.UserID > 0 {
		applicant = map[string]any{
			"user_id":    s.User.UserID,
			"user_fname": s.User.UserFname,
			"user_lname": s.User.UserLname,
			"email":      s.User.Email,
		}
	}

	// 9) response
	resp := gin.H{
		"submission": gin.H{
			"submission_id":         s.SubmissionID,
			"submission_number":     s.SubmissionNumber,
			"submission_type":       s.SubmissionType,
			"user_id":               s.UserID,
			"year_id":               s.YearID,
			"category_id":           s.CategoryID,
			"subcategory_id":        s.SubcategoryID,
			"subcategory_budget_id": s.SubcategoryBudgetID,
			"status_id":             s.StatusID,
			"submitted_at":          s.SubmittedAt,
			"created_at":            s.CreatedAt,
			"updated_at":            s.UpdatedAt,
			"user":                  s.User, // owner object (อาจเป็น nil ได้)
			"year":                  s.Year,
			"status":                s.Status,
			"category":              s.Category,
			"subcategory":           s.Subcategory,
		},
		"details":           details,   // gin.H หรือ nil
		"submission_users":  suOut,     // []gin.H
		"documents":         docOut,    // []gin.H
		"applicant":         applicant, // map หรือ nil
		"applicant_user_id": s.UserID,
	}

	c.JSON(http.StatusOK, resp)
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

// ===== REPLACE WHOLE FUNCTION =====
func ApproveSubmission(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}
	userID, _ := c.Get("userID")

	var req struct {
		// amounts (PR)
		RewardApproveAmount         *float64 `json:"reward_approve_amount"`
		RevisionFeeApproveAmount    *float64 `json:"revision_fee_approve_amount"`
		PublicationFeeApproveAmount *float64 `json:"publication_fee_approve_amount"`
		TotalApproveAmount          *float64 `json:"total_approve_amount"`
		AnnounceReferenceNumber     string   `json:"announce_reference_number"`
		// legacy fallback
		ApprovedAmount  *float64 `json:"approved_amount"`
		ApprovalComment string   `json:"approval_comment"` // จะเก็บที่ admin_comment ได้ถ้าต้องการ
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

	// load
	var submission models.Submission
	if err := tx.Preload("PublicationRewardDetail").
		Where("submission_id = ? AND deleted_at IS NULL", submissionID).
		First(&submission).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	allowed, err := utils.StatusMatchesCodes(
		submission.StatusID,
		utils.StatusCodePending,
		utils.StatusCodeDraft,
		utils.StatusCodeDeptHeadPending,
		utils.StatusCodeDeptHeadRecommended,
	)
	if err != nil || !allowed {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only submissions awaiting review can be approved"})
		return
	}

	now := time.Now()
	var adminID *int
	if uid, ok := userID.(int); ok {
		adminID = &uid
	}

	// ✅ central truth
	updates := map[string]interface{}{
		"status_id":         2, // approved
		"updated_at":        now,
		"admin_approved_by": adminID,
		"admin_approved_at": now,
	}
	// (ออปชัน) เก็บความเห็นของแอดมินตอนอนุมัติ
	if strings.TrimSpace(req.ApprovalComment) != "" {
		updates["admin_comment"] = strings.TrimSpace(req.ApprovalComment)
	}

	if err := tx.Model(&models.Submission{}).
		Where("submission_id = ?", submissionID).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission status"})
		return
	}

	// PR amounts
	if submission.SubmissionType == "publication_reward" {
		var d models.PublicationRewardDetail
		if submission.PublicationRewardDetail != nil {
			d = *submission.PublicationRewardDetail
		} else {
			_ = tx.Where("submission_id = ?", submissionID).First(&d).Error
		}
		d.SubmissionID = submissionID

		if req.RewardApproveAmount != nil {
			d.RewardApproveAmount = *req.RewardApproveAmount
		}
		if req.RevisionFeeApproveAmount != nil {
			d.RevisionFeeApproveAmount = *req.RevisionFeeApproveAmount
		}
		if req.PublicationFeeApproveAmount != nil {
			d.PublicationFeeApproveAmount = *req.PublicationFeeApproveAmount
		}
		if req.TotalApproveAmount != nil {
			d.TotalApproveAmount = *req.TotalApproveAmount
		} else if req.ApprovedAmount != nil {
			d.TotalApproveAmount = *req.ApprovedAmount
		} else {
			d.TotalApproveAmount = d.RewardApproveAmount + d.RevisionFeeApproveAmount + d.PublicationFeeApproveAmount
		}
		d.AnnounceReferenceNumber = strings.TrimSpace(req.AnnounceReferenceNumber)

		if d.DetailID == 0 {
			if err := tx.Create(&d).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create detail"})
				return
			}
		} else if err := tx.Save(&d).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update detail"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction failed"})
		return
	}

	var out models.Submission
	_ = config.DB.Preload("PublicationRewardDetail").
		Where("submission_id = ?", submissionID).
		First(&out).Error

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Submission approved successfully",
		"submission": out,
	})
}

// ==============================
// REPLACED: RejectSubmission (single source of truth)
// ==============================

// ===== REPLACE WHOLE FUNCTION =====
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
		Comment         string `json:"comment"` // ออปชัน
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

	allowed, err := utils.StatusMatchesCodes(
		submission.StatusID,
		utils.StatusCodePending,
		utils.StatusCodeDraft,
		utils.StatusCodeDeptHeadPending,
		utils.StatusCodeDeptHeadRecommended,
		utils.StatusCodeDeptHeadNotRecommended,
	)
	if err != nil || !allowed {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only submissions awaiting review can be rejected"})
		return
	}

	now := time.Now()
	var adminID *int
	if uid, ok := userID.(int); ok {
		adminID = &uid
	}

	updates := map[string]interface{}{
		"status_id":              3, // rejected
		"updated_at":             now,
		"admin_rejected_by":      adminID,
		"admin_rejected_at":      now,
		"admin_rejection_reason": strings.TrimSpace(req.RejectionReason),
	}
	if strings.TrimSpace(req.Comment) != "" {
		updates["admin_comment"] = strings.TrimSpace(req.Comment)
	}

	if err := tx.Model(&models.Submission{}).
		Where("submission_id = ?", submissionID).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission status"})
		return
	}

	// Audit
	desc := req.RejectionReason
	if err := tx.Create(&models.AuditLog{
		UserID:       *adminID,
		Action:       "reject",
		EntityType:   "submission",
		EntityID:     &submission.SubmissionID,
		EntityNumber: &submission.SubmissionNumber,
		Description:  &desc,
		IPAddress:    c.ClientIP(),
		CreatedAt:    now,
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record audit log"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject submission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Submission rejected successfully",
	})
}
