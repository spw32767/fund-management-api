// controllers/announcement.go
package controllers

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	allowedAnnouncementMimeTypes = map[string]string{
		"application/pdf":    "application/pdf",
		"application/msword": "application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	}
	announcementExtensionToMime = map[string]string{
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	}
	allowedFundFormMimeTypes = map[string]string{
		"application/pdf":    "application/pdf",
		"application/msword": "application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.ms-excel": "application/vnd.ms-excel",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}
	fundFormExtensionToMime = map[string]string{
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}
)

func canonicalMime(rawCT, filename string, allowed map[string]string, extMap map[string]string) (string, bool) {
	ct := strings.TrimSpace(rawCT)
	if ct != "" {
		if parsed, _, err := mime.ParseMediaType(ct); err == nil {
			ct = parsed
		}
		ct = strings.ToLower(ct)
		if canonical, ok := allowed[ct]; ok {
			return canonical, true
		}
	}

	if extMap != nil {
		ext := strings.ToLower(filepath.Ext(filename))
		if canonical, ok := extMap[ext]; ok {
			return canonical, true
		}
	}

	return "", false
}

var (
	announcementViewsOnce     sync.Once
	hasAnnouncementViewsTable bool
)

func announcementViewsAvailable() bool {
	announcementViewsOnce.Do(func() {
		hasAnnouncementViewsTable = config.DB.Migrator().HasTable(&models.AnnouncementView{})
	})
	return hasAnnouncementViewsTable
}

// ===== ANNOUNCEMENT CONTROLLERS =====

// GetAnnouncements - ดึงประกาศทั้งหมด
func GetAnnouncements(c *gin.Context) {
	// Query parameters
	announcementType := c.Query("type")
	status := c.Query("status")
	priority := c.Query("priority")
	activeOnly := c.Query("active_only") == "true"
	search := strings.TrimSpace(c.Query("q"))
	yearID := strings.TrimSpace(c.Query("year_id"))

	// Build query
	query := config.DB.Model(&models.Announcement{}).
		Preload("Creator").
		Preload("Year").
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
	if yearID != "" {
		if parsedYear, err := strconv.Atoi(yearID); err == nil {
			query = query.Where("year_id = ?", parsedYear)
		}
	}
	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"title LIKE ? OR file_name LIKE ? OR description LIKE ?",
			like,
			like,
			like,
		)
	}

	// Order by display_order (NULL ไปท้าย) แล้วค่อย fallback ที่เวลาอัปเดต/เผยแพร่
	query = query.
		Order("display_order IS NULL, display_order ASC").
		Order("COALESCE(published_at, update_at) DESC")

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
	if err := config.DB.Preload("Creator").Preload("Year").
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

