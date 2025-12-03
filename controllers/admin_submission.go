// controllers/admin_submission.go
package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const researchFundCategoryKeyword = "ทุนส่งเสริมการวิจัย"

var (
	errSubmissionNotResearchFund   = errors.New("submission does not belong to research fund category")
	errPaymentCapExceeded          = errors.New("payment would exceed approved amount")
	errSubmissionClosedForPayments = errors.New("submission is closed; reopen before recording payments")
	errMissingFundDetail           = errors.New("submission is missing fund application detail")
	errMissingApplicant            = errors.New("submission is missing applicant information")
	errPaymentAttachmentRequired   = errors.New("payment events require at least one attachment")

	researchFundCategoryCache            sync.Map
	researchFundCategoryKeywordCollapsed = normalizeCategoryName(researchFundCategoryKeyword)
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
		Preload("PublicationRewardDetail"). // PR detail
		Preload("PublicationRewardDetail.ExternalFunds", func(db *gorm.DB) *gorm.DB {
			return db.Where("publication_reward_external_funds.deleted_at IS NULL OR publication_reward_external_funds.deleted_at = '0000-00-00 00:00:00'").
				Order("external_fund_id ASC").
				Preload("Document").
				Preload("Document.File")
		}).
		Preload("ResearchFundEvents", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Preload("ResearchFundEvents.Creator").
		Preload("ResearchFundEvents.StatusAfter").
		Preload("ResearchFundEvents.Files").
		Preload("ResearchFundEvents.Files.File")

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
		Joins("LEFT JOIN publication_reward_external_funds pref ON pref.document_id = submission_documents.document_id AND (pref.deleted_at IS NULL OR pref.deleted_at = '0000-00-00 00:00:00')").
		Select("submission_documents.*, pref.external_fund_id AS external_funding_id").
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
		trimmedOriginal := strings.TrimSpace(d.OriginalName)
		var originalName any
		if trimmedOriginal != "" {
			originalName = trimmedOriginal
		} else {
			originalName = nil
		}
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
		item["original_name"] = originalName
		if d.DocumentType.DocumentTypeID != 0 {
			item["document_type"] = gin.H{
				"document_type_id":   d.DocumentType.DocumentTypeID,
				"document_type_name": d.DocumentType.DocumentTypeName,
				"required":           d.DocumentType.Required,
			}
		}
		if d.ExternalFundingID != nil {
			item["external_funding_id"] = d.ExternalFundingID
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

	// 9) research fund specific payload
	researchEventsPayload := buildResearchFundEventsPayload(s.ResearchFundEvents)
	researchSummary := buildResearchFundSummary(&s)

	var researchFundPayload gin.H
	detectionMeta := buildResearchFundDetectionMeta(&s)
	if isResearchFundSubmission(&s) {
		if researchEventsPayload == nil {
			researchEventsPayload = []gin.H{}
		}
		if researchSummary == nil {
			researchSummary = gin.H{}
		}
		payload := gin.H{
			"events":  researchEventsPayload,
			"summary": researchSummary,
		}
		if len(detectionMeta) > 0 {
			payload["meta"] = detectionMeta
		}
		researchFundPayload = payload
	} else if len(detectionMeta) > 0 {
		researchFundPayload = gin.H{"meta": detectionMeta}
	}

	// 10) response
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

	if researchFundPayload != nil {
		resp["research_fund"] = researchFundPayload
	}

	c.JSON(http.StatusOK, resp)
}

func AdminListSubmissions(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	perPage := 50

	// 1) ดึงรายการคำร้อง
	var submissions []models.Submission
	q := config.DB.
		Preload("User"). // อาจไม่ทำงานเพราะโครงสร้าง model, เราจะ backfill ให้ด้วย
		Preload("Year").
		Preload("Status").
		Preload("Category").
		Preload("Subcategory").
		Where("deleted_at IS NULL")

	if y := c.Query("year_id"); y != "" {
		q = q.Where("year_id = ?", y)
	}
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	q = q.Order(sortBy + " " + sortOrder)

	var total int64
	q.Model(&models.Submission{}).Count(&total)

	if err := q.Limit(perPage).Offset((page - 1) * perPage).Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch admin submissions"})
		return
	}

	// 2) หา user_id ที่ยังไม่มี s.User (Preload ไม่เติม)
	missingIDs := make([]int, 0, len(submissions))
	seen := map[int]struct{}{}
	for _, s := range submissions {
		if (s.User == nil || s.User.UserID == 0) && s.UserID > 0 {
			if _, dup := seen[s.UserID]; !dup {
				seen[s.UserID] = struct{}{}
				missingIDs = append(missingIDs, s.UserID)
			}
		}
	}
	log.Printf("[AdminListSubmissions] page=%d items=%d preload_missing=%d", page, len(submissions), len(missingIDs))

	// 3) ดึง users สำหรับ missingIDs ทีเดียว แล้วทำ map
	usersByID := map[int]models.User{}
	if len(missingIDs) > 0 {
		var users []models.User
		if err := config.DB.Where("user_id IN ?", missingIDs).Find(&users).Error; err == nil {
			for _, u := range users {
				usersByID[u.UserID] = u
			}
		}
		log.Printf("[AdminListSubmissions] backfilled_users=%d", len(usersByID))
	}

	// 4) ประกอบ response: ใส่ userObj เสมอ (จาก s.User หรือ backfill)
	out := make([]gin.H, 0, len(submissions))
	for _, s := range submissions {
		u := s.User
		if (u == nil || u.UserID == 0) && s.UserID > 0 {
			if v, ok := usersByID[s.UserID]; ok {
				// ใช้ที่ backfill มา
				u = &v
			}
		}

		var userObj gin.H
		if u != nil && u.UserID > 0 {
			userObj = gin.H{
				"user_id":    u.UserID,
				"user_fname": u.UserFname,
				"user_lname": u.UserLname,
				"email":      u.Email,
			}
		}

		out = append(out, gin.H{
			"submission_id":     s.SubmissionID,
			"submission_number": s.SubmissionNumber,
			"submission_type":   s.SubmissionType,
			"user_id":           s.UserID,
			"year_id":           s.YearID,
			"category_id":       s.CategoryID,
			"subcategory_id":    s.SubcategoryID,
			"status_id":         s.StatusID,
			"submitted_at":      s.SubmittedAt,
			"created_at":        s.CreatedAt,
			"updated_at":        s.UpdatedAt,

			// ให้ FE ใช้ชื่อได้ทุกแบบ (เผื่อโค้ดฝั่ง FE เรียก key ต่างกัน)
			"User":      userObj,
			"user":      userObj,
			"applicant": userObj,

			// ข้อมูลความสัมพันธ์อื่น ๆ (ตามที่ต้องการ)
			"Category":    s.Category,
			"Subcategory": s.Subcategory,
			"Status":      s.Status,
			"Year":        s.Year,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	c.JSON(http.StatusOK, gin.H{
		"submissions": out,
		"pagination": gin.H{
			"page":        page,
			"per_page":    perPage,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

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
		"updated_at":                     now, // ✅ แก้เป็น updated_at
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
	} else if submission.SubmissionType == "fund_application" {
		approvedAmount := 0.0
		if req.TotalApproveAmount != nil {
			approvedAmount = *req.TotalApproveAmount
		} else if req.ApprovedAmount != nil {
			approvedAmount = *req.ApprovedAmount
		}

		announceRef := strings.TrimSpace(req.AnnounceReferenceNumber)

		if err := tx.Model(&models.FundApplicationDetail{}).
			Where("submission_id = ?", submissionID).
			Updates(map[string]interface{}{
				"approved_amount":           approvedAmount,
				"announce_reference_number": announceRef,
			}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update fund application detail"})
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
		AdminRejectionReason string `json:"admin_rejection_reason"`
		RejectionReason      string `json:"rejection_reason"` // fallback from legacy clients
		Comment              string `json:"comment"`          // ออปชัน
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rejection reason is required"})
		return
	}

	rejectionReason := strings.TrimSpace(req.AdminRejectionReason)
	if rejectionReason == "" {
		rejectionReason = strings.TrimSpace(req.RejectionReason)
	}

	if rejectionReason == "" {
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
		"admin_rejection_reason": rejectionReason,
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
	desc := rejectionReason
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

// RequestSubmissionRevision allows admins to send a submission back to the applicant for
// additional information. The submission is unlocked for editing and moved to the
// needs-more-info status.
func RequestSubmissionRevision(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var req struct {
		Comment string `json:"comment"`
		Reason  string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !strings.Contains(err.Error(), "EOF") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	message := strings.TrimSpace(req.Comment)
	if message == "" {
		message = strings.TrimSpace(req.Reason)
	}
	if message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Revision comment is required"})
		return
	}

	userIDVal, _ := c.Get("userID")
	adminID, _ := userIDVal.(int)

	needsMoreInfoID, err := utils.GetStatusIDByCode(utils.StatusCodeNeedsMoreInfo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve revision status"})
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	var submission models.Submission
	if err := tx.First(&submission, submissionID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	allowed, err := utils.StatusMatchesCodes(
		submission.StatusID,
		utils.StatusCodePending,
		utils.StatusCodeDeptHeadPending,
		utils.StatusCodeDeptHeadRecommended,
		utils.StatusCodeNeedsMoreInfo,
	)
	if err != nil || !allowed {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission is not eligible for revision"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status_id":              needsMoreInfoID,
		"updated_at":             now,
		"reviewed_at":            now,
		"submitted_at":           gorm.Expr("NULL"),
		"admin_comment":          message,
		"admin_approved_by":      gorm.Expr("NULL"),
		"admin_approved_at":      gorm.Expr("NULL"),
		"admin_rejected_by":      gorm.Expr("NULL"),
		"admin_rejected_at":      gorm.Expr("NULL"),
		"admin_rejection_reason": gorm.Expr("NULL"),
		"head_approved_by":       gorm.Expr("NULL"),
		"head_approved_at":       gorm.Expr("NULL"),
		"head_rejected_by":       gorm.Expr("NULL"),
		"head_rejected_at":       gorm.Expr("NULL"),
		"head_rejection_reason":  gorm.Expr("NULL"),
	}

	if err := tx.Model(&models.Submission{}).
		Where("submission_id = ?", submissionID).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	desc := fmt.Sprintf("Admin requested revision: %s", message)
	audit := models.AuditLog{
		UserID:       adminID,
		Action:       "request_revision",
		EntityType:   "submission",
		EntityID:     &submission.SubmissionID,
		EntityNumber: &submission.SubmissionNumber,
		Description:  &desc,
		IPAddress:    c.ClientIP(),
		CreatedAt:    now,
	}
	if err := tx.Create(&audit).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record audit log"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to request revision"})
		return
	}

	if err := notifyNeedsMoreInfo(submission.SubmissionID, "admin", message); err != nil {
		log.Printf("notify admin revision request failed: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Revision requested successfully",
	})
}

// ListResearchFundEvents returns the admin event feed for a research fund submission.
func ListResearchFundEvents(c *gin.Context) {
	submission, ok := getResearchFundSubmissionOrAbort(c, true)
	if !ok {
		return
	}

	events := buildResearchFundEventsPayload(submission.ResearchFundEvents)
	summary := buildResearchFundSummary(submission)
	if summary == nil {
		summary = gin.H{}
	}

	meta := buildResearchFundDetectionMeta(submission)
	if meta == nil {
		meta = gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{
		"submission_id": submission.SubmissionID,
		"events":        events,
		"summary":       summary,
		"meta":          meta,
	})
}

// CreateResearchFundEvent records a new admin event for the research fund submission.
func CreateResearchFundEvent(c *gin.Context) {
	submission, ok := getResearchFundSubmissionOrAbort(c, false)
	if !ok {
		return
	}

	amountStr := strings.TrimSpace(c.PostForm("amount"))
	eventType := strings.TrimSpace(c.PostForm("event_type"))
	if eventType == "" {
		if amountStr != "" {
			eventType = models.ResearchFundEventTypePayment
		} else {
			eventType = models.ResearchFundEventTypeNote
		}
	}
	if !isValidResearchFundEventType(eventType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported event type"})
		return
	}

	comment := strings.TrimSpace(c.PostForm("comment"))

	var amountPtr *float64
	if eventType == models.ResearchFundEventTypePayment {
		if amountStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "amount is required for payment events"})
			return
		}
		amountVal, err := strconv.ParseFloat(amountStr, 64)
		if err != nil || amountVal <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "amount must be a positive number"})
			return
		}
		amountPtr = &amountVal
	} else if amountStr != "" {
		if amountVal, err := strconv.ParseFloat(amountStr, 64); err == nil {
			amountPtr = &amountVal
		}
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "multipart/form-data is required"})
		return
	}

	var files []*multipart.FileHeader
	if form != nil {
		files = form.File["files"]
	}
	userID := c.GetInt("userID")
	now := time.Now()

	var createdEvent models.ResearchFundAdminEvent
	var savedPaths []string

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		var current models.Submission
		if err := tx.Select("status_id").First(&current, "submission_id = ?", submission.SubmissionID).Error; err != nil {
			return err
		}
		submission.StatusID = current.StatusID

		var totalPaid float64
		closed, err := utils.IsSubmissionClosed(current.StatusID)
		if err != nil {
			return err
		}
		if eventType == models.ResearchFundEventTypePayment {
			if submission.FundApplicationDetail == nil {
				return errMissingFundDetail
			}
			if err := tx.Model(&models.ResearchFundAdminEvent{}).
				Where("submission_id = ? AND amount IS NOT NULL", submission.SubmissionID).
				Select("COALESCE(SUM(amount),0)").Scan(&totalPaid).Error; err != nil {
				return err
			}
		}

		if err := validateResearchFundEvent(submission, eventType, amountPtr, len(files), totalPaid, closed); err != nil {
			return err
		}

		event := models.ResearchFundAdminEvent{
			SubmissionID: submission.SubmissionID,
			Comment:      comment,
			Amount:       amountPtr,
			CreatedBy:    userID,
			CreatedAt:    now,
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		createdEvent = event

		if len(files) == 0 {
			return nil
		}

		if submission.User == nil && submission.UserID > 0 {
			var owner models.User
			if err := tx.First(&owner, "user_id = ?", submission.UserID).Error; err == nil {
				submission.User = &owner
			}
		}
		if submission.User == nil {
			return errMissingApplicant
		}

		uploadPath := os.Getenv("UPLOAD_PATH")
		if uploadPath == "" {
			uploadPath = "./uploads"
		}

		userFolderPath, err := utils.CreateUserFolderIfNotExists(*submission.User, uploadPath)
		if err != nil {
			return err
		}
		submissionFolderPath, err := utils.CreateSubmissionFolder(userFolderPath, submission.SubmissionType, submission.SubmissionID, submission.SubmissionNumber, submission.CreatedAt)
		if err != nil {
			return err
		}
		eventFolderPath, err := utils.CreateAdminEventFolder(submissionFolderPath, event.EventID)
		if err != nil {
			return err
		}

		savedPaths = make([]string, 0, len(files))

		for _, fh := range files {
			filename := utils.GenerateUniqueFilename(eventFolderPath, fh.Filename)
			destPath := filepath.Join(eventFolderPath, filename)
			if err := c.SaveUploadedFile(fh, destPath); err != nil {
				return err
			}
			savedPaths = append(savedPaths, destPath)

			stat, err := os.Stat(destPath)
			if err != nil {
				return err
			}

			metadataBytes, _ := json.Marshal(map[string]any{
				"submission_id": submission.SubmissionID,
				"event_id":      event.EventID,
			})

			fileUpload := models.FileUpload{
				OriginalName: fh.Filename,
				StoredPath:   destPath,
				FolderType:   models.FileFolderTypeAdminEvent,
				Metadata:     string(metadataBytes),
				FileSize:     stat.Size(),
				MimeType:     fh.Header.Get("Content-Type"),
				UploadedBy:   userID,
				UploadedAt:   now,
				CreateAt:     now,
				UpdateAt:     now,
			}
			if err := createFileUploadRecord(tx, &fileUpload); err != nil {
				return err
			}

			link := models.ResearchFundEventFile{
				EventID:   event.EventID,
				FileID:    fileUpload.FileID,
				CreatedAt: now,
			}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		for _, path := range savedPaths {
			_ = os.Remove(path)
		}

		switch {
		case errors.Is(err, errPaymentCapExceeded),
			errors.Is(err, errSubmissionClosedForPayments),
			errors.Is(err, errMissingFundDetail),
			errors.Is(err, errMissingApplicant),
			errors.Is(err, errPaymentAttachmentRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			log.Printf("[CreateResearchFundEvent] submission %d error: %v", submission.SubmissionID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create research fund event"})
		}
		return
	}

	refreshed, err := loadResearchFundSubmissionByID(submission.SubmissionID, true)
	if err != nil {
		log.Printf("[CreateResearchFundEvent] refresh submission %d error: %v", submission.SubmissionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load updated submission"})
		return
	}

	events := buildResearchFundEventsPayload(refreshed.ResearchFundEvents)
	summary := buildResearchFundSummary(refreshed)
	if summary == nil {
		summary = gin.H{}
	}

	var eventPayload gin.H
	single := buildResearchFundEventsPayload([]models.ResearchFundAdminEvent{createdEvent})
	if len(single) > 0 {
		eventPayload = single[0]
	}

	c.JSON(http.StatusCreated, gin.H{
		"event":   eventPayload,
		"events":  events,
		"summary": summary,
	})
}

// ToggleResearchFundClosure closes or reopens the submission for further payments.
func ToggleResearchFundClosure(c *gin.Context) {
	submission, ok := getResearchFundSubmissionOrAbort(c, false)
	if !ok {
		return
	}

	var req struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	userID := c.GetInt("userID")
	now := time.Now()

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		var current models.Submission
		if err := tx.Select("status_id").First(&current, "submission_id = ?", submission.SubmissionID).Error; err != nil {
			return err
		}
		submission.StatusID = current.StatusID

		closed, err := utils.IsSubmissionClosed(current.StatusID)
		if err != nil {
			return err
		}

		approvedStatusID, err := utils.GetStatusIDByCode(utils.StatusCodeApproved)
		if err != nil {
			return err
		}
		closedStatusID, err := utils.GetStatusIDByCode(utils.StatusCodeAdminClosed)
		if err != nil {
			return err
		}

		closingAllowed := true
		if !closed {
			if err := utils.EnsureStatusIn(current.StatusID, utils.StatusCodeApproved); err != nil {
				closingAllowed = false
			}
		}

		submissionUpdates, detailUpdates, eventComment, statusAfterID, err := applyClosureTransition(submission, closed, approvedStatusID, closedStatusID, now, req.Comment, closingAllowed)
		if err != nil {
			return err
		}

		if err := tx.Model(&models.Submission{}).
			Where("submission_id = ?", submission.SubmissionID).
			Updates(submissionUpdates).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.FundApplicationDetail{}).
			Where("submission_id = ?", submission.SubmissionID).
			Updates(detailUpdates).Error; err != nil {
			return err
		}

		event := models.ResearchFundAdminEvent{
			SubmissionID:  submission.SubmissionID,
			StatusAfterID: statusAfterID,
			Comment:       eventComment,
			CreatedBy:     userID,
			CreatedAt:     now,
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		if strings.Contains(err.Error(), "must be approved") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[ToggleResearchFundClosure] submission %d error: %v", submission.SubmissionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to toggle closure"})
		return
	}

	refreshed, err := loadResearchFundSubmissionByID(submission.SubmissionID, true)
	if err != nil {
		log.Printf("[ToggleResearchFundClosure] refresh submission %d error: %v", submission.SubmissionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load updated submission"})
		return
	}

	summary := buildResearchFundSummary(refreshed)
	if summary == nil {
		summary = gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{
		"summary": summary,
		"events":  buildResearchFundEventsPayload(refreshed.ResearchFundEvents),
	})
}

func loadResearchFundSubmissionByID(submissionID int, preloadEvents bool) (*models.Submission, error) {
	query := config.DB.
		Preload("User").
		Preload("Category").
		Preload("Subcategory").
		Preload("Subcategory.Category").
		Preload("FundApplicationDetail").
		Preload("FundApplicationDetail.Subcategory").
		Preload("FundApplicationDetail.Subcategory.Category")

	if preloadEvents {
		query = query.
			Preload("ResearchFundEvents", func(db *gorm.DB) *gorm.DB {
				return db.Order("created_at ASC")
			}).
			Preload("ResearchFundEvents.Creator").
			Preload("ResearchFundEvents.StatusAfter").
			Preload("ResearchFundEvents.Files").
			Preload("ResearchFundEvents.Files.File")
	}

	var submission models.Submission
	if err := query.First(&submission, "submission_id = ?", submissionID).Error; err != nil {
		return nil, err
	}
	return &submission, nil
}

func getResearchFundSubmissionOrAbort(c *gin.Context, preloadEvents bool) (*models.Submission, bool) {
	idStr := c.Param("id")
	sid, err := strconv.Atoi(idStr)
	if err != nil || sid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission id"})
		return nil, false
	}

	submission, err := loadResearchFundSubmissionByID(sid, preloadEvents)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		default:
			log.Printf("[getResearchFundSubmissionOrAbort] %d error: %v", sid, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load submission"})
		}
		return nil, false
	}

	detectionMeta := buildResearchFundDetectionMeta(submission)
	if !isResearchFundSubmission(submission) {
		log.Printf(
			"[getResearchFundSubmissionOrAbort] submission %d rejected for research timeline: %+v",
			submission.SubmissionID,
			detectionMeta,
		)

		payload := gin.H{"error": errSubmissionNotResearchFund.Error()}
		if detectionMeta != nil {
			payload["meta"] = detectionMeta
		}
		c.JSON(http.StatusForbidden, payload)
		return nil, false
	}

	if submission.User == nil && submission.UserID > 0 {
		var owner models.User
		if err := config.DB.First(&owner, "user_id = ?", submission.UserID).Error; err == nil {
			submission.User = &owner
		}
	}

	return submission, true
}

