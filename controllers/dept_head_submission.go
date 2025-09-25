package controllers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	deptHeadStatusReviewing = "DEPTHEAD_REVIEWING"
	deptHeadStatusApproved  = "DEPTHEAD_APPROVED"
	deptHeadStatusRejected  = "DEPTHEAD_REJECTED"
)

var (
	deptHeadFindStatusByCodeFunc = findStatusByCode
	deptHeadLoadSubmissionFunc   = loadSubmissionForDeptHead
	deptHeadRecordDecisionFunc   = recordDeptHeadDecision
	deptHeadBuildResponseFunc    = buildDeptHeadSubmissionResponse
	deptHeadNotifyDecisionFunc   = notifyDeptHeadDecision
	deptHeadBeginTxFunc          = func() *gorm.DB { return config.DB.Begin() }
)

type deptHeadDecisionRequest struct {
	Comment string `json:"comment"`
}

func normalizeStatusSlug(slug string) string {
	return strings.ToUpper(strings.TrimSpace(slug))
}

func findStatusByCode(slug string) (*models.ApplicationStatus, error) {
	normalized := normalizeStatusSlug(slug)
	if normalized == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var status models.ApplicationStatus
	if err := config.DB.Where("UPPER(status_code) = ?", normalized).First(&status).Error; err != nil {
		return nil, err
	}
	return &status, nil
}

func ensureDeptHeadRole(c *gin.Context) bool {
	roleID, exists := c.Get("roleID")
	if !exists || roleID.(int) != 4 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Dept head access required"})
		return false
	}
	return true
}