// CreateAnnouncement - สร้างประกาศ (Admin only)
// CreateAnnouncement - สร้างประกาศ (บันทึกไฟล์ที่ uploads/announcements ชื่อเดิม)
func CreateAnnouncement(c *gin.Context) {
	roleID, ok := c.Get("roleID")
	if !ok || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}
	userID, _ := c.Get("userID")

	// bind + merge ค่า form เสมอ (เผื่อ multipart)
	var req models.AnnouncementCreateRequest
	_ = c.ShouldBind(&req)
	pf := func(k string) string { return strings.TrimSpace(c.PostForm(k)) }
	if v := pf("title"); v != "" {
		req.Title = v
	}
	if v := c.PostForm("description"); v != "" {
		req.Description = &v
	}
	if v := pf("announcement_type"); v != "" {
		req.AnnouncementType = v
	}
	if v := pf("priority"); v != "" {
		req.Priority = v
	}
	if v := pf("status"); v != "" {
		req.Status = v
	}
	if v := pf("announcement_reference_number"); v != "" {
		req.AnnouncementReferenceNumber = &v
	}
	if v := pf("year_id"); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			req.YearID = &iv
		}
	}
	if v := pf("display_order"); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			req.DisplayOrder = &iv
		}
	}
	if v := pf("published_at"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			req.PublishedAt = &t
		}
	}
	if v := pf("expired_at"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			req.ExpiredAt = &t
		}
	}

	// validate minimum
	if strings.TrimSpace(req.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if req.AnnouncementType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "announcement_type is required"})
		return
	}
	if req.Priority == "" {
		req.Priority = "normal"
	}
	if req.Status == "" {
		req.Status = "active"
	}

	// ไฟล์ (จำเป็น)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}
	defer file.Close()

	// ตรวจชนิด/ขนาดตามที่อนุญาต
	ct, ok := canonicalMime(header.Header.Get("Content-Type"), header.Filename, allowedAnnouncementMimeTypes, announcementExtensionToMime)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
		return
	}
	if header.Size > 20*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 20MB limit"})
		return
	}

	// ==== จุดสำคัญ: โฟลเดอร์ + ชื่อไฟล์ ====
	uploadDir := "uploads/announcements" // ไม่แยกปี/เดือน
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}
	// ใช้ชื่อไฟล์เดิม (sanitize กันอักขระต้องห้าม) — ไม่มี timestamp
	safeName := utils.SanitizeFilename(header.Filename)
	dstPath := filepath.Join(uploadDir, safeName)

	// ถ้าไฟล์ปลายทางมีอยู่แล้ว: เขียนทับ (ลบไฟล์เก่าก่อนเพื่อความชัวร์)
	if _, statErr := os.Stat(dstPath); statErr == nil {
		_ = os.Remove(dstPath)
	}

	if err := c.SaveUploadedFile(header, dstPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	now := time.Now()
	ann := models.Announcement{
		Title:                       req.Title,
		Description:                 req.Description,
		FileName:                    safeName, // ใช้ชื่อเดิม
		FilePath:                    dstPath,  // เก็บ relative path
		FileSize:                    &header.Size,
		MimeType:                    &ct,
		AnnouncementType:            req.AnnouncementType,
		Priority:                    req.Priority,
		DisplayOrder:                req.DisplayOrder,
		Status:                      req.Status,
		PublishedAt:                 req.PublishedAt,
		ExpiredAt:                   req.ExpiredAt,
		YearID:                      req.YearID,
		AnnouncementReferenceNumber: req.AnnouncementReferenceNumber,
		CreatedBy:                   userID.(int),
		CreateAt:                    now,
		UpdateAt:                    now,
	}

	if err := config.DB.Create(&ann).Error; err != nil {
		_ = os.Remove(dstPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create announcement"})
		return
	}

	config.DB.Preload("Creator").Preload("Year").First(&ann, ann.AnnouncementID)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Announcement created successfully",
		"data":    ann.ToResponse(),
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

	// Parse form/json (รองรับทั้ง JSON และ multipart)
	var req models.AnnouncementUpdateRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// ---- (อัปโหลดไฟล์ใหม่ถ้ามี) เหมือนเดิม ----
	file, header, err := c.Request.FormFile("file")
	var newFilePath string
	var oldFilePath string
	if err == nil {
		defer file.Close()

		// validate ชนิด/ขนาด
		ct, ok := canonicalMime(header.Header.Get("Content-Type"), header.Filename, allowedAnnouncementMimeTypes, announcementExtensionToMime)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
			return
		}
		if header.Size > 20*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 20MB limit"})
			return
		}

		// ==== จุดสำคัญ: โฟลเดอร์ + ชื่อไฟล์ ====
		uploadDir := "uploads/announcements" // ไม่แยกปี/เดือน
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
			return
		}
		safeName := utils.SanitizeFilename(header.Filename)
		dstPath := filepath.Join(uploadDir, safeName)

		// ถ้าไฟล์ปลายทางมีอยู่แล้ว ให้ลบทิ้งเพื่อเขียนทับ
		if _, statErr := os.Stat(dstPath); statErr == nil {
			_ = os.Remove(dstPath)
		}
		if err := c.SaveUploadedFile(header, dstPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		// อัปเดตฟิลด์ไฟล์ใน record
		oldFilePath = announcement.FilePath
		newFilePath = dstPath
		announcement.FileName = safeName
		announcement.FilePath = dstPath
		announcement.FileSize = &header.Size
		announcement.MimeType = &ct
	}

	// ---- อัปเดตฟิลด์ตาม request ----
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
	if req.DisplayOrder != nil {
		announcement.DisplayOrder = req.DisplayOrder
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

	// ✅ เพิ่มสองบรรทัดสำคัญที่ขาดไป
	if req.YearID != nil {
		announcement.YearID = req.YearID
	}
	if req.AnnouncementReferenceNumber != nil {
		announcement.AnnouncementReferenceNumber = req.AnnouncementReferenceNumber
	}

	announcement.UpdateAt = now

	if err := config.DB.Save(&announcement).Error; err != nil {
		if newFilePath != "" {
			os.Remove(newFilePath)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update announcement"})
		return
	}

	if oldFilePath != "" && newFilePath != "" && oldFilePath != newFilePath {
		_ = os.Remove(oldFilePath)
	}

	config.DB.Preload("Creator").Preload("Year").First(&announcement, announcement.AnnouncementID)
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
	if announcementViewsAvailable() {
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

			if userID, ok := getUserIDFromContext(c); ok {
				uid := int(userID)
				view.UserID = &uid
			}

			config.DB.Create(&view)
		}()
	}

	// Set headers for download
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", announcement.FileName))
	c.Header("Content-Type", "application/octet-stream")
	c.File(announcement.FilePath)
}