func validateResearchFundEvent(submission *models.Submission, eventType string, amount *float64, attachmentCount int, existingPaid float64, isClosed bool) error {
	if !isResearchFundSubmission(submission) {
		return errSubmissionNotResearchFund
	}

	switch eventType {
	case models.ResearchFundEventTypePayment:
		if amount == nil || *amount <= 0 {
			return fmt.Errorf("amount must be a positive number")
		}
		if attachmentCount == 0 {
			return errPaymentAttachmentRequired
		}
		if submission.FundApplicationDetail == nil {
			return errMissingFundDetail
		}
		if isClosed {
			return errSubmissionClosedForPayments
		}
		if existingPaid+*amount > submission.FundApplicationDetail.ApprovedAmount+1e-6 {
			return errPaymentCapExceeded
		}
	default:
		if !isValidResearchFundEventType(eventType) {
			return fmt.Errorf("unsupported event type")
		}
	}

	return nil
}

func applyClosureTransition(submission *models.Submission, currentlyClosed bool, approvedStatusID, closedStatusID int, now time.Time, comment string, closingAllowed bool) (map[string]any, map[string]any, string, *int, error) {
	if submission == nil {
		return nil, nil, "", nil, fmt.Errorf("submission is required")
	}
	if !isResearchFundSubmission(submission) {
		return nil, nil, "", nil, errSubmissionNotResearchFund
	}

	if currentlyClosed {
		submissionUpdates := map[string]any{
			"status_id": approvedStatusID,
			"closed_at": nil,
		}
		detailUpdates := map[string]any{"closed_at": nil}
		statusAfter := approvedStatusID
		return submissionUpdates, detailUpdates, buildClosureComment(comment, false), &statusAfter, nil
	}

	if !closingAllowed {
		return nil, nil, "", nil, fmt.Errorf("submission must be approved before closure")
	}

	submissionUpdates := map[string]any{
		"status_id": closedStatusID,
		"closed_at": now,
	}
	detailUpdates := map[string]any{"closed_at": now}
	statusAfter := closedStatusID
	return submissionUpdates, detailUpdates, buildClosureComment(comment, true), &statusAfter, nil
}