// GetDeptHeadSubmissions handles GET /api/v1/dept-head/submissions
func GetDeptHeadSubmissions(c *gin.Context) {
	if !ensureDeptHeadRole(c) {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// ใช้ตัวเลขล้วน: ถ้าไม่ส่งมา ให้ default = 6 (อยู่ระหว่างการพิจารณาจากหัวหน้าสาขา)
	statusID := 6
	if v := strings.TrimSpace(c.Query("status_id")); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status_id"})
			return
		}
		statusID = parsed
	}

	// ตรวจว่ามี status_id นี้จริงในตาราง application_status
	var status models.ApplicationStatus
	if err := config.DB.First(&status, statusID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status_id"})
		return
	}

	submissionType := strings.TrimSpace(c.Query("type"))
	offset := (page - 1) * limit

	query := config.DB.Model(&models.Submission{}).
		Preload("User").
		Preload("Status").
		Preload("Category").
		Preload("Subcategory").
		Where("deleted_at IS NULL").
		Where("status_id = ?", statusID)

	if submissionType != "" {
		query = query.Where("submission_type = ?", submissionType)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	var submissions []models.Submission
	if err := query.Order("COALESCE(submitted_at, created_at) DESC").
		Offset(offset).Limit(limit).Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	totalPages := (total + int64(limit) - 1) / int64(limit)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"submissions": submissions,
		"status": gin.H{
			"id":   status.ApplicationStatusID,
			"code": status.StatusCode,
			"name": status.StatusName,
		},
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

func buildDeptHeadSubmissionResponse(db *gorm.DB, submissionID int) (gin.H, error) {
	var submission models.Submission
	if err := db.Preload("User").
		Preload("Year").
		Preload("Status").
		Preload("Category").
		Preload("Subcategory").
		Preload("FundApplicationDetail").
		Preload("FundApplicationDetail.Subcategory").
		Preload("FundApplicationDetail.Subcategory.Category").
		Preload("PublicationRewardDetail").
		First(&submission, submissionID).Error; err != nil {
		return nil, err
	}

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
			"category":              submission.Category,
			"subcategory":           submission.Subcategory,
		},
		"submission_users": []gin.H{},
		"documents":        []gin.H{},
		"reviews":          []models.SubmissionReview{},
		"status_history":   []models.SubmissionStatusHistory{},
	}

	// Include applicant shortcut
	if submission.User.UserID != 0 {
		response["applicant"] = submission.User
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

	var submissionUsers []models.SubmissionUser
	if err := db.Where("submission_id = ?", submissionID).
		Preload("User").
		Order("display_order ASC").
		Find(&submissionUsers).Error; err == nil {
		for _, su := range submissionUsers {
			if su.User == nil {
				continue
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
	}

	var documents []models.SubmissionDocument
	if err := db.Where("submission_id = ?", submissionID).
		Preload("DocumentType").
		Preload("File").
		Order("display_order ASC").
		Find(&documents).Error; err == nil {
		for _, doc := range documents {
			docInfo := gin.H{
				"document_id":      doc.DocumentID,
				"submission_id":    doc.SubmissionID,
				"file_id":          doc.FileID,
				"document_type_id": doc.DocumentTypeID,
				"description":      doc.Description,
				"display_order":    doc.DisplayOrder,
				"is_required":      doc.IsRequired,
				"is_verified":      doc.IsVerified,
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
	}

	var reviews []models.SubmissionReview
	if err := db.Where("submission_id = ?", submissionID).
		Preload("Reviewer").
		Order("reviewed_at DESC, review_id DESC").
		Find(&reviews).Error; err == nil {
		response["reviews"] = reviews
	}

	var histories []models.SubmissionStatusHistory
	if err := db.Where("submission_id = ?", submissionID).
		Preload("ChangedByUser").
		Order("created_at DESC, history_id DESC").
		Find(&histories).Error; err == nil {
		response["status_history"] = histories
	}

	return response, nil
}

// GetDeptHeadSubmission handles GET /api/v1/dept-head/submissions/:id
func GetDeptHeadSubmission(c *gin.Context) {
	if !ensureDeptHeadRole(c) {
		return
	}

	// บังคับ content-type ให้แน่ใจว่าเป็น JSON
	c.Header("Content-Type", "application/json; charset=utf-8")

	submissionID, err := strconv.Atoi(c.Param("id"))
	if err != nil || submissionID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid submission ID"})
		return
	}

	resp, err := deptHeadBuildResponseFunc(config.DB, submissionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Submission not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to load submission"})
		return
	}

	if resp == nil {
		// กัน edge case ไม่ให้คืน body ว่าง
		c.JSON(http.StatusOK, gin.H{"success": true, "submission": gin.H{}})
		return
	}

	resp["success"] = true
	c.JSON(http.StatusOK, resp)
}

func loadSubmissionForDeptHead(tx *gorm.DB, submissionID int) (*models.Submission, error) {
	db := tx
	if db == nil {
		db = config.DB
	}
	if db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	var submission models.Submission
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&submission, submissionID).Error; err != nil {
		return nil, err
	}
	return &submission, nil
}

func recordDeptHeadDecision(tx *gorm.DB, submission *models.Submission, newStatusID int, reviewerID int, reviewStatus string, comment string, ip string) error {
	db := tx
	if db == nil {
		db = config.DB
	}
	if db == nil {
		return fmt.Errorf("database not configured")
	}

	now := time.Now()

	oldStatus := submission.StatusID
	submission.StatusID = newStatusID
	submission.UpdatedAt = now

	if err := db.Save(submission).Error; err != nil {
		return err
	}

	var maxRound int64
	if err := db.Model(&models.SubmissionReview{}).
		Where("submission_id = ?", submission.SubmissionID).
		Select("COALESCE(MAX(review_round), 0)").
		Scan(&maxRound).Error; err != nil {
		return err
	}

	review := models.SubmissionReview{
		SubmissionID: submission.SubmissionID,
		ReviewerID:   reviewerID,
		ReviewRound:  int(maxRound) + 1,
		ReviewStatus: reviewStatus,
	}
	if comment != "" {
		commentCopy := comment
		review.Comments = &commentCopy
	}
	reviewTime := now
	review.ReviewedAt = &reviewTime

	if err := db.Create(&review).Error; err != nil {
		return err
	}

	history := models.SubmissionStatusHistory{
		SubmissionID: submission.SubmissionID,
		NewStatusID:  newStatusID,
		ChangedBy:    reviewerID,
		CreatedAt:    now,
	}
	if oldStatus > 0 {
		oldStatusCopy := oldStatus
		history.OldStatusID = &oldStatusCopy
	}
	if comment != "" {
		commentCopy := comment
		history.Reason = &commentCopy
	}

	if err := db.Create(&history).Error; err != nil {
		return err
	}

	desc := fmt.Sprintf("dept head %s submission", reviewStatus)
	if comment != "" {
		desc = fmt.Sprintf("%s: %s", desc, comment)
	}

	action := "review"
	switch reviewStatus {
	case "approved":
		action = "approve"
	case "rejected":
		action = "reject"
	}

	ip = strings.TrimSpace(ip)

	descCopy := desc
	submissionIDCopy := submission.SubmissionID
	submissionNumberCopy := submission.SubmissionNumber

	audit := models.AuditLog{
		UserID:       reviewerID,
		Action:       action,
		EntityType:   "submission",
		EntityID:     &submissionIDCopy,
		EntityNumber: &submissionNumberCopy,
		Description:  &descCopy,
		IPAddress:    ip,
		CreatedAt:    now,
	}

	if err := db.Create(&audit).Error; err != nil {
		return err
	}

	return nil
}

func notifyDeptHeadDecision(submission models.Submission, decision string, comment string) {
	db := config.DB
	if db == nil {
		return
	}

	userID := submission.UserID
	if userID == 0 {
		return
	}

	submissionNumber := submission.SubmissionNumber
	comment = strings.TrimSpace(comment)

	var title, message, typ string
	switch decision {
	case "approved":
		title = "หัวหน้าสาขาเห็นควรคำร้องของคุณ"
		message = fmt.Sprintf("คำร้องหมายเลข %s ได้รับการเห็นควรจากหัวหน้าสาขา", submissionNumber)
		typ = "success"
	case "rejected":
		title = "หัวหน้าสาขาไม่เห็นควรคำร้องของคุณ"
		message = fmt.Sprintf("คำร้องหมายเลข %s ถูกไม่เห็นควรโดยหัวหน้าสาขา", submissionNumber)
		typ = "warning"
	default:
		title = "อัปเดตคำร้อง"
		message = fmt.Sprintf("คำร้องหมายเลข %s มีการอัปเดต", submissionNumber)
		typ = "info"
	}

	if comment != "" {
		message = fmt.Sprintf("%s\nหมายเหตุ: %s", message, comment)
	}

	go func() {
		submissionID := uint(submission.SubmissionID)
		if err := db.Exec(`CALL CreateNotification(?,?,?,?,?)`, userID, title, message, typ, submission.SubmissionID).Error; err != nil {
			notif := models.Notification{
				UserID:              uint(userID),
				Title:               title,
				Message:             message,
				Type:                typ,
				RelatedSubmissionID: &submissionID,
				IsRead:              false,
				CreateAt:            time.Now(),
			}
			if err := db.Create(&notif).Error; err != nil {
				log.Printf("notifyDeptHeadDecision: failed to create notification: %v", err)
			}
		}
	}()
}

// DeptHeadApproveSubmission handles POST /api/v1/dept-head/submissions/:id/approve
func DeptHeadApproveSubmission(c *gin.Context) {
	if !ensureDeptHeadRole(c) {
		return
	}

	submissionID, err := strconv.Atoi(c.Param("id"))
	if err != nil || submissionID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var req deptHeadDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}
	comment := strings.TrimSpace(req.Comment)

	reviewingStatus, err := deptHeadFindStatusByCodeFunc(deptHeadStatusReviewing)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Reviewing status not configured"})
		return
	}
	approvedStatus, err := deptHeadFindStatusByCodeFunc(deptHeadStatusApproved)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Approved status not configured"})
		return
	}

	reviewerID := c.GetInt("userID")

	tx := deptHeadBeginTxFunc()
	rollback := func() {}
	commit := func() error { return nil }
	if tx != nil {
		rollback = func() { tx.Rollback() }
		commit = func() error { return tx.Commit().Error }
	}
	defer func() {
		if r := recover(); r != nil {
			rollback()
		}
	}()

	submission, err := deptHeadLoadSubmissionFunc(tx, submissionID)
	if err != nil {
		rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load submission"})
		return
	}

	if submission.StatusID != reviewingStatus.ApplicationStatusID {
		rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission is not awaiting dept head review"})
		return
	}

	if err := deptHeadRecordDecisionFunc(tx, submission, approvedStatus.ApplicationStatusID, reviewerID, "approved", comment, c.ClientIP()); err != nil {
		rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	if err := commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	deptHeadNotifyDecisionFunc(*submission, "approved", comment)

	resp, err := deptHeadBuildResponseFunc(config.DB, submissionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Submission approved"})
		return
	}

	resp["success"] = true
	resp["message"] = "Submission approved"
	c.JSON(http.StatusOK, resp)
}

