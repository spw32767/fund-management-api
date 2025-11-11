package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ensureAdmin checks if the current request is performed by an admin user
func ensureAdmin(c *gin.Context) bool {
	roleID, exists := c.Get("roleID")
	if !exists || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return false
	}
	return true
}

// ===================== PROJECTS =====================

// GetProjects lists all projects for admin management
func GetProjects(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	var projects []models.Project
	query := config.DB.Model(&models.Project{}).
		Preload("Type").
		Preload("BudgetPlan").
		Preload("Attachments", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		})

	if typeID := c.Query("type_id"); typeID != "" {
		query = query.Where("type_id = ?", typeID)
	}
	if planID := c.Query("plan_id"); planID != "" {
		query = query.Where("plan_id = ?", planID)
	}

	if err := query.Order("event_date DESC, project_id DESC").Find(&projects).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch projects"})
		return
	}

	responses := make([]gin.H, 0, len(projects))
	for _, project := range projects {
		responses = append(responses, gin.H{
			"project_id":    project.ProjectID,
			"project_name":  project.ProjectName,
			"type_id":       project.TypeID,
			"type":          project.Type,
			"plan_id":       project.PlanID,
			"budget_plan":   project.BudgetPlan,
			"event_date":    project.EventDate.Format("2006-01-02"),
			"budget_amount": project.BudgetAmount,
			"participants":  project.Participants,
			"notes":         project.Notes,
			"created_by":    project.CreatedBy,
			"created_at":    project.CreatedAt,
			"updated_at":    project.UpdatedAt,
			"attachments":   project.Attachments,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"projects": responses,
		"total":    len(responses),
	})
}

// CreateProject handles project creation
func CreateProject(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	type request struct {
		ProjectName  string  `json:"project_name" binding:"required"`
		TypeID       uint    `json:"type_id" binding:"required"`
		EventDate    string  `json:"event_date" binding:"required"`
		PlanID       uint    `json:"plan_id" binding:"required"`
		BudgetAmount float64 `json:"budget_amount" binding:"required"`
		Participants *int    `json:"participants"`
		Notes        *string `json:"notes"`
	}

	var req request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	eventDate, err := time.Parse("2006-01-02", req.EventDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event_date format. Use YYYY-MM-DD"})
		return
	}

	if err := ensureProjectTypeExists(req.TypeID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ensureBudgetPlanExists(req.PlanID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	participants := 0
	if req.Participants != nil {
		if *req.Participants < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "participants cannot be negative"})
			return
		}
		participants = *req.Participants
	}

	var createdByPtr *int
	if userID, ok := c.Get("userID"); ok {
		if v, ok := userID.(int); ok {
			createdByPtr = &v
		}
	}

	project := models.Project{
		ProjectName:  req.ProjectName,
		TypeID:       req.TypeID,
		EventDate:    eventDate,
		PlanID:       req.PlanID,
		BudgetAmount: req.BudgetAmount,
		Participants: participants,
		Notes:        req.Notes,
		CreatedBy:    createdByPtr,
	}

	if err := config.DB.Create(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}

	if err := config.DB.Preload("Type").Preload("BudgetPlan").First(&project, project.ProjectID).Error; err != nil {
		c.JSON(http.StatusCreated, gin.H{"success": true, "project_id": project.ProjectID})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Project created successfully",
		"project": gin.H{
			"project_id":    project.ProjectID,
			"project_name":  project.ProjectName,
			"type_id":       project.TypeID,
			"type":          project.Type,
			"plan_id":       project.PlanID,
			"budget_plan":   project.BudgetPlan,
			"event_date":    project.EventDate.Format("2006-01-02"),
			"budget_amount": project.BudgetAmount,
			"participants":  project.Participants,
			"notes":         project.Notes,
			"created_by":    project.CreatedBy,
			"created_at":    project.CreatedAt,
			"updated_at":    project.UpdatedAt,
		},
	})
}

