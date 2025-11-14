package controllers

import (
	"errors"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"

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

func fetchProjectsWithFilters(c *gin.Context) ([]models.Project, error) {
	var projects []models.Project
	query := config.DB.Model(&models.Project{}).
		Preload("Type").
		Preload("BudgetPlan").
		Preload("Attachments", func(db *gorm.DB) *gorm.DB {
			return db.Where("delete_at IS NULL").Order("display_order ASC, file_id ASC")
		}).
		Preload("Members", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC, member_id ASC")
		}).
		Preload("Members.User")

	if typeID := strings.TrimSpace(c.Query("type_id")); typeID != "" {
		query = query.Where("type_id = ?", typeID)
	}
	if planID := strings.TrimSpace(c.Query("plan_id")); planID != "" {
		query = query.Where("plan_id = ?", planID)
	}

	if err := query.Order("event_date DESC, project_id DESC").Find(&projects).Error; err != nil {
		return nil, err
	}

	return projects, nil
}

func filterProjectAttachments(attachments []models.ProjectAttachment, includeAll bool) []models.ProjectAttachment {
	if includeAll {
		return attachments
	}

	filtered := make([]models.ProjectAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.IsPublic {
			filtered = append(filtered, attachment)
		}
	}

	return filtered
}

func formatProjectAttachmentResponse(attachment models.ProjectAttachment, includeAdminFields bool) gin.H {
	response := gin.H{
		"file_id":       attachment.FileID,
		"project_id":    attachment.ProjectID,
		"original_name": attachment.OriginalName,
		"stored_path":   attachment.StoredPath,
		"file_size":     attachment.FileSize,
		"mime_type":     attachment.MimeType,
		"is_public":     attachment.IsPublic,
		"uploaded_at":   attachment.UploadedAt,
		"display_order": attachment.DisplayOrder,
	}

	if attachment.FileID != 0 {
		response["download_url"] = fmt.Sprintf("/projects/%d/attachments/%d", attachment.ProjectID, attachment.FileID)
	}

	if includeAdminFields {
		if attachment.FileHash != nil {
			response["file_hash"] = attachment.FileHash
		}
		if attachment.UploadedBy != nil {
			response["uploaded_by"] = attachment.UploadedBy
		}
		response["create_at"] = attachment.CreateAt
		response["update_at"] = attachment.UpdateAt
		if attachment.DeleteAt != nil {
			response["delete_at"] = attachment.DeleteAt
		}
	}

	return response
}

func formatProjectAttachments(attachments []models.ProjectAttachment, includeAdminFields bool) []gin.H {
	formatted := make([]gin.H, 0, len(attachments))
	for _, attachment := range attachments {
		formatted = append(formatted, formatProjectAttachmentResponse(attachment, includeAdminFields))
	}
	return formatted
}

func formatProjectMemberUser(user models.User) gin.H {
	if user.UserID == 0 {
		return nil
	}

	response := gin.H{
		"user_id":     user.UserID,
		"user_fname":  user.UserFname,
		"user_lname":  user.UserLname,
		"email":       user.Email,
		"role_id":     user.RoleID,
		"position_id": user.PositionID,
	}

	if user.Prefix != nil {
		response["prefix"] = user.Prefix
	}
	if user.ManagePosition != nil {
		response["manage_position"] = user.ManagePosition
	}
	if user.PositionTitle != nil {
		response["position_title"] = user.PositionTitle
	}
	if user.PositionEn != nil {
		response["position_en"] = user.PositionEn
	}
	if user.PrefixPositionEn != nil {
		response["prefix_position_en"] = user.PrefixPositionEn
	}
	if user.NameEn != nil {
		response["name_en"] = user.NameEn
	}
	if user.SuffixEn != nil {
		response["suffix_en"] = user.SuffixEn
	}
	if user.Tel != nil {
		response["tel"] = user.Tel
	}
	if user.Position.PositionID != 0 {
		response["position"] = gin.H{
			"position_id":   user.Position.PositionID,
			"position_name": user.Position.PositionName,
		}
	}
	if user.Role.RoleID != 0 {
		response["role"] = gin.H{
			"role_id": user.Role.RoleID,
			"role":    user.Role.Role,
		}
	}

	return response
}

func formatProjectMemberResponse(member models.ProjectMember, includeAdminFields bool) gin.H {
	response := gin.H{
		"member_id":      member.MemberID,
		"project_id":     member.ProjectID,
		"user_id":        member.UserID,
		"duty":           member.Duty,
		"workload_hours": member.WorkloadHours,
		"display_order":  member.DisplayOrder,
		"notes":          member.Notes,
		"user":           formatProjectMemberUser(member.User),
	}

	if includeAdminFields {
		response["created_at"] = member.CreatedAt
		if member.UpdatedAt != nil {
			response["updated_at"] = member.UpdatedAt
		}
	}

	return response
}

