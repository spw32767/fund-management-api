// controllers/announcement.go
package controllers

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

// ===== ANNOUNCEMENT CONTROLLERS =====

// GetAnnouncements - ดึงประกาศทั้งหมด
func GetAnnouncements(c *gin.Context) {
	// Query parameters
	announcementType := c.Query("type")
	status := c.Query("status")
	priority := c.Query("priority")
	activeOnly := c.Query("active_only") == "true"

	// Build query
	query := config.DB.Model(&models.Announcement{}).
		Preload("Creator").
		Where("delete_at IS NULL")

	// Apply filters
	if announcementType != "" {
		query = query.Where("announcement_type = ?", announcementType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if priority != "" {
		query = query.Where("priority = ?", priority)
	}
	if activeOnly {
		query = query.Where("status = ?", "active")
	}

	// Order by priority and published_at
	query = query.Order("CASE priority WHEN 'urgent' THEN 1 WHEN 'high' THEN 2 ELSE 3 END, published_at DESC")

	var announcements []models.Announcement
	if err := query.Find(&announcements).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch announcements"})
		return
	}

	// Convert to response format
	var responses []models.AnnouncementResponse
	for _, announcement := range announcements {
		responses = append(responses, announcement.ToResponse())
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    responses,
		"count":   len(responses),
	})
}

// GetAnnouncement - ดึงประกาศโดย ID
func GetAnnouncement(c *gin.Context) {
	id := c.Param("id")

	var announcement models.Announcement
	if err := config.DB.Preload("Creator").
		Where("announcement_id = ? AND delete_at IS NULL", id).
		First(&announcement).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Announcement not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    announcement.ToResponse(),
	})
}

// CreateAnnouncement - สร้างประกาศใหม่ (Admin only)
func CreateAnnouncement(c *gin.Context) {
	// Check admin role
	roleID, exists := c.Get("roleID")
	if !exists || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	userID, _ := c.Get("userID")

	// Parse form data
	var req models.AnnouncementCreateRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Handle file upload
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}
	defer file.Close()

	// Validate file type
	allowedTypes := map[string]bool{
		"application/pdf":    true,
		"application/msword": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/vnd.ms-excel": true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
		"image/jpeg": true,
		"image/png":  true,
	}

	contentType := header.Header.Get("Content-Type")
	if !allowedTypes[contentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
		return
	}

	// Validate file size (10MB max)
	if header.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
		return
	}

	// Create upload directory
	uploadDir := fmt.Sprintf("uploads/announcements/%d/%02d", time.Now().Year(), time.Now().Month())
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	// Generate unique filename
	ext := filepath.Ext(header.Filename)
	timestamp := time.Now().Format("20060102_150405")
	safeTitle := utils.SanitizeFilename(req.Title)
	filename := fmt.Sprintf("%s_%s%s", safeTitle, timestamp, ext)
	filePath := filepath.Join(uploadDir, filename)

	// Save file
	if err := c.SaveUploadedFile(header, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Create announcement record
	now := time.Now()
	announcement := models.Announcement{
		Title:            req.Title,
		Description:      req.Description,
		FileName:         header.Filename,
		FilePath:         filePath,
		FileSize:         &header.Size,
		MimeType:         &contentType,
		AnnouncementType: req.AnnouncementType,
		Priority:         utils.DefaultString(req.Priority, "normal"),
		Status:           utils.DefaultString(req.Status, "active"),
		PublishedAt:      req.PublishedAt,
		ExpiredAt:        req.ExpiredAt,
		CreatedBy:        userID.(int),
		CreateAt:         now,
		UpdateAt:         now,
	}

	if err := config.DB.Create(&announcement).Error; err != nil {
		// Remove uploaded file if database insert fails
		os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create announcement"})
		return
	}

	// Load creator info for response
	config.DB.Preload("Creator").First(&announcement, announcement.AnnouncementID)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Announcement created successfully",
		"data":    announcement.ToResponse(),
	})
}

