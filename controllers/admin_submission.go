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

// ---------------------------
// GET /api/v1/admin/submissions/:id/details
// ---------------------------
// - ส่ง submission.announce_reference_number ใน payload
// - ถ้าเป็น publication_reward:
//   - ถ้ายังไม่อนุมัติ -> เคลียร์เลขที่ detail (ไม่โชว์ก่อนอนุมัติ)
//   - ถ้าอนุมัติแล้ว -> ถ้า submission ยังไม่มีเลข แต่ detail มี -> sync ขึ้น submission
func GetSubmissionDetails(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var sub models.Submission
	q := config.DB.
		Preload("User").
		Preload("Year").
		Preload("Status").
		Preload("FundApplicationDetail").
		Preload("FundApplicationDetail.Subcategory").
		Preload("FundApplicationDetail.Subcategory.Category").
		Preload("PublicationRewardDetail")

	if err := q.First(&sub, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// co-authors
	var subUsers []models.SubmissionUser
	_ = config.DB.Where("submission_id = ?", id).
		Preload("User").
		Order("display_order ASC").
		Find(&subUsers)

	// docs
	var docs []models.SubmissionDocument
	_ = config.DB.Where("submission_id = ?", id).
		Preload("DocumentType").
		Preload("File").
		Find(&docs)

	// sync/clear announce ref
	if sub.SubmissionType == "publication_reward" && sub.PublicationRewardDetail != nil {
		if sub.StatusID != 2 {
			// ยังไม่อนุมัติ -> ไม่โชว์เลขใน detail
			sub.PublicationRewardDetail.AnnounceReferenceNumber = ""
		} else {
			// อนุมัติแล้ว -> ถ้า submission ยังว่างแต่ detail มี -> sync
			if strings.TrimSpace(sub.AnnounceReferenceNumber) == "" &&
				strings.TrimSpace(sub.PublicationRewardDetail.AnnounceReferenceNumber) != "" {
				sub.AnnounceReferenceNumber = sub.PublicationRewardDetail.AnnounceReferenceNumber
			}
		}
	}

	// build response
	resp := gin.H{
		"submission": gin.H{
			"submission_id":             sub.SubmissionID,
			"submission_number":         sub.SubmissionNumber,
			"submission_type":           sub.SubmissionType,
			"user_id":                   sub.UserID,
			"year_id":                   sub.YearID,
			"category_id":               sub.CategoryID,
			"subcategory_id":            sub.SubcategoryID,
			"subcategory_budget_id":     sub.SubcategoryBudgetID,
			"status_id":                 sub.StatusID,
			"created_at":                sub.CreatedAt,
			"submitted_at":              sub.SubmittedAt,
			"updated_at":                sub.UpdatedAt,
			"approved_at":               sub.ApprovedAt,
			"approved_by":               sub.ApprovedBy,
			"announce_reference_number": sub.AnnounceReferenceNumber, // ★ สำคัญ
			"user":                      sub.User,
			"year":                      sub.Year,
			"status":                    sub.Status,
		},
		"details":          nil,
		"submission_users": []gin.H{},
		"documents":        []gin.H{},
	}

	// details
	if sub.SubmissionType == "publication_reward" && sub.PublicationRewardDetail != nil {
		resp["details"] = gin.H{
			"type": "publication_reward",
			"data": sub.PublicationRewardDetail,
		}
	} else if sub.SubmissionType == "fund_application" && sub.FundApplicationDetail != nil {
		resp["details"] = gin.H{
			"type": "fund_application",
			"data": sub.FundApplicationDetail,
		}
	}

	// submission users
	for _, su := range subUsers {
		if su.User == nil {
			var u models.User
			if err := config.DB.Where("user_id = ?", su.UserID).First(&u).Error; err == nil {
				su.User = &u
			} else {
				continue
			}
		}
		resp["submission_users"] = append(resp["submission_users"].([]gin.H), gin.H{
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

	// docs
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
		resp["documents"] = append(resp["documents"].([]gin.H), item)
	}

	c.JSON(http.StatusOK, resp)
}

// ---------------------------
// POST /api/v1/admin/submissions/:id/approve
// ---------------------------
// - เปลี่ยนสถานะ → อนุมัติ (2), บันทึก approved_at/by, announce_reference_number
// - ถ้าเป็น publication_reward → บันทึกจำนวนเงินอนุมัติ + sync announce_reference_number ลง detail
// - commit แล้ว re-query รายละเอียดส่งกลับ
func ApproveSubmission(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	uidAny, _ := c.Get("userID")
	var uidPtr *int
	if uid, ok := uidAny.(int); ok {
		uidPtr = &uid
	}

	var req struct {
		RewardApproveAmount         *float64 `json:"reward_approve_amount"`
		RevisionFeeApproveAmount    *float64 `json:"revision_fee_approve_amount"`
		PublicationFeeApproveAmount *float64 `json:"publication_fee_approve_amount"`
		TotalApproveAmount          *float64 `json:"total_approve_amount"`
		AnnounceReferenceNumber     string   `json:"announce_reference_number"`

		ApprovedAmount  *float64 `json:"approved_amount"`  // legacy
		ApprovalComment string   `json:"approval_comment"` // legacy
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

	// load submission (+ detail)
	var sub models.Submission
	if err := tx.Preload("PublicationRewardDetail").
		Where("submission_id = ? AND deleted_at IS NULL", id).
		First(&sub).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Only pending or revision-requested
	if sub.StatusID != 1 && sub.StatusID != 4 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only pending or revision-requested submissions can be approved"})
		return
	}

	now := time.Now()

	// update submissions
	sub.StatusID = 2
	sub.ApprovedAt = &now
	sub.ApprovedBy = uidPtr
	if x := strings.TrimSpace(req.AnnounceReferenceNumber); x != "" {
		sub.AnnounceReferenceNumber = x
	}
	if err := tx.Save(&sub).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission status"})
		return
	}

	// publication_reward: update detail
	if sub.SubmissionType == "publication_reward" {
		var d models.PublicationRewardDetail
		if sub.PublicationRewardDetail != nil {
			d = *sub.PublicationRewardDetail
		} else {
			// fetch or create shell
			if err := tx.Where("submission_id = ?", id).First(&d).Error; err != nil {
				// ไม่มี record -> สร้างใหม่ (กันกรณีพิเศษ)
				d.SubmissionID = id
				if err := tx.Create(&d).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare reward detail"})
					return
				}
			}
		}

		updates := map[string]interface{}{
			"approved_at":               now,
			"approved_by":               uidPtr,
			"announce_reference_number": strings.TrimSpace(req.AnnounceReferenceNumber),
			"update_at":                 now,
		}

		// รองรับ flow ใหม่ (granular) และ legacy (approved_amount)
		if req.RewardApproveAmount != nil {
			updates["reward_approve_amount"] = *req.RewardApproveAmount
		}
		if req.RevisionFeeApproveAmount != nil {
			updates["revision_fee_approve_amount"] = *req.RevisionFeeApproveAmount
		}
		if req.PublicationFeeApproveAmount != nil {
			updates["publication_fee_approve_amount"] = *req.PublicationFeeApproveAmount
		}
		if req.TotalApproveAmount != nil {
			updates["total_approve_amount"] = *req.TotalApproveAmount
		}
		if req.ApprovedAmount != nil {
			// legacy single-field
			updates["total_approve_amount"] = *req.ApprovedAmount
		}

		if err := tx.Model(&models.PublicationRewardDetail{}).
			Where("submission_id = ?", id).
			Updates(updates).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update reward detail"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Commit failed"})
		return
	}

	// Re-query details for fresh UI state
	c.Params = gin.Params{gin.Param{Key: "id", Value: idStr}}
	GetSubmissionDetails(c)
}

