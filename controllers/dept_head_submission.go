package controllers

import (
	"fmt"
	"log"
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

// controllers/dept_head_submission.go

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

// REPLACE WHOLE FUNCTION
func DeptHeadRecommendSubmission(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	// รองรับทั้ง head_comment (ใหม่) และ comment (เผื่อหน้าเก่าส่งมา)
	var req struct {
		HeadComment   *string `json:"head_comment"`
		Comment       *string `json:"comment"`
		HeadSignature string  `json:"head_signature"`
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
	if err != nil || !allowed {
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

	// รวมข้อความจาก head_comment หรือ comment (เลือกอันที่ไม่ว่าง)
	pick := func(ptrs ...*string) string {
		for _, p := range ptrs {
			if p != nil && strings.TrimSpace(*p) != "" {
				return strings.TrimSpace(*p)
			}
		}
		return ""
	}
	headComment := pick(req.HeadComment, req.Comment)

	now := time.Now()
	updates := map[string]interface{}{
		"status_id":        targetStatus.ApplicationStatusID,
		"updated_at":       now,
		"reviewed_at":      now,
		"head_approved_at": now,
		"head_approved_by": userID,

		// บันทึกลงคอลัมน์ใหม่เท่านั้น
		"head_comment": func() interface{} {
			if headComment != "" {
				return headComment
			}
			return gorm.Expr("NULL")
		}(),

		// เคลียร์ร่องรอย reject เก่า (ถ้ามี)
		"head_rejected_by":      gorm.Expr("NULL"),
		"head_rejected_at":      gorm.Expr("NULL"),
		"head_rejection_reason": gorm.Expr("NULL"),
		// ไม่อัปเดตคอลัมน์ legacy: comment
	}

	signature := strings.TrimSpace(req.HeadSignature)
	if signature != "" {
		updates["head_signature"] = signature
	} else {
		updates["head_signature"] = gorm.Expr("NULL")
	}

	if err := tx.Model(&models.Submission{}).
		Where("submission_id = ?", submissionID).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	// (optional) บันทึก audit log แบบเดิม
	logDescription := headComment
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
		RejectionReason string  `json:"rejection_reason" binding:"required"`
		HeadComment     *string `json:"head_comment"`
		Comment         *string `json:"comment"` // เผื่อของเก่าส่งมา
		HeadSignature   string  `json:"head_signature"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.RejectionReason) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rejection reason is required"})
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
	if err := tx.First(&submission, submissionID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	allowed, err := utils.StatusMatchesCodes(submission.StatusID, utils.StatusCodeDeptHeadPending)
	if err != nil || !allowed {
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

	pick := func(ptrs ...*string) string {
		for _, p := range ptrs {
			if p != nil && strings.TrimSpace(*p) != "" {
				return strings.TrimSpace(*p)
			}
		}
		return ""
	}
	headComment := pick(req.HeadComment, req.Comment)

	now := time.Now()
	updates := map[string]interface{}{
		"status_id":   rejectStatus.ApplicationStatusID,
		"updated_at":  now,
		"reviewed_at": now,

		"head_approved_at": gorm.Expr("NULL"),
		"head_approved_by": gorm.Expr("NULL"),

		"head_rejected_by":      userID,
		"head_rejected_at":      now,
		"head_rejection_reason": strings.TrimSpace(req.RejectionReason),
	}
	if headComment != "" {
		updates["head_comment"] = headComment
	}

	signature := strings.TrimSpace(req.HeadSignature)
	if signature != "" {
		updates["head_signature"] = signature
	}

	if err := tx.Model(&models.Submission{}).
		Where("submission_id = ?", submissionID).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	desc := "Department head rejected submission"
	auditLog := models.AuditLog{
		UserID:       userID,
		Action:       "review",
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

// DeptHeadRequestRevision allows department heads to request additional information and
// unlock the submission for further edits by the applicant.
func DeptHeadRequestRevision(c *gin.Context) {
	submissionIDStr := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var req struct {
		Comment       *string `json:"comment"`
		HeadComment   *string `json:"head_comment"`
		HeadSignature string  `json:"head_signature"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !strings.Contains(err.Error(), "EOF") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	pick := func(values ...*string) string {
		for _, value := range values {
			if value == nil {
				continue
			}
			if trimmed := strings.TrimSpace(*value); trimmed != "" {
				return trimmed
			}
		}
		return ""
	}

	message := pick(req.HeadComment, req.Comment)
	if message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Revision comment is required"})
		return
	}

	userIDVal, _ := c.Get("userID")
	userID := userIDVal.(int)

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
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load submission"})
		}
		return
	}

	allowed, err := utils.StatusMatchesCodes(
		submission.StatusID,
		utils.StatusCodeDeptHeadPending,
		utils.StatusCodeNeedsMoreInfo,
	)
	if err != nil || !allowed {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission is not awaiting department review"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status_id":             needsMoreInfoID,
		"updated_at":            now,
		"reviewed_at":           now,
		"submitted_at":          gorm.Expr("NULL"),
		"head_comment":          message,
		"comment":               message,
		"head_approved_by":      gorm.Expr("NULL"),
		"head_approved_at":      gorm.Expr("NULL"),
		"head_rejected_by":      gorm.Expr("NULL"),
		"head_rejected_at":      gorm.Expr("NULL"),
		"head_rejection_reason": gorm.Expr("NULL"),
	}

	signature := strings.TrimSpace(req.HeadSignature)
	if signature != "" {
		updates["head_signature"] = signature
	} else {
		updates["head_signature"] = gorm.Expr("NULL")
	}

	if err := tx.Model(&models.Submission{}).
		Where("submission_id = ?", submissionID).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	desc := fmt.Sprintf("Department head requested revision: %s", message)
	auditLog := models.AuditLog{
		UserID:       userID,
		Action:       "request_revision",
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to request revision"})
		return
	}

	if err := notifyNeedsMoreInfo(submission.SubmissionID, "dept_head", message); err != nil {
		log.Printf("notify dept head revision request failed: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Revision requested successfully",
	})
}