// UpdateAnnouncement - แก้ไขประกาศ (Admin only)
func UpdateAnnouncement(c *gin.Context) {
	// Check admin role
	roleID, exists := c.Get("roleID")
	if !exists || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	id := c.Param("id")

	// Find existing announcement
	var announcement models.Announcement
	if err := config.DB.Where("announcement_id = ? AND delete_at IS NULL", id).
		First(&announcement).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Announcement not found"})
		return
	}

	// Parse form data
	var req models.AnnouncementUpdateRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Handle file upload (optional for update)
	file, header, err := c.Request.FormFile("file")
	var newFilePath string
	var oldFilePath string

	if err == nil {
		defer file.Close()

		// Validate file type
		allowedTypes := map[string]bool{
			"application/pdf":    true,
			"application/msword": true,
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
			"application/vnd.ms-excel": true,
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
			"image/jpeg": true,
			"image/png":  true,
		}

		contentType := header.Header.Get("Content-Type")
		if !allowedTypes[contentType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
			return
		}

		// Validate file size (10MB max)
		if header.Size > 10*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
			return
		}

		// Create upload directory
		uploadDir := fmt.Sprintf("uploads/announcements/%d/%02d", time.Now().Year(), time.Now().Month())
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
			return
		}

		// Generate unique filename
		ext := filepath.Ext(header.Filename)
		timestamp := time.Now().Format("20060102_150405")
		title := announcement.Title
		if req.Title != nil {
			title = *req.Title
		}
		safeTitle := utils.SanitizeFilename(title)
		filename := fmt.Sprintf("%s_%s%s", safeTitle, timestamp, ext)
		newFilePath = filepath.Join(uploadDir, filename)

		// Save new file
		if err := c.SaveUploadedFile(header, newFilePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		// Keep track of old file for deletion later
		oldFilePath = announcement.FilePath

		// Update file-related fields
		announcement.FileName = header.Filename
		announcement.FilePath = newFilePath
		announcement.FileSize = &header.Size
		announcement.MimeType = &contentType
	}

	// Update fields
	now := time.Now()
	if req.Title != nil {
		announcement.Title = *req.Title
	}
	if req.Description != nil {
		announcement.Description = req.Description
	}
	if req.AnnouncementType != nil {
		announcement.AnnouncementType = *req.AnnouncementType
	}
	if req.Priority != nil {
		announcement.Priority = *req.Priority
	}
	if req.Status != nil {
		announcement.Status = *req.Status
	}
	if req.PublishedAt != nil {
		announcement.PublishedAt = req.PublishedAt
	}
	if req.ExpiredAt != nil {
		announcement.ExpiredAt = req.ExpiredAt
	}
	announcement.UpdateAt = now

	// Save to database
	if err := config.DB.Save(&announcement).Error; err != nil {
		// Remove new file if database update fails
		if newFilePath != "" {
			os.Remove(newFilePath)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update announcement"})
		return
	}

	// Remove old file if new file was uploaded successfully
	if oldFilePath != "" && newFilePath != "" {
		os.Remove(oldFilePath)
	}

	// Load creator info for response
	config.DB.Preload("Creator").First(&announcement, announcement.AnnouncementID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Announcement updated successfully",
		"data":    announcement.ToResponse(),
	})
}

// DeleteAnnouncement - ลบประกาศ (Admin only)
func DeleteAnnouncement(c *gin.Context) {
	// Check admin role
	roleID, exists := c.Get("roleID")
	if !exists || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	id := c.Param("id")

	// Find announcement
	var announcement models.Announcement
	if err := config.DB.Where("announcement_id = ? AND delete_at IS NULL", id).
		First(&announcement).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Announcement not found"})
		return
	}

	// Soft delete
	now := time.Now()
	announcement.DeleteAt = &now

	if err := config.DB.Save(&announcement).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete announcement"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Announcement deleted successfully",
	})
}

// DownloadAnnouncementFile - ดาวน์โหลดไฟล์ประกาศ
func DownloadAnnouncementFile(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")

	// Find announcement
	var announcement models.Announcement
	if err := config.DB.Where("announcement_id = ? AND delete_at IS NULL", id).
		First(&announcement).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Announcement not found"})
		return
	}

	// Check if file exists
	if _, err := os.Stat(announcement.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Track download (optional)
	go func() {
		ipAddress := c.ClientIP()
		userAgent := c.GetHeader("User-Agent")

		// Record download in tracking table (if enabled)
		view := models.AnnouncementView{
			AnnouncementID: announcement.AnnouncementID,
			IPAddress:      &ipAddress,
			UserAgent:      &userAgent,
			ViewedAt:       time.Now(),
		}

		if userID != nil {
			view.UserID = userID.(*int)
		}

		config.DB.Create(&view)
	}()

	// Set headers for download
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", announcement.FileName))
	c.Header("Content-Type", "application/octet-stream")
	c.File(announcement.FilePath)
}