func buildResearchFundEventsPayload(events []models.ResearchFundAdminEvent) []gin.H {
	if len(events) == 0 {
		return []gin.H{}
	}

	out := make([]gin.H, 0, len(events))
	for _, evt := range events {
		payload := gin.H{
			"event_id":      evt.EventID,
			"submission_id": evt.SubmissionID,
			"event_type":    deriveResearchFundEventType(evt),
			"comment":       evt.Comment,
			"created_by":    evt.CreatedBy,
			"created_at":    evt.CreatedAt,
		}
		if evt.Amount != nil {
			payload["amount"] = *evt.Amount
		}
		if evt.StatusAfterID != nil {
			payload["status_after_id"] = *evt.StatusAfterID
		}
		if evt.StatusAfter != nil && evt.StatusAfter.ApplicationStatusID != 0 {
			payload["status_after"] = gin.H{
				"application_status_id": evt.StatusAfter.ApplicationStatusID,
				"status_code":           evt.StatusAfter.StatusCode,
				"status_name":           evt.StatusAfter.StatusName,
			}
		}
		if evt.Creator != nil && evt.Creator.UserID != 0 {
			payload["creator"] = gin.H{
				"user_id":    evt.Creator.UserID,
				"user_fname": evt.Creator.UserFname,
				"user_lname": evt.Creator.UserLname,
				"email":      evt.Creator.Email,
			}
		}

		files := make([]gin.H, 0, len(evt.Files))
		for _, ef := range evt.Files {
			filePayload := gin.H{
				"event_file_id": ef.EventFileID,
				"event_id":      ef.EventID,
				"file_id":       ef.FileID,
				"created_at":    ef.CreatedAt,
			}
			if ef.File.FileID != 0 {
				filePayload["file"] = gin.H{
					"file_id":       ef.File.FileID,
					"original_name": ef.File.OriginalName,
					"stored_path":   ef.File.StoredPath,
					"folder_type":   ef.File.FolderType,
					"metadata":      ef.File.Metadata,
					"mime_type":     ef.File.MimeType,
					"file_size":     ef.File.FileSize,
					"uploaded_at":   ef.File.UploadedAt,
				}
			}
			files = append(files, filePayload)
		}
		payload["files"] = files
		out = append(out, payload)
	}

	return out
}