// ---------------------------
// PATCH /api/v1/admin/submissions/:id/publication-reward/approval-amounts
// ---------------------------
// ใช้สำหรับแก้ "จำนวนเงินอนุมัติ" ของ Publication Reward เฉย ๆ (ไม่เปลี่ยนสถานะ)
// เสร็จแล้ว re-query รายละเอียดส่งกลับเหมือนเดิม
func UpdatePublicationRewardApprovalAmounts(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var req struct {
		RewardApproveAmount         *float64 `json:"reward_approve_amount"`
		RevisionFeeApproveAmount    *float64 `json:"revision_fee_approve_amount"`
		PublicationFeeApproveAmount *float64 `json:"publication_fee_approve_amount"`
		TotalApproveAmount          *float64 `json:"total_approve_amount"`
		ApprovalComment             string   `json:"approval_comment"` // หากมีใช้งานอยู่
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

	// โหลด submission + detail
	var sub models.Submission
	if err := tx.Preload("PublicationRewardDetail").
		Where("submission_id = ? AND deleted_at IS NULL", id).
		First(&sub).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	if sub.SubmissionType != "publication_reward" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only publication_reward submissions are supported"})
		return
	}

	// มี/ไม่มี record ก็อัปเดตแบบ WHERE submission_id
	updates := map[string]interface{}{}
	if req.RewardApproveAmount != nil {
		updates["reward_approve_amount"] = *req.RewardApproveAmount
	}
	if req.RevisionFeeApproveAmount != nil {
		updates["revision_fee_approve_amount"] = *req.RevisionFeeApproveAmount
	}
	if req.PublicationFeeApproveAmount != nil {
		updates["publication_fee_approve_amount"] = *req.PublicationFeeApproveAmount
	}
	if req.TotalApproveAmount != nil {
		updates["total_approve_amount"] = *req.TotalApproveAmount
	}
	// ถ้าไม่มีอะไรให้อัปเดต
	if len(updates) == 0 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}

	// ensure row exists
	var d models.PublicationRewardDetail
	if err := tx.Where("submission_id = ?", id).First(&d).Error; err != nil {
		// ถ้าไม่เจอให้สร้าง shell
		d.SubmissionID = id // NOTE: ใช้ int ให้ตรง type model
		if err := tx.Create(&d).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare reward detail"})
			return
		}
	}

	if err := tx.Model(&models.PublicationRewardDetail{}).
		Where("submission_id = ?", id).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update approval amounts"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Commit failed"})
		return
	}

	// ส่งรายละเอียดล่าสุดกลับ
	c.Params = gin.Params{gin.Param{Key: "id", Value: idStr}}
	GetSubmissionDetails(c)
}

