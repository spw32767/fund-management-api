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
	validRoles := map[string]bool{
		"first_author":         true,
		"corresponding_author": true,
		"co_author":            true,
		"advisor":              true,
		"coordinator":          true,
	}

	if validRoles[frontendRole] {
		return frontendRole
	}

	// Default fallback
	return "co_author"
}

// AddSubmissionUser เพิ่ม user ลงใน submission (co-author, advisor, etc.)
func AddSubmissionUser(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type AddUserRequest struct {
		UserID        int    `json:"user_id" binding:"required"`
		Role          string `json:"role"`
		OrderSequence int    `json:"order_sequence"`
		IsActive      bool   `json:"is_active"`
		IsPrimary     bool   `json:"is_primary"` // เพิ่ม field นี้
	}

	var req AddUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Map และ validate role
	dbRole := mapFrontendRoleToDatabase(req.Role)
	if dbRole == "" {
		dbRole = "co_author"
	}

	// Find submission
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

	// Check if user already exists in submission
	var existingUser models.SubmissionUser
	if err := config.DB.Where("submission_id = ? AND user_id = ?", submissionID, req.UserID).First(&existingUser).Error; err == nil {
		// Update existing user instead of creating new
		existingUser.Role = dbRole
		existingUser.IsPrimary = req.IsPrimary
		existingUser.DisplayOrder = req.OrderSequence

		if err := config.DB.Save(&existingUser).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update existing user"})
			return
		}

		// Load user data
		config.DB.Preload("User").First(&existingUser, existingUser.ID)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "User updated successfully",
			"user":    existingUser,
		})
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
		IsPrimary:    req.IsPrimary,
		DisplayOrder: orderSequence,
		CreatedAt:    time.Now(),
	}

	if err := config.DB.Create(&submissionUser).Error; err != nil {
		log.Printf("AddSubmissionUser: Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to add user to submission",
			"details": err.Error(),
		})
		return
	}

	// Load user data
	config.DB.Preload("User").First(&submissionUser, submissionUser.ID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User added to submission successfully",
		"user":    submissionUser,
	})
}

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

	// Categorize users by new roles
	var mainAuthors []models.SubmissionUser  // first_author, corresponding_author
	var coauthors []models.SubmissionUser    // co_author
	var others []models.SubmissionUser       // advisor, coordinator
	var primaryAuthor *models.SubmissionUser // คนที่ is_primary = true

	for i, user := range users {
		switch user.Role {
		case "first_author", "corresponding_author":
			mainAuthors = append(mainAuthors, user)
		case "co_author":
			coauthors = append(coauthors, user)
		default:
			others = append(others, user)
		}

		// หา primary author
		if user.IsPrimary {
			primaryAuthor = &users[i]
		}
	}

	// Prepare response
	response := gin.H{
		"success": true,
		"data": gin.H{
			"submission_id":   submissionID,
			"submission_type": submission.SubmissionType,
			"total_users":     len(users),
			"main_authors":    mainAuthors,   // first_author, corresponding_author
			"coauthors":       coauthors,     // co_author
			"others":          others,        // advisor, etc.
			"primary_author":  primaryAuthor, // คนที่ is_primary = true
			"all_users":       users,
		},
		"users": users, // For backward compatibility
		"total": len(users),
	}

	// Add summary statistics
	response["summary"] = gin.H{
		"total_authors":      len(mainAuthors) + len(coauthors),
		"main_authors_count": len(mainAuthors),
		"coauthors_count":    len(coauthors),
		"others_count":       len(others),
		"has_primary":        primaryAuthor != nil,
		"primary_author_role": func() string {
			if primaryAuthor != nil {
				return primaryAuthor.Role
			}
			return ""
		}(),
	}

	c.JSON(http.StatusOK, response)
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