func deriveResearchFundEventType(evt models.ResearchFundAdminEvent) string {
	if evt.StatusAfterID != nil {
		return models.ResearchFundEventTypeClosure
	}
	if evt.IsPayment() {
		return models.ResearchFundEventTypePayment
	}
	return models.ResearchFundEventTypeNote
}

func buildResearchFundSummary(submission *models.Submission) gin.H {
	if submission == nil || !isResearchFundSubmission(submission) {
		return nil
	}

	totalPaid := 0.0
	for _, evt := range submission.ResearchFundEvents {
		if evt.IsPayment() && evt.Amount != nil {
			totalPaid += *evt.Amount
		}
	}

	approvedAmount := 0.0
	if submission.FundApplicationDetail != nil {
		approvedAmount = submission.FundApplicationDetail.ApprovedAmount
	}

	remaining := approvedAmount - totalPaid
	closed, err := utils.IsSubmissionClosed(submission.StatusID)
	if err != nil {
		closed = false
	}

	summary := gin.H{
		"total_events":      len(submission.ResearchFundEvents),
		"total_paid_amount": totalPaid,
		"approved_amount":   approvedAmount,
		"remaining_amount":  remaining,
		"is_closed":         closed,
		"closed_at":         submission.ClosedAt,
	}

	if submission.FundApplicationDetail != nil {
		summary["detail_closed_at"] = submission.FundApplicationDetail.ClosedAt
	}

	return summary
}

