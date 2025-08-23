// controllers/admin_submission.go
package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GetSubmissionDetails - ดึงข้อมูล submission แบบละเอียด
func GetSubmissionDetails(c *gin.Context) {
	submissionID := c.Param("id")

	// Validate submissionID
	if submissionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission ID is required"})
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

	// ดึง submission users (co-authors) พร้อม error handling ที่ดีขึ้น
	var submissionUsers []models.SubmissionUser
	if err := config.DB.Where("submission_id = ?", submissionID).
		Preload("User").
		Order("display_order ASC").
		Find(&submissionUsers).Error; err != nil {
		// Log error but don't fail the whole request
		// submissionUsers will be empty slice
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
			"submission_id":     submission.SubmissionID,
			"submission_number": submission.SubmissionNumber,
			"submission_type":   submission.SubmissionType,
			"user_id":           submission.UserID,
			"year_id":           submission.YearID,
			"status_id":         submission.StatusID,
			"submitted_at":      submission.SubmittedAt,
			"created_at":        submission.CreatedAt,
			"updated_at":        submission.UpdatedAt,
			"user":              submission.User,
			"year":              submission.Year,
			"status":            submission.Status,
		},
		"details":          nil,
		"submission_users": []gin.H{},
		"documents":        []gin.H{},
	}

	// เพิ่มรายละเอียดตาม submission type
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

	// Format submission users - เพิ่ม NIL CHECK
	for _, su := range submissionUsers {
		// ✅ เพิ่ม nil check เพื่อป้องกัน panic
		if su.User == nil {
			// ถ้า User เป็น nil ให้พยายามโหลดข้อมูล User แยก
			var user models.User
			if err := config.DB.Where("user_id = ?", su.UserID).First(&user).Error; err == nil {
				su.User = &user
			} else {
				// ถ้าโหลด User ไม่ได้ให้ skip หรือใส่ข้อมูลเปล่า
				continue // หรือใส่ข้อมูลเปล่าแทน
			}
		}

		userInfo := gin.H{
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
		}
		response["submission_users"] = append(response["submission_users"].([]gin.H), userInfo)
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

		// Add document type info if available
		if doc.DocumentType.DocumentTypeID != 0 {
			docInfo["document_type"] = gin.H{
				"document_type_id":   doc.DocumentType.DocumentTypeID,
				"document_type_name": doc.DocumentType.DocumentTypeName,
				"required":           doc.DocumentType.Required,
			}
		}

		// Add file info if available
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

// ApproveSubmission - อนุมัติ submission พร้อมระบุจำนวนเงิน
func ApproveSubmission(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")

	var request struct {
		ApprovedAmount  float64 `json:"approved_amount" binding:"required"`
		ApprovalComment string  `json:"approval_comment"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	// เริ่ม transaction
	tx := config.DB.Begin()

	// อัปเดต submission status
	var submission models.Submission
	if err := tx.First(&submission, submissionID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// ตรวจสอบสถานะปัจจุบัน
	if submission.StatusID != 1 && submission.StatusID != 4 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only pending or revision-requested submissions can be approved"})
		return
	}

	// อัปเดต status เป็น approved (status_id = 2)
	now := time.Now()
	submission.StatusID = 2
	submission.UpdatedAt = now
	approvedByID := userID.(int)
	submission.ApprovedBy = &approvedByID
	submission.ApprovedAt = &now

	if err := tx.Save(&submission).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission status"})
		return
	}

	// อัปเดตจำนวนเงินที่อนุมัติตาม submission type
	if submission.SubmissionType == "publication_reward" {
		var detail models.PublicationRewardDetail
		if err := tx.Where("submission_id = ?", submissionID).First(&detail).Error; err == nil {
			detail.ApprovedAmount = &request.ApprovedAmount
			detail.ApprovalComment = &request.ApprovalComment
			detail.ApprovedAt = &now
			detail.ApprovedBy = &approvedByID
			detail.UpdateAt = now
			tx.Save(&detail)
		}
	} else if submission.SubmissionType == "fund_application" {
		var detail models.FundApplicationDetail
		if err := tx.Where("submission_id = ?", submissionID).First(&detail).Error; err == nil {
			detail.ApprovedAmount = request.ApprovedAmount
			detail.Comment = request.ApprovalComment
			detail.ClosedAt = &now
			detail.ApprovedBy = &approvedByID
			detail.ApprovedAt = &now
			detail.UpdateAt = now
			tx.Save(&detail)
		}
	}

	// บันทึก audit log
	auditLog := models.AuditLog{
		UserID:       userID.(int),
		Action:       "approve",
		EntityType:   "submission",
		EntityID:     &submission.SubmissionID,
		EntityNumber: &submission.SubmissionNumber,
		Description:  &request.ApprovalComment,
		IPAddress:    c.ClientIP(),
		CreatedAt:    now,
	}
	tx.Create(&auditLog)

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve submission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Submission approved successfully",
		"submission": gin.H{
			"submission_id":     submission.SubmissionID,
			"submission_number": submission.SubmissionNumber,
			"status_id":         submission.StatusID,
			"approved_amount":   request.ApprovedAmount,
		},
	})
}

// RejectSubmission - ปฏิเสธ submission พร้อมเหตุผล
func RejectSubmission(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")

	var request struct {
		RejectionReason string `json:"rejection_reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rejection reason is required"})
		return
	}

	// เริ่ม transaction
	tx := config.DB.Begin()

	// อัปเดต submission status
	var submission models.Submission
	if err := tx.First(&submission, submissionID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// ตรวจสอบสถานะปัจจุบัน
	if submission.StatusID != 1 && submission.StatusID != 4 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only pending or revision-requested submissions can be rejected"})
		return
	}

	// อัปเดต status เป็น rejected (status_id = 3)
	now := time.Now()
	submission.StatusID = 3
	submission.UpdatedAt = now

	if err := tx.Save(&submission).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission status"})
		return
	}

	// บันทึกเหตุผลการปฏิเสธ
	if submission.SubmissionType == "publication_reward" {
		var detail models.PublicationRewardDetail
		if err := tx.Where("submission_id = ?", submissionID).First(&detail).Error; err == nil {
			detail.RejectionReason = &request.RejectionReason
			detail.RejectedAt = &now
			rejectedByID := userID.(int)
			detail.RejectedBy = &rejectedByID
			detail.UpdateAt = now
			tx.Save(&detail)
		}
	} else if submission.SubmissionType == "fund_application" {
		var detail models.FundApplicationDetail
		if err := tx.Where("submission_id = ?", submissionID).First(&detail).Error; err == nil {
			detail.Comment = request.RejectionReason
			detail.ClosedAt = &now
			rejectedByID := userID.(int)
			detail.RejectedBy = &rejectedByID
			detail.RejectedAt = &now
			detail.UpdateAt = now
			tx.Save(&detail)
		}
	}

	// บันทึก audit log
	auditLog := models.AuditLog{
		UserID:       userID.(int),
		Action:       "reject",
		EntityType:   "submission",
		EntityID:     &submission.SubmissionID,
		EntityNumber: &submission.SubmissionNumber,
		Description:  &request.RejectionReason,
		IPAddress:    c.ClientIP(),
		CreatedAt:    now,
	}
	tx.Create(&auditLog)

	// Commit transaction
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
			"rejection_reason":  request.RejectionReason,
		},
	})
}

