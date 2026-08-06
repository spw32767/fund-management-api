// controllers/submissions_listing.go - Submissions Listing Controllers

package controllers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"

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
		query = query.Where("submissions.year_id = ?", yearID)
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
		query = query.Where("submissions.year_id = ?", yearID)
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

// enrichAdminSubmissionListDetails adds only the compact form fields required
// by the admin table. It deliberately avoids the full details endpoint shape
// (documents, events, co-authors, external funds, etc.).
func enrichAdminSubmissionListDetails(submissions []models.Submission) error {
	if len(submissions) == 0 {
		return nil
	}

	ids := make([]int, 0, len(submissions))
	userIDs := make([]int, 0, len(submissions))
	seenUserIDs := make(map[int]struct{}, len(submissions))
	for _, submission := range submissions {
		ids = append(ids, submission.SubmissionID)
		if submission.UserID > 0 {
			if _, seen := seenUserIDs[submission.UserID]; !seen {
				seenUserIDs[submission.UserID] = struct{}{}
				userIDs = append(userIDs, submission.UserID)
			}
		}
	}

	// Do not rely solely on GORM's User preload here. Some existing databases
	// have legacy user soft-delete columns, which can make that preload return
	// nil even though submissions.user_id is valid. One compact batch lookup is
	// deterministic and avoids an N+1 fallback.
	var users []models.User
	if len(userIDs) > 0 {
		if err := config.DB.Select("user_id, user_fname, user_lname, email").
			Where("user_id IN ?", userIDs).Find(&users).Error; err != nil {
			return err
		}
	}
	usersByID := make(map[int]*models.User, len(users))
	for i := range users {
		usersByID[users[i].UserID] = &users[i]
	}

	// The category relation can be returned as a zero-value object on legacy
	// records even though submissions.category_id is valid. Resolve the
	// display category explicitly from the IDs in this page, in one batch.
	categoryIDs := make([]int, 0, len(submissions))
	seenCategoryIDs := make(map[int]struct{}, len(submissions))
	for _, submission := range submissions {
		categoryID := 0
		if submission.CategoryID != nil {
			categoryID = *submission.CategoryID
		}
		if categoryID == 0 && submission.Subcategory != nil {
			categoryID = submission.Subcategory.CategoryID
		}
		if categoryID > 0 {
			if _, seen := seenCategoryIDs[categoryID]; !seen {
				seenCategoryIDs[categoryID] = struct{}{}
				categoryIDs = append(categoryIDs, categoryID)
			}
		}
	}

	var categories []models.FundCategory
	if len(categoryIDs) > 0 {
		if err := config.DB.Select("category_id, category_name, status, year_id").
			Where("category_id IN ?", categoryIDs).Find(&categories).Error; err != nil {
			return err
		}
	}
	categoriesByID := make(map[int]models.FundCategory, len(categories))
	for _, category := range categories {
		categoriesByID[category.CategoryID] = category
	}

	var fundDetails []models.FundApplicationDetail
	if err := config.DB.Select(
		"submission_id, project_title, requested_amount, approved_amount",
	).Where("submission_id IN ?", ids).Find(&fundDetails).Error; err != nil {
		return err
	}
	fundBySubmission := make(map[int]*models.FundApplicationDetail, len(fundDetails))
	for i := range fundDetails {
		fundBySubmission[fundDetails[i].SubmissionID] = &fundDetails[i]
	}

	var publicationDetails []models.PublicationRewardDetail
	if err := config.DB.Select(
		"submission_id, paper_title, journal_name, publication_date, volume_issue, page_numbers, indexing, quartile, "+
			"reward_amount, reward_approve_amount, revision_fee, revision_fee_approve_amount, "+
			"publication_fee, publication_fee_approve_amount, external_funding_amount, total_amount, total_approve_amount",
	).Where("submission_id IN ?", ids).Find(&publicationDetails).Error; err != nil {
		return err
	}
	publicationBySubmission := make(map[int]*models.PublicationRewardDetail, len(publicationDetails))
	for i := range publicationDetails {
		publicationBySubmission[publicationDetails[i].SubmissionID] = &publicationDetails[i]
	}

	for i := range submissions {
		submission := &submissions[i]
		categoryID := 0
		if submission.CategoryID != nil {
			categoryID = *submission.CategoryID
		}
		if categoryID == 0 && submission.Subcategory != nil {
			categoryID = submission.Subcategory.CategoryID
		}
		if category, ok := categoriesByID[categoryID]; ok {
			categoryCopy := category
			submission.Category = &categoryCopy
			categoryName := strings.TrimSpace(category.CategoryName)
			submission.CategoryName = &categoryName
		}
		if submission.User == nil {
			submission.User = usersByID[submission.UserID]
		}
		if submission.User != nil {
			name := strings.TrimSpace(strings.TrimSpace(submission.User.UserFname) + " " + strings.TrimSpace(submission.User.UserLname))
			if name == "" {
				name = strings.TrimSpace(submission.User.Email)
			}
			if name != "" {
				submission.ApplicantName = &name
			}
		}
		if detail, ok := fundBySubmission[submission.SubmissionID]; ok {
			submission.FundApplicationDetail = detail
		}
		if detail, ok := publicationBySubmission[submission.SubmissionID]; ok {
			submission.PublicationRewardDetail = detail
		}
	}

	return nil
}