func getSubmissionCategoryName(submission *models.Submission) string {
	candidates := collectResearchFundCategoryCandidates(submission)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func isResearchFundCategoryID(id int) bool {
	if id <= 0 {
		return false
	}

	if cached, ok := researchFundCategoryCache.Load(id); ok {
		if matched, valid := cached.(bool); valid {
			return matched
		}
	}

	var category models.FundCategory
	err := config.DB.Select("category_name").First(&category, "category_id = ?", id).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[isResearchFundCategoryID] lookup category %d failed: %v", id, err)
		}
		researchFundCategoryCache.Store(id, false)
		return false
	}

	matched := matchesResearchFundCategoryName(category.CategoryName)
	researchFundCategoryCache.Store(id, matched)
	return matched
}

func matchesResearchFundCategoryName(value string) bool {
	normalized := normalizeCategoryName(value)
	if normalized == "" || researchFundCategoryKeywordCollapsed == "" {
		return false
	}
	if strings.Contains(normalized, researchFundCategoryKeywordCollapsed) {
		return true
	}
	if strings.Contains(researchFundCategoryKeywordCollapsed, normalized) {
		return true
	}
	return false
}

func normalizeCategoryName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lowered := strings.ToLower(trimmed)
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '​' || r == '‌' || r == '‍' {
			return -1
		}
		return r
	}, lowered)
	return cleaned
}

