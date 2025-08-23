// controllers/submissions_listing.go - Submissions Listing Controllers

package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
)

// ===================== SUBMISSIONS LISTING =====================

// GetAllSubmissions returns paginated list of submissions with filters
func GetAllSubmissions(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	submissionType := c.Query("type")
	status := c.Query("status")
	yearID := c.Query("year_id")
	priority := c.Query("priority")
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	// Validate pagination
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Validate sort parameters
	allowedSortFields := map[string]bool{
		"created_at":        true,
		"updated_at":        true,
		"submitted_at":      true,
		"submission_number": true,
		"priority":          true,
		"status_id":         true,
	}
	if !allowedSortFields[sortBy] {
		sortBy = "created_at"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// Build base query
	var submissions []models.Submission
	query := config.DB.Preload("User").Preload("Year").Preload("Status").
		Where("deleted_at IS NULL")

	// Permission-based filtering
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	// Apply filters
	if submissionType != "" {
		query = query.Where("submission_type = ?", submissionType)
	}
	if status != "" {
		query = query.Where("status_id = ?", status)
	}
	if yearID != "" {
		query = query.Where("year_id = ?", yearID)
	}
	// if priority != "" {
	// 	query = query.Where("priority = ?", priority)
	// }

	// Search functionality
	if search != "" {
		searchTerm := "%" + search + "%"
		query = query.Where(
			"submission_number LIKE ? OR user_id IN (SELECT user_id FROM users WHERE CONCAT(user_fname, ' ', user_lname) LIKE ?)",
			searchTerm, searchTerm,
		)
	}

	// Get total count for pagination
	var totalCount int64
	query.Model(&models.Submission{}).Count(&totalCount)

	// Apply sorting and pagination
	orderClause := sortBy + " " + strings.ToUpper(sortOrder)
	if err := query.Order(orderClause).Offset(offset).Limit(limit).Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	// Calculate pagination info
	totalPages := (totalCount + int64(limit) - 1) / int64(limit)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"submissions": submissions,
		"pagination": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total_count":  totalCount,
			"total_pages":  totalPages,
			"has_next":     page < int(totalPages),
			"has_prev":     page > 1,
		},
		"filters": gin.H{
			"type":     submissionType,
			"status":   status,
			"year_id":  yearID,
			"priority": priority,
			"search":   search,
		},
		"sorting": gin.H{
			"sort_by":    sortBy,
			"sort_order": sortOrder,
		},
	})
}