type adminSubmissionListFundDetail struct {
	ProjectTitle    string  `json:"project_title,omitempty"`
	RequestedAmount float64 `json:"requested_amount"`
	ApprovedAmount  float64 `json:"approved_amount"`
}

type adminSubmissionListPublicationDetail struct {
	PaperTitle                  string    `json:"paper_title,omitempty"`
	JournalName                 string    `json:"journal_name,omitempty"`
	PublicationDate             time.Time `json:"publication_date,omitempty"`
	VolumeIssue                 string    `json:"volume_issue,omitempty"`
	PageNumbers                 string    `json:"page_numbers,omitempty"`
	Indexing                    string    `json:"indexing,omitempty"`
	Quartile                    string    `json:"quartile,omitempty"`
	RewardAmount                float64   `json:"reward_amount"`
	RewardApproveAmount         float64   `json:"reward_approve_amount"`
	RevisionFee                 float64   `json:"revision_fee"`
	RevisionFeeApproveAmount    float64   `json:"revision_fee_approve_amount"`
	PublicationFee              float64   `json:"publication_fee"`
	PublicationFeeApproveAmount float64   `json:"publication_fee_approve_amount"`
	ExternalFundingAmount       float64   `json:"external_funding_amount"`
	TotalAmount                 float64   `json:"total_amount"`
	TotalApproveAmount          float64   `json:"total_approve_amount"`
}

type adminSubmissionListItem struct {
	SubmissionID            int                                   `json:"submission_id"`
	SubmissionNumber        string                                `json:"submission_number"`
	SubmissionType          string                                `json:"submission_type"`
	UserID                  int                                   `json:"user_id"`
	YearID                  int                                   `json:"year_id"`
	CategoryID              *int                                  `json:"category_id,omitempty"`
	SubcategoryID           *int                                  `json:"subcategory_id,omitempty"`
	SubcategoryBudgetID     *int                                  `json:"subcategory_budget_id,omitempty"`
	StatusID                int                                   `json:"status_id"`
	SubmittedAt             *time.Time                            `json:"submitted_at,omitempty"`
	ApprovedAt              *time.Time                            `json:"approved_at,omitempty"`
	AdminApprovedAt         *time.Time                            `json:"admin_approved_at,omitempty"`
	CreatedAt               time.Time                             `json:"created_at"`
	UpdatedAt               time.Time                             `json:"updated_at"`
	ApplicantName           string                                `json:"applicant_name,omitempty"`
	ApplicantEmail          string                                `json:"applicant_email,omitempty"`
	CategoryName            string                                `json:"category_name,omitempty"`
	SubcategoryName         string                                `json:"subcategory_name,omitempty"`
	Title                   string                                `json:"title,omitempty"`
	PaperTitle              string                                `json:"paper_title,omitempty"`
	RequestedAmount         float64                               `json:"requested_amount"`
	ApprovedAmount          float64                               `json:"approved_amount"`
	TotalAmount             float64                               `json:"total_amount"`
	TotalApproveAmount      float64                               `json:"total_approve_amount"`
	User                    map[string]any                        `json:"user,omitempty"`
	Year                    *models.Year                          `json:"year,omitempty"`
	Status                  *models.ApplicationStatus             `json:"status,omitempty"`
	Category                *models.FundCategory                  `json:"category,omitempty"`
	Subcategory             *models.FundSubcategory               `json:"subcategory,omitempty"`
	FundApplicationDetail   *adminSubmissionListFundDetail        `json:"fund_application_detail,omitempty"`
	PublicationRewardDetail *adminSubmissionListPublicationDetail `json:"publication_reward_detail,omitempty"`
}