// ---------------------------
// POST /api/v1/admin/submissions/:id/reject
// ---------------------------
// เปลี่ยนสถานะเป็น "ปฏิเสธ" (สมมติ status_id = 3)
// - เคลียร์ announce_reference_number ใน submission (กันโชว์ผิด)
// - เคลียร์เลขใน detail ไว้ด้วย (เพื่อความสอดคล้องกับ GetSubmissionDetails)
// เสร็จแล้ว re-query รายละเอียดส่งกลับ
func RejectSubmission(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var req struct {
		Reason string `json:"reason"` // เผื่อ routes เดิมส่งมา แต่ไม่บังคับใช้ใน model
	}
	_ = c.ShouldBindJSON(&req)

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var sub models.Submission
	if err := tx.Preload("PublicationRewardDetail").
		Where("submission_id = ? AND deleted_at IS NULL", id).
		First(&sub).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// อนุญาต reject เฉพาะจากสถานะรอดำเนินการ/ให้แก้ไข (1/4); ถ้าอนุมัติแล้ว (2) ก็ไม่ให้ reject
	if sub.StatusID == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Approved submission cannot be rejected"})
		return
	}

	// เปลี่ยนสถานะเป็นปฏิเสธ (3)
	sub.StatusID = 3
	// เคลียร์ข้อมูลอนุมัติที่อาจหลงเหลือ
	sub.ApprovedAt = nil
	sub.ApprovedBy = nil
	sub.AnnounceReferenceNumber = ""

	if err := tx.Save(&sub).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject submission"})
		return
	}

	// ถ้าเป็น publication_reward: เคลียร์เลขอ้างอิงใน detail ด้วย (กันเผลอโชว์)
	if sub.SubmissionType == "publication_reward" {
		if err := tx.Model(&models.PublicationRewardDetail{}).
			Where("submission_id = ?", id).
			Updates(map[string]interface{}{
				"announce_reference_number": "",
				"approved_by":               nil,
				"approved_at":               nil,
			}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update reward detail on reject"})
			return
		}
	}

	// ถ้ามีการบันทึกเหตุผลการ reject ในระบบเดิม สามารถเพิ่ม table/log ได้ที่นี่ (ข้ามไว้เพื่อความเข้ากันได้)

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Commit failed"})
		return
	}

	// ส่งรายละเอียดล่าสุดกลับ
	c.Params = gin.Params{gin.Param{Key: "id", Value: idStr}}
	GetSubmissionDetails(c)
}