// ViewAnnouncementFile - ดูไฟล์ประกาศ (inline)
func ViewAnnouncementFile(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")

	// Find announcement
	var announcement models.Announcement
	if err := config.DB.Where("announcement_id = ? AND delete_at IS NULL", id).
		First(&announcement).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Announcement not found"})
		return
	}

	// Check if file exists
	if _, err := os.Stat(announcement.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Track view (optional)
	go func() {
		ipAddress := c.ClientIP()
		userAgent := c.GetHeader("User-Agent")

		view := models.AnnouncementView{
			AnnouncementID: announcement.AnnouncementID,
			IPAddress:      &ipAddress,
			UserAgent:      &userAgent,
			ViewedAt:       time.Now(),
		}

		if userID != nil {
			view.UserID = userID.(*int)
		}

		config.DB.Create(&view)
	}()

	// Set headers for inline viewing
	if announcement.MimeType != nil {
		c.Header("Content-Type", *announcement.MimeType)
	}
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", announcement.FileName))
	c.File(announcement.FilePath)
}

// ===== FUND FORM CONTROLLERS =====

// GetFundForms - ดึงแบบฟอร์มทั้งหมด
func GetFundForms(c *gin.Context) {
	// Query parameters
	formType := c.Query("type")
	fundCategory := c.Query("category")
	status := c.Query("status")
	activeOnly := c.Query("active_only") == "true"
	requiredOnly := c.Query("required_only") == "true"

	// Build query
	query := config.DB.Model(&models.FundForm{}).
		Preload("Creator").
		Where("delete_at IS NULL")

	// Apply filters
	if formType != "" {
		query = query.Where("form_type = ?", formType)
	}
	if fundCategory != "" {
		query = query.Where("fund_category = ? OR fund_category = 'both'", fundCategory)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if activeOnly {
		query = query.Where("status = ?", "active")
	}
	if requiredOnly {
		query = query.Where("is_required = ?", true)
	}

	// Order by form_type and create_at
	query = query.Order("CASE form_type WHEN 'application' THEN 1 WHEN 'guidelines' THEN 2 WHEN 'report' THEN 3 WHEN 'evaluation' THEN 4 ELSE 5 END, create_at DESC")

	var forms []models.FundForm
	if err := query.Find(&forms).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch fund forms"})
		return
	}

	// Convert to response format
	var responses []models.FundFormResponse
	for _, form := range forms {
		responses = append(responses, form.ToResponse())
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    responses,
		"count":   len(responses),
	})
}

// GetFundForm - ดึงแบบฟอร์มโดย ID
func GetFundForm(c *gin.Context) {
	id := c.Param("id")

	var form models.FundForm
	if err := config.DB.Preload("Creator").
		Where("form_id = ? AND delete_at IS NULL", id).
		First(&form).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fund form not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    form.ToResponse(),
	})
}