func submissionMatchesResearchFundCategoryID(submission *models.Submission) bool {
	if submission == nil {
		return false
	}

	if submission.CategoryID != nil && isResearchFundCategoryID(*submission.CategoryID) {
		return true
	}

	if submission.Category != nil && isResearchFundCategoryID(submission.Category.CategoryID) {
		return true
	}

	if submission.Subcategory != nil {
		if isResearchFundCategoryID(submission.Subcategory.CategoryID) {
			return true
		}
		if isResearchFundCategoryID(submission.Subcategory.Category.CategoryID) {
			return true
		}
	}

	if submission.FundApplicationDetail != nil {
		detail := submission.FundApplicationDetail
		if detail.Subcategory != nil {
			if isResearchFundCategoryID(detail.Subcategory.CategoryID) {
				return true
			}
			if isResearchFundCategoryID(detail.Subcategory.Category.CategoryID) {
				return true
			}
		}
	}

	return false
}

func collectResearchFundCategoryCandidates(submission *models.Submission) []string {
	if submission == nil {
		return nil
	}

	seen := map[string]struct{}{}
	var candidates []string

	add := func(values ...string) {
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			lowered := strings.ToLower(trimmed)
			if _, dup := seen[lowered]; dup {
				continue
			}
			seen[lowered] = struct{}{}
			candidates = append(candidates, trimmed)
		}
	}

	if submission.CategoryName != nil {
		add(*submission.CategoryName)
	}
	if submission.Category != nil {
		add(submission.Category.CategoryName)
	}
	if submission.SubcategoryName != nil {
		add(*submission.SubcategoryName)
	}
	if submission.Subcategory != nil {
		add(submission.Subcategory.SubcategoryName)
		if submission.Subcategory.Category.CategoryID != 0 {
			add(submission.Subcategory.Category.CategoryName)
		}
	}
	if submission.FundApplicationDetail != nil {
		detail := submission.FundApplicationDetail
		if detail.Subcategory != nil {
			add(detail.Subcategory.SubcategoryName)
			if detail.Subcategory.Category.CategoryID != 0 {
				add(detail.Subcategory.Category.CategoryName)
			}
		}
	}

	return candidates
}

