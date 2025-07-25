package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Helper function to generate application number
func generateApplicationNumber() string {
	// Format: APP-YYYYMMDD-XXXX
	now := time.Now()
	dateStr := now.Format("20060102")

	// Count today's applications
	var count int64
	config.DB.Model(&models.FundApplication{}).
		Where("DATE(create_at) = DATE(NOW())").
		Count(&count)

	return fmt.Sprintf("APP-%s-%04d", dateStr, count+1)
}

// checkSubcategoryVisibility - Helper function to check if user can see subcategory
func checkSubcategoryVisibility(targetRolesJSON *string, userRoleID int) bool {
	// Admin sees everything
	if userRoleID == 3 {
		return true
	}

	// If no target_roles, everyone can see
	if targetRolesJSON == nil || *targetRolesJSON == "" {
		return true
	}

	// Parse target_roles
	var targetRoles []string
	if err := json.Unmarshal([]byte(*targetRolesJSON), &targetRoles); err != nil {
		// If parsing fails, hide by default
		return false
	}

	// Check if user's role is in target_roles
	userRoleStr := fmt.Sprintf("%d", userRoleID)
	for _, role := range targetRoles {
		if role == userRoleStr {
			return true
		}
	}

	return false
}

// GetApplications returns list of applications
func GetApplications(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var applications []models.FundApplication
	query := config.DB.Preload("User").Preload("Year").Preload("Subcategory").
		Preload("Subcategory.Category").Preload("ApplicationStatus").
		Where("fund_applications.delete_at IS NULL")

	// Filter by user if not admin
	if roleID.(int) != 3 { // 3 = admin role
		query = query.Where("user_id = ?", userID)
	}

	// Apply filters from query params
	if status := c.Query("status"); status != "" {
		query = query.Where("application_status_id = ?", status)
	}

	if year := c.Query("year"); year != "" {
		query = query.Where("year_id = ?", year)
	}

	if err := query.Find(&applications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch applications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"applications": applications,
		"total":        len(applications),
	})
}

// GetApplication returns single application by ID
func GetApplication(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var application models.FundApplication
	query := config.DB.Preload("User").Preload("Year").Preload("Subcategory").
		Preload("Subcategory.Category").Preload("Subcategory.SubcategoryBudget").
		Preload("ApplicationStatus").
		Where("application_id = ? AND fund_applications.delete_at IS NULL", id)

	// Check permission if not admin
	if roleID.(int) != 3 {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&application).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"application": application,
	})
}

// CreateApplication creates new fund application
func CreateApplication(c *gin.Context) {
	type CreateApplicationRequest struct {
		YearID             int     `json:"year_id" binding:"required"`
		SubcategoryID      int     `json:"subcategory_id" binding:"required"`
		ProjectTitle       string  `json:"project_title" binding:"required"`
		ProjectDescription string  `json:"project_description" binding:"required"`
		RequestedAmount    float64 `json:"requested_amount" binding:"required,gt=0"`
	}

	var req CreateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("userID")

	// Check if subcategory exists and has budget
	var subcategory models.FundSubcategory
	if err := config.DB.Preload("SubcategoryBudget").
		Where("subcategory_id = ? AND status = 'active'", req.SubcategoryID).
		First(&subcategory).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory"})
		return
	}

	// Check budget constraints
	budget := subcategory.SubcategoryBudget
	if budget.RemainingGrant <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No remaining grants available"})
		return
	}

	if req.RequestedAmount > budget.MaxAmountPerGrant {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Requested amount exceeds maximum allowed (%.2f)", budget.MaxAmountPerGrant),
		})
		return
	}

	if req.RequestedAmount > budget.RemainingBudget {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Insufficient budget remaining"})
		return
	}

	// Generate application number
	applicationNumber := generateApplicationNumber()

	// Create application
	now := time.Now()
	application := models.FundApplication{
		UserID:              userID.(int),
		YearID:              req.YearID,
		SubcategoryID:       req.SubcategoryID,
		ApplicationStatusID: 1, // 1 = รอพิจารณา
		ApplicationNumber:   applicationNumber,
		ProjectTitle:        req.ProjectTitle,
		ProjectDescription:  req.ProjectDescription,
		RequestedAmount:     req.RequestedAmount,
		ApprovedAmount:      0,
		SubmittedAt:         &now,
		CreateAt:            &now,
		UpdateAt:            &now,
	}

	if err := config.DB.Create(&application).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create application"})
		return
	}

	// Load relations
	config.DB.Preload("User").Preload("Year").Preload("Subcategory").
		Preload("ApplicationStatus").First(&application)

	c.JSON(http.StatusCreated, gin.H{
		"message":     "Application created successfully",
		"application": application,
	})
}

