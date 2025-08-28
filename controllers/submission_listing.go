// controllers/submissions_listing.go - Submissions Listing Controllers

package controllers

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	yearIDStr := c.Query("year_id") // Keep as string first
	categoryID := c.Query("category")
	subcategoryID := c.Query("subcategory")
	userID := c.Query("user_id")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	// Convert yearID to integer if provided
	var yearID int
	var hasYearFilter bool
	if yearIDStr != "" && yearIDStr != "0" {
		yearID, _ = strconv.Atoi(yearIDStr)
		hasYearFilter = true
		// Debug log
		log.Printf("Admin Submissions Filter - Year ID: %d (from string: %s)", yearID, yearIDStr)
	}

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
		Preload("PublicationRewardDetail").
		Preload("FundApplicationDetail")

	// ✅ IMPORTANT: Apply year filter FIRST with proper integer comparison
	if hasYearFilter {
		query = query.Where("submissions.year_id = ?", yearID)
		log.Printf("Applying year filter: year_id = %d", yearID)
	}

	// Apply other filters
	if submissionType != "" {
		query = query.Where("submissions.submission_type = ?", submissionType)
	}
	if status != "" {
		statusInt, _ := strconv.Atoi(status)
		query = query.Where("submissions.status_id = ?", statusInt)
	}
	if categoryID != "" {
		catID, _ := strconv.Atoi(categoryID)
		query = query.Where("submissions.category_id = ?", catID)
	}
	if subcategoryID != "" {
		subCatID, _ := strconv.Atoi(subcategoryID)
		query = query.Where("submissions.subcategory_id = ?", subCatID)
	}
	if userID != "" {
		uid, _ := strconv.Atoi(userID)
		query = query.Where("submissions.user_id = ?", uid)
	}
	if dateFrom != "" {
		query = query.Where("DATE(submissions.created_at) >= ?", dateFrom)
	}
	if dateTo != "" {
		query = query.Where("DATE(submissions.created_at) <= ?", dateTo)
	}

	// Add search functionality
	if search != "" {
		searchTerm := "%" + search + "%"
		query = query.Where(
			`submissions.submission_number LIKE ? OR 
			 submissions.title LIKE ? OR
			 submissions.submission_id IN (SELECT submission_id FROM fund_application_details WHERE project_title LIKE ?) OR 
			 submissions.submission_id IN (SELECT submission_id FROM publication_reward_details WHERE paper_title LIKE ? OR paper_title_en LIKE ?) OR
			 submissions.user_id IN (SELECT user_id FROM users WHERE CONCAT(user_fname, ' ', user_lname) LIKE ? OR email LIKE ?)`,
			searchTerm, searchTerm, searchTerm, searchTerm, searchTerm, searchTerm, searchTerm,
		)
	}

	// Get total count for pagination (with all filters applied)
	var totalCount int64
	countQuery := query.Model(&models.Submission{})
	countQuery.Count(&totalCount)

	log.Printf("Total submissions with filters: %d", totalCount)

	// Apply sorting
	orderClause := sortBy + " " + sortOrder

	// Get submissions with pagination
	var err error
	if err = query.Order(orderClause).Offset(offset).Limit(limit).Find(&submissions).Error; err != nil {
		log.Printf("Error fetching submissions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	// Debug: Log the actual year_ids in the results
	for _, sub := range submissions {
		log.Printf("Submission %d has year_id: %d", sub.SubmissionID, sub.YearID)
	}

	// ✅ FIXED: Calculate statistics with proper year filter
	var stats struct {
		TotalSubmissions int64 `json:"total_submissions"`
		PendingCount     int64 `json:"pending_count"`
		ApprovedCount    int64 `json:"approved_count"`
		RejectedCount    int64 `json:"rejected_count"`
		RevisionCount    int64 `json:"revision_count"`
	}

	// Use the total from above
	stats.TotalSubmissions = totalCount

	// Build base query for statistics
	statsBaseQuery := config.DB.Model(&models.Submission{})

	// Apply the same filters for statistics
	if hasYearFilter {
		statsBaseQuery = statsBaseQuery.Where("year_id = ?", yearID)
	}
	if submissionType != "" {
		statsBaseQuery = statsBaseQuery.Where("submission_type = ?", submissionType)
	}
	if categoryID != "" {
		catID, _ := strconv.Atoi(categoryID)
		statsBaseQuery = statsBaseQuery.Where("category_id = ?", catID)
	}
	if subcategoryID != "" {
		subCatID, _ := strconv.Atoi(subcategoryID)
		statsBaseQuery = statsBaseQuery.Where("subcategory_id = ?", subCatID)
	}
	if userID != "" {
		uid, _ := strconv.Atoi(userID)
		statsBaseQuery = statsBaseQuery.Where("user_id = ?", uid)
	}
	if dateFrom != "" {
		statsBaseQuery = statsBaseQuery.Where("DATE(created_at) >= ?", dateFrom)
	}
	if dateTo != "" {
		statsBaseQuery = statsBaseQuery.Where("DATE(created_at) <= ?", dateTo)
	}
	if search != "" {
		searchTerm := "%" + search + "%"
		statsBaseQuery = statsBaseQuery.Where(
			`submission_number LIKE ? OR 
			 title LIKE ? OR
			 submission_id IN (SELECT submission_id FROM fund_application_details WHERE project_title LIKE ?) OR 
			 submission_id IN (SELECT submission_id FROM publication_reward_details WHERE paper_title LIKE ? OR paper_title_en LIKE ?) OR
			 user_id IN (SELECT user_id FROM users WHERE CONCAT(user_fname, ' ', user_lname) LIKE ? OR email LIKE ?)`,
			searchTerm, searchTerm, searchTerm, searchTerm, searchTerm, searchTerm, searchTerm,
		)
	}

	// Count each status separately - Use Session to create independent queries
	statsBaseQuery.Session(&gorm.Session{}).Where("status_id = ?", 1).Count(&stats.PendingCount)
	statsBaseQuery.Session(&gorm.Session{}).Where("status_id = ?", 2).Count(&stats.ApprovedCount)
	statsBaseQuery.Session(&gorm.Session{}).Where("status_id = ?", 3).Count(&stats.RejectedCount)
	statsBaseQuery.Session(&gorm.Session{}).Where("status_id = ?", 4).Count(&stats.RevisionCount)

	log.Printf("Statistics - Total: %d, Pending: %d, Approved: %d, Rejected: %d, Revision: %d",
		stats.TotalSubmissions, stats.PendingCount, stats.ApprovedCount, stats.RejectedCount, stats.RevisionCount)

	// Calculate total pages
	totalPages := (totalCount + int64(limit) - 1) / int64(limit)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"submissions": submissions,
		"pagination": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total_count":  totalCount,
			"total_pages":  totalPages,
		},
		"statistics": stats,
		"filters": gin.H{
			"type":        submissionType,
			"status":      status,
			"year_id":     yearIDStr, // Return the original string
			"category":    categoryID,
			"subcategory": subcategoryID,
			"user_id":     userID,
			"date_from":   dateFrom,
			"date_to":     dateTo,
			"search":      search,
			"sort_by":     sortBy,
			"sort_order":  sortOrder,
		},
		"debug": gin.H{
			"year_filter_applied": hasYearFilter,
			"year_id_int":         yearID,
			"year_id_str":         yearIDStr,
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
