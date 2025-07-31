// controllers/coauthors.go - Co-authors Management Controllers

package controllers

import (
	"net/http"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
)

// ===================== CO-AUTHORS MANAGEMENT =====================

// AddCoauthor adds a co-author to a submission
func AddCoauthor(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type AddCoauthorRequest struct {
		UserID       int    `json:"user_id" binding:"required"`
		Role         string `json:"role"`          // "coauthor", "supervisor", "collaborator"
		DisplayOrder int    `json:"display_order"` // ลำดับผู้แต่ง
	}

	var req AddCoauthorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set default role
	if req.Role == "" {
		req.Role = "coauthor"
	}

	// Validate role
	validRoles := map[string]bool{
		"coauthor":     true,
		"supervisor":   true,
		"collaborator": true,
	}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}

	// Find submission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Check if submission is editable
	if !submission.IsEditable() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify submitted submission"})
		return
	}

	// Validate co-author user exists
	var coauthor models.User
	if err := config.DB.Where("user_id = ? AND delete_at IS NULL", req.UserID).First(&coauthor).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Co-author user not found"})
		return
	}

	// Check if user is already a co-author
	var existingCoauthor models.SubmissionUser
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, req.UserID).First(&existingCoauthor).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User is already a co-author"})
		return
	}

	// Prevent adding submission owner as co-author
	if submission.UserID == req.UserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot add submission owner as co-author"})
		return
	}

	// Auto-assign display order if not provided
	if req.DisplayOrder == 0 {
		var maxOrder int
		config.DB.Model(&models.SubmissionUser{}).
			Where("submission_id = ?", submissionID).
			Select("COALESCE(MAX(display_order), 1)").
			Scan(&maxOrder)
		req.DisplayOrder = maxOrder + 1
	}

	// Create submission user
	now := time.Now()
	submissionUser := models.SubmissionUser{
		SubmissionID: submission.SubmissionID,
		UserID:       req.UserID,
		Role:         req.Role,
		IsPrimary:    false,            // เพิ่ม field นี้
		DisplayOrder: req.DisplayOrder, // ใช้ DisplayOrder แทน OrderSequence
		CreatedAt:    now,
	}

	if err := config.DB.Create(&submissionUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add co-author"})
		return
	}

	// Load relations for response
	config.DB.Preload("User").First(&submissionUser, submissionUser.ID)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Co-author added successfully",
		"coauthor": submissionUser,
	})
}

// GetCoauthors returns all co-authors for a submission
func GetCoauthors(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Find submission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Get co-authors
	var coauthors []models.SubmissionUser
	if err := config.DB.Preload("User").
		Where("submission_id = ?", submissionID).
		Order("display_order ASC, created_at ASC").
		Find(&coauthors).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch co-authors"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"coauthors": coauthors,
		"total":     len(coauthors),
	})
}

// UpdateCoauthor updates a co-author's information
func UpdateCoauthor(c *gin.Context) {
	submissionID := c.Param("id")
	coauthorUserID := c.Param("user_id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type UpdateCoauthorRequest struct {
		Role         string `json:"role"`
		DisplayOrder int    `json:"display_order"`
	}

	var req UpdateCoauthorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find submission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Check if submission is editable
	if !submission.IsEditable() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify submitted submission"})
		return
	}

	// Find co-author
	var submissionUser models.SubmissionUser
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, coauthorUserID).First(&submissionUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Co-author not found"})
		return
	}

	// Validate role if provided
	if req.Role != "" {
		validRoles := map[string]bool{
			"coauthor":     true,
			"supervisor":   true,
			"collaborator": true,
		}
		if !validRoles[req.Role] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
			return
		}
		submissionUser.Role = req.Role
	}

	if err := config.DB.Save(&submissionUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update co-author"})
		return
	}

	// Load relations for response
	config.DB.Preload("User").First(&submissionUser, submissionUser.ID)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Co-author updated successfully",
		"coauthor": submissionUser,
	})
}

// RemoveCoauthor removes a co-author from a submission
func RemoveCoauthor(c *gin.Context) {
	submissionID := c.Param("id")
	coauthorUserID := c.Param("user_id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Find submission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Check if submission is editable
	if !submission.IsEditable() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify submitted submission"})
		return
	}

	// Find and delete co-author
	var submissionUser models.SubmissionUser
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, coauthorUserID).First(&submissionUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Co-author not found"})
		return
	}

	if err := config.DB.Delete(&submissionUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove co-author"})
		return
	}

	// Reorder remaining co-authors
	var remainingCoauthors []models.SubmissionUser
	config.DB.Where("submission_id = ?", submissionID).
		Order("order_sequence ASC").
		Find(&remainingCoauthors)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Co-author removed successfully",
	})
}

// GetSubmissionWithCoauthors returns submission with all co-authors
func GetSubmissionWithCoauthors(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var submission models.Submission
	query := config.DB.Preload("User").Preload("Year").Preload("Status").
		Preload("SubmissionUsers.User").
		Preload("Documents.File").Preload("Documents.DocumentType")

	// Check permission
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.Where("submission_id = ? AND deleted_at IS NULL", submissionID).First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Load type-specific details
	switch submission.SubmissionType {
	case "fund_application":
		var fundDetail models.FundApplicationDetail
		if err := config.DB.Preload("Subcategory").Where("submission_id = ?", submission.SubmissionID).First(&fundDetail).Error; err == nil {
			submission.FundApplicationDetail = &fundDetail
		}
	case "publication_reward":
		var pubDetail models.PublicationRewardDetail
		if err := config.DB.Where("submission_id = ?", submission.SubmissionID).First(&pubDetail).Error; err == nil {
			submission.PublicationRewardDetail = &pubDetail
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"submission": submission,
	})
}