// UpdateApplication updates existing application
func UpdateApplication(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")

	type UpdateApplicationRequest struct {
		ProjectTitle       string  `json:"project_title"`
		ProjectDescription string  `json:"project_description"`
		RequestedAmount    float64 `json:"requested_amount"`
	}

	var req UpdateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find application
	var application models.FundApplication
	if err := config.DB.Where("application_id = ? AND user_id = ? AND delete_at IS NULL", id, userID).
		First(&application).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
		return
	}

	// Check if can be edited (only pending applications)
	if application.ApplicationStatusID != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot edit approved or rejected applications"})
		return
	}

	// Update fields
	now := time.Now()
	if req.ProjectTitle != "" {
		application.ProjectTitle = req.ProjectTitle
	}
	if req.ProjectDescription != "" {
		application.ProjectDescription = req.ProjectDescription
	}
	if req.RequestedAmount > 0 {
		// Check budget constraints
		var subcategory models.FundSubcategory
		config.DB.Preload("SubcategoryBudget").First(&subcategory, application.SubcategoryID)

		if req.RequestedAmount > subcategory.SubcategoryBudget.MaxAmountPerGrant {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Requested amount exceeds maximum allowed (%.2f)",
					subcategory.SubcategoryBudget.MaxAmountPerGrant),
			})
			return
		}

		application.RequestedAmount = req.RequestedAmount
	}
	application.UpdateAt = &now

	if err := config.DB.Save(&application).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update application"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Application updated successfully",
		"application": application,
	})
}

// DeleteApplication soft deletes an application
func DeleteApplication(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")

	var application models.FundApplication
	if err := config.DB.Where("application_id = ? AND user_id = ? AND delete_at IS NULL", id, userID).
		First(&application).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
		return
	}

	// Check if can be deleted (only pending applications)
	if application.ApplicationStatusID != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete approved or rejected applications"})
		return
	}

	// Soft delete
	now := time.Now()
	application.DeleteAt = &now

	if err := config.DB.Save(&application).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete application"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Application deleted successfully"})
}

// ApproveApplication approves a fund application (admin only)
func ApproveApplication(c *gin.Context) {
	id := c.Param("id")

	type ApprovalRequest struct {
		ApprovedAmount float64 `json:"approved_amount" binding:"required,gt=0"`
		Comment        string  `json:"comment"`
	}

	var req ApprovalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find application
	var application models.FundApplication
	if err := config.DB.Where("application_id = ? AND delete_at IS NULL", id).
		First(&application).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
		return
	}

	// Check if already processed
	if application.ApplicationStatusID != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Application already processed"})
		return
	}

	// Call stored procedure to update budget
	if err := config.DB.Exec("CALL UpdateBudgetAfterApproval(?, ?)",
		application.ApplicationID, req.ApprovedAmount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update budget"})
		return
	}

	// Update application status
	now := time.Now()
	userID, _ := c.Get("userID")

	// Get admin user info
	var adminUser models.User
	config.DB.First(&adminUser, userID)

	application.ApplicationStatusID = 2 // Approved
	application.ApprovedAmount = req.ApprovedAmount
	application.ApprovedAt = &now
	application.ApprovedBy = fmt.Sprintf("%s %s", adminUser.UserFname, adminUser.UserLname)
	application.Comment = req.Comment
	application.UpdateAt = &now

	if err := config.DB.Save(&application).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve application"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Application approved successfully",
		"application": application,
	})
}

// RejectApplication rejects a fund application (admin only)
func RejectApplication(c *gin.Context) {
	id := c.Param("id")

	type RejectRequest struct {
		Comment string `json:"comment" binding:"required"`
	}

	var req RejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find application
	var application models.FundApplication
	if err := config.DB.Where("application_id = ? AND delete_at IS NULL", id).
		First(&application).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Application not found"})
		return
	}

	// Check if already processed
	if application.ApplicationStatusID != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Application already processed"})
		return
	}

	// Update application
	now := time.Now()
	application.ApplicationStatusID = 3 // Rejected
	application.Comment = req.Comment
	application.UpdateAt = &now
	application.ClosedAt = &now

	if err := config.DB.Save(&application).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject application"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Application rejected",
		"application": application,
	})
}