// CreateFundForm - สร้างแบบฟอร์มใหม่ (Admin only)
func CreateFundForm(c *gin.Context) {
	// Check admin role
	roleID, exists := c.Get("roleID")
	if !exists || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	userID, _ := c.Get("userID")

	// Parse form data
	var req models.FundFormCreateRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Handle file upload
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}
	defer file.Close()

	// Validate file type
	allowedTypes := map[string]bool{
		"application/pdf":    true,
		"application/msword": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/vnd.ms-excel": true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
	}

	contentType := header.Header.Get("Content-Type")
	if !allowedTypes[contentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
		return
	}

	// Validate file size (10MB max)
	if header.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
		return
	}

	// Create upload directory
	uploadDir := fmt.Sprintf("uploads/fund_forms/%s", req.FundCategory)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	// Generate unique filename
	ext := filepath.Ext(header.Filename)
	timestamp := time.Now().Format("20060102_150405")
	safeTitle := utils.SanitizeFilename(req.Title)
	filename := fmt.Sprintf("%s_v%s_%s%s", safeTitle, timestamp, ext)
	filePath := filepath.Join(uploadDir, filename)

	// Save file
	if err := c.SaveUploadedFile(header, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Create fund form record
	now := time.Now()
	form := models.FundForm{
		Title:        req.Title,
		Description:  req.Description,
		FileName:     header.Filename,
		FilePath:     filePath,
		FileSize:     &header.Size,
		MimeType:     &contentType,
		FormType:     req.FormType,
		FundCategory: req.FundCategory,
		IsRequired:   req.IsRequired,
		Status:       utils.DefaultString(req.Status, "active"),
		CreatedBy:    userID.(int),
		CreateAt:     now,
		UpdateAt:     now,
	}

	if err := config.DB.Create(&form).Error; err != nil {
		// Remove uploaded file if database insert fails
		os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create fund form"})
		return
	}

	// Load creator info for response
	config.DB.Preload("Creator").First(&form, form.FormID)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Fund form created successfully",
		"data":    form.ToResponse(),
	})
}

// UpdateFundForm - แก้ไขแบบฟอร์ม (Admin only)
func UpdateFundForm(c *gin.Context) {
	// Check admin role
	roleID, exists := c.Get("roleID")
	if !exists || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	id := c.Param("id")

	// Find existing form
	var form models.FundForm
	if err := config.DB.Where("form_id = ? AND delete_at IS NULL", id).
		First(&form).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fund form not found"})
		return
	}

	// Parse form data
	var req models.FundFormUpdateRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Handle file upload (optional for update)
	file, header, err := c.Request.FormFile("file")
	var newFilePath string
	var oldFilePath string

	if err == nil {
		defer file.Close()

		// Validate file type
		allowedTypes := map[string]bool{
			"application/pdf":    true,
			"application/msword": true,
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
			"application/vnd.ms-excel": true,
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
		}

		contentType := header.Header.Get("Content-Type")
		if !allowedTypes[contentType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
			return
		}

		// Validate file size (10MB max)
		if header.Size > 10*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
			return
		}

		// Determine upload directory
		fundCategory := form.FundCategory
		if req.FundCategory != nil {
			fundCategory = *req.FundCategory
		}
		uploadDir := fmt.Sprintf("uploads/fund_forms/%s", fundCategory)
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
			return
		}

		// Generate unique filename
		ext := filepath.Ext(header.Filename)
		timestamp := time.Now().Format("20060102_150405")
		title := form.Title
		if req.Title != nil {
			title = *req.Title
		}
		safeTitle := utils.SanitizeFilename(title)

		filename := fmt.Sprintf("%s_v%s_%s%s", safeTitle, timestamp, ext)
		newFilePath = filepath.Join(uploadDir, filename)

		// Save new file
		if err := c.SaveUploadedFile(header, newFilePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		// Keep track of old file for deletion later
		oldFilePath = form.FilePath

		// Update file-related fields
		form.FileName = header.Filename
		form.FilePath = newFilePath
		form.FileSize = &header.Size
		form.MimeType = &contentType
	}

	// Update fields
	now := time.Now()
	if req.Title != nil {
		form.Title = *req.Title
	}
	if req.Description != nil {
		form.Description = req.Description
	}
	if req.FormType != nil {
		form.FormType = *req.FormType
	}
	if req.FundCategory != nil {
		form.FundCategory = *req.FundCategory
	}
	if req.IsRequired != nil {
		form.IsRequired = *req.IsRequired
	}
	if req.Status != nil {
		form.Status = *req.Status
	}
	form.UpdateAt = now

	// Save to database
	if err := config.DB.Save(&form).Error; err != nil {
		// Remove new file if database update fails
		if newFilePath != "" {
			os.Remove(newFilePath)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update fund form"})
		return
	}

	// Remove old file if new file was uploaded successfully
	if oldFilePath != "" && newFilePath != "" {
		os.Remove(oldFilePath)
	}

	// Load creator info for response
	config.DB.Preload("Creator").First(&form, form.FormID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Fund form updated successfully",
		"data":    form.ToResponse(),
	})
}