func toAdminSubmissionListItems(submissions []models.Submission) []adminSubmissionListItem {
	items := make([]adminSubmissionListItem, 0, len(submissions))
	for _, submission := range submissions {
		item := adminSubmissionListItem{
			SubmissionID:        submission.SubmissionID,
			SubmissionNumber:    submission.SubmissionNumber,
			SubmissionType:      submission.SubmissionType,
			UserID:              submission.UserID,
			YearID:              submission.YearID,
			CategoryID:          submission.CategoryID,
			SubcategoryID:       submission.SubcategoryID,
			SubcategoryBudgetID: submission.SubcategoryBudgetID,
			StatusID:            submission.StatusID,
			SubmittedAt:         submission.SubmittedAt,
			ApprovedAt:          submission.ApprovedAt,
			AdminApprovedAt:     submission.AdminApprovedAt,
			CreatedAt:           submission.CreatedAt,
			UpdatedAt:           submission.UpdatedAt,
			Year:                &submission.Year,
			Status:              &submission.Status,
			Category:            submission.Category,
			Subcategory:         submission.Subcategory,
		}
		if submission.CategoryName != nil {
			item.CategoryName = strings.TrimSpace(*submission.CategoryName)
		}
		if item.CategoryName == "" && item.Category != nil {
			item.CategoryName = item.Category.CategoryName
		}
		if submission.Subcategory != nil {
			item.SubcategoryName = submission.Subcategory.SubcategoryName
		}
		if submission.User != nil {
			item.ApplicantName = strings.TrimSpace(strings.TrimSpace(submission.User.UserFname) + " " + strings.TrimSpace(submission.User.UserLname))
			item.ApplicantEmail = submission.User.Email
			item.User = map[string]any{
				"user_id":    submission.User.UserID,
				"user_fname": submission.User.UserFname,
				"user_lname": submission.User.UserLname,
				"email":      submission.User.Email,
			}
		}
		if submission.FundApplicationDetail != nil {
			detail := submission.FundApplicationDetail
			item.Title = detail.ProjectTitle
			item.RequestedAmount = detail.RequestedAmount
			item.ApprovedAmount = detail.ApprovedAmount
			item.FundApplicationDetail = &adminSubmissionListFundDetail{
				ProjectTitle:    detail.ProjectTitle,
				RequestedAmount: detail.RequestedAmount,
				ApprovedAmount:  detail.ApprovedAmount,
			}
		}
		if submission.PublicationRewardDetail != nil {
			detail := submission.PublicationRewardDetail
			item.Title = detail.PaperTitle
			item.PaperTitle = detail.PaperTitle
			item.TotalAmount = detail.TotalAmount
			item.TotalApproveAmount = detail.TotalApproveAmount
			item.RequestedAmount = detail.TotalAmount
			item.ApprovedAmount = detail.TotalApproveAmount
			item.PublicationRewardDetail = &adminSubmissionListPublicationDetail{
				PaperTitle:                  detail.PaperTitle,
				JournalName:                 detail.JournalName,
				PublicationDate:             detail.PublicationDate,
				VolumeIssue:                 detail.VolumeIssue,
				PageNumbers:                 detail.PageNumbers,
				Indexing:                    detail.Indexing,
				Quartile:                    detail.Quartile,
				RewardAmount:                detail.RewardAmount,
				RewardApproveAmount:         detail.RewardApproveAmount,
				RevisionFee:                 detail.RevisionFee,
				RevisionFeeApproveAmount:    detail.RevisionFeeApproveAmount,
				PublicationFee:              detail.PublicationFee,
				PublicationFeeApproveAmount: detail.PublicationFeeApproveAmount,
				ExternalFundingAmount:       detail.ExternalFundingAmount,
				TotalAmount:                 detail.TotalAmount,
				TotalApproveAmount:          detail.TotalApproveAmount,
			}
		}
		items = append(items, item)
	}
	return items
}