// GetCategories - Fixed SQL syntax
func GetCategories(c *gin.Context) {
	yearID := c.Query("year_id")

	var categories []models.FundCategory

	// สร้าง query ใหม่โดยไม่มี semicolon
	query := config.DB.Table("fund_categories").
		Where("status = ?", "active").
		Where("delete_at IS NULL")

	if yearID != "" {
		query = query.Where("year_id = ?", yearID)
	}

	if err := query.Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch categories",
			"debug": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"categories": categories,
	})
}

// GetSubcategories - Fixed version without semicolon
func GetSubcategories(c *gin.Context) {
	categoryID := c.Query("category_id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	fmt.Printf("\n=== GetSubcategories Debug ===\n")
	fmt.Printf("categoryID: %s\n", categoryID)
	fmt.Printf("userID: %v\n", userID)
	fmt.Printf("roleID: %v\n", roleID)

	// Use query builder - เพิ่ม form_type, form_url ใน SELECT
	query := config.DB.Table("fund_subcategories fs").
		Select("fs.*, fs.form_type, fs.form_url, sb.*"). // เพิ่มฟิลด์ใหม่
		Joins("LEFT JOIN subcategory_budgets sb ON fs.subcategory_id = sb.subcategory_id AND sb.delete_at IS NULL AND sb.status = 'active'").
		Where("fs.status = ?", "active").
		Where("fs.delete_at IS NULL")

	if categoryID != "" {
		query = query.Where("fs.category_id = ?", categoryID)
	}

	// Apply role-based filtering
	roleIDStr := fmt.Sprintf("%d", roleID.(int))
	if roleID.(int) != 3 { // Not admin
		query = query.Where("(fs.target_roles IS NULL OR fs.target_roles = '' OR JSON_CONTAINS(fs.target_roles, ?))",
			fmt.Sprintf(`"%s"`, roleIDStr))
		fmt.Printf("Applied role filtering for role: %s\n", roleIDStr)
	}

	var subcategories []models.FundSubcategory
	if err := query.Find(&subcategories).Error; err != nil {
		fmt.Printf("Query error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch subcategories",
			"debug": err.Error(),
		})
		return
	}

	fmt.Printf("Found %d subcategories\n", len(subcategories))
	fmt.Printf("=== End GetSubcategories Debug ===\n\n")

	c.JSON(http.StatusOK, gin.H{
		"subcategories": subcategories,
		"role_id":       roleID,
		"user_id":       userID,
		"total":         len(subcategories),
	})
}

// GetYears returns all active years
func GetYears(c *gin.Context) {
	var years []models.Year
	if err := config.DB.Where("status = 'active' AND delete_at IS NULL").
		Order("year DESC").Find(&years).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch years"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"years": years,
	})
}

