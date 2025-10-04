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

// ===== REPLACE WHOLE FUNCTION =====
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

	now := time.Now()
	updates := map[string]interface{}{
		"status_id":             rejectStatus.ApplicationStatusID,
		"updated_at":            now,
		"reviewed_at":           now,
		"head_approved_at":      now, // ตัดสินใจแล้ว
		"head_approved_by":      userID,
		"closed_at":             now,
		"head_rejected_by":      userID,
		"head_rejected_at":      now,
		"head_rejection_reason": strings.TrimSpace(req.RejectionReason),
	}
	if strings.TrimSpace(req.Comment) != "" {
		updates["head_comment"] = strings.TrimSpace(req.Comment)
	}

	if err := tx.Model(&models.Submission{}).
		Where("submission_id = ?", submissionID).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	// (ไม่อัปเดตตารางรายละเอียดอีกต่อไป)

	// Audit
	desc := req.RejectionReason
	if err := tx.Create(&models.AuditLog{
		UserID:       userID,
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rejection"})
		return
	}

	// payload (ตามพฤติกรรมเดิม)
	payload, err := buildSubmissionDetailPayload(submissionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	payload["success"] = true
	c.JSON(http.StatusOK, payload)
}

// controllers/dept_head_submission.go

func buildSubmissionDetailPayload(submissionID int) (gin.H, error) {
	// โหลด submission พร้อมความสัมพันธ์ที่จำเป็น (เพิ่ม Category/Subcategory)
	var submission models.Submission
	if err := config.DB.
		Preload("User").
		Preload("Year").
		Preload("Status").
		Preload("Category").
		Preload("Subcategory").
		Preload("SubmissionUsers.User"). // <- preload ผู้ร่วม + user ไว้เลย
		Preload("PublicationRewardDetail").
		Preload("FundApplicationDetail").
		Where("submission_id = ? AND deleted_at IS NULL", submissionID).
		First(&submission).Error; err != nil {
		return nil, err
	}

	// ---- applicant (เหมือนฝั่ง Admin) ----
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

	// ---- details (ตามชนิดคำขอ) ----
	details := gin.H{"type": submission.SubmissionType, "data": nil}
	switch submission.SubmissionType {
	case "publication_reward":
		details["data"] = submission.PublicationRewardDetail
	case "fund_application":
		details["data"] = submission.FundApplicationDetail
	}

	// ---- submission_users (ปลอดภัย) ----
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

	// ---- documents (ถ้ามีระบบเอกสาร ให้เติมตามจริงของโปรเจกต์) ----
	documents := []gin.H{}

	// ---- ประกอบ payload submission (เติมฟิลด์ที่ขาดให้ครบ) ----
	submissionPayload := gin.H{
		"submission_id":     submission.SubmissionID,
		"submission_number": submission.SubmissionNumber,
		"submission_type":   submission.SubmissionType,
		"user_id":           submission.UserID,
		"year_id":           submission.YearID,
		"status_id":         submission.StatusID,

		// ✅ สำคัญกับ "สถานะคำร้อง"
		"created_at":   submission.CreatedAt,
		"updated_at":   submission.UpdatedAt,
		"submitted_at": submission.SubmittedAt,
		"reviewed_at":  submission.ReviewedAt,

		// ✅ ข้อมูล Dept/Admin approve/reject
		"head_approved_by":  submission.HeadApprovedBy,
		"head_approved_at":  submission.HeadApprovedAt,
		"admin_approved_by": submission.AdminApprovedBy,
		"admin_approved_at": submission.AdminApprovedAt,

		"head_rejected_by":       submission.HeadRejectedBy,
		"head_rejected_at":       submission.HeadRejectedAt,
		"head_rejection_reason":  submission.HeadRejectionReason,
		"head_comment":           submission.HeadComment,
		"admin_rejected_by":      submission.AdminRejectedBy,
		"admin_rejected_at":      submission.AdminRejectedAt,
		"admin_rejection_reason": submission.AdminRejectionReason,
		"admin_comment":          submission.AdminComment,

		// legacy aggregate (คงไว้เพื่อความเข้ากันได้)
		"rejected_by":      submission.RejectedBy,
		"rejected_at":      submission.RejectedAt,
		"rejection_reason": submission.RejectionReason,
		"comment":          submission.Comment,

		// ✅ ข้อมูล fund สำหรับโชว์ชื่อทุน/ทุนย่อย
		"category_id":           submission.CategoryID,
		"subcategory_id":        submission.SubcategoryID,
		"subcategory_budget_id": submission.SubcategoryBudgetID,
		"category":              submission.Category,
		"subcategory":           submission.Subcategory,

		"user":   submission.User,
		"year":   submission.Year,
		"status": submission.Status,
	}

	// ---- ส่งคืน payload ครบชุด ----
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