// UpdateProject updates an existing project
func UpdateProject(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	var project models.Project
	if err := config.DB.First(&project, projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load project"})
		return
	}

	type request struct {
		ProjectName  *string  `json:"project_name"`
		TypeID       *uint    `json:"type_id"`
		EventDate    *string  `json:"event_date"`
		PlanID       *uint    `json:"plan_id"`
		BudgetAmount *float64 `json:"budget_amount"`
		Participants *int     `json:"participants"`
		Notes        *string  `json:"notes"`
	}

	var req request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}

	if req.ProjectName != nil {
		updates["project_name"] = *req.ProjectName
	}
	if req.TypeID != nil {
		if err := ensureProjectTypeExists(*req.TypeID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates["type_id"] = *req.TypeID
	}
	if req.PlanID != nil {
		if err := ensureBudgetPlanExists(*req.PlanID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates["plan_id"] = *req.PlanID
	}
	if req.EventDate != nil {
		parsedDate, err := time.Parse("2006-01-02", *req.EventDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event_date format. Use YYYY-MM-DD"})
			return
		}
		updates["event_date"] = parsedDate
	}
	if req.BudgetAmount != nil {
		updates["budget_amount"] = *req.BudgetAmount
	}
	if req.Participants != nil {
		if *req.Participants < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "participants cannot be negative"})
			return
		}
		updates["participants"] = *req.Participants
	}
	if req.Notes != nil {
		updates["notes"] = req.Notes
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No updates provided"})
		return
	}

	if err := config.DB.Model(&project).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project"})
		return
	}

	if err := config.DB.Preload("Type").Preload("BudgetPlan").First(&project, projectID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Project updated successfully",
		"project": gin.H{
			"project_id":    project.ProjectID,
			"project_name":  project.ProjectName,
			"type_id":       project.TypeID,
			"type":          project.Type,
			"plan_id":       project.PlanID,
			"budget_plan":   project.BudgetPlan,
			"event_date":    project.EventDate.Format("2006-01-02"),
			"budget_amount": project.BudgetAmount,
			"participants":  project.Participants,
			"notes":         project.Notes,
			"created_by":    project.CreatedBy,
			"created_at":    project.CreatedAt,
			"updated_at":    project.UpdatedAt,
		},
	})
}

// DeleteProject removes a project and its attachments
func DeleteProject(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	tx := config.DB.Begin()

	if err := tx.Where("project_id = ?", projectID).Delete(&models.ProjectAttachment{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete project attachments"})
		return
	}

	if err := tx.Delete(&models.Project{}, projectID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete project"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize project deletion"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Project deleted successfully",
	})
}

// ===================== PROJECT TYPES =====================

// GetProjectTypes lists all project types for admin management
func GetProjectTypes(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	var types []models.ProjectType
	if err := config.DB.Order("display_order ASC, type_id ASC").Find(&types).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch project types"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"types":   types,
		"total":   len(types),
	})
}

// CreateProjectType adds a new project type
func CreateProjectType(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	type request struct {
		NameTH       string `json:"name_th" binding:"required"`
		NameEN       string `json:"name_en"`
		DisplayOrder *int   `json:"display_order"`
		IsActive     *bool  `json:"is_active"`
	}

	var req request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order := 1
	if req.DisplayOrder != nil {
		order = *req.DisplayOrder
	} else {
		var maxOrder int
		config.DB.Model(&models.ProjectType{}).Select("COALESCE(MAX(display_order),0)").Scan(&maxOrder)
		order = maxOrder + 1
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	projectType := models.ProjectType{
		NameTH:       req.NameTH,
		NameEN:       req.NameEN,
		DisplayOrder: order,
		IsActive:     isActive,
	}

	if err := config.DB.Create(&projectType).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project type"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Project type created successfully",
		"type":    projectType,
	})
}

