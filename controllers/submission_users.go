// controllers/submission_users.go - Submission Users Management Controllers

package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
)

// ===================== SUBMISSION USERS MANAGEMENT =====================

// AddSubmissionUser adds a user to a submission (compatible with Frontend API)
func AddSubmissionUser(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type AddUserRequest struct {
		UserID        int    `json:"user_id" binding:"required"`
		Role          string `json:"role"`           // "co_author", "coauthor", "supervisor", etc.
		OrderSequence int    `json:"order_sequence"` // ลำดับผู้แต่ง
		IsActive      bool   `json:"is_active"`      // สถานะ active (Frontend ส่งมา)
	}

	var req AddUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Map Frontend role to Database role
	dbRole := mapFrontendRoleToDatabase(req.Role)

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

	// Validate user exists
	var user models.User
	if err := config.DB.Where("user_id = ? AND delete_at IS NULL", req.UserID).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}

	// Check if user is already in submission
	var existingUser models.SubmissionUser
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, req.UserID).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User is already in this submission"})
		return
	}

	// Prevent adding submission owner as co-author
	if submission.UserID == req.UserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot add submission owner as co-author"})
		return
	}

	// Auto-assign order sequence if not provided
	if req.OrderSequence == 0 {
		var maxOrder int
		config.DB.Model(&models.SubmissionUser{}).
			Where("submission_id = ?", submissionID).
			Select("COALESCE(MAX(display_order), 1)").
			Scan(&maxOrder)
		req.OrderSequence = maxOrder + 1
	}

	// Create submission user - ใช้ field names ที่ตรงกับ database
	now := time.Now()
	submissionUser := models.SubmissionUser{
		SubmissionID: submission.SubmissionID,
		UserID:       req.UserID,
		Role:         dbRole,            // ใช้ role ที่แปลงแล้ว
		IsPrimary:    false,             // co-author ไม่เป็น primary
		DisplayOrder: req.OrderSequence, // ใช้ display_order แทน order_sequence
		CreatedAt:    now,
	}

	if err := config.DB.Create(&submissionUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to add user to submission",
			"details": err.Error(),
		})
		return
	}

	// Load relations for response
	config.DB.Preload("User").First(&submissionUser, submissionUser.ID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User added to submission successfully",
		"user":    submissionUser,
	})
}

// GetSubmissionUsers returns all users for a submission (compatible with Frontend API)
func GetSubmissionUsers(c *gin.Context) {
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

	// Get all users
	var users []models.SubmissionUser
	if err := config.DB.Preload("User").
		Where("submission_id = ?", submissionID).
		Order("display_order ASC, created_at ASC").
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submission users"})
		return
	}

	// Transform for Frontend - แยก users ตาม role
	var coauthors []models.SubmissionUser
	var allUsers []models.SubmissionUser

	for _, user := range users {
		allUsers = append(allUsers, user)
		if user.Role == "coauthor" {
			coauthors = append(coauthors, user)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"users":     allUsers,  // ส่งผู้ใช้ทั้งหมด
		"coauthors": coauthors, // ส่ง co-authors เฉพาะสำหรับ backward compatibility
		"total":     len(allUsers),
	})
}

// UpdateSubmissionUser updates a user's information in submission
func UpdateSubmissionUser(c *gin.Context) {
	submissionID := c.Param("id")
	targetUserID := c.Param("user_id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type UpdateUserRequest struct {
		Role          string `json:"role"`
		OrderSequence int    `json:"order_sequence"`
		IsActive      bool   `json:"is_active"`
	}

	var req UpdateUserRequest
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

	// Find submission user
	var submissionUser models.SubmissionUser
	targetUserIDInt, _ := strconv.Atoi(targetUserID)
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, targetUserIDInt).First(&submissionUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found in submission"})
		return
	}

	// Update fields
	if req.Role != "" {
		submissionUser.Role = mapFrontendRoleToDatabase(req.Role)
	}

	if req.OrderSequence > 0 {
		submissionUser.DisplayOrder = req.OrderSequence
	}

	if err := config.DB.Save(&submissionUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	// Load relations for response
	config.DB.Preload("User").First(&submissionUser, submissionUser.ID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User updated successfully",
		"user":    submissionUser,
	})
}