// RequestRevision - ขอข้อมูลเพิ่มเติม
func RequestRevision(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")

	var request struct {
		RevisionRequest string `json:"revision_request" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Revision request details are required"})
		return
	}

	// เริ่ม transaction
	tx := config.DB.Begin()

	// อัปเดต submission status
	var submission models.Submission
	if err := tx.First(&submission, submissionID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// ตรวจสอบสถานะปัจจุบัน
	if submission.StatusID != 1 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only pending submissions can be requested for revision"})
		return
	}

	// อัปเดต status เป็น revision required (status_id = 4)
	now := time.Now()
	submission.StatusID = 4
	submission.UpdatedAt = now

	if err := tx.Save(&submission).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission status"})
		return
	}

	// บันทึกข้อมูลที่ต้องการเพิ่มเติม
	if submission.SubmissionType == "publication_reward" {
		var detail models.PublicationRewardDetail
		if err := tx.Where("submission_id = ?", submissionID).First(&detail).Error; err == nil {
			detail.RevisionRequest = &request.RevisionRequest
			detail.RevisionRequestedAt = &now
			requestedByID := userID.(int)
			detail.RevisionRequestedBy = &requestedByID
			detail.UpdateAt = now
			tx.Save(&detail)
		}
	} else if submission.SubmissionType == "fund_application" {
		var detail models.FundApplicationDetail
		if err := tx.Where("submission_id = ?", submissionID).First(&detail).Error; err == nil {
			// บันทึกใน comment field
			revisionComment := "ต้องการข้อมูลเพิ่มเติม: " + request.RevisionRequest
			detail.Comment = revisionComment
			detail.UpdateAt = now
			tx.Save(&detail)
		}
	}

	// บันทึก audit log
	auditLog := models.AuditLog{
		UserID:       userID.(int),
		Action:       "review",
		EntityType:   "submission",
		EntityID:     &submission.SubmissionID,
		EntityNumber: &submission.SubmissionNumber,
		Description:  &request.RevisionRequest,
		IPAddress:    c.ClientIP(),
		CreatedAt:    now,
	}
	tx.Create(&auditLog)

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to request revision"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Revision requested successfully",
		"submission": gin.H{
			"submission_id":     submission.SubmissionID,
			"submission_number": submission.SubmissionNumber,
			"status_id":         submission.StatusID,
			"revision_request":  request.RevisionRequest,
		},
	})
}

// ExportSubmissions - Export submissions เป็น PDF หรือ DOCX
func ExportSubmissions(c *gin.Context) {
	// Query parameters
	exportFormat := c.DefaultQuery("format", "pdf") // pdf หรือ docx
	yearID := c.Query("year_id")
	statusID := c.Query("status_id")
	submissionType := c.Query("type")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	// Build query
	query := config.DB.Model(&models.Submission{}).
		Preload("User").
		Preload("Year").
		Preload("Status").
		Where("deleted_at IS NULL")

	// Apply filters
	if yearID != "" {
		query = query.Where("year_id = ?", yearID)
	}
	if statusID != "" {
		query = query.Where("status_id = ?", statusID)
	}
	if submissionType != "" {
		query = query.Where("submission_type = ?", submissionType)
	}
	if dateFrom != "" && dateTo != "" {
		query = query.Where("DATE(created_at) BETWEEN ? AND ?", dateFrom, dateTo)
	}

	// Get submissions
	var submissions []models.Submission
	if err := query.Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	// เตรียมข้อมูลสำหรับ export
	exportData := []map[string]interface{}{}
	for _, sub := range submissions {
		data := map[string]interface{}{
			"submission_number": sub.SubmissionNumber,
			"submission_type":   sub.SubmissionType,
			"user_name":         sub.User.UserFname + " " + sub.User.UserLname,
			"year":              sub.Year.Year,
			"status":            sub.Status.StatusName,
			"submitted_at":      sub.SubmittedAt,
			"created_at":        sub.CreatedAt,
		}

		// เพิ่มข้อมูลเฉพาะตาม type
		if sub.SubmissionType == "publication_reward" {
			var detail models.PublicationRewardDetail
			if err := config.DB.Where("submission_id = ?", sub.SubmissionID).First(&detail).Error; err == nil {
				data["paper_title"] = detail.PaperTitle
				data["journal_name"] = detail.JournalName
				data["journal_quartile"] = detail.Quartile
				if detail.ApprovedAmount != nil {
					data["approved_amount"] = *detail.ApprovedAmount
				}
			}
		} else if sub.SubmissionType == "fund_application" {
			var detail models.FundApplicationDetail
			if err := config.DB.Where("submission_id = ?", sub.SubmissionID).First(&detail).Error; err == nil {
				data["project_title"] = detail.ProjectTitle
				data["requested_amount"] = detail.RequestedAmount
				data["approved_amount"] = detail.ApprovedAmount
			}
		}

		exportData = append(exportData, data)
	}

	// ส่งข้อมูลกลับเป็น JSON ก่อน (ต้องใช้ library เพิ่มเติมสำหรับสร้าง PDF/DOCX จริง)
	// ในการ implement จริง จะต้องใช้ library เช่น:
	// - PDF: github.com/jung-kurt/gofpdf หรือ github.com/johnfercher/maroto
	// - DOCX: github.com/lukasjarosch/go-docx

	c.JSON(http.StatusOK, gin.H{
		"format":  exportFormat,
		"data":    exportData,
		"total":   len(exportData),
		"message": "Export data prepared. Implementation of PDF/DOCX generation required.",
	})
}