// UpdateProjectType updates an existing project type (except ID)
func UpdateProjectType(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	typeID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid type ID"})
		return
	}

	var projectType models.ProjectType
	if err := config.DB.First(&projectType, typeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Project type not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load project type"})
		return
	}

	type request struct {
		NameTH       *string `json:"name_th"`
		NameEN       *string `json:"name_en"`
		DisplayOrder *int    `json:"display_order"`
		IsActive     *bool   `json:"is_active"`
	}

	var req request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}

	if req.NameTH != nil {
		updates["name_th"] = *req.NameTH
	}
	if req.NameEN != nil {
		updates["name_en"] = *req.NameEN
	}
	if req.DisplayOrder != nil {
		updates["display_order"] = *req.DisplayOrder
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No updates provided"})
		return
	}

	if err := config.DB.Model(&projectType).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project type"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Project type updated successfully",
	})
}

// DeleteProjectType prevents deletion to keep sequential IDs
func DeleteProjectType(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "Deleting project types is not allowed"})
}

// ===================== PROJECT BUDGET PLANS =====================

// GetProjectBudgetPlans lists all budget plans for admin management
func GetProjectBudgetPlans(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	var plans []models.ProjectBudgetPlan
	if err := config.DB.Order("display_order ASC, plan_id ASC").Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch project budget plans"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"plans":   plans,
		"total":   len(plans),
	})
}

// CreateProjectBudgetPlan adds a new budget plan
func CreateProjectBudgetPlan(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	type request struct {
		NameTH       string `json:"name_th" binding:"required"`
		NameEN       string `json:"name_en"`
		DisplayOrder *int   `json:"display_order"`
		IsActive     *bool  `json:"is_active"`
	}

	var req request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order := 1
	if req.DisplayOrder != nil {
		order = *req.DisplayOrder
	} else {
		var maxOrder int
		config.DB.Model(&models.ProjectBudgetPlan{}).Select("COALESCE(MAX(display_order),0)").Scan(&maxOrder)
		order = maxOrder + 1
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	plan := models.ProjectBudgetPlan{
		NameTH:       req.NameTH,
		NameEN:       req.NameEN,
		DisplayOrder: order,
		IsActive:     isActive,
	}

	if err := config.DB.Create(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project budget plan"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Project budget plan created successfully",
		"plan":    plan,
	})
}

// UpdateProjectBudgetPlan updates an existing budget plan (except ID)
func UpdateProjectBudgetPlan(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	planID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plan ID"})
		return
	}

	var plan models.ProjectBudgetPlan
	if err := config.DB.First(&plan, planID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Project budget plan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load project budget plan"})
		return
	}

	type request struct {
		NameTH       *string `json:"name_th"`
		NameEN       *string `json:"name_en"`
		DisplayOrder *int    `json:"display_order"`
		IsActive     *bool   `json:"is_active"`
	}

	var req request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}

	if req.NameTH != nil {
		updates["name_th"] = *req.NameTH
	}
	if req.NameEN != nil {
		updates["name_en"] = *req.NameEN
	}
	if req.DisplayOrder != nil {
		updates["display_order"] = *req.DisplayOrder
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No updates provided"})
		return
	}

	if err := config.DB.Model(&plan).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project budget plan"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Project budget plan updated successfully",
	})
}

// DeleteProjectBudgetPlan prevents deletion to keep sequential IDs
func DeleteProjectBudgetPlan(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "Deleting project budget plans is not allowed"})
}

// ensureProjectTypeExists checks if a project type exists
func ensureProjectTypeExists(typeID uint) error {
	var count int64
	if err := config.DB.Model(&models.ProjectType{}).Where("type_id = ?", typeID).Count(&count).Error; err != nil {
		return errors.New("failed to verify project type")
	}
	if count == 0 {
		return errors.New("project type not found")
	}
	return nil
}

// ensureBudgetPlanExists checks if a project budget plan exists
func ensureBudgetPlanExists(planID uint) error {
	var count int64
	if err := config.DB.Model(&models.ProjectBudgetPlan{}).Where("plan_id = ?", planID).Count(&count).Error; err != nil {
		return errors.New("failed to verify project budget plan")
	}
	if count == 0 {
		return errors.New("project budget plan not found")
	}
	return nil
}
