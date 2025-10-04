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
// GetTeacherSubmissions returns submissions for authenticated teacher (and dept head)
func GetTeacherSubmissions(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Allow both teacher (1) and dept_head (4) to view "my submissions"
	rid := roleID.(int)
	if rid != 1 && rid != 4 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Teacher or Dept Head access required"})
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

	// Build query for current user's submissions
	var submissions []models.Submission
	query := config.DB.Preload("Year").Preload("Status").Preload("Category").
		Joins("LEFT JOIN fund_categories ON submissions.category_id = fund_categories.category_id").
		Joins("LEFT JOIN fund_subcategories ON fund_subcategories.subcategory_id = submissions.subcategory_id AND fund_subcategories.category_id = submissions.category_id").
		Select("submissions.*, fund_categories.category_name AS category_name, CASE WHEN fund_subcategories.subcategory_id IS NULL THEN NULL ELSE submissions.subcategory_id END AS subcategory_id, fund_subcategories.subcategory_name AS subcategory_name").
		Where("submissions.user_id = ? AND submissions.deleted_at IS NULL", userID)

	if submissionType != "" {
		query = query.Where("submission_type = ?", submissionType)
	}
	if status != "" {
		query = query.Where("status_id = ?", status)
	}
	if yearID != "" {
		query = query.Where("year_id = ?", yearID)
	}

	var totalCount int64
	query.Model(&models.Submission{}).Count(&totalCount)

	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	// Load type-specific details
	for i := range submissions {
		switch submissions[i].SubmissionType {
		case "fund_application":
			fundDetail := &models.FundApplicationDetail{}
			if err := config.DB.Preload("Subcategory.Category").Where("submission_id = ?", submissions[i].SubmissionID).First(fundDetail).Error; err == nil {
				if submissions[i].StatusID != 2 {
					fundDetail.AnnounceReferenceNumber = ""
				}
				submissions[i].FundApplicationDetail = fundDetail
				submissions[i].AnnounceReferenceNumber = fundDetail.AnnounceReferenceNumber
			}
		case "publication_reward":
			pubDetail := &models.PublicationRewardDetail{}
			if err := config.DB.Where("submission_id = ?", submissions[i].SubmissionID).First(pubDetail).Error; err == nil {
				if submissions[i].StatusID != 2 {
					pubDetail.AnnounceReferenceNumber = ""
				}
				submissions[i].PublicationRewardDetail = pubDetail
				submissions[i].AnnounceReferenceNumber = pubDetail.AnnounceReferenceNumber
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
	query := config.DB.Preload("User").Preload("Year").Preload("Status").Preload("Category").Preload("Subcategory").
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

// GetAdminSubmissions returns admin list + stats with consistent filters
func GetAdminSubmissions(c *gin.Context) {
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	submissionType := c.Query("type")
	status := c.Query("status")
	yearIDStr := c.Query("year_id")
	categoryID := c.Query("category")
	subcategoryID := c.Query("subcategory")
	userID := c.Query("user_id")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := strings.ToLower(c.DefaultQuery("sort_order", "desc"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	offset := (page - 1) * limit
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// year filter
	var yearID int
	var hasYearFilter bool
	if yearIDStr != "" && yearIDStr != "0" {
		yearID, _ = strconv.Atoi(yearIDStr)
		hasYearFilter = true
		log.Printf("Admin Submissions Filter - year_id=%d", yearID)
	}

	// Allowed sort fields (table-qualified for safety)
	allowedSort := map[string]string{
		"created_at":        "submissions.created_at",
		"updated_at":        "submissions.updated_at",
		"submitted_at":      "submissions.submitted_at",
		"submission_number": "submissions.submission_number",
		"status_id":         "submissions.status_id",
	}
	sortField, ok := allowedSort[sortBy]
	if !ok {
		sortField = "submissions.created_at"
	}
	orderClause := sortField + " " + strings.ToUpper(sortOrder)

	// ---------- Base list query (with preloads) ----------
	var submissions []models.Submission
	listQ := config.DB.Preload("User").Preload("Year").Preload("Status").Preload("Category").Preload("Subcategory").
		Where("submissions.deleted_at IS NULL")

	// Apply filters (identical set used later for stats)
	if hasYearFilter {
		listQ = listQ.Where("submissions.year_id = ?", yearID)
	}
	if submissionType != "" {
		listQ = listQ.Where("submissions.submission_type = ?", submissionType)
	}
	if status != "" {
		if st, err := strconv.Atoi(status); err == nil {
			listQ = listQ.Where("submissions.status_id = ?", st)
		}
	}
	if categoryID != "" {
		if cat, err := strconv.Atoi(categoryID); err == nil {
			listQ = listQ.Where("submissions.category_id = ?", cat)
		}
	}
	if subcategoryID != "" {
		if sub, err := strconv.Atoi(subcategoryID); err == nil {
			listQ = listQ.Where("submissions.subcategory_id = ?", sub)
		}
	}
	if userID != "" {
		if uid, err := strconv.Atoi(userID); err == nil {
			listQ = listQ.Where("submissions.user_id = ?", uid)
		}
	}
	if dateFrom != "" {
		listQ = listQ.Where("DATE(submissions.created_at) >= ?", dateFrom)
	}
	if dateTo != "" {
		listQ = listQ.Where("DATE(submissions.created_at) <= ?", dateTo)
	}
	if search != "" {
		st := "%" + search + "%"
		listQ = listQ.Where(`
            submissions.submission_number LIKE ? OR
            submissions.title LIKE ? OR
            submissions.submission_id IN (SELECT submission_id FROM fund_application_details WHERE project_title LIKE ?) OR
            submissions.submission_id IN (SELECT submission_id FROM publication_reward_details WHERE paper_title LIKE ? OR paper_title_en LIKE ?) OR
            submissions.user_id IN (SELECT user_id FROM users WHERE CONCAT(user_fname, ' ', user_lname) LIKE ? OR email LIKE ?)`,
			st, st, st, st, st, st, st,
		)
	}

	// Count (with all filters)
	var totalCount int64
	listQ.Model(&models.Submission{}).Count(&totalCount)

	// Fetch page
	if err := listQ.Order(orderClause).Offset(offset).Limit(limit).Find(&submissions).Error; err != nil {
		log.Printf("GetAdminSubmissions list error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submissions"})
		return
	}

	// ---------- Statistics (clone filters, count by status) ----------
	type Stats struct {
		TotalSubmissions int64 `json:"total_submissions"`
		PendingCount     int64 `json:"pending_count"`
		ApprovedCount    int64 `json:"approved_count"`
		RejectedCount    int64 `json:"rejected_count"`
		RevisionCount    int64 `json:"revision_count"`
	}
	stats := Stats{TotalSubmissions: totalCount}

	// base stats filter builder
	baseStats := func() *gorm.DB {
		q := config.DB.Model(&models.Submission{}).Where("submissions.deleted_at IS NULL")
		if hasYearFilter {
			q = q.Where("submissions.year_id = ?", yearID)
		}
		if submissionType != "" {
			q = q.Where("submissions.submission_type = ?", submissionType)
		}
		if categoryID != "" {
			if cat, err := strconv.Atoi(categoryID); err == nil {
				q = q.Where("submissions.category_id = ?", cat)
			}
		}
		if subcategoryID != "" {
			if sub, err := strconv.Atoi(subcategoryID); err == nil {
				q = q.Where("submissions.subcategory_id = ?", sub)
			}
		}
		if userID != "" {
			if uid, err := strconv.Atoi(userID); err == nil {
				q = q.Where("submissions.user_id = ?", uid)
			}
		}
		if dateFrom != "" {
			q = q.Where("DATE(submissions.created_at) >= ?", dateFrom)
		}
		if dateTo != "" {
			q = q.Where("DATE(submissions.created_at) <= ?", dateTo)
		}
		if search != "" {
			st := "%" + search + "%"
			q = q.Where(`
                submissions.submission_number LIKE ? OR
                submissions.title LIKE ? OR
                submissions.submission_id IN (SELECT submission_id FROM fund_application_details WHERE project_title LIKE ?) OR
                submissions.submission_id IN (SELECT submission_id FROM publication_reward_details WHERE paper_title LIKE ? OR paper_title_en LIKE ?) OR
                submissions.user_id IN (SELECT user_id FROM users WHERE CONCAT(user_fname, ' ', user_lname) LIKE ? OR email LIKE ?)`,
				st, st, st, st, st, st, st,
			)
		}
		return q
	}

	baseStats().Session(&gorm.Session{}).Where("submissions.status_id = ?", 1).Count(&stats.PendingCount)
	baseStats().Session(&gorm.Session{}).Where("submissions.status_id = ?", 2).Count(&stats.ApprovedCount)
	baseStats().Session(&gorm.Session{}).Where("submissions.status_id = ?", 3).Count(&stats.RejectedCount)
	baseStats().Session(&gorm.Session{}).Where("submissions.status_id = ?", 4).Count(&stats.RevisionCount)

	// ---------- Response ----------
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
			"type":        submissionType,
			"status":      status,
			"year_id":     yearIDStr, // echo back what was requested
			"category":    categoryID,
			"subcategory": subcategoryID,
			"user_id":     userID,
			"date_from":   dateFrom,
			"date_to":     dateTo,
			"search":      search,
		},
		"sorting": gin.H{
			"sort_by":    sortBy,
			"sort_order": sortOrder,
		},
		"statistics": stats,
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