// DeptHeadRejectSubmission handles POST /api/v1/dept-head/submissions/:id/reject
func DeptHeadRejectSubmission(c *gin.Context) {
	if !ensureDeptHeadRole(c) {
		return
	}

	submissionID, err := strconv.Atoi(c.Param("id"))
	if err != nil || submissionID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var req deptHeadDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}
	comment := strings.TrimSpace(req.Comment)

	reviewingStatus, err := deptHeadFindStatusByCodeFunc(deptHeadStatusReviewing)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Reviewing status not configured"})
		return
	}
	rejectedStatus, err := deptHeadFindStatusByCodeFunc(deptHeadStatusRejected)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Rejected status not configured"})
		return
	}

	reviewerID := c.GetInt("userID")

	tx := deptHeadBeginTxFunc()
	rollback := func() {}
	commit := func() error { return nil }
	if tx != nil {
		rollback = func() { tx.Rollback() }
		commit = func() error { return tx.Commit().Error }
	}
	defer func() {
		if r := recover(); r != nil {
			rollback()
		}
	}()

	submission, err := deptHeadLoadSubmissionFunc(tx, submissionID)
	if err != nil {
		rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load submission"})
		return
	}

	if submission.StatusID != reviewingStatus.ApplicationStatusID {
		rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission is not awaiting dept head review"})
		return
	}

	if err := deptHeadRecordDecisionFunc(tx, submission, rejectedStatus.ApplicationStatusID, reviewerID, "rejected", comment, c.ClientIP()); err != nil {
		rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	if err := commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	deptHeadNotifyDecisionFunc(*submission, "rejected", comment)

	resp, err := deptHeadBuildResponseFunc(config.DB, submissionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Submission rejected"})
		return
	}

	resp["success"] = true
	resp["message"] = "Submission rejected"
	c.JSON(http.StatusOK, resp)
}