// GetTeacherSubcategories - Fixed SQL syntax for production server
func GetTeacherSubcategories(c *gin.Context) {
	categoryID := c.Query("category_id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Debug log
	fmt.Printf("\n=== GetTeacherSubcategories Debug ===\n")
	fmt.Printf("categoryID: %s\n", categoryID)
	fmt.Printf("userID: %v\n", userID)
	fmt.Printf("roleID: %v\n", roleID)

	var results []map[string]interface{}

	// Build base query - เพิ่ม form_type, form_url ใน SELECT
	baseQuery := `
		SELECT DISTINCT
			fs.subcategory_id,
			fs.subcategory_name,
			fs.category_id,
			fs.status,
			fs.fund_condition,
			fs.target_roles,
			fs.form_type,              -- เพิ่มฟิลด์ใหม่
			fs.form_url,               -- เพิ่มฟิลด์ใหม่
			fs.comment as sub_comment,
			sb.subcategory_budget_id,
			sb.allocated_amount,
			sb.used_amount,
			sb.remaining_budget,
			sb.max_grants,
			sb.max_amount_per_grant,
			sb.remaining_grant,
			sb.level,
			sb.fund_description,
			sb.comment as budget_comment
		FROM fund_subcategories fs
		LEFT JOIN subcategory_budgets sb ON fs.subcategory_id = sb.subcategory_id
			AND sb.delete_at IS NULL
			AND sb.status = 'active'
		WHERE fs.delete_at IS NULL 
			AND fs.status = 'active'`

	var conditions []string
	var args []interface{}

	// Add filters
	if categoryID != "" {
		conditions = append(conditions, "fs.category_id = ?")
		args = append(args, categoryID)
	}

	// Role-based filtering
	roleIDStr := fmt.Sprintf("%d", roleID.(int))
	if roleID.(int) != 3 { // Not admin
		conditions = append(conditions, "(fs.target_roles IS NULL OR fs.target_roles = '' OR JSON_CONTAINS(fs.target_roles, ?))")
		args = append(args, fmt.Sprintf(`"%s"`, roleIDStr))
		fmt.Printf("Applied role filtering for role: %s\n", roleIDStr)
	}

	// Append conditions
	if len(conditions) > 0 {
		baseQuery += " AND " + strings.Join(conditions, " AND ")
	}

	// Add ordering
	baseQuery += " ORDER BY fs.subcategory_id, CASE WHEN sb.level = 'ต้น' THEN 1 WHEN sb.level = 'กลาง' THEN 2 WHEN sb.level = 'สูง' THEN 3 ELSE 4 END"

	fmt.Printf("Final SQL: %s\n", baseQuery)
	fmt.Printf("Args: %v\n", args)

	// Execute query with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	rows, err := config.DB.WithContext(ctx).Raw(baseQuery, args...).Rows()
	if err != nil {
		fmt.Printf("Query error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch subcategories",
			"debug": err.Error(),
		})
		return
	}
	defer rows.Close()

	// Process results
	processedIDs := make(map[string]bool)

	for rows.Next() {
		var (
			subcategorieID       int
			subcategorieName     string
			categoryID           int
			status               string
			fundCondition        *string
			targetRoles          *string
			formType             *string // เพิ่มตัวแปรใหม่
			formURL              *string // เพิ่มตัวแปรใหม่
			subComment           *string
			subcategorieBudgetID *int
			allocatedAmount      *float64
			usedAmount           *float64
			remainingBudget      *float64
			maxGrants            *int
			maxAmountPerGrant    *float64
			remainingGrant       *int
			level                *string
			fundDescription      *string
			budgetComment        *string
		)

		err := rows.Scan(
			&subcategorieID,
			&subcategorieName,
			&categoryID,
			&status,
			&fundCondition,
			&targetRoles,
			&formType, // เพิ่มใน Scan
			&formURL,  // เพิ่มใน Scan
			&subComment,
			&subcategorieBudgetID,
			&allocatedAmount,
			&usedAmount,
			&remainingBudget,
			&maxGrants,
			&maxAmountPerGrant,
			&remainingGrant,
			&level,
			&fundDescription,
			&budgetComment,
		)
		if err != nil {
			fmt.Printf("Scan error: %v\n", err)
			continue
		}

		// Create unique ID
		uniqueID := fmt.Sprintf("%d", subcategorieID)
		displayName := subcategorieName

		// Handle level-based naming
		if subcategorieBudgetID != nil {
			if level != nil && *level != "" {
				uniqueID = fmt.Sprintf("%d_%s", subcategorieID, *level)
				displayName = fmt.Sprintf("%s (ระดับ%s)", subcategorieName, *level)
			} else if fundDescription != nil && *fundDescription != "" {
				uniqueID = fmt.Sprintf("%d_budget_%d", subcategorieID, *subcategorieBudgetID)
				displayName = fmt.Sprintf("%s - %s", subcategorieName, *fundDescription)
			}
		}

		// Skip if already processed
		if processedIDs[uniqueID] {
			continue
		}
		processedIDs[uniqueID] = true

		// Create result object - เพิ่มฟิลด์ใหม่
		result := map[string]interface{}{
			"subcategory_id":          uniqueID,
			"original_subcategory_id": subcategorieID,
			"subcategory_name":        displayName,
			"category_id":             categoryID,
			"status":                  status,
			"fund_condition":          fundCondition,
			"target_roles":            targetRoles,
			"form_type":               formType, // เพิ่มฟิลด์ใหม่
			"form_url":                formURL,  // เพิ่มฟิลด์ใหม่
			"comment":                 subComment,
			"visible_to_role":         roleID,
		}

		// Add budget information if available (เหมือนเดิม)
		if subcategorieBudgetID != nil {
			result["subcategorie_budget_id"] = *subcategorieBudgetID
			result["allocated_amount"] = 0.0
			result["used_amount"] = 0.0
			result["remaining_budget"] = 0.0
			result["max_amount_per_grant"] = 0.0
			result["is_unlimited_grants"] = true

			if allocatedAmount != nil {
				result["allocated_amount"] = *allocatedAmount
			}
			if usedAmount != nil {
				result["used_amount"] = *usedAmount
			}
			if remainingBudget != nil {
				result["remaining_budget"] = *remainingBudget
			}
			if maxAmountPerGrant != nil {
				result["max_amount_per_grant"] = *maxAmountPerGrant
			}
			if maxGrants != nil {
				result["max_grants"] = *maxGrants
				result["is_unlimited_grants"] = false
				if remainingGrant != nil {
					result["remaining_grant"] = *remainingGrant
				}
			}
			if level != nil {
				result["level"] = *level
			}
			if fundDescription != nil {
				result["fund_description"] = *fundDescription
			}
			if budgetComment != nil {
				result["budget_comment"] = *budgetComment
			}
		} else {
			// Default values when no budget (เหมือนเดิม)
			result["allocated_amount"] = 0.0
			result["used_amount"] = 0.0
			result["remaining_budget"] = 0.0
			result["max_grants"] = nil
			result["max_amount_per_grant"] = 0.0
			result["remaining_grant"] = nil
			result["is_unlimited_grants"] = false
		}

		results = append(results, result)
		fmt.Printf("Added fund: %s (ID: %s)\n", displayName, uniqueID)
	}

	// Check for any errors during iteration
	if err = rows.Err(); err != nil {
		fmt.Printf("Rows iteration error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error processing results",
			"debug": err.Error(),
		})
		return
	}

	fmt.Printf("Final result: %d subcategories for role %s\n", len(results), roleIDStr)
	fmt.Printf("=== End Debug ===\n\n")

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"subcategories": results,
		"role_id":       roleID,
		"user_id":       userID,
		"total":         len(results),
	})
}

