package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var deptStatusLabels = map[string]string{
	"pending":     services.StatusDeptHeadPendingLabel,
	"agree":       services.StatusDeptHeadRecommendedLabel,
	"recommended": services.StatusDeptHeadRecommendedLabel,
	"disagree":    services.StatusDeptHeadRejectedLabel,
	"rejected":    services.StatusDeptHeadRejectedLabel,
}

// GetDeptHeadReviewSubmissions lists submissions currently waiting for department head action.
func GetDeptHeadReviewSubmissions(c *gin.Context) {
	statusKey := strings.TrimSpace(c.Query("status"))
	if statusKey == "" {
		statusKey = "pending"
	}

	label, ok := deptStatusLabels[statusKey]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status filter"})
		return
	}

	statusID, err := services.GetStatusIDByName(label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var submissions []models.Submission
	query := config.DB.Preload("User").
		Preload("Year").
		Preload("Status").
		Preload("Category").
		Preload("Subcategory").
		Preload("FundApplicationDetail").
		Preload("PublicationRewardDetail").
		Where("deleted_at IS NULL").
		Where("status_id = ?", statusID)

	if err := query.Order("submitted_at DESC NULLS LAST, updated_at DESC").Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	enrichSubmissionDetails(submissions)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"submissions": submissions,
		"total":       len(submissions),
	})
}

// DeptHeadDecision handles agree/disagree decisions from department heads.
func DeptHeadDecision(c *gin.Context) {
	submissionIDParam := c.Param("id")
	submissionID, err := strconv.Atoi(submissionIDParam)
	if err != nil || submissionID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission ID"})
		return
	}

	var req struct {
		Decision string `json:"decision" binding:"required"`
		Comment  string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	decision := strings.ToLower(strings.TrimSpace(req.Decision))
	if decision != "agree" && decision != "disagree" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Decision must be either 'agree' or 'disagree'"})
		return
	}

	statusNames := []string{
		services.StatusDeptHeadPendingLabel,
		services.StatusDeptHeadRecommendedLabel,
		services.StatusDeptHeadRejectedLabel,
	}
	statusIDs, err := services.GetStatusIDsByNames(statusNames)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pendingID := statusIDs[services.StatusDeptHeadPendingLabel]
	recommendedID := statusIDs[services.StatusDeptHeadRecommendedLabel]
	rejectedID := statusIDs[services.StatusDeptHeadRejectedLabel]
	if pendingID == 0 || recommendedID == 0 || rejectedID == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Department review statuses are not configured"})
		return
	}

	userIDValue, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User context missing"})
		return
	}
	userID, ok := userIDValue.(int)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user context"})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var submission models.Submission
	if err := tx.Preload("User").Where("submission_id = ? AND deleted_at IS NULL", submissionID).First(&submission).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load submission"})
		return
	}

	if submission.StatusID != pendingID {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"error": "Submission is not awaiting department review"})
		return
	}

	targetStatusID := recommendedID
	reviewStatus := "approved"
	message := "Submission marked for admin consideration"
	if decision == "disagree" {
		targetStatusID = rejectedID
		reviewStatus = "rejected"
		message = "Submission rejected by department head"
	}

	now := time.Now()
	comment := strings.TrimSpace(req.Comment)

	if err := tx.Model(&models.Submission{}).
		Where("submission_id = ?", submission.SubmissionID).
		Updates(map[string]interface{}{
			"status_id":  targetStatusID,
			"updated_at": now,
		}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	var reviewRound int64
	if err := tx.Model(&models.SubmissionReview{}).
		Where("submission_id = ?", submission.SubmissionID).
		Count(&reviewRound).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record review"})
		return
	}

	review := models.SubmissionReview{
		SubmissionID: submission.SubmissionID,
		ReviewerID:   userID,
		ReviewRound:  int(reviewRound) + 1,
		ReviewStatus: reviewStatus,
		ReviewedAt:   now,
	}
	if comment != "" {
		review.Comments = &comment
	}
	note := fmt.Sprintf("role=dept_head;decision=%s", decision)
	review.InternalNotes = &note

	if err := tx.Create(&review).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save review record"})
		return
	}

	oldStatus := submission.StatusID
	history := models.SubmissionStatusHistory{
		SubmissionID: submission.SubmissionID,
		OldStatusID:  &oldStatus,
		NewStatusID:  targetStatusID,
		ChangedBy:    userID,
		CreatedAt:    now,
	}
	if comment != "" {
		history.Reason = &comment
	}
	historyNote := fmt.Sprintf("dept_head_decision:%s", decision)
	history.Notes = &historyNote

	if err := tx.Create(&history).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to log status history"})
		return
	}

	auditValues := map[string]interface{}{
		"decision":  decision,
		"comment":   comment,
		"status_id": targetStatusID,
	}
	serialized, _ := json.Marshal(auditValues)
	entityID := submission.SubmissionID
	audit := models.AuditLog{
		UserID:       userID,
		Action:       "review",
		EntityType:   "submission",
		EntityID:     &entityID,
		EntityNumber: nil,
		NewValues:    ptr(string(serialized)),
		Description:  ptr(message),
		IPAddress:    c.ClientIP(),
	}
	if submission.SubmissionNumber != "" {
		number := submission.SubmissionNumber
		audit.EntityNumber = &number
	}
	userAgent := c.GetHeader("User-Agent")
	if strings.TrimSpace(userAgent) != "" {
		ua := userAgent
		audit.UserAgent = &ua
	}

	if err := tx.Create(&audit).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write audit log"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize decision"})
		return
	}

	var updated models.Submission
	if err := config.DB.Preload("User").Preload("Year").Preload("Status").
		Preload("Category").Preload("Subcategory").
		Preload("FundApplicationDetail").Preload("PublicationRewardDetail").
		First(&updated, submission.SubmissionID).Error; err == nil {
		enrichSubmissionDetails([]models.Submission{updated})
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"message":    message,
			"submission": updated,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
	})
}

func ptr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func enrichSubmissionDetails(submissions []models.Submission) {
	for i := range submissions {
		submission := &submissions[i]
		switch submission.SubmissionType {
		case "fund_application":
			if submission.FundApplicationDetail == nil {
				detail := &models.FundApplicationDetail{}
				if err := config.DB.Preload("Subcategory.Category").
					Where("submission_id = ?", submission.SubmissionID).
					First(detail).Error; err == nil {
					submission.FundApplicationDetail = detail
				}
			}
			if submission.FundApplicationDetail != nil {
				if submission.StatusID != 2 {
					submission.FundApplicationDetail.AnnounceReferenceNumber = ""
				}
				submission.AnnounceReferenceNumber = submission.FundApplicationDetail.AnnounceReferenceNumber
			}
		case "publication_reward":
			if submission.PublicationRewardDetail == nil {
				detail := &models.PublicationRewardDetail{}
				if err := config.DB.Where("submission_id = ?", submission.SubmissionID).
					First(detail).Error; err == nil {
					submission.PublicationRewardDetail = detail
				}
			}
			if submission.PublicationRewardDetail != nil {
				if submission.StatusID != 2 {
					submission.PublicationRewardDetail.AnnounceReferenceNumber = ""
				}
				submission.AnnounceReferenceNumber = submission.PublicationRewardDetail.AnnounceReferenceNumber
			}
		}
	}
}