func formatProjectMembers(members []models.ProjectMember, includeAdminFields bool) []gin.H {
	formatted := make([]gin.H, 0, len(members))
	for _, member := range members {
		formatted = append(formatted, formatProjectMemberResponse(member, includeAdminFields))
	}
	return formatted
}

func formatProjectResponse(project models.Project, includeAdminFields bool, includeAllAttachments bool) gin.H {
	response := gin.H{
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
		"attachments":   formatProjectAttachments(filterProjectAttachments(project.Attachments, includeAllAttachments), includeAdminFields),
		"members":       formatProjectMembers(project.Members, includeAdminFields),
	}

	if includeAdminFields {
		response["created_by"] = project.CreatedBy
		response["created_at"] = project.CreatedAt
		response["updated_at"] = project.UpdatedAt
	}

	return response
}

// ===================== PROJECTS =====================

// GetProjects lists all projects for admin management
func GetProjects(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	projects, err := fetchProjectsWithFilters(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch projects"})
		return
	}

	responses := make([]gin.H, 0, len(projects))
	for _, project := range projects {
		responses = append(responses, formatProjectResponse(project, true, true))
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"projects": responses,
		"total":    len(responses),
	})
}

// GetProjectsForMembers lists public projects for authenticated members
func GetProjectsForMembers(c *gin.Context) {
	projects, err := fetchProjectsWithFilters(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch projects"})
		return
	}

	responses := make([]gin.H, 0, len(projects))
	for _, project := range projects {
		responses = append(responses, formatProjectResponse(project, false, false))
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

	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "multipart/form-data with attachment is required"})
		return
	}

	payload, attachment, err := bindCreateProjectPayload(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if attachment == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาแนบไฟล์โครงการ"})
		return
	}

	eventDate, err := time.Parse("2006-01-02", payload.EventDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event_date format. Use YYYY-MM-DD"})
		return
	}

	if err := ensureProjectTypeExists(payload.TypeID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ensureBudgetPlanExists(payload.PlanID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if payload.BudgetAmount < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "budget_amount must be zero or positive"})
		return
	}

	participants := 0
	if payload.Participants != nil {
		participants = *payload.Participants
	}

	var createdByPtr *int
	if userID, ok := c.Get("userID"); ok {
		if v, ok := userID.(int); ok {
			createdByPtr = &v
		}
	}

	var notesPtr *string
	if payload.Notes != nil {
		notesValue := strings.TrimSpace(*payload.Notes)
		notesPtr = &notesValue
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}

	project := models.Project{
		ProjectName:  payload.ProjectName,
		TypeID:       payload.TypeID,
		EventDate:    eventDate,
		PlanID:       payload.PlanID,
		BudgetAmount: payload.BudgetAmount,
		Participants: participants,
		Notes:        notesPtr,
		CreatedBy:    createdByPtr,
	}

	if err := tx.Create(&project).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}

	if _, err := replaceProjectAttachment(c, tx, project.ProjectID, attachment, createdByPtr); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save project attachment"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize project creation"})
		return
	}

	if err := config.DB.Preload("Type").
		Preload("BudgetPlan").
		Preload("Attachments", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC, file_id ASC")
		}).
		Preload("Members", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC, member_id ASC")
		}).
		Preload("Members.User").
		First(&project, project.ProjectID).Error; err != nil {
		c.JSON(http.StatusCreated, gin.H{"success": true, "project_id": project.ProjectID, "members": []gin.H{}})
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
			"attachments":   project.Attachments,
			"members":       formatProjectMembers(project.Members, true),
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

	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	isMultipart := strings.HasPrefix(contentType, "multipart/form-data")

	var payload *projectUpdatePayload
	var attachment *multipart.FileHeader

	if isMultipart {
		var err error
		payload, attachment, err = bindUpdateProjectPayload(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else {
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

		payload = &projectUpdatePayload{
			ProjectName:  trimStringPointer(req.ProjectName),
			TypeID:       req.TypeID,
			EventDate:    trimStringPointer(req.EventDate),
			PlanID:       req.PlanID,
			BudgetAmount: req.BudgetAmount,
			Participants: req.Participants,
			Notes:        trimStringPointer(req.Notes),
		}
	}

	updates := map[string]interface{}{}

	if payload.ProjectName != nil {
		name := strings.TrimSpace(*payload.ProjectName)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "project_name cannot be empty"})
			return
		}
		updates["project_name"] = name
	}
	if payload.TypeID != nil {
		if err := ensureProjectTypeExists(*payload.TypeID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates["type_id"] = *payload.TypeID
	}
	if payload.PlanID != nil {
		if err := ensureBudgetPlanExists(*payload.PlanID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates["plan_id"] = *payload.PlanID
	}
	if payload.EventDate != nil {
		dateStr := strings.TrimSpace(*payload.EventDate)
		if dateStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "event_date cannot be empty"})
			return
		}
		parsedDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event_date format. Use YYYY-MM-DD"})
			return
		}
		updates["event_date"] = parsedDate
	}
	if payload.BudgetAmount != nil {
		if *payload.BudgetAmount < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "budget_amount must be zero or positive"})
			return
		}
		updates["budget_amount"] = *payload.BudgetAmount
	}
	if payload.Participants != nil {
		if *payload.Participants < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "participants cannot be negative"})
			return
		}
		updates["participants"] = *payload.Participants
	}
	if payload.Notes != nil {
		noteValue := strings.TrimSpace(*payload.Notes)
		notePtr := noteValue
		updates["notes"] = &notePtr
	}

	if len(updates) == 0 && attachment == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No updates provided"})
		return
	}

	var uploaderPtr *int
	if userID, ok := c.Get("userID"); ok {
		if v, ok := userID.(int); ok {
			uploaderPtr = &v
		}
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project"})
		return
	}

	if len(updates) > 0 {
		if err := tx.Model(&project).Updates(updates).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project"})
			return
		}
	}

	if attachment != nil {
		if _, err := replaceProjectAttachment(c, tx, project.ProjectID, attachment, uploaderPtr); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save project attachment"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize project update"})
		return
	}

	if err := config.DB.Preload("Type").
		Preload("BudgetPlan").
		Preload("Attachments", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC, file_id ASC")
		}).
		Preload("Members", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC, member_id ASC")
		}).
		Preload("Members.User").
		First(&project, projectID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "members": []gin.H{}})
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
			"attachments":   project.Attachments,
			"members":       formatProjectMembers(project.Members, true),
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
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete project"})
		return
	}

	var attachments []models.ProjectAttachment
	if err := tx.Where("project_id = ?", projectID).Find(&attachments).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load project attachments"})
		return
	}

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

	removeProjectAttachmentFiles(attachments)

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

	trimmedNameTH := strings.TrimSpace(req.NameTH)
	if trimmedNameTH == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ต้องระบุชื่อภาษาไทย"})
		return
	}

	duplicate, err := projectTypeNameExists(trimmedNameTH, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบข้อมูลได้"})
		return
	}
	if duplicate {
		c.JSON(http.StatusConflict, gin.H{"error": "ชื่อซ้ำกัน"})
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
		NameTH:       trimmedNameTH,
		NameEN:       strings.TrimSpace(req.NameEN),
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
		trimmed := strings.TrimSpace(*req.NameTH)
		if trimmed == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ต้องระบุชื่อภาษาไทย"})
			return
		}
		duplicate, err := projectTypeNameExists(trimmed, uint(typeID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบข้อมูลได้"})
			return
		}
		if duplicate {
			c.JSON(http.StatusConflict, gin.H{"error": "ชื่อซ้ำกัน"})
			return
		}
		updates["name_th"] = trimmed
	}
	if req.NameEN != nil {
		updates["name_en"] = strings.TrimSpace(*req.NameEN)
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

	trimmedNameTH := strings.TrimSpace(req.NameTH)
	if trimmedNameTH == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ต้องระบุชื่อภาษาไทย"})
		return
	}

	duplicate, err := budgetPlanNameExists(trimmedNameTH, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบข้อมูลได้"})
		return
	}
	if duplicate {
		c.JSON(http.StatusConflict, gin.H{"error": "ชื่อซ้ำกัน"})
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
		NameTH:       trimmedNameTH,
		NameEN:       strings.TrimSpace(req.NameEN),
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
		trimmed := strings.TrimSpace(*req.NameTH)
		if trimmed == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ต้องระบุชื่อภาษาไทย"})
			return
		}
		duplicate, err := budgetPlanNameExists(trimmed, uint(planID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบข้อมูลได้"})
			return
		}
		if duplicate {
			c.JSON(http.StatusConflict, gin.H{"error": "ชื่อซ้ำกัน"})
			return
		}
		updates["name_th"] = trimmed
	}
	if req.NameEN != nil {
		updates["name_en"] = strings.TrimSpace(*req.NameEN)
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

// ReorderProjectTypes updates the display order based on the provided sequence
func ReorderProjectTypes(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	type request struct {
		Order []uint `json:"order"`
	}

	var req request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Order) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่พบข้อมูลลำดับ"})
		return
	}

	tx := config.DB.Begin()
	for index, id := range req.Order {
		order := index + 1
		if err := tx.Model(&models.ProjectType{}).Where("type_id = ?", id).Update("display_order", order).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกลำดับได้"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกลำดับได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "บันทึกลำดับประเภทโครงการเรียบร้อย"})
}

