package controllers

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

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
		Where("subcategorie_id = ? AND status = 'active'", req.SubcategoryID).
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

// GetCategories returns all active fund categories
func GetCategories(c *gin.Context) {
	yearID := c.Query("year_id")

	var categories []models.FundCategory
	query := config.DB.Where("status = 'active' AND delete_at IS NULL")

	if yearID != "" {
		query = query.Where("year_id = ?", yearID)
	}

	if err := query.Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch categories"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"categories": categories,
	})
}

// GetSubcategories returns subcategories with budget info
func GetSubcategories(c *gin.Context) {
	categoryID := c.Query("category_id")

	var subcategories []models.FundSubcategory
	query := config.DB.Preload("Category").Preload("SubcategoryBudget").
		Where("status = 'active' AND fund_subcategorie.delete_at IS NULL")

	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}

	if err := query.Find(&subcategories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subcategories"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"subcategories": subcategories,
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