// GetTeacherSubmissions returns submissions for authenticated teacher
func GetTeacherSubmissions(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Ensure user is teacher
	if roleID.(int) != 1 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Teacher access required"})
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	submissionType := c.Query("type")
	status := c.Query("status")
	yearID := c.Query("year_id")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	offset := (page - 1) * limit

	// Build query for teacher's submissions
	var submissions []models.Submission
	query := config.DB.Preload("Year").Preload("Status").
		Where("user_id = ? AND deleted_at IS NULL", userID)

	// Apply filters
	if submissionType != "" {
		query = query.Where("submission_type = ?", submissionType)
	}
	if status != "" {
		query = query.Where("status_id = ?", status)
	}
	if yearID != "" {
		query = query.Where("year_id = ?", yearID)
	}

	// Get total count
	var totalCount int64
	query.Model(&models.Submission{}).Count(&totalCount)

	// Get submissions with pagination
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	// Load type-specific details for each submission
	for i := range submissions {
		switch submissions[i].SubmissionType {
		case "fund_application":
			var fundDetail models.FundApplicationDetail
			if err := config.DB.Preload("Subcategory").Where("submission_id = ?", submissions[i].SubmissionID).First(&fundDetail).Error; err == nil {
				submissions[i].FundApplicationDetail = &fundDetail
			}
		case "publication_reward":
			var pubDetail models.PublicationRewardDetail
			if err := config.DB.Where("submission_id = ?", submissions[i].SubmissionID).First(&pubDetail).Error; err == nil {
				if submissions[i].StatusID != 2 {
					pubDetail.AnnounceReferenceNumber = ""
				}
				submissions[i].PublicationRewardDetail = &pubDetail
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"submissions": submissions,
		"pagination": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total_count":  totalCount,
			"total_pages":  (totalCount + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetStaffSubmissions returns submissions for staff review
func GetStaffSubmissions(c *gin.Context) {
	roleID, _ := c.Get("roleID")

	// Ensure user is staff
	if roleID.(int) != 2 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Staff access required"})
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	submissionType := c.Query("type")
	status := c.Query("status")
	priority := c.Query("priority")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Build query for submissions that need staff review
	var submissions []models.Submission
	query := config.DB.Preload("User").Preload("Year").Preload("Status").
		Where("deleted_at IS NULL AND submitted_at IS NOT NULL") // Only submitted submissions

	// Apply filters
	if submissionType != "" {
		query = query.Where("submission_type = ?", submissionType)
	}
	if status != "" {
		query = query.Where("status_id = ?", status)
	}
	if priority != "" {
		query = query.Where("priority = ?", priority)
	}

	// Get total count
	var totalCount int64
	query.Model(&models.Submission{}).Count(&totalCount)

	// Get submissions with pagination
	if err := query.Order("priority DESC, submitted_at ASC").Offset(offset).Limit(limit).Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"submissions": submissions,
		"pagination": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total_count":  totalCount,
			"total_pages":  (totalCount + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetAdminSubmissions returns all submissions for admin
func GetAdminSubmissions(c *gin.Context) {
	roleID, _ := c.Get("roleID")

	// Ensure user is admin
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	submissionType := c.Query("type")
	status := c.Query("status")
	yearID := c.Query("year_id")
	userID := c.Query("user_id")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	offset := (page - 1) * limit

	// Build comprehensive query for admin
	var submissions []models.Submission
	query := config.DB.Preload("User").Preload("Year").Preload("Status").
		Where("deleted_at IS NULL")

	// Apply filters
	if submissionType != "" {
		query = query.Where("submission_type = ?", submissionType)
	}
	if status != "" {
		query = query.Where("status_id = ?", status)
	}
	if yearID != "" {
		query = query.Where("year_id = ?", yearID)
	}
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if dateFrom != "" {
		query = query.Where("created_at >= ?", dateFrom)
	}
	if dateTo != "" {
		query = query.Where("created_at <= ?", dateTo)
	}

	// Get total count
	var totalCount int64
	query.Model(&models.Submission{}).Count(&totalCount)

	// Get submissions with pagination
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	// Get summary statistics
	var stats struct {
		TotalSubmissions int64 `json:"total_submissions"`
		PendingCount     int64 `json:"pending_count"`
		ApprovedCount    int64 `json:"approved_count"`
		RejectedCount    int64 `json:"rejected_count"`
	}

	config.DB.Model(&models.Submission{}).Where("deleted_at IS NULL").Count(&stats.TotalSubmissions)
	config.DB.Model(&models.Submission{}).Where("deleted_at IS NULL AND status_id = 1").Count(&stats.PendingCount)
	config.DB.Model(&models.Submission{}).Where("deleted_at IS NULL AND status_id = 2").Count(&stats.ApprovedCount)
	config.DB.Model(&models.Submission{}).Where("deleted_at IS NULL AND status_id = 3").Count(&stats.RejectedCount)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"submissions": submissions,
		"pagination": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total_count":  totalCount,
			"total_pages":  (totalCount + int64(limit) - 1) / int64(limit),
		},
		"statistics": stats,
		"filters": gin.H{
			"type":      submissionType,
			"status":    status,
			"year_id":   yearID,
			"user_id":   userID,
			"date_from": dateFrom,
			"date_to":   dateTo,
		},
	})
}

// SearchSubmissions provides advanced search functionality
func SearchSubmissions(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Parse search parameters
	keyword := c.Query("q")
	submissionType := c.Query("type")
	status := c.Query("status")
	yearID := c.Query("year_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "15"))

	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Search keyword is required"})
		return
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 15
	}
	offset := (page - 1) * limit

	// Build search query
	var submissions []models.Submission
	searchTerm := "%" + keyword + "%"

	query := config.DB.Preload("User").Preload("Year").Preload("Status").
		Where("deleted_at IS NULL")

	// Permission-based filtering
	if roleID.(int) != 3 { // Not admin
		query = query.Where("user_id = ?", userID)
	}

	// Search in multiple fields
	query = query.Where(`
        submission_number LIKE ? OR
        user_id IN (
            SELECT user_id FROM users 
            WHERE CONCAT(user_fname, ' ', user_lname) LIKE ? OR email LIKE ?
        ) OR
        submission_id IN (
            SELECT submission_id FROM fund_application_details 
            WHERE project_title LIKE ? OR project_description LIKE ?
        ) OR
        submission_id IN (
            SELECT submission_id FROM publication_reward_details 
            WHERE paper_title LIKE ? OR journal_name LIKE ?
        )
    `, searchTerm, searchTerm, searchTerm, searchTerm, searchTerm, searchTerm, searchTerm)

	// Apply additional filters
	if submissionType != "" {
		query = query.Where("submission_type = ?", submissionType)
	}
	if status != "" {
		query = query.Where("status_id = ?", status)
	}
	if yearID != "" {
		query = query.Where("year_id = ?", yearID)
	}

	// Get total count
	var totalCount int64
	query.Model(&models.Submission{}).Count(&totalCount)

	// Get search results
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"submissions": submissions,
		"search_term": keyword,
		"pagination": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total_count":  totalCount,
			"total_pages":  (totalCount + int64(limit) - 1) / int64(limit),
		},
	})
}