// ReorderProjectBudgetPlans updates the display order for budget plans based on the provided sequence
func ReorderProjectBudgetPlans(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	type request struct {
		Order []uint `json:"order"`
	}

	var req request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Order) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่พบข้อมูลลำดับ"})
		return
	}

	tx := config.DB.Begin()
	for index, id := range req.Order {
		order := index + 1
		if err := tx.Model(&models.ProjectBudgetPlan{}).Where("plan_id = ?", id).Update("display_order", order).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกลำดับได้"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกลำดับได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "บันทึกลำดับแผนงบประมาณเรียบร้อย"})
}

// ===================== PROJECT MEMBERS =====================

type projectMemberCreateRequest struct {
	UserID        uint     `json:"user_id" binding:"required"`
	Duty          string   `json:"duty" binding:"required"`
	WorkloadHours *float64 `json:"workload_hours"`
	Notes         *string  `json:"notes"`
}

type projectMemberUpdateRequest struct {
	UserID        *uint    `json:"user_id"`
	Duty          *string  `json:"duty"`
	WorkloadHours *float64 `json:"workload_hours"`
	Notes         *string  `json:"notes"`
}

// GetProjectMemberCandidates lists eligible users (non-admin) for project participation
func GetProjectMemberCandidates(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	var users []models.User
	if err := config.DB.Preload("Role").Preload("Position").
		Where("delete_at IS NULL").
		Where("role_id <> ?", 3).
		Order("user_fname ASC, user_lname ASC").
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถดึงรายชื่อผู้ใช้ได้"})
		return
	}

	responses := make([]gin.H, 0, len(users))
	for _, user := range users {
		formatted := formatProjectMemberUser(user)
		if formatted != nil {
			responses = append(responses, formatted)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"users":   responses,
		"total":   len(responses),
	})
}

