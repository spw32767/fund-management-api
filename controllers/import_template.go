package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var allowedImportTemplateMimeTypes = map[string]string{
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ".xlsx",
	"application/vnd.ms-excel": ".xls",
}

var importTemplateExtensionToMime = map[string]string{
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".xls":  "application/vnd.ms-excel",
}

// GetImportTemplatesAdmin lists active import templates for admin panel
func GetImportTemplatesAdmin(c *gin.Context) {
	var templates []models.ImportTemplate
	status := strings.TrimSpace(c.Query("status"))
	templateType := strings.TrimSpace(c.Query("template_type"))
	activeOnly := c.Query("active_only") == "true"

	query := config.DB.Model(&models.ImportTemplate{}).
		Preload("Creator").
		Preload("Year").
		Where("delete_at IS NULL")

	if activeOnly {
		query = query.Where("status = ?", "active")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if templateType != "" {
		query = query.Where("template_type = ?", templateType)
	}

	query = query.Order("display_order IS NULL, display_order ASC").
		Order("update_at DESC")

	if err := query.Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch import templates"})
		return
	}

	responses := make([]models.ImportTemplateResponse, 0, len(templates))
	for _, t := range templates {
		responses = append(responses, t.ToResponse())
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    responses,
		"count":   len(responses),
	})
}