// GetSubcategoryForRole - Unified function for role-based subcategory retrieval using raw SQL
func GetSubcategoryForRole(c *gin.Context) {
	// This function can use the same logic as GetTeacherSubcategories
	// since it handles all roles properly
	GetTeacherSubcategories(c)
}

// GetAllSubcategoriesAdmin - Admin endpoint to see all subcategories without filtering
func GetAllSubcategoriesAdmin(c *gin.Context) {
	categoryID := c.Query("category_id")

	// Build query - เพิ่ม form_type, form_url ใน SELECT
	baseQuery := `
		SELECT 
			fs.subcategory_id,
			fs.subcategory_name,
			fs.category_id,
			fs.status,
			fs.fund_condition,
			fs.target_roles,
			fs.form_type,           -- เพิ่มฟิลด์ใหม่
			fs.form_url,            -- เพิ่มฟิลด์ใหม่
			fs.comment,
			fc.category_name,
			COUNT(DISTINCT sb.subcategory_budget_id) as budget_count,
			COALESCE(SUM(sb.allocated_amount), 0) as total_allocated,
			COALESCE(SUM(sb.remaining_budget), 0) as total_remaining
		FROM fund_subcategories fs
		LEFT JOIN fund_categories fc ON fs.category_id = fc.category_id
		LEFT JOIN subcategory_budgets sb ON fs.subcategory_id = sb.subcategory_id
			AND sb.delete_at IS NULL
		WHERE fs.delete_at IS NULL`

	var args []interface{}

	// Add filters
	if categoryID != "" {
		baseQuery += " AND fs.category_id = ?"
		args = append(args, categoryID)
	}

	baseQuery += ` GROUP BY fs.subcategory_id, fs.subcategory_name, fs.category_id, 
			fs.status, fs.fund_condition, fs.target_roles, fs.form_type, fs.form_url,  
			fs.comment, fc.category_name
		ORDER BY fs.subcategory_id`

	// Execute query
	rows, err := config.DB.Raw(baseQuery, args...).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch subcategories",
		})
		return
	}
	defer rows.Close()

	// Process results
	var results []map[string]interface{}
	roleStats := map[string]int{
		"total":           0,
		"teacher_visible": 0,
		"staff_visible":   0,
		"admin_only":      0,
		"all_roles":       0,
	}

	for rows.Next() {
		var (
			subcategorieID   int
			subcategorieName string
			categoryID       int
			status           string
			fundCondition    *string
			targetRoles      *string
			formType         *string // เพิ่มตัวแปรใหม่
			formURL          *string // เพิ่มตัวแปรใหม่
			comment          *string
			categoryName     *string
			budgetCount      int
			totalAllocated   float64
			totalRemaining   float64
		)

		err := rows.Scan(
			&subcategorieID,
			&subcategorieName,
			&categoryID,
			&status,
			&fundCondition,
			&targetRoles,
			&formType, // เพิ่มใน Scan
			&formURL,  // เพิ่มใน Scan
			&comment,
			&categoryName,
			&budgetCount,
			&totalAllocated,
			&totalRemaining,
		)
		if err != nil {
			continue
		}

		// Parse target roles
		var targetRolesList []string
		if targetRoles != nil && *targetRoles != "" {
			json.Unmarshal([]byte(*targetRoles), &targetRolesList)
		}

		// Update statistics (เหมือนเดิม)
		roleStats["total"]++
		if checkSubcategoryVisibility(targetRoles, 1) {
			roleStats["teacher_visible"]++
		}
		if checkSubcategoryVisibility(targetRoles, 2) {
			roleStats["staff_visible"]++
		}
		if targetRoles == nil || *targetRoles == "" {
			roleStats["all_roles"]++
		} else if len(targetRolesList) == 1 && targetRolesList[0] == "3" {
			roleStats["admin_only"]++
		}

		// Create result - เพิ่มฟิลด์ใหม่
		result := map[string]interface{}{
			"subcategory_id":    subcategorieID,
			"subcategory_name":  subcategorieName,
			"category_id":       categoryID,
			"category_name":     categoryName,
			"status":            status,
			"fund_condition":    fundCondition,
			"target_roles":      targetRolesList,
			"target_roles_json": targetRoles,
			"form_type":         formType, // เพิ่มฟิลด์ใหม่
			"form_url":          formURL,  // เพิ่มฟิลด์ใหม่
			"comment":           comment,
			"has_budget":        budgetCount > 0,
			"budget_count":      budgetCount,
			"total_allocated":   totalAllocated,
			"total_remaining":   totalRemaining,
		}

		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"subcategories": results,
		"total":         len(results),
		"statistics":    roleStats,
	})
}