// GetProjectMembers lists members assigned to a project ordered by display_order
func GetProjectMembers(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	projectIDValue, err := strconv.ParseUint(c.Param("projectId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}
	projectID := uint(projectIDValue)

	if err := ensureProjectExists(projectID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบโครงการ"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบโครงการได้"})
		return
	}

	var members []models.ProjectMember
	if err := config.DB.Where("project_id = ?", projectID).
		Preload("User.Role").
		Preload("User.Position").
		Order("display_order ASC, member_id ASC").
		Find(&members).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถดึงข้อมูลผู้ร่วมโครงการได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"members": formatProjectMembers(members, true),
		"total":   len(members),
	})
}

// CreateProjectMember adds a new member to a project
func CreateProjectMember(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	projectIDValue, err := strconv.ParseUint(c.Param("projectId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}
	projectID := uint(projectIDValue)

	if err := ensureProjectExists(projectID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบโครงการ"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบโครงการได้"})
		return
	}

	var req projectMemberCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	duty := strings.TrimSpace(req.Duty)
	if duty == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาระบุหน้าที่"})
		return
	}
	if utf8.RuneCountInString(duty) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "หน้าที่ต้องไม่เกิน 255 ตัวอักษร"})
		return
	}

	user, err := loadEligibleProjectUser(req.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่พบผู้ใช้หรือไม่สามารถเลือกผู้ใช้นี้ได้"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบผู้ใช้ได้"})
		return
	}

	var duplicateCount int64
	if err := config.DB.Model(&models.ProjectMember{}).
		Where("project_id = ? AND user_id = ?", projectID, req.UserID).
		Count(&duplicateCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบข้อมูลซ้ำได้"})
		return
	}
	if duplicateCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "ผู้ใช้นี้ถูกเพิ่มในโครงการแล้ว"})
		return
	}

	workload := 0.0
	if req.WorkloadHours != nil {
		workload, err = roundWorkloadHours(*req.WorkloadHours)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	notes, err := normalizeOptionalNotes(req.Notes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var maxOrder int64
	if err := config.DB.Model(&models.ProjectMember{}).
		Where("project_id = ?", projectID).
		Select("COALESCE(MAX(display_order), 0)").
		Scan(&maxOrder).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถกำหนดลำดับได้"})
		return
	}

	member := models.ProjectMember{
		ProjectID:     projectID,
		UserID:        req.UserID,
		Duty:          duty,
		WorkloadHours: workload,
		DisplayOrder:  int(maxOrder) + 1,
		Notes:         notes,
	}

	if err := config.DB.Create(&member).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกผู้ร่วมโครงการได้"})
		return
	}

	if err := config.DB.Preload("User.Role").Preload("User.Position").First(&member, member.MemberID).Error; err != nil {
		c.JSON(http.StatusCreated, gin.H{"success": true, "member_id": member.MemberID})
		return
	}

	member.User = *user

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"member":  formatProjectMemberResponse(member, true),
	})
}