// ViewAnnouncementFile - ดูไฟล์ประกาศ (inline)
func ViewAnnouncementFile(c *gin.Context) {
	id := c.Param("id")

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
	if announcementViewsAvailable() {
		go func() {
			ipAddress := c.ClientIP()
			userAgent := c.GetHeader("User-Agent")

			view := models.AnnouncementView{
				AnnouncementID: announcement.AnnouncementID,
				IPAddress:      &ipAddress,
				UserAgent:      &userAgent,
				ViewedAt:       time.Now(),
			}

			if userID, ok := getUserIDFromContext(c); ok {
				uid := int(userID)
				view.UserID = &uid
			}

			config.DB.Create(&view)
		}()
	}

	// Set headers for inline viewing
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
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
	formType := strings.TrimSpace(c.Query("form_type"))
	if formType == "" {
		formType = c.Query("type")
	}
	fundCategory := strings.TrimSpace(c.Query("fund_category"))
	if fundCategory == "" {
		fundCategory = c.Query("category")
	}
	status := c.Query("status")
	activeOnly := c.Query("active_only") == "true"
	requiredOnly := c.Query("required_only") == "true"
	search := strings.TrimSpace(c.Query("q"))
	yearID := strings.TrimSpace(c.Query("year_id"))

	// Build query
	query := config.DB.Model(&models.FundForm{}).
		Preload("Creator").
		Preload("Year").
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
	if yearID != "" {
		if parsedYear, err := strconv.Atoi(yearID); err == nil {
			query = query.Where("year_id = ?", parsedYear)
		}
	}
	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"title LIKE ? OR file_name LIKE ? OR description LIKE ?",
			like,
			like,
			like,
		)
	}

	// Order by creation time (newest first)
	query = query.
		Order("display_order IS NULL, display_order ASC").
		Order("update_at DESC")

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
	if err := config.DB.Preload("Creator").Preload("Year").
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