// UpdateSubcategoryTargetRoles - Admin only endpoint to update target_roles
func UpdateSubcategoryTargetRoles(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	subcategoryID := c.Param("id")

	type UpdateTargetRolesRequest struct {
		TargetRoles []string `json:"target_roles"`
	}

	var req UpdateTargetRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find subcategory
	var subcategory models.FundSubcategory
	if err := config.DB.Where("subcategory_id = ? AND delete_at IS NULL", subcategoryID).
		First(&subcategory).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subcategory not found"})
		return
	}

	// Convert target_roles to JSON string
	var targetRolesJSON string
	if len(req.TargetRoles) > 0 {
		jsonBytes, err := json.Marshal(req.TargetRoles)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target_roles format"})
			return
		}
		targetRolesJSON = string(jsonBytes)
	} else {
		targetRolesJSON = ""
	}

	// Update subcategory
	now := time.Now()
	subcategory.TargetRoles = &targetRolesJSON
	subcategory.UpdateAt = &now

	if err := config.DB.Save(&subcategory).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update subcategory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "Target roles updated successfully",
		"subcategory": subcategory,
	})
}

// CreateSubcategoryWithRoles - Admin only endpoint to create subcategory with target_roles
func CreateSubcategoryWithRoles(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	type CreateSubcategoryRequest struct {
		CategoryID      int      `json:"category_id" binding:"required"`
		SubcategoryName string   `json:"subcategory_name" binding:"required"`
		YearID          int      `json:"year_id" binding:"required"`
		FundCondition   string   `json:"fund_condition"`
		TargetRoles     []string `json:"target_roles"`
		FormType        string   `json:"form_type"` // เพิ่มฟิลด์ใหม่
		FormURL         string   `json:"form_url"`  // เพิ่มฟิลด์ใหม่
		Comment         string   `json:"comment"`
	}

	var req CreateSubcategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert target_roles to JSON string
	var targetRolesJSON *string
	if len(req.TargetRoles) > 0 {
		jsonBytes, err := json.Marshal(req.TargetRoles)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target_roles format"})
			return
		}
		jsonStr := string(jsonBytes)
		targetRolesJSON = &jsonStr
	}

	// Create new subcategory
	now := time.Now()
	subcategory := models.FundSubcategory{
		CategoryID:      req.CategoryID,
		SubcategoryName: req.SubcategoryName,
		//YearID:          req.YearID,
		FundCondition: &req.FundCondition,
		TargetRoles:   targetRolesJSON,
		Status:        "active",
		Comment:       &req.Comment,
		CreateAt:      &now,
		UpdateAt:      &now,
	}

	if err := config.DB.Create(&subcategory).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create subcategory"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":     true,
		"message":     "Subcategory created successfully",
		"subcategory": subcategory,
	})
}