// GetAdminSubmissions returns admin list + stats with consistent filters
func GetAdminSubmissions(c *gin.Context) {
	hasReadAll := false
	hasAdminPage := false
	if permissionVals, exists := c.Get("permissions"); exists {
		if permissionCodes, ok := permissionVals.([]string); ok {
			for _, code := range permissionCodes {
				normalized := strings.TrimSpace(strings.ToLower(code))
				switch normalized {
				case "submission.read.all":
					hasReadAll = true
				case "ui.page.admin.applications.view":
					hasAdminPage = true
				}
			}
		}
	}

	if !hasReadAll && !hasAdminPage {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions for this resource"})
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "1000"))
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
	// Hardened limit handling: default to 1000 if <= 0, and cap at 1000
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
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
	// Deterministic ordering (tie-breaker by primary key)
	orderClause := fmt.Sprintf(
		"%s %s, submissions.submission_id %s",
		sortField, strings.ToUpper(sortOrder), strings.ToUpper(sortOrder),
	)

	// ---------- Base list query (with preloads) ----------
	var submissions []models.Submission
	// Keep the list query lightweight. Form details are loaded below with two
	// explicit batch queries selecting only fields required by the table.
	listQ := config.DB.Preload("User").Preload("Year").Preload("Status").Preload("Category").Preload("Subcategory").
		Where("submissions.deleted_at IS NULL")

	draftStatusID, errDraft := utils.GetStatusIDByCode(utils.StatusCodeDraft)
	pendingStatusID, errPending := utils.GetStatusIDByCode(utils.StatusCodePending)
	approvedStatusID, errApproved := utils.GetStatusIDByCode(utils.StatusCodeApproved)
	rejectedStatusID, errRejected := utils.GetStatusIDByCode(utils.StatusCodeRejected)
	revisionStatusID, errRevision := utils.GetStatusIDByCode(utils.StatusCodeNeedsMoreInfo)
	deptHeadPendingStatusID, errDeptHeadPending := utils.GetStatusIDByCode(utils.StatusCodeDeptHeadPending)

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
		} else if resolved, err := utils.GetStatusIDByCode(status); err == nil {
			listQ = listQ.Where("submissions.status_id = ?", resolved)
		}
	} else if errDraft == nil && draftStatusID > 0 {
		listQ = listQ.Where("submissions.status_id <> ?", draftStatusID)
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
		listQ = listQ.Where("submissions.created_at >= ?", dateFrom)
	}
	if dateTo != "" {
		listQ = listQ.Where("submissions.created_at < DATE_ADD(?, INTERVAL 1 DAY)", dateTo)
	}
	if search != "" {
		st := "%" + search + "%"
		listQ = listQ.Where(`
            submissions.submission_number LIKE ? OR
            submissions.submission_id IN (SELECT submission_id FROM fund_application_details WHERE project_title LIKE ?) OR
            submissions.submission_id IN (SELECT submission_id FROM publication_reward_details WHERE paper_title LIKE ? OR paper_title_en LIKE ?) OR
            submissions.user_id IN (SELECT user_id FROM users WHERE CONCAT(user_fname, ' ', user_lname) LIKE ? OR email LIKE ?)`,
			st, st, st, st, st, st,
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

	if err := enrichAdminSubmissionListDetails(submissions); err != nil {
		log.Printf("GetAdminSubmissions list detail enrichment error: %v", err)
	}

	// ---------- Statistics (clone filters, count by status) ----------
	type Stats struct {
		TotalSubmissions     int64 `json:"total_submissions"`
		DeptHeadPendingCount int64 `json:"dept_head_pending_count"`
		PendingCount         int64 `json:"pending_count"`
		ApprovedCount        int64 `json:"approved_count"`
		RejectedCount        int64 `json:"rejected_count"`
		RevisionCount        int64 `json:"revision_count"`
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
			q = q.Where("submissions.created_at >= ?", dateFrom)
		}
		if dateTo != "" {
			q = q.Where("submissions.created_at < DATE_ADD(?, INTERVAL 1 DAY)", dateTo)
		}
		if search != "" {
			st := "%" + search + "%"
			q = q.Where(`
                submissions.submission_number LIKE ? OR
                submissions.submission_id IN (SELECT submission_id FROM fund_application_details WHERE project_title LIKE ?) OR
                submissions.submission_id IN (SELECT submission_id FROM publication_reward_details WHERE paper_title LIKE ? OR paper_title_en LIKE ?) OR
                submissions.user_id IN (SELECT user_id FROM users WHERE CONCAT(user_fname, ' ', user_lname) LIKE ? OR email LIKE ?)`,
				st, st, st, st, st, st,
			)
		}
		if status == "" && errDraft == nil && draftStatusID > 0 {
			q = q.Where("submissions.status_id <> ?", draftStatusID)
		}
		return q
	}

	// Count all status buckets in one pass. The previous implementation ran a
	// separate COUNT query for every status on every list request.
	if errPending == nil || errApproved == nil || errRejected == nil || errRevision == nil || errDeptHeadPending == nil {
		var aggregate struct {
			DeptHeadPending int64 `gorm:"column:dept_head_pending_count"`
			Pending         int64 `gorm:"column:pending_count"`
			Approved        int64 `gorm:"column:approved_count"`
			Rejected        int64 `gorm:"column:rejected_count"`
			Revision        int64 `gorm:"column:revision_count"`
		}
		statsQ := baseStats().Select(
			"COALESCE(SUM(CASE WHEN submissions.status_id = ? THEN 1 ELSE 0 END), 0) AS dept_head_pending_count, "+
				"COALESCE(SUM(CASE WHEN submissions.status_id = ? THEN 1 ELSE 0 END), 0) AS pending_count, "+
				"COALESCE(SUM(CASE WHEN submissions.status_id = ? THEN 1 ELSE 0 END), 0) AS approved_count, "+
				"COALESCE(SUM(CASE WHEN submissions.status_id = ? THEN 1 ELSE 0 END), 0) AS rejected_count, "+
				"COALESCE(SUM(CASE WHEN submissions.status_id = ? THEN 1 ELSE 0 END), 0) AS revision_count",
			deptHeadPendingStatusID, pendingStatusID, approvedStatusID, rejectedStatusID, revisionStatusID,
		)
		if err := statsQ.Scan(&aggregate).Error; err != nil {
			log.Printf("GetAdminSubmissions statistics error: %v", err)
		} else {
			stats.DeptHeadPendingCount = aggregate.DeptHeadPending
			stats.PendingCount = aggregate.Pending
			stats.ApprovedCount = aggregate.Approved
			stats.RejectedCount = aggregate.Rejected
			stats.RevisionCount = aggregate.Revision
		}
	}

	// ---------- Response ----------
	totalPages := (totalCount + int64(limit) - 1) / int64(limit)
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"submissions": toAdminSubmissionListItems(submissions),
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
		query = query.Where("submissions.year_id = ?", yearID)
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