// UpdateProjectMember updates duty, workload, notes or assigned user of a project member
func UpdateProjectMember(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	projectIDValue, err := strconv.ParseUint(c.Param("projectId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}
	memberIDValue, err := strconv.ParseUint(c.Param("memberId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid member ID"})
		return
	}
	projectID := uint(projectIDValue)
	memberID := uint(memberIDValue)

	if err := ensureProjectExists(projectID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบโครงการ"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบโครงการได้"})
		return
	}

	var member models.ProjectMember
	if err := config.DB.Where("project_id = ?", projectID).
		Preload("User.Role").
		Preload("User.Position").
		First(&member, memberID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบผู้ร่วมโครงการ"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดข้อมูลผู้ร่วมโครงการได้"})
		return
	}

	var req projectMemberUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}

	if req.UserID != nil && member.UserID != *req.UserID {
		user, err := loadEligibleProjectUser(*req.UserID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่พบผู้ใช้หรือไม่สามารถเลือกผู้ใช้นี้ได้"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบผู้ใช้ได้"})
			return
		}

		var duplicateCount int64
		if err := config.DB.Model(&models.ProjectMember{}).
			Where("project_id = ? AND user_id = ? AND member_id <> ?", projectID, *req.UserID, memberID).
			Count(&duplicateCount).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบข้อมูลซ้ำได้"})
			return
		}
		if duplicateCount > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "ผู้ใช้นี้ถูกเพิ่มในโครงการแล้ว"})
			return
		}

		updates["user_id"] = *req.UserID
		member.User = *user
	}

	if req.Duty != nil {
		duty := strings.TrimSpace(*req.Duty)
		if duty == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาระบุหน้าที่"})
			return
		}
		if utf8.RuneCountInString(duty) > 255 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "หน้าที่ต้องไม่เกิน 255 ตัวอักษร"})
			return
		}
		updates["duty"] = duty
	}

	if req.WorkloadHours != nil {
		workload, err := roundWorkloadHours(*req.WorkloadHours)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates["workload_hours"] = workload
	}

	if req.Notes != nil {
		notes, err := normalizeOptionalNotes(req.Notes)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates["notes"] = notes
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่มีข้อมูลที่ต้องการอัปเดต"})
		return
	}

	if err := config.DB.Model(&member).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถอัปเดตข้อมูลผู้ร่วมโครงการได้"})
		return
	}

	if err := config.DB.Preload("User.Role").Preload("User.Position").First(&member, memberID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"member":  formatProjectMemberResponse(member, true),
	})
}

// DeleteProjectMember removes a member from a project and reorders remaining members
func DeleteProjectMember(c *gin.Context) {
	if !ensureAdmin(c) {
		return
	}

	projectIDValue, err := strconv.ParseUint(c.Param("projectId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}
	memberIDValue, err := strconv.ParseUint(c.Param("memberId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid member ID"})
		return
	}
	projectID := uint(projectIDValue)
	memberID := uint(memberIDValue)

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบผู้ร่วมโครงการได้"})
		return
	}

	var member models.ProjectMember
	if err := tx.Where("project_id = ?", projectID).First(&member, memberID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบผู้ร่วมโครงการ"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบผู้ร่วมโครงการได้"})
		return
	}

	if err := tx.Delete(&member).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบผู้ร่วมโครงการได้"})
		return
	}

	if err := recalculateProjectMemberDisplayOrder(tx, projectID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถจัดลำดับผู้ร่วมโครงการใหม่ได้"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบผู้ร่วมโครงการได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "ลบผู้ร่วมโครงการเรียบร้อยแล้ว",
	})
}