// RemoveSubmissionUser removes a user from submission
func RemoveSubmissionUser(c *gin.Context) {
	submissionID := c.Param("id")
	targetUserID := c.Param("user_id")
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

	// Find and delete submission user
	targetUserIDInt, _ := strconv.Atoi(targetUserID)
	result := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, targetUserIDInt).Delete(&models.SubmissionUser{})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove user"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found in submission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User removed from submission successfully",
	})
}

// AddMultipleUsers adds multiple users to submission at once
func AddMultipleUsers(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type BatchUserRequest struct {
		Users []struct {
			UserID        int    `json:"user_id" binding:"required"`
			Role          string `json:"role"`
			OrderSequence int    `json:"order_sequence"`
			IsActive      bool   `json:"is_active"`
		} `json:"users" binding:"required"`
	}

	var req BatchUserRequest
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

	var results []models.SubmissionUser
	var errors []string

	for i, userReq := range req.Users {
		// Validate user exists
		var user models.User
		if err := config.DB.Where("user_id = ? AND delete_at IS NULL", userReq.UserID).First(&user).Error; err != nil {
			errors = append(errors, fmt.Sprintf("User %d not found", userReq.UserID))
			continue
		}

		// Check if already exists
		var existing models.SubmissionUser
		if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, userReq.UserID).First(&existing).Error; err == nil {
			errors = append(errors, fmt.Sprintf("User %d already in submission", userReq.UserID))
			continue
		}

		// Create submission user
		orderSequence := userReq.OrderSequence
		if orderSequence == 0 {
			orderSequence = i + 2 // Start from 2
		}

		submissionUser := models.SubmissionUser{
			SubmissionID: submission.SubmissionID,
			UserID:       userReq.UserID,
			Role:         mapFrontendRoleToDatabase(userReq.Role),
			IsPrimary:    false,
			DisplayOrder: orderSequence,
			CreatedAt:    time.Now(),
		}

		if err := config.DB.Create(&submissionUser).Error; err != nil {
			errors = append(errors, fmt.Sprintf("Failed to add user %d: %v", userReq.UserID, err))
			continue
		}

		// Load relations
		config.DB.Preload("User").First(&submissionUser, submissionUser.ID)
		results = append(results, submissionUser)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Batch operation completed",
		"added":   len(results),
		"total":   len(req.Users),
		"users":   results,
		"errors":  errors,
	})
}

// SetCoauthors replaces all existing co-authors with new ones
func SetCoauthors(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type SetCoauthorsRequest struct {
		Coauthors []struct {
			UserID        int    `json:"user_id" binding:"required"`
			Role          string `json:"role"`
			OrderSequence int    `json:"order_sequence"`
			IsActive      bool   `json:"is_active"`
		} `json:"coauthors" binding:"required"`
	}

	var req SetCoauthorsRequest
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

	// Start transaction
	tx := config.DB.Begin()

	// Delete existing co-authors (only co-authors, not other roles)
	if err := tx.Where("submission_id = ? AND role = ?", submissionID, "coauthor").Delete(&models.SubmissionUser{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove existing co-authors"})
		return
	}

	// Add new co-authors
	var results []models.SubmissionUser
	for i, coauthorReq := range req.Coauthors {
		orderSequence := coauthorReq.OrderSequence
		if orderSequence == 0 {
			orderSequence = i + 2 // Start from 2
		}

		submissionUser := models.SubmissionUser{
			SubmissionID: submission.SubmissionID,
			UserID:       coauthorReq.UserID,
			Role:         "coauthor", // Force to coauthor
			IsPrimary:    false,
			DisplayOrder: orderSequence,
			CreatedAt:    time.Now(),
		}

		if err := tx.Create(&submissionUser).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to add co-author",
				"details": err.Error(),
			})
			return
		}

		results = append(results, submissionUser)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit changes"})
		return
	}

	// Load relations for all results
	for i := range results {
		config.DB.Preload("User").First(&results[i], results[i].ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "Co-authors set successfully",
		"total":     len(results),
		"coauthors": results,
	})
}