// TestTargetRoles - Test endpoint to check target_roles in database
func TestTargetRoles(c *gin.Context) {
	fmt.Printf("\n=== Testing Target Roles ===\n")

	rows, err := config.DB.Raw(`
		SELECT subcategory_id, subcategory_name, target_roles 
		FROM fund_subcategories 
		WHERE delete_at IS NULL 
		ORDER BY subcategory_id
	`).Rows()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var results []gin.H
	for rows.Next() {
		var id int
		var name string
		var targetRoles *string

		rows.Scan(&id, &name, &targetRoles)

		var roles []string
		if targetRoles != nil && *targetRoles != "" {
			json.Unmarshal([]byte(*targetRoles), &roles)
		}

		result := gin.H{
			"id":           id,
			"name":         name,
			"target_roles": roles,
		}
		results = append(results, result)

		fmt.Printf("ID %d: %s -> %v\n", id, name, roles)
	}

	fmt.Printf("=== End Test ===\n\n")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
		"total":   len(results),
	})
}

// DebugUserRoleAccess - Debug endpoint to check what user can see
func DebugUserRoleAccess(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Get all subcategories
	query := `
		SELECT subcategory_id, subcategory_name, target_roles
		FROM fund_subcategories
		WHERE delete_at IS NULL AND status = 'active'
		ORDER BY subcategory_id`

	rows, err := config.DB.Raw(query).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	visible := []map[string]interface{}{}
	hidden := []map[string]interface{}{}

	for rows.Next() {
		var (
			id          int
			name        string
			targetRoles *string
		)
		rows.Scan(&id, &name, &targetRoles)

		info := map[string]interface{}{
			"id":           id,
			"name":         name,
			"target_roles": targetRoles,
		}

		if checkSubcategoryVisibility(targetRoles, roleID.(int)) {
			visible = append(visible, info)
		} else {
			hidden = append(hidden, info)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":       userID,
		"role_id":       roleID,
		"visible":       visible,
		"hidden":        hidden,
		"visible_count": len(visible),
		"hidden_count":  len(hidden),
		"total_count":   len(visible) + len(hidden),
	})
}

// TestRoleFiltering - Test endpoint to verify role filtering logic
func TestRoleFiltering(c *gin.Context) {
	testCases := []struct {
		TargetRoles string
		UserRole    int
		Expected    bool
		Description string
	}{
		{`["1"]`, 1, true, "Teacher can see teacher-only fund"},
		{`["1"]`, 2, false, "Staff cannot see teacher-only fund"},
		{`["1","2"]`, 1, true, "Teacher can see teacher+staff fund"},
		{`["1","2"]`, 2, true, "Staff can see teacher+staff fund"},
		{`["3"]`, 1, false, "Teacher cannot see admin-only fund"},
		{`[]`, 1, true, "Empty array = all can see"},
		{``, 1, true, "Null/empty = all can see"},
		{`["1"]`, 3, true, "Admin can see everything"},
	}

	results := []map[string]interface{}{}

	for _, tc := range testCases {
		var targetRoles *string
		if tc.TargetRoles != "" {
			targetRoles = &tc.TargetRoles
		}

		actual := checkSubcategoryVisibility(targetRoles, tc.UserRole)
		passed := actual == tc.Expected

		results = append(results, map[string]interface{}{
			"description":  tc.Description,
			"target_roles": tc.TargetRoles,
			"user_role":    tc.UserRole,
			"expected":     tc.Expected,
			"actual":       actual,
			"passed":       passed,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"test_results": results,
	})
}