func loadEligibleProjectUser(userID uint) (*models.User, error) {
	var user models.User
	if err := config.DB.Preload("Role").Preload("Position").
		Where("user_id = ? AND delete_at IS NULL AND role_id <> ?", userID, 3).
		First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func ensureProjectExists(projectID uint) error {
	if projectID == 0 {
		return gorm.ErrRecordNotFound
	}
	return config.DB.Select("project_id").First(&models.Project{}, projectID).Error
}

func roundWorkloadHours(value float64) (float64, error) {
	if value < 0 {
		return 0, fmt.Errorf("จำนวนชั่วโมงต้องไม่เป็นค่าติดลบ")
	}
	if value > 9999.99 {
		return 0, fmt.Errorf("จำนวนชั่วโมงต้องไม่เกิน 9999.99")
	}
	rounded := math.Round(value*100) / 100
	return rounded, nil
}

func normalizeOptionalNotes(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}

	if utf8.RuneCountInString(trimmed) > 255 {
		return nil, fmt.Errorf("หมายเหตุต้องไม่เกิน 255 ตัวอักษร")
	}

	return &trimmed, nil
}

func recalculateProjectMemberDisplayOrder(tx *gorm.DB, projectID uint) error {
	var members []models.ProjectMember
	if err := tx.Where("project_id = ?", projectID).
		Order("display_order ASC, member_id ASC").
		Find(&members).Error; err != nil {
		return err
	}

	for index, member := range members {
		desired := index + 1
		if member.DisplayOrder != desired {
			if err := tx.Model(&models.ProjectMember{}).
				Where("member_id = ?", member.MemberID).
				Update("display_order", desired).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func projectTypeNameExists(nameTH string, excludeID uint) (bool, error) {
	trimmed := strings.TrimSpace(nameTH)
	if trimmed == "" {
		return false, nil
	}

	query := config.DB.Model(&models.ProjectType{}).Where("TRIM(name_th) = ?", trimmed)
	if excludeID > 0 {
		query = query.Where("type_id <> ?", excludeID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

type projectCreatePayload struct {
	ProjectName  string
	TypeID       uint
	EventDate    string
	PlanID       uint
	BudgetAmount float64
	Participants *int
	Notes        *string
}

type projectUpdatePayload struct {
	ProjectName  *string
	TypeID       *uint
	EventDate    *string
	PlanID       *uint
	BudgetAmount *float64
	Participants *int
	Notes        *string
}

func budgetPlanNameExists(nameTH string, excludeID uint) (bool, error) {
	trimmed := strings.TrimSpace(nameTH)
	if trimmed == "" {
		return false, nil
	}

	query := config.DB.Model(&models.ProjectBudgetPlan{}).Where("TRIM(name_th) = ?", trimmed)
	if excludeID > 0 {
		query = query.Where("plan_id <> ?", excludeID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func bindCreateProjectPayload(c *gin.Context) (*projectCreatePayload, *multipart.FileHeader, error) {
	name := strings.TrimSpace(c.PostForm("project_name"))
	if name == "" {
		return nil, nil, errors.New("project_name is required")
	}

	typeValue := strings.TrimSpace(c.PostForm("type_id"))
	if typeValue == "" {
		return nil, nil, errors.New("type_id is required")
	}
	typeID, err := strconv.ParseUint(typeValue, 10, 64)
	if err != nil {
		return nil, nil, errors.New("invalid type_id")
	}

	eventDate := strings.TrimSpace(c.PostForm("event_date"))
	if eventDate == "" {
		return nil, nil, errors.New("event_date is required")
	}

	planValue := strings.TrimSpace(c.PostForm("plan_id"))
	if planValue == "" {
		return nil, nil, errors.New("plan_id is required")
	}
	planID, err := strconv.ParseUint(planValue, 10, 64)
	if err != nil {
		return nil, nil, errors.New("invalid plan_id")
	}

	budgetValue := strings.TrimSpace(c.PostForm("budget_amount"))
	if budgetValue == "" {
		return nil, nil, errors.New("budget_amount is required")
	}
	budgetAmount, err := strconv.ParseFloat(budgetValue, 64)
	if err != nil {
		return nil, nil, errors.New("invalid budget_amount")
	}

	var participantsPtr *int
	if participantsValue, exists := c.GetPostForm("participants"); exists {
		trimmed := strings.TrimSpace(participantsValue)
		if trimmed != "" {
			parsed, parseErr := strconv.Atoi(trimmed)
			if parseErr != nil {
				return nil, nil, errors.New("participants must be a number")
			}
			if parsed < 0 {
				return nil, nil, errors.New("participants cannot be negative")
			}
			participantsPtr = &parsed
		}
	}

	var notesPtr *string
	if notesValue, exists := c.GetPostForm("notes"); exists {
		trimmed := strings.TrimSpace(notesValue)
		notesPtr = &trimmed
	}

	file, err := c.FormFile("attachment")
	if err != nil {
		if !errors.Is(err, http.ErrMissingFile) {
			return nil, nil, err
		}
		file = nil
	}

	payload := &projectCreatePayload{
		ProjectName:  name,
		TypeID:       uint(typeID),
		EventDate:    eventDate,
		PlanID:       uint(planID),
		BudgetAmount: budgetAmount,
		Participants: participantsPtr,
		Notes:        notesPtr,
	}

	return payload, file, nil
}

func bindUpdateProjectPayload(c *gin.Context) (*projectUpdatePayload, *multipart.FileHeader, error) {
	payload := &projectUpdatePayload{}

	if nameValue, exists := c.GetPostForm("project_name"); exists {
		trimmed := strings.TrimSpace(nameValue)
		payload.ProjectName = &trimmed
	}

	if typeValue, exists := c.GetPostForm("type_id"); exists {
		trimmed := strings.TrimSpace(typeValue)
		if trimmed != "" {
			parsed, err := strconv.ParseUint(trimmed, 10, 64)
			if err != nil {
				return nil, nil, errors.New("invalid type_id")
			}
			typeID := uint(parsed)
			payload.TypeID = &typeID
		}
	}

	if eventValue, exists := c.GetPostForm("event_date"); exists {
		trimmed := strings.TrimSpace(eventValue)
		payload.EventDate = &trimmed
	}

	if planValue, exists := c.GetPostForm("plan_id"); exists {
		trimmed := strings.TrimSpace(planValue)
		if trimmed != "" {
			parsed, err := strconv.ParseUint(trimmed, 10, 64)
			if err != nil {
				return nil, nil, errors.New("invalid plan_id")
			}
			planID := uint(parsed)
			payload.PlanID = &planID
		}
	}

	if budgetValue, exists := c.GetPostForm("budget_amount"); exists {
		trimmed := strings.TrimSpace(budgetValue)
		if trimmed != "" {
			parsed, err := strconv.ParseFloat(trimmed, 64)
			if err != nil {
				return nil, nil, errors.New("invalid budget_amount")
			}
			payload.BudgetAmount = &parsed
		}
	}

	if participantsValue, exists := c.GetPostForm("participants"); exists {
		trimmed := strings.TrimSpace(participantsValue)
		if trimmed != "" {
			parsed, err := strconv.Atoi(trimmed)
			if err != nil {
				return nil, nil, errors.New("participants must be a number")
			}
			payload.Participants = &parsed
		}
	}

	if notesValue, exists := c.GetPostForm("notes"); exists {
		trimmed := strings.TrimSpace(notesValue)
		payload.Notes = &trimmed
	}

	file, err := c.FormFile("attachment")
	if err != nil {
		if !errors.Is(err, http.ErrMissingFile) {
			return nil, nil, err
		}
		file = nil
	}

	return payload, file, nil
}

func trimStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func replaceProjectAttachment(c *gin.Context, tx *gorm.DB, projectID uint, file *multipart.FileHeader, uploaderID *int) (*models.ProjectAttachment, error) {
	if file == nil {
		return nil, nil
	}

	attachment, err := saveProjectAttachment(c, tx, projectID, file, uploaderID)
	if err != nil {
		return nil, err
	}

	if err := deleteOtherProjectAttachments(tx, projectID, attachment.FileID); err != nil {
		removeProjectAttachmentFile(attachment)
		return nil, err
	}

	return attachment, nil
}

func saveProjectAttachment(c *gin.Context, tx *gorm.DB, projectID uint, file *multipart.FileHeader, uploaderID *int) (*models.ProjectAttachment, error) {
	if file == nil {
		return nil, nil
	}

	uploadRoot := getUploadRoot()
	projectFolder := filepath.Join(uploadRoot, "projects", fmt.Sprintf("%d", projectID))
	if err := utils.EnsureDirectoryExists(projectFolder); err != nil {
		return nil, err
	}

	originalName := strings.TrimSpace(file.Filename)
	if originalName == "" {
		originalName = file.Filename
	}
	storedName := utils.GenerateUniqueFilename(projectFolder, originalName)
	destination := filepath.Join(projectFolder, storedName)

	if err := c.SaveUploadedFile(file, destination); err != nil {
		return nil, err
	}

	relativePath := filepath.ToSlash(filepath.Join("projects", fmt.Sprintf("%d", projectID), storedName))
	mimeType := file.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = utils.GetMimeTypeFromExtension(filepath.Ext(storedName))
	}

	now := time.Now()
	attachment := models.ProjectAttachment{
		ProjectID:    projectID,
		OriginalName: originalName,
		StoredPath:   relativePath,
		FileSize:     uint64(file.Size),
		MimeType:     mimeType,
		IsPublic:     false,
		UploadedAt:   now,
		CreateAt:     now,
		UpdateAt:     now,
		DisplayOrder: 1,
	}
	if uploaderID != nil {
		attachment.UploadedBy = uploaderID
	}

	if err := tx.Create(&attachment).Error; err != nil {
		_ = os.Remove(destination)
		return nil, err
	}

	return &attachment, nil
}

func deleteOtherProjectAttachments(tx *gorm.DB, projectID uint, keepFileID uint) error {
	var existing []models.ProjectAttachment
	if err := tx.Where("project_id = ? AND file_id <> ?", projectID, keepFileID).Find(&existing).Error; err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}

	uploadRoot := getUploadRoot()
	for _, att := range existing {
		if att.StoredPath == "" {
			continue
		}
		fullPath := filepath.Join(uploadRoot, filepath.FromSlash(att.StoredPath))
		if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	ids := make([]uint, 0, len(existing))
	for _, att := range existing {
		ids = append(ids, att.FileID)
	}

	if len(ids) == 0 {
		return nil
	}

	if err := tx.Where("file_id IN ?", ids).Delete(&models.ProjectAttachment{}).Error; err != nil {
		return err
	}

	return nil
}

func removeProjectAttachmentFile(att *models.ProjectAttachment) {
	if att == nil || att.StoredPath == "" {
		return
	}
	uploadRoot := getUploadRoot()
	fullPath := filepath.Join(uploadRoot, filepath.FromSlash(att.StoredPath))
	if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Ignore cleanup errors to avoid masking the original failure
	}
}

func removeProjectAttachmentFiles(attachments []models.ProjectAttachment) {
	if len(attachments) == 0 {
		return
	}
	uploadRoot := getUploadRoot()
	for _, att := range attachments {
		if att.StoredPath == "" {
			continue
		}
		fullPath := filepath.Join(uploadRoot, filepath.FromSlash(att.StoredPath))
		if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			// Ignore cleanup errors
		}
	}
}

func getUploadRoot() string {
	uploadRoot := os.Getenv("UPLOAD_PATH")
	if uploadRoot == "" {
		uploadRoot = "./uploads"
	}
	return uploadRoot
}

// DownloadProjectAttachment streams a public project attachment for viewing
func DownloadProjectAttachment(c *gin.Context) {
	projectIDParam := c.Param("projectId")
	attachmentIDParam := c.Param("fileId")

	projectID, err := strconv.ParseUint(projectIDParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	attachmentID, err := strconv.ParseUint(attachmentIDParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid attachment id"})
		return
	}

	var attachment models.ProjectAttachment
	query := config.DB.Where(
		"project_id = ? AND file_id = ? AND delete_at IS NULL",
		projectID,
		attachmentID,
	)

	if roleIDValue, exists := c.Get("roleID"); !exists || roleIDValue.(int) != 3 {
		query = query.Where("is_public = ?", true)
	}

	if err := query.First(&attachment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load attachment"})
		return
	}

	storedPath := strings.TrimSpace(attachment.StoredPath)
	if storedPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment path missing"})
		return
	}

	uploadRoot := filepath.Clean(getUploadRoot())
	fullPath := filepath.Join(uploadRoot, filepath.FromSlash(storedPath))

	// Prevent path traversal outside the upload root
	if !strings.HasPrefix(fullPath, uploadRoot+string(os.PathSeparator)) && fullPath != uploadRoot {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid attachment path"})
		return
	}

	if _, err := os.Stat(fullPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read attachment"})
		return
	}

	mimeType := strings.TrimSpace(attachment.MimeType)
	if mimeType == "" {
		mimeType = utils.GetMimeTypeFromExtension(filepath.Ext(attachment.OriginalName))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
	}

	displayName := strings.TrimSpace(attachment.OriginalName)
	if displayName == "" {
		displayName = filepath.Base(fullPath)
	}

	encodedName := url.PathEscape(displayName)
	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"; filename*=UTF-8''%s", displayName, encodedName))
	c.File(fullPath)
}

// ensureProjectTypeExists checks if a project type exists
func ensureProjectTypeExists(typeID uint) error {
	var record models.ProjectType
	if err := config.DB.Select("type_id", "is_active").First(&record, "type_id = ?", typeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("project type not found")
		}
		return errors.New("failed to verify project type")
	}
	if !record.IsActive {
		return errors.New("project type is not active")
	}
	return nil
}

// ensureBudgetPlanExists checks if a project budget plan exists
func ensureBudgetPlanExists(planID uint) error {
	var record models.ProjectBudgetPlan
	if err := config.DB.Select("plan_id", "is_active").First(&record, "plan_id = ?", planID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("project budget plan not found")
		}
		return errors.New("failed to verify project budget plan")
	}
	if !record.IsActive {
		return errors.New("project budget plan is not active")
	}
	return nil
}
