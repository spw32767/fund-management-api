// controllers/submission_details_shared.go
package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetSubmissionDetailsShared - รายละเอียดคำร้องแบบกลาง (admin/dephead ใช้ร่วมกัน)
// - ถ้า role = dept_head ให้ "redact" ฟิลด์จำนวนเงินอนุมัติใน PublicationRewardDetail
// - ซ่อนเลขอ้างอิงประกาศหากยังไม่อนุมัติ (เหมือนเดิม)
func GetSubmissionDetailsShared(c *gin.Context) {
	submissionID := c.Param("id")
	if strings.TrimSpace(submissionID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Submission ID is required"})
		return
	}

	// โหลด submission + preload relations
	var submission models.Submission
	q := config.DB.
		Preload("User").
		Preload("Year").
		Preload("Status").
		Preload("FundApplicationDetail").
		Preload("FundApplicationDetail.Subcategory").
		Preload("FundApplicationDetail.Subcategory.Category").
		Preload("PublicationRewardDetail")

	if err := q.First(&submission, submissionID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// โหลด co-authors
	var submissionUsers []models.SubmissionUser
	if err := config.DB.Where("submission_id = ?", submissionID).
		Preload("User").
		Order("display_order ASC").
		Find(&submissionUsers).Error; err != nil {
		submissionUsers = []models.SubmissionUser{}
	}

	// โหลดเอกสารแนบ
	var documents []models.SubmissionDocument
	config.DB.Where("submission_id = ?", submissionID).
		Preload("DocumentType").
		Preload("File").
		Find(&documents)

	// ==== Role-based redaction ====
	roleNameAny, _ := c.Get("roleName") // middleware ส่วนใหญ่จะใส่ roleName/roleID ไว้แล้ว
	roleName, _ := roleNameAny.(string)
	isDept := strings.EqualFold(roleName, "dept_head") || strings.EqualFold(roleName, "department_head")

	// สร้าง response format แบบเดียวกับ admin (คงโครงสร้างเดิม)
	resp := gin.H{
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

	// ใส่ details ตามประเภท พร้อมเงื่อนไขซ่อนเลขอ้างอิงประกาศ
	if submission.SubmissionType == "publication_reward" && submission.PublicationRewardDetail != nil {
		// ซ่อนเลขอ้างอิงประกาศหากยังไม่ได้อนุมัติ (status != 2) — พฤติกรรมเดียวกับ admin
		if submission.StatusID != 2 {
			submission.PublicationRewardDetail.AnnounceReferenceNumber = ""
		}
		// ถ้าเป็น Dept Head → ซ่อนจำนวนเงินอนุมัติทั้งหมด
		if isDept {
			submission.PublicationRewardDetail.RewardApproveAmount = 0
			submission.PublicationRewardDetail.RevisionFeeApproveAmount = 0
			submission.PublicationRewardDetail.PublicationFeeApproveAmount = 0
			submission.PublicationRewardDetail.TotalApproveAmount = 0
		}
		resp["details"] = gin.H{
			"type": "publication_reward",
			"data": submission.PublicationRewardDetail,
		}
	} else if submission.SubmissionType == "fund_application" && submission.FundApplicationDetail != nil {
		if submission.StatusID != 2 {
			submission.FundApplicationDetail.AnnounceReferenceNumber = ""
		}
		// (กรณี fund_application ตอนนี้ยังไม่มี field อนุมัติย่อยๆให้ซ่อน ถ้ามีให้เติมตรงนี้)
		resp["details"] = gin.H{
			"type": "fund_application",
			"data": submission.FundApplicationDetail,
		}
	}

	// แนบ co-authors
	for _, su := range submissionUsers {
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

	// แนบเอกสารแนบ
	for _, d := range documents {
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