// controllers/dept_head_submission.go

func buildSubmissionDetailPayload(submissionID int) (gin.H, error) {
	// โหลด submission พร้อมความสัมพันธ์ที่จำเป็น (ตามของเดิม)
	var submission models.Submission
	if err := config.DB.
		Preload("User").
		Preload("Year").
		Preload("Status").
		Preload("Category").
		Preload("Subcategory").
		Preload("SubmissionUsers.User").
		Preload("PublicationRewardDetail").
		Preload("FundApplicationDetail").
		Preload("FundApplicationDetail.Subcategory").
		Preload("FundApplicationDetail.Subcategory.Category").
		Where("submission_id = ? AND deleted_at IS NULL", submissionID).
		First(&submission).Error; err != nil {
		return nil, err
	}

	// ---- applicant (เหมือนเดิม) ----
	var applicant map[string]any
	if submission.User != nil && submission.User.UserID > 0 {
		applicant = map[string]any{
			"user_id":    submission.User.UserID,
			"user_fname": submission.User.UserFname,
			"user_lname": submission.User.UserLname,
			"email":      submission.User.Email,
		}
	} else if submission.UserID > 0 {
		var u models.User
		if err := config.DB.
			Select("user_id, user_fname, user_lname, email").
			Where("user_id = ?", submission.UserID).
			First(&u).Error; err == nil && u.UserID > 0 {
			applicant = map[string]any{
				"user_id":    u.UserID,
				"user_fname": u.UserFname,
				"user_lname": u.UserLname,
				"email":      u.Email,
			}
			submission.User = &u
		}
	}

	// ---- details (ใส่ประกาศสำหรับ fund_application ให้แน่ใจว่ามี) ----
	var detailsData any = nil
	switch submission.SubmissionType {
	case "publication_reward":
		detailsData = submission.PublicationRewardDetail

	case "fund_application":
		// ดึงสองคอลัมน์ที่ struct ไม่มี เพื่อ inject เข้า response
		var extra struct {
			MainAnnoucement             *int `gorm:"column:main_annoucement"`
			ActivitySupportAnnouncement *int `gorm:"column:activity_support_announcement"`
		}
		_ = config.DB.
			Table("fund_application_details").
			Select("main_annoucement, activity_support_announcement").
			Where("submission_id = ?", submissionID).
			Take(&extra).Error // ถ้าไม่เจอ / nil ก็ปล่อยให้เป็น nil

		if submission.FundApplicationDetail != nil {
			fad := submission.FundApplicationDetail
			// สร้าง map ตอบกลับพร้อมประกาศ 2 ตัว
			detailsData = gin.H{
				"detail_id":                 fad.DetailID,
				"submission_id":             fad.SubmissionID,
				"subcategory_id":            fad.SubcategoryID,
				"project_title":             fad.ProjectTitle,
				"project_description":       fad.ProjectDescription,
				"requested_amount":          fad.RequestedAmount,
				"approved_amount":           fad.ApprovedAmount,
				"announce_reference_number": fad.AnnounceReferenceNumber,
				"subcategory":               fad.Subcategory,

				// >>> ประกาศที่ต้องการ <<<
				"main_annoucement":              extra.MainAnnoucement,
				"activity_support_announcement": extra.ActivitySupportAnnouncement,
			}
		} else {
			// กันเคสไม่ preload detail ด้วย
			detailsData = gin.H{
				"main_annoucement":              extra.MainAnnoucement,
				"activity_support_announcement": extra.ActivitySupportAnnouncement,
			}
		}
	}

	details := gin.H{"type": submission.SubmissionType, "data": detailsData}

	// ---- submission_users (เหมือนเดิม) ----
	submissionUsers := make([]gin.H, 0, len(submission.SubmissionUsers))
	for _, su := range submission.SubmissionUsers {
		if su.User != nil && su.User.UserID > 0 {
			submissionUsers = append(submissionUsers, gin.H{
				"user_id":    su.User.UserID,
				"user_fname": su.User.UserFname,
				"user_lname": su.User.UserLname,
				"email":      su.User.Email,
			})
		}
	}

	// ---- documents ----
	var documents []models.SubmissionDocument
	if err := config.DB.
		Joins("LEFT JOIN document_types dt ON dt.document_type_id = submission_documents.document_type_id").
		Select("submission_documents.*, dt.document_type_name").
		Preload("File").
		Preload("DocumentType").
		Where("submission_documents.submission_id = ?", submissionID).
		Order("submission_documents.display_order, submission_documents.created_at").
		Find(&documents).Error; err != nil {
		documents = []models.SubmissionDocument{}
	}

	// ---- payload หลัก (เหมือนเดิม) ----
	submissionPayload := gin.H{
		"submission_id":     submission.SubmissionID,
		"submission_number": submission.SubmissionNumber,
		"submission_type":   submission.SubmissionType,
		"user_id":           submission.UserID,
		"year_id":           submission.YearID,
		"status_id":         submission.StatusID,

		"created_at":   submission.CreatedAt,
		"updated_at":   submission.UpdatedAt,
		"submitted_at": submission.SubmittedAt,
		"reviewed_at":  submission.ReviewedAt,

		"head_approved_by":       submission.HeadApprovedBy,
		"head_approved_at":       submission.HeadApprovedAt,
		"head_rejected_by":       submission.HeadRejectedBy,
		"head_rejected_at":       submission.HeadRejectedAt,
		"head_rejection_reason":  submission.HeadRejectionReason,
		"head_comment":           submission.HeadComment,
		"head_signature":         submission.HeadSignature,
		"admin_approved_by":      submission.AdminApprovedBy,
		"admin_approved_at":      submission.AdminApprovedAt,
		"admin_rejected_by":      submission.AdminRejectedBy,
		"admin_rejected_at":      submission.AdminRejectedAt,
		"admin_rejection_reason": submission.AdminRejectionReason,
		"admin_comment":          submission.AdminComment,

		"category_id":           submission.CategoryID,
		"subcategory_id":        submission.SubcategoryID,
		"subcategory_budget_id": submission.SubcategoryBudgetID,
		"category":              submission.Category,
		"subcategory":           submission.Subcategory,
		"category_name": func() string {
			if submission.Category != nil {
				return submission.Category.CategoryName
			}
			return ""
		}(),
		"subcategory_name": func() string {
			if submission.Subcategory != nil && submission.Subcategory.SubcategoryName != "" {
				return submission.Subcategory.SubcategoryName
			}
			if submission.FundApplicationDetail != nil &&
				submission.FundApplicationDetail.Subcategory != nil &&
				submission.FundApplicationDetail.Subcategory.SubcategoryName != "" {
				return submission.FundApplicationDetail.Subcategory.SubcategoryName
			}
			return ""
		}(),

		"user":   submission.User,
		"year":   submission.Year,
		"status": submission.Status,
	}

	// (ตอนสร้าง resp)
	resp := gin.H{
		"submission":        submissionPayload,
		"details":           details,
		"submission_users":  submissionUsers,
		"documents":         documents,
		"applicant":         applicant,
		"applicant_user_id": submission.UserID,
		"success":           true,
	}
	return resp, nil
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