// AddMultipleUsers เพิ่ม users หลายคนพร้อมกัน (รวม owner + coauthors)
func AddMultipleUsers(c *gin.Context) {
	submissionID := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	type UserRequest struct {
		UserID        int    `json:"user_id" binding:"required"`
		Role          string `json:"role"`           // "first_author", "corresponding_author", "co_author"
		OrderSequence int    `json:"order_sequence"` // ลำดับการแสดงผล
		IsActive      bool   `json:"is_active"`      // สถานะ active
		IsPrimary     bool   `json:"is_primary"`     // เป็น primary author หรือไม่
	}

	type AddMultipleUsersRequest struct {
		Users []UserRequest `json:"users" binding:"required"`
	}

	var req AddMultipleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("AddMultipleUsers: JSON binding error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	log.Printf("AddMultipleUsers: Processing %d users for submission %s", len(req.Users), submissionID)

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

	var results []models.SubmissionUser
	var errors []string
	var warnings []string

	for i, userReq := range req.Users {
		log.Printf("Processing user %d: UserID=%d, Role=%s, IsPrimary=%t",
			i+1, userReq.UserID, userReq.Role, userReq.IsPrimary)

		// Validate user exists
		var user models.User
		if err := config.DB.Where("user_id = ? AND delete_at IS NULL", userReq.UserID).
			First(&user).Error; err != nil {
			errorMsg := fmt.Sprintf("User %d not found", userReq.UserID)
			log.Printf("AddMultipleUsers: %s", errorMsg)
			errors = append(errors, errorMsg)
			continue
		}

		// Check if user already exists in submission
		var existing models.SubmissionUser
		if err := tx.Where("submission_id = ? AND user_id = ?", submissionID, userReq.UserID).
			First(&existing).Error; err == nil {
			warningMsg := fmt.Sprintf("User %d already in submission - updating role", userReq.UserID)
			log.Printf("AddMultipleUsers: %s", warningMsg)

			// อัพเดต role และ is_primary ของ user ที่มีอยู่
			existing.Role = mapFrontendRoleToDatabase(userReq.Role)
			existing.IsPrimary = userReq.IsPrimary
			existing.DisplayOrder = userReq.OrderSequence

			if err := tx.Save(&existing).Error; err != nil {
				errorMsg := fmt.Sprintf("Failed to update user %d: %v", userReq.UserID, err)
				errors = append(errors, errorMsg)
				continue
			}

			// Load relations
			tx.Preload("User").First(&existing, existing.ID)
			results = append(results, existing)
			warnings = append(warnings, warningMsg)
			continue
		}

		// Validate and map role
		dbRole := mapFrontendRoleToDatabase(userReq.Role)

		// Set defaults
		orderSequence := userReq.OrderSequence
		if orderSequence == 0 {
			orderSequence = i + 1
		}

		// Create submission user
		submissionUser := models.SubmissionUser{
			SubmissionID: submission.SubmissionID,
			UserID:       userReq.UserID,
			Role:         dbRole,
			IsPrimary:    userReq.IsPrimary,
			DisplayOrder: orderSequence,
			CreatedAt:    time.Now(),
		}

		log.Printf("Creating submission_user: SubmissionID=%d, UserID=%d, Role=%s, IsPrimary=%t, DisplayOrder=%d",
			submissionUser.SubmissionID, submissionUser.UserID, submissionUser.Role,
			submissionUser.IsPrimary, submissionUser.DisplayOrder)

		if err := tx.Create(&submissionUser).Error; err != nil {
			errorMsg := fmt.Sprintf("Failed to add user %d: %v", userReq.UserID, err)
			log.Printf("AddMultipleUsers: %s", errorMsg)
			errors = append(errors, errorMsg)
			continue
		}

		results = append(results, submissionUser)
		log.Printf("Successfully created submission_user with ID: %d", submissionUser.ID)
	}

	// Check results
	log.Printf("Processing complete: %d successful, %d errors, %d warnings",
		len(results), len(errors), len(warnings))

	// Decide whether to commit or rollback
	if len(results) == 0 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"error":    "No users were successfully processed",
			"errors":   errors,
			"warnings": warnings,
		})
		return
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		log.Printf("AddMultipleUsers: Commit error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to save changes to database",
			"details": err.Error(),
		})
		return
	}

	log.Printf("Transaction committed successfully")

	// Load full user data for response
	for i := range results {
		if err := config.DB.Preload("User").First(&results[i], results[i].ID).Error; err != nil {
			log.Printf("Warning: Failed to load user data for result %d: %v", results[i].ID, err)
		}
	}

	// Prepare detailed response
	response := gin.H{
		"success": true,
		"message": fmt.Sprintf("Successfully processed users. Added/Updated: %d", len(results)),
		"data": gin.H{
			"submission_id": submissionID,
			"processed":     len(results),
			"total":         len(req.Users),
			"users":         results,
		},
		"added": len(results),
		"total": len(req.Users),
	}

	if len(errors) > 0 {
		response["errors"] = errors
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	log.Printf("AddMultipleUsers completed successfully. Processed %d users", len(results))
	c.JSON(http.StatusOK, response)
}