// CreateFundForm - สร้างแบบฟอร์ม (ไฟล์อยู่ uploads/fund_forms ชื่อเดิม)
func CreateFundForm(c *gin.Context) {
	// 1) ตรวจสิทธิ์
	roleID, ok := c.Get("roleID")
	if !ok || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}
	userID, _ := c.Get("userID")

	// 2) Bind + merge จาก PostForm (รองรับ multipart)
	var req models.FundFormCreateRequest
	_ = c.ShouldBind(&req)
	pf := func(k string) string { return strings.TrimSpace(c.PostForm(k)) }

	if v := pf("title"); v != "" {
		req.Title = v
	}
	if v := c.PostForm("description"); v != "" {
		req.Description = &v
	}
	if v := pf("form_type"); v != "" {
		req.FormType = v
	}
	if v := pf("fund_category"); v != "" {
		req.FundCategory = v
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

	// 3) Validate ตามสคีมาจริง
	if strings.TrimSpace(req.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if req.FormType == "" {
		req.FormType = "application"
	} else {
		allowedFT := map[string]bool{"application": true, "report": true, "evaluation": true, "guidelines": true, "other": true}
		if !allowedFT[req.FormType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid form_type"})
			return
		}
	}
	if req.FundCategory == "" {
		req.FundCategory = "both"
	} else {
		allowedFC := map[string]bool{"research_fund": true, "promotion_fund": true, "both": true}
		if !allowedFC[req.FundCategory] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid fund_category"})
			return
		}
	}
	if req.Status == "" {
		req.Status = "active"
	} else if req.Status != "active" && req.Status != "inactive" && req.Status != "archived" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	// 4) รับไฟล์ (บังคับ)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}
	defer file.Close()

	ct, ok := canonicalMime(header.Header.Get("Content-Type"), header.Filename, allowedFundFormMimeTypes, fundFormExtensionToMime)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
		return
	}
	if header.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
		return
	}

	// 5) โฟลเดอร์ปลายทาง + ชื่อไฟล์เดิม (sanitize) — ไม่มี timestamp
	uploadDir := "uploads/fund_forms"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}
	safeName := utils.SanitizeFilename(header.Filename)
	dstPath := filepath.Join(uploadDir, safeName)

	// ถ้ามีไฟล์ชื่อซ้ำ ให้ลบทิ้งก่อนเพื่อเขียนทับ
	if _, statErr := os.Stat(dstPath); statErr == nil {
		_ = os.Remove(dstPath)
	}
	if err := c.SaveUploadedFile(header, dstPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// 6) เขียน DB
	now := time.Now()
	form := models.FundForm{
		Title:        req.Title,
		Description:  req.Description,
		FileName:     safeName,
		FilePath:     dstPath,
		FileSize:     &header.Size,
		MimeType:     &ct,
		FormType:     req.FormType,
		FundCategory: req.FundCategory,
		IsRequired:   req.IsRequired,
		DisplayOrder: req.DisplayOrder,
		Status:       req.Status,
		YearID:       req.YearID,
		CreatedBy:    userID.(int),
		CreateAt:     now,
		UpdateAt:     now,
	}
	if err := config.DB.Create(&form).Error; err != nil {
		_ = os.Remove(dstPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create fund form"})
		return
	}

	config.DB.Preload("Creator").Preload("Year").First(&form, form.FormID)
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Fund form created successfully", "data": form.ToResponse()})
}

// UpdateFundForm - แทนที่ไฟล์/แก้เมทาดาทา (ไฟล์อยู่ uploads/fund_forms ชื่อเดิม)
func UpdateFundForm(c *gin.Context) {
	// 1) ตรวจสิทธิ์
	roleID, ok := c.Get("roleID")
	if !ok || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	id := c.Param("id")
	var form models.FundForm
	if err := config.DB.Where("form_id = ? AND delete_at IS NULL", id).First(&form).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fund form not found"})
		return
	}

	// 2) Bind + merge
	var req models.FundFormUpdateRequest
	_ = c.ShouldBind(&req)
	pf := func(k string) string { return strings.TrimSpace(c.PostForm(k)) }
	if v := pf("title"); v != "" {
		req.Title = &v
	}
	if v := c.PostForm("description"); v != "" {
		req.Description = &v
	}
	if v := pf("form_type"); v != "" {
		req.FormType = &v
	}
	if v := pf("fund_category"); v != "" {
		req.FundCategory = &v
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

	// 3) (ถ้ามีไฟล์ใหม่) → เซฟลง uploads/fund_forms ด้วยชื่อเดิม
	file, header, err := c.Request.FormFile("file")
	var newPath, oldPath string
	if err == nil {
		defer file.Close()

		ct, ok := canonicalMime(header.Header.Get("Content-Type"), header.Filename, allowedFundFormMimeTypes, fundFormExtensionToMime)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
			return
		}
		if header.Size > 10*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
			return
		}

		uploadDir := "uploads/fund_forms"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
			return
		}
		safeName := utils.SanitizeFilename(header.Filename)
		dstPath := filepath.Join(uploadDir, safeName)

		if _, statErr := os.Stat(dstPath); statErr == nil {
			_ = os.Remove(dstPath)
		}
		if err := c.SaveUploadedFile(header, dstPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		oldPath = form.FilePath
		newPath = dstPath
		form.FileName = safeName
		form.FilePath = dstPath
		form.FileSize = &header.Size
		form.MimeType = &ct
	}

	// 4) อัปเดตเมทาดาทาตามที่ส่งมา
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
		form.IsRequired = req.IsRequired
	}
	if req.DisplayOrder != nil {
		form.DisplayOrder = req.DisplayOrder
	}
	if req.Status != nil {
		form.Status = *req.Status
	}
	if req.YearID != nil {
		form.YearID = req.YearID
	}
	form.UpdateAt = now

	if err := config.DB.Save(&form).Error; err != nil {
		if newPath != "" {
			_ = os.Remove(newPath)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update fund form"})
		return
	}
	if oldPath != "" && newPath != "" && oldPath != newPath {
		_ = os.Remove(oldPath)
	}

	config.DB.Preload("Creator").Preload("Year").First(&form, form.FormID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Fund form updated successfully", "data": form.ToResponse()})
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
	// download count removed in current schema

	// Count forms
	config.DB.Model(&models.FundForm{}).Where("delete_at IS NULL").Count(&stats.TotalForms)
	config.DB.Model(&models.FundForm{}).Where("delete_at IS NULL AND status = 'active'").Count(&stats.ActiveForms)
	// download count removed in current schema

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}