// DeleteFundForm - ลบแบบฟอร์ม (Admin only)
func DeleteFundForm(c *gin.Context) {
	// Check admin role
	roleID, exists := c.Get("roleID")
	if !exists || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	id := c.Param("id")

	// Find form
	var form models.FundForm
	if err := config.DB.Where("form_id = ? AND delete_at IS NULL", id).
		First(&form).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fund form not found"})
		return
	}

	// Soft delete
	now := time.Now()
	form.DeleteAt = &now

	if err := config.DB.Save(&form).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete fund form"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Fund form deleted successfully",
	})
}

// DownloadFundForm - ดาวน์โหลดแบบฟอร์ม
func DownloadFundForm(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")

	// Find form
	var form models.FundForm
	if err := config.DB.Where("form_id = ? AND delete_at IS NULL", id).
		First(&form).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fund form not found"})
		return
	}

	// Check if file exists
	if _, err := os.Stat(form.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Track download (optional)
	go func() {
		ipAddress := c.ClientIP()
		userAgent := c.GetHeader("User-Agent")

		download := models.FormDownload{
			FormID:       form.FormID,
			IPAddress:    &ipAddress,
			UserAgent:    &userAgent,
			DownloadedAt: time.Now(),
		}

		if userID != nil {
			download.UserID = userID.(*int)
		}

		config.DB.Create(&download)
	}()

	// Set headers for download
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", form.FileName))
	c.Header("Content-Type", "application/octet-stream")
	c.File(form.FilePath)
}

// ViewFundForm - ดูแบบฟอร์ม (inline)
func ViewFundForm(c *gin.Context) {
	id := c.Param("id")

	// Find form
	var form models.FundForm
	if err := config.DB.Where("form_id = ? AND delete_at IS NULL", id).
		First(&form).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fund form not found"})
		return
	}

	// Check if file exists
	if _, err := os.Stat(form.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Set headers for inline viewing
	if form.MimeType != nil {
		c.Header("Content-Type", *form.MimeType)
	}
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", form.FileName))
	c.File(form.FilePath)
}

// ===== HELPER FUNCTIONS =====

// GetAnnouncementStats - สถิติประกาศ (Admin only)
func GetAnnouncementStats(c *gin.Context) {
	// Check admin role
	roleID, exists := c.Get("roleID")
	if !exists || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	var stats struct {
		TotalAnnouncements   int64 `json:"total_announcements"`
		ActiveAnnouncements  int64 `json:"active_announcements"`
		ExpiredAnnouncements int64 `json:"expired_announcements"`
		TotalViews           int64 `json:"total_views"`
		TotalDownloads       int64 `json:"total_downloads"`
		TotalForms           int64 `json:"total_forms"`
		ActiveForms          int64 `json:"active_forms"`
		FormDownloads        int64 `json:"form_downloads"`
	}

	// Count announcements
	config.DB.Model(&models.Announcement{}).Where("delete_at IS NULL").Count(&stats.TotalAnnouncements)
	config.DB.Model(&models.Announcement{}).Where("delete_at IS NULL AND status = 'active'").Count(&stats.ActiveAnnouncements)
	config.DB.Model(&models.Announcement{}).Where("delete_at IS NULL AND expired_at IS NOT NULL AND expired_at < NOW()").Count(&stats.ExpiredAnnouncements)

	// Sum views and downloads
	config.DB.Model(&models.Announcement{}).Where("delete_at IS NULL").Select("COALESCE(SUM(view_count), 0)").Scan(&stats.TotalViews)
	config.DB.Model(&models.Announcement{}).Where("delete_at IS NULL").Select("COALESCE(SUM(download_count), 0)").Scan(&stats.TotalDownloads)

	// Count forms
	config.DB.Model(&models.FundForm{}).Where("delete_at IS NULL").Count(&stats.TotalForms)
	config.DB.Model(&models.FundForm{}).Where("delete_at IS NULL AND status = 'active'").Count(&stats.ActiveForms)
	config.DB.Model(&models.FundForm{}).Where("delete_at IS NULL").Select("COALESCE(SUM(download_count), 0)").Scan(&stats.FormDownloads)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}
