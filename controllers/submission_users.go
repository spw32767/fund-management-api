// ==== ใน controllers/ ====
// ลบไฟล์ controllers/coauthor.go ออกทั้งหมด (หรือ comment ออก)
// และใช้แค่ controllers/submission_users.go

// ==== ใน controllers/submission_users.go ====
// ปรับปรุง comments และ function descriptions ให้ชัดเจน

package controllers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
)

// ===================== SUBMISSION USERS MANAGEMENT (UNIFIED) =====================
// ใช้จัดการ co-authors, advisors, team members และ users อื่นๆ ใน submission

// Helper function to map frontend role to database role
func mapFrontendRoleToDatabase(frontendRole string) string {
	roleMap := map[string]string{
		"co_author":            "coauthor",
		"coauthor":             "coauthor",
		"team_member":          "team_member",
		"advisor":              "advisor",
		"coordinator":          "coordinator",
		"owner":                "owner",
		"first_author":         "owner", // Map to owner
		"corresponding_author": "owner", // Map to owner
		"":                     "coauthor",
	}

	if dbRole, exists := roleMap[frontendRole]; exists {
		return dbRole
	}
	return "coauthor" // default fallback
}

// AddSubmissionUser เพิ่ม user ลงใน submission (co-author, advisor, etc.)
func AddSubmissionUser(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type AddUserRequest struct {
		UserID        int    `json:"user_id" binding:"required"`
		Role          string `json:"role"`           // "coauthor", "advisor", "team_member", etc.
		OrderSequence int    `json:"order_sequence"` // ลำดับในการแสดงผล
		IsActive      bool   `json:"is_active"`      // สถานะ active
	}

	var req AddUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	log.Printf("AddSubmissionUser: Adding user %d to submission %s with role %s",
		req.UserID, submissionID, req.Role)

	// Map role
	dbRole := mapFrontendRoleToDatabase(req.Role)

	// Find submission and check permission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

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

	// Prevent adding submission owner as co-author (but allow other roles)
	if submission.UserID == req.UserID && dbRole == "coauthor" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot add submission owner as co-author"})
		return
	}

	// Auto-assign order sequence if not provided
	orderSequence := req.OrderSequence
	if orderSequence == 0 {
		var maxOrder int
		config.DB.Model(&models.SubmissionUser{}).
			Where("submission_id = ?", submissionID).
			Select("COALESCE(MAX(display_order), 1)").
			Scan(&maxOrder)
		orderSequence = maxOrder + 1
	}

	// Create submission user
	submissionUser := models.SubmissionUser{
		SubmissionID: submission.SubmissionID,
		UserID:       req.UserID,
		Role:         dbRole,
		IsPrimary:    false,
		DisplayOrder: orderSequence,
		CreatedAt:    time.Now(),
	}

	if err := config.DB.Create(&submissionUser).Error; err != nil {
		log.Printf("AddSubmissionUser: Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add user to submission"})
		return
	}

	// Load user data
	config.DB.Preload("User").First(&submissionUser, submissionUser.ID)

	log.Printf("AddSubmissionUser: Successfully added user %d to submission %s", req.UserID, submissionID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User added to submission successfully",
		"user":    submissionUser,
	})
}

// GetSubmissionUsers ดู users ทั้งหมดใน submission
func GetSubmissionUsers(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Find submission and check permission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}

	// Get all users in submission
	var users []models.SubmissionUser
	if err := config.DB.Preload("User").
		Where("submission_id = ?", submissionID).
		Order("display_order ASC").
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submission users"})
		return
	}

	// Mark applicant and filter out from list
	applicantID := submission.UserID
	filtered := make([]models.SubmissionUser, 0, len(users))
	for i := range users {
		users[i].IsApplicant = users[i].UserID == applicantID
		if users[i].IsApplicant {
			continue
		}
		if users[i].User == nil {
			var u models.User
			if err := config.DB.Where("user_id = ?", users[i].UserID).First(&u).Error; err == nil {
				users[i].User = &u
			}
		}
		filtered = append(filtered, users[i])
	}

	// Separate by role for easier frontend handling
	var coauthors []models.SubmissionUser
	var others []models.SubmissionUser

	for _, user := range filtered {
		if user.Role == "coauthor" {
			coauthors = append(coauthors, user)
		} else {
			others = append(others, user)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"users":             filtered,
		"coauthors":         coauthors,
		"others":            others,
		"total":             len(filtered),
		"applicant_user":    submission.User,
		"applicant_user_id": applicantID,
	})
}