func isResearchFundSubmission(submission *models.Submission) bool {
	if submissionMatchesResearchFundCategoryID(submission) {
		return true
	}

	keyword := strings.ToLower(researchFundCategoryKeyword)
	for _, candidate := range collectResearchFundCategoryCandidates(submission) {
		if strings.Contains(strings.ToLower(candidate), keyword) {
			return true
		}
	}
	return false
}

func buildResearchFundDetectionMeta(submission *models.Submission) gin.H {
	if submission == nil {
		return gin.H{}
	}

	candidates := collectResearchFundCategoryCandidates(submission)
	keyword := strings.ToLower(researchFundCategoryKeyword)
	detection := gin.H{
		"keywords":   []string{researchFundCategoryKeyword},
		"candidates": candidates,
	}

	if submission.CategoryID != nil {
		detection["category_id"] = submission.CategoryID
	}
	if submission.CategoryName != nil {
		if trimmed := strings.TrimSpace(*submission.CategoryName); trimmed != "" {
			detection["category_name"] = trimmed
		}
	}
	if submission.Category != nil {
		if trimmed := strings.TrimSpace(submission.Category.CategoryName); trimmed != "" {
			detection["category_relation_name"] = trimmed
		}
	}
	if submission.SubcategoryID != nil {
		detection["subcategory_id"] = submission.SubcategoryID
	}
	if submission.SubcategoryName != nil {
		if trimmed := strings.TrimSpace(*submission.SubcategoryName); trimmed != "" {
			detection["subcategory_name"] = trimmed
		}
	}
	if submission.Subcategory != nil {
		if trimmed := strings.TrimSpace(submission.Subcategory.SubcategoryName); trimmed != "" {
			detection["subcategory_relation_name"] = trimmed
		}
		if submission.Subcategory.Category.CategoryID != 0 {
			if trimmed := strings.TrimSpace(submission.Subcategory.Category.CategoryName); trimmed != "" {
				detection["subcategory_category_name"] = trimmed
			}
		}
	}
	if submission.FundApplicationDetail != nil {
		detail := submission.FundApplicationDetail
		if detail.SubcategoryID != 0 {
			detection["detail_subcategory_id"] = detail.SubcategoryID
		}
		if detail.Subcategory != nil {
			if trimmed := strings.TrimSpace(detail.Subcategory.SubcategoryName); trimmed != "" {
				detection["detail_subcategory_name"] = trimmed
			}
			if detail.Subcategory.Category.CategoryID != 0 {
				if trimmed := strings.TrimSpace(detail.Subcategory.Category.CategoryName); trimmed != "" {
					detection["detail_category_name"] = trimmed
				}
			}
		}
	}

	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), keyword) {
			detection["matched_candidate"] = candidate
			break
		}
	}

	return detection
}

func isValidResearchFundEventType(eventType string) bool {
	switch eventType {
	case models.ResearchFundEventTypeNote, models.ResearchFundEventTypePayment:
		return true
	default:
		return false
	}
}

func buildClosureComment(comment string, closing bool) string {
	trimmed := strings.TrimSpace(comment)
	if trimmed != "" {
		return trimmed
	}
	if closing {
		return "Submission closed by admin"
	}
	return "Submission reopened by admin"
}