// CreateImportTemplateAdmin handles upload and creation of new import templates
func CreateImportTemplateAdmin(c *gin.Context) {
	roleID, ok := c.Get("roleID")
	if !ok || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}
	userID, _ := c.Get("userID")

	var req models.ImportTemplateCreateRequest
	_ = c.ShouldBind(&req)

	pf := func(k string) string { return strings.TrimSpace(c.PostForm(k)) }

	if v := pf("title"); v != "" {
		req.Title = v
	}
	if v := c.PostForm("description"); v != "" {
		req.Description = &v
	}
	if v := pf("template_type"); v != "" {
		req.TemplateType = v
	}
	if v := pf("is_required"); v != "" {
		b := v == "1" || strings.EqualFold(v, "true")
		req.IsRequired = &b
	}
	if v := pf("display_order"); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			req.DisplayOrder = &iv
		}
	}
	if v := pf("status"); v != "" {
		req.Status = v
	}
	if v := pf("year_id"); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			req.YearID = &iv
		}
	}

	if strings.TrimSpace(req.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	if req.TemplateType == "" {
		req.TemplateType = "other"
	} else {
		allowed := map[string]bool{"user_import": true, "legacy_submission": true, "other": true}
		if !allowed[req.TemplateType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template_type"})
			return
		}
	}

	if req.Status == "" {
		req.Status = "active"
	} else if req.Status != "active" && req.Status != "inactive" && req.Status != "archived" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}
	defer file.Close()

	ct, ok := canonicalMime(header.Header.Get("Content-Type"), header.Filename, allowedImportTemplateMimeTypes, importTemplateExtensionToMime)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
		return
	}
	if header.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
		return
	}

	uploadDir := "uploads/import_templates"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	safeName := utils.GenerateUniqueFilename(uploadDir, header.Filename)
	dstPath := filepath.Join(uploadDir, safeName)

	if err := c.SaveUploadedFile(header, dstPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	now := time.Now()
	template := models.ImportTemplate{
		Title:        req.Title,
		Description:  req.Description,
		FileName:     safeName,
		FilePath:     dstPath,
		FileSize:     &header.Size,
		MimeType:     &ct,
		TemplateType: req.TemplateType,
		IsRequired:   req.IsRequired,
		DisplayOrder: req.DisplayOrder,
		Status:       req.Status,
		YearID:       req.YearID,
		CreatedBy:    userID.(int),
		CreateAt:     now,
		UpdateAt:     now,
	}

	if err := config.DB.Create(&template).Error; err != nil {
		_ = os.Remove(dstPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create import template"})
		return
	}

	config.DB.Preload("Creator").Preload("Year").First(&template, template.TemplateID)
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Template created successfully", "data": template.ToResponse()})
}

// UpdateImportTemplateAdmin updates metadata and optionally replaces file
func UpdateImportTemplateAdmin(c *gin.Context) {
	roleID, ok := c.Get("roleID")
	if !ok || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	id := c.Param("id")
	var template models.ImportTemplate
	if err := config.DB.Where("template_id = ? AND delete_at IS NULL", id).First(&template).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Import template not found"})
		return
	}

	var req models.ImportTemplateUpdateRequest
	_ = c.ShouldBind(&req)

	pf := func(k string) string { return strings.TrimSpace(c.PostForm(k)) }
	if v := pf("title"); v != "" {
		req.Title = &v
	}
	if v := c.PostForm("description"); v != "" {
		req.Description = &v
	}
	if v := pf("template_type"); v != "" {
		req.TemplateType = &v
	}
	if v := pf("is_required"); v != "" {
		b := v == "1" || strings.EqualFold(v, "true")
		req.IsRequired = &b
	}
	if v := pf("display_order"); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			req.DisplayOrder = &iv
		}
	}
	if v := pf("status"); v != "" {
		req.Status = &v
	}
	if v := pf("year_id"); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			req.YearID = &iv
		}
	}

	if req.TemplateType != nil {
		allowed := map[string]bool{"user_import": true, "legacy_submission": true, "other": true}
		if !allowed[*req.TemplateType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template_type"})
			return
		}
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "inactive" && *req.Status != "archived" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
	}

	file, header, err := c.Request.FormFile("file")
	var newPath, oldPath string
	if err == nil {
		defer file.Close()

		ct, ok := canonicalMime(header.Header.Get("Content-Type"), header.Filename, allowedImportTemplateMimeTypes, importTemplateExtensionToMime)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
			return
		}
		if header.Size > 10*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
			return
		}

		uploadDir := "uploads/import_templates"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
			return
		}

		safeName := utils.GenerateUniqueFilename(uploadDir, header.Filename)
		dstPath := filepath.Join(uploadDir, safeName)

		if err := c.SaveUploadedFile(header, dstPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		oldPath = template.FilePath
		newPath = dstPath
		template.FileName = safeName
		template.FilePath = dstPath
		template.FileSize = &header.Size
		template.MimeType = &ct
	}

	now := time.Now()
	if req.Title != nil {
		template.Title = *req.Title
	}
	if req.Description != nil {
		template.Description = req.Description
	}
	if req.TemplateType != nil {
		template.TemplateType = *req.TemplateType
	}
	if req.IsRequired != nil {
		template.IsRequired = req.IsRequired
	}
	if req.DisplayOrder != nil {
		template.DisplayOrder = req.DisplayOrder
	}
	if req.Status != nil {
		template.Status = *req.Status
	}
	if req.YearID != nil {
		template.YearID = req.YearID
	}
	template.UpdateAt = now

	if err := config.DB.Save(&template).Error; err != nil {
		if newPath != "" {
			_ = os.Remove(newPath)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update import template"})
		return
	}
	if oldPath != "" && newPath != "" && oldPath != newPath {
		_ = os.Remove(oldPath)
	}

	config.DB.Preload("Creator").Preload("Year").First(&template, template.TemplateID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Template updated successfully", "data": template.ToResponse()})
}

// DeleteImportTemplateAdmin performs a soft delete
func DeleteImportTemplateAdmin(c *gin.Context) {
	roleID, ok := c.Get("roleID")
	if !ok || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	id := c.Param("id")
	var template models.ImportTemplate
	if err := config.DB.Where("template_id = ? AND delete_at IS NULL", id).First(&template).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Import template not found"})
		return
	}

	now := time.Now()
	template.DeleteAt = &now

	if err := config.DB.Save(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete import template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Template deleted successfully"})
}