// SetCoauthors - ใช้สำหรับ PublicationRewardForm (replace all co-authors)
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
		log.Printf("SetCoauthors: JSON binding error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid JSON format",
			"details": err.Error(),
		})
		return
	}

	log.Printf("SetCoauthors: Processing %d coauthors for submission %s", len(req.Coauthors), submissionID)

	// Find submission and check permission
	var submission models.Submission
	query := config.DB.Preload("User").Where("submission_id = ? AND deleted_at IS NULL", submissionID)

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

	// Begin transaction
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
	}()

	// Delete existing co-authors only (preserve other roles)
	if err := tx.Where("submission_id = ? AND role = ?", submissionID, "coauthor").
		Delete(&models.SubmissionUser{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove existing co-authors"})
		return
	}

	// Add new co-authors
	var results []models.SubmissionUser
	var errors []string

	for i, coauthorReq := range req.Coauthors {
		// Validate user exists
		var user models.User
		if err := config.DB.Where("user_id = ? AND delete_at IS NULL", coauthorReq.UserID).
			First(&user).Error; err != nil {
			errors = append(errors, fmt.Sprintf("User %d not found", coauthorReq.UserID))
			continue
		}

		// Prevent adding submission owner as co-author
		if submission.UserID == coauthorReq.UserID {
			errors = append(errors, fmt.Sprintf("Cannot add submission owner (User %d) as co-author", coauthorReq.UserID))
			continue
		}

		// Set order sequence
		orderSequence := coauthorReq.OrderSequence
		if orderSequence == 0 {
			orderSequence = i + 2 // Start from 2 (1 is main author)
		}

		submissionUser := models.SubmissionUser{
			SubmissionID: submission.SubmissionID,
			UserID:       coauthorReq.UserID,
			Role:         "coauthor",
			IsPrimary:    false,
			DisplayOrder: orderSequence,
			CreatedAt:    time.Now(),
		}

		if err := tx.Create(&submissionUser).Error; err != nil {
			errors = append(errors, fmt.Sprintf("Failed to add user %d: %v", coauthorReq.UserID, err))
			continue
		}

		results = append(results, submissionUser)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save changes"})
		return
	}

	// Load user data for results
	for i := range results {
		config.DB.Preload("User").First(&results[i], results[i].ID)
	}

	response := gin.H{
		"success":   true,
		"message":   fmt.Sprintf("Co-authors set successfully. Added: %d", len(results)),
		"coauthors": results,
		"total":     len(results),
	}

	if len(errors) > 0 {
		response["errors"] = errors
	}

	log.Printf("SetCoauthors: Successfully added %d co-authors to submission %s", len(results), submissionID)
	c.JSON(http.StatusOK, response)
}

// UpdateSubmissionUser แก้ไข user ใน submission
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

	// Find submission and check permission
	var submission models.Submission
	query := config.DB.Preload("User").Where("submission_id = ? AND deleted_at IS NULL", submissionID)

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

	// Find and update submission user
	var submissionUser models.SubmissionUser
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, targetUserID).
		First(&submissionUser).Error; err != nil {
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

	// Load user data
	config.DB.Preload("User").First(&submissionUser, submissionUser.ID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User updated successfully",
		"user":    submissionUser,
	})
}

// RemoveSubmissionUser ลบ user จาก submission
func RemoveSubmissionUser(c *gin.Context) {
	submissionID := c.Param("id")
	targetUserID := c.Param("user_id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Find submission and check permission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

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

	// Remove user from submission
	result := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, targetUserID).
		Delete(&models.SubmissionUser{})

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

// AddMultipleUsers เพิ่ม users หลายคนพร้อมกัน
func AddMultipleUsers(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type AddMultipleUsersRequest struct {
		Users []struct {
			UserID        int    `json:"user_id" binding:"required"`
			Role          string `json:"role"`
			OrderSequence int    `json:"order_sequence"`
			IsActive      bool   `json:"is_active"`
		} `json:"users" binding:"required"`
	}

	var req AddMultipleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find submission and check permission
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)

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

	// Process each user
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

		// Map role and set defaults
		dbRole := mapFrontendRoleToDatabase(userReq.Role)
		orderSequence := userReq.OrderSequence
		if orderSequence == 0 {
			orderSequence = i + 2 // Start from 2
		}

		submissionUser := models.SubmissionUser{
			SubmissionID: submission.SubmissionID,
			UserID:       userReq.UserID,
			Role:         dbRole,
			IsPrimary:    userReq.IsActive || userReq.UserID == submission.UserID, // Updated line
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
