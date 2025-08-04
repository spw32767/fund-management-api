// controllers/publication.go
package controllers

import (
	"encoding/json"
	"fmt"
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

// GetPublicationRewards returns list of publication rewards
func GetPublicationRewards(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var rewards []models.PublicationReward
	query := config.DB.Preload("User").Preload("Coauthors.User").
		Preload("Documents").Preload("Comments.User").
		Where("publication_rewards.delete_at IS NULL")

	// Filter by user if not admin
	if roleID.(int) != 3 { // 3 = admin role
		query = query.Where("user_id = ?", userID)
	}

	// Apply filters from query params
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if year := c.Query("year"); year != "" {
		query = query.Where("journal_year = ?", year)
	}

	if err := query.Find(&rewards).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch publication rewards"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rewards": rewards,
		"total":   len(rewards),
	})
}

// GetPublicationReward returns single publication reward by ID
func GetPublicationReward(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var reward models.PublicationReward
	query := config.DB.Preload("User").Preload("Coauthors.User").
		Preload("Documents").Preload("Comments.User").
		Where("reward_id = ? AND publication_rewards.delete_at IS NULL", id)

	// Check permission if not admin
	if roleID.(int) != 3 {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&reward).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Publication reward not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reward": reward,
	})
}

// CreatePublicationReward creates new publication reward request with file upload
// CreatePublicationReward สร้าง publication reward (User-Based Folders)
func CreatePublicationReward(c *gin.Context) {
	userID, _ := c.Get("userID")

	// Parse multipart form
	err := c.Request.ParseMultipartForm(32 << 20) // 32 MB max
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form"})
		return
	}

	// Get JSON data from form
	jsonData := c.PostForm("data")
	if jsonData == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No data provided"})
		return
	}

	// Parse JSON data - ใช้ field ที่ตรงกับ models.PublicationReward จริง
	type CreateRewardRequest struct {
		AuthorStatus    string `json:"author_status"`
		ArticleTitle    string `json:"article_title"`
		JournalName     string `json:"journal_name"`
		JournalIssue    string `json:"journal_issue"`
		JournalPages    string `json:"journal_pages"`
		JournalMonth    string `json:"journal_month"`
		JournalYear     string `json:"journal_year"` // เปลี่ยนเป็น string
		DOI             string `json:"doi"`
		JournalQuartile string `json:"journal_quartile"` // เปลี่ยนจาก quartile
		// ลบ fields ที่ไม่มีใน models
		CoauthorIds []int `json:"coauthor_ids"`
	}

	var req CreateRewardRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON data"})
		return
	}

	// Get user info
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}

	// Begin transaction
	tx := config.DB.Begin()
	now := time.Now()

	// Create publication reward record - ใช้เฉพาะ fields ที่มีจริงใน models.PublicationReward
	reward := models.PublicationReward{
		UserID:          userID.(int),
		AuthorStatus:    req.AuthorStatus,
		ArticleTitle:    req.ArticleTitle,
		JournalName:     req.JournalName,
		JournalIssue:    req.JournalIssue,
		JournalPages:    req.JournalPages,
		JournalMonth:    req.JournalMonth,
		JournalYear:     req.JournalYear, // เป็น string แล้ว
		DOI:             req.DOI,
		JournalQuartile: req.JournalQuartile, // ใช้ชื่อ field ที่ถูกต้อง
		RewardNumber:    generateRewardNumber(),
		CreateAt:        &now,
		UpdateAt:        &now,
	}

	if err := tx.Create(&reward).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create publication reward"})
		return
	}

	// Create user folder and submission folder
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	userFolderPath, err := utils.CreateUserFolderIfNotExists(user, uploadPath)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user directory"})
		return
	}

	submissionFolderPath, err := utils.CreateSubmissionFolder(
		userFolderPath, "publication", reward.RewardID, now)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create submission directory"})
		return
	}

	// Process file uploads (same logic as UploadPublicationDocument)
	for fieldName, files := range c.Request.MultipartForm.File {
		var documentType string
		if strings.HasPrefix(fieldName, "doc_") {
			parts := strings.Split(fieldName, "_")
			if len(parts) >= 2 {
				documentType = parts[1]
			}
		}

		if documentType == "" {
			continue
		}

		for _, fileHeader := range files {
			// Use original filename with safety checks
			safeOriginalName := utils.SanitizeForFilename(fileHeader.Filename)
			finalFilename := fmt.Sprintf("pub_%s_%s", documentType, safeOriginalName)
			uniqueFilename := utils.GenerateUniqueFilename(submissionFolderPath, finalFilename)
			dst := filepath.Join(submissionFolderPath, uniqueFilename)

			// Save file
			if err := c.SaveUploadedFile(fileHeader, dst); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
				return
			}

			// Create FileUpload record
			fileUpload := models.FileUpload{
				OriginalName: fileHeader.Filename,
				StoredPath:   dst,
				FileSize:     fileHeader.Size,
				MimeType:     fileHeader.Header.Get("Content-Type"),
				FileHash:     "", // ไม่ใช้ hash ในระบบ user-based
				IsPublic:     false,
				UploadedBy:   userID.(int),
				UploadedAt:   now,
				CreateAt:     now,
				UpdateAt:     now,
			}

			if err := tx.Create(&fileUpload).Error; err != nil {
				tx.Rollback()
				os.Remove(dst)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file info"})
				return
			}

			// Create document record - ใช้ fields ที่มีจริงใน models.PublicationDocument
			fileType := strings.TrimPrefix(filepath.Ext(fileHeader.Filename), ".")
			doc := models.PublicationDocument{
				RewardID:         reward.RewardID,
				DocumentType:     documentType,
				OriginalFilename: fileHeader.Filename,
				StoredFilename:   uniqueFilename,
				FileType:         fileType,
				UploadedBy:       userID.(int),
				UploadedAt:       &now,
				CreateAt:         &now,
			}

			if err := tx.Create(&doc).Error; err != nil {
				tx.Rollback()
				os.Remove(dst)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save document record"})
				return
			}
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save publication reward"})
		return
	}

	// Load relations
	config.DB.Preload("User").Preload("Coauthors.User").Preload("Documents").First(&reward, reward.RewardID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Publication reward created successfully",
		"reward":  reward,
		"folder":  submissionFolderPath,
	})
}

// UpdatePublicationReward updates existing publication reward
func UpdatePublicationReward(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")

	var reward models.PublicationReward
	if err := config.DB.Where("reward_id = ? AND user_id = ? AND delete_at IS NULL", id, userID).
		First(&reward).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Publication reward not found"})
		return
	}

	// Check if can be edited (only draft and submitted)
	if reward.Status != "draft" && reward.Status != "submitted" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot edit approved or paid rewards"})
		return
	}

	// Parse request body
	var updateData map[string]interface{}
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update only allowed fields
	allowedFields := []string{
		"article_title", "journal_name", "journal_issue", "journal_pages",
		"journal_month", "journal_year", "journal_url", "doi",
		"journal_tier", "journal_quartile", "in_isi", "in_scopus",
		"article_type", "journal_type", "editor_fee",
		"publication_fee_university", "publication_fee_college",
		"university_ranking", "bank_account", "bank_name", "phone_number",
		"has_university_fund", "university_fund_ref",
	}

	updates := make(map[string]interface{})
	for _, field := range allowedFields {
		if val, ok := updateData[field]; ok {
			updates[field] = val
		}
	}

	// Recalculate if author status or quartile changed
	if authorStatus, ok := updateData["author_status"]; ok {
		if quartile, ok := updateData["journal_quartile"]; ok {
			updates["publication_reward"] = calculatePublicationReward(
				authorStatus.(string),
				quartile.(string),
			)
		}
	}

	// Update timestamp
	now := time.Now()
	updates["update_at"] = now

	if err := config.DB.Model(&reward).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update publication reward"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Publication reward updated successfully",
		"reward":  reward,
	})
}

// DeletePublicationReward soft deletes a publication reward
func DeletePublicationReward(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")

	var reward models.PublicationReward
	if err := config.DB.Where("reward_id = ? AND user_id = ? AND delete_at IS NULL", id, userID).
		First(&reward).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Publication reward not found"})
		return
	}

	// Check if can be deleted (only draft)
	if reward.Status != "draft" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Can only delete draft rewards"})
		return
	}

	// Soft delete
	now := time.Now()
	reward.DeleteAt = &now

	if err := config.DB.Save(&reward).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete publication reward"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Publication reward deleted successfully"})
}

// ApprovePublicationReward approves a publication reward (admin only)
func ApprovePublicationReward(c *gin.Context) {
	id := c.Param("id")

	var reward models.PublicationReward
	if err := config.DB.Where("reward_id = ? AND delete_at IS NULL", id).
		First(&reward).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Publication reward not found"})
		return
	}

	// Check if already processed
	if reward.Status != "submitted" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reward not in submitted status"})
		return
	}

	// Update status
	now := time.Now()
	reward.Status = "approved"
	reward.ApprovedAt = &now
	reward.UpdateAt = &now

	if err := config.DB.Save(&reward).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve publication reward"})
		return
	}

	// Add comment if provided
	var requestBody struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&requestBody); err == nil && requestBody.Comment != "" {
		userID, _ := c.Get("userID")
		comment := models.PublicationComment{
			RewardID:      reward.RewardID,
			CommentBy:     userID.(int),
			CommentText:   requestBody.Comment,
			CommentStatus: "approved",
			CreateAt:      &now,
		}
		config.DB.Create(&comment)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Publication reward approved successfully",
		"reward":  reward,
	})
}

// GetPublicationRewardRates returns reward rates configuration
func GetPublicationRewardRates(c *gin.Context) {
	year := c.Query("year")
	if year == "" {
		year = strconv.Itoa(time.Now().Year() + 543) // Buddhist year
	}

	var rates []models.PublicationRewardRate
	if err := config.DB.Where("year = ? AND is_active = ?", year, true).
		Find(&rates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reward rates"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rates": rates,
		"year":  year,
	})
}

// Helper function to calculate publication reward
func calculatePublicationReward(authorStatus, quartile string) float64 {
	// In real implementation, this would fetch from publication_reward_rates table
	rates := map[string]map[string]float64{
		"first_author": {
			"Q1": 100000,
			"Q2": 75000,
			"Q3": 50000,
			"Q4": 25000,
		},
		"corresponding_author": {
			"Q1": 50000,
			"Q2": 30000,
			"Q3": 15000,
			"Q4": 7500,
		},
		"co_author": {
			"Q1": 0,
			"Q2": 0,
			"Q3": 0,
			"Q4": 0,
		},
	}

	if statusRates, ok := rates[authorStatus]; ok {
		if amount, ok := statusRates[quartile]; ok {
			return amount
		}
	}

	return 0
}

// Helper function to generate reward number
func generateRewardNumber() string {
	// Format: S-XXXX (where XXXX is sequential number)
	var count int64
	config.DB.Model(&models.PublicationReward{}).Count(&count)
	return fmt.Sprintf("S-%04d", count+1)
}

// Upload/Get Publication Documents Made by Co-pilot
// UploadPublicationDocument handles document upload for publication rewards
// UploadPublicationDocument แก้ไขให้ตรงกับ schema จริง
func UploadPublicationDocument(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")

	// Verify ownership
	var reward models.PublicationReward
	if err := config.DB.Where("reward_id = ? AND user_id = ? AND delete_at IS NULL", id, userID).
		First(&reward).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Publication reward not found"})
		return
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form"})
		return
	}

	// Get user info
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}

	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	// Create user folder if not exists
	userFolderPath, err := utils.CreateUserFolderIfNotExists(user, uploadPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user directory"})
		return
	}

	// Create publication submission folder
	submissionFolderPath, err := utils.CreateSubmissionFolder(
		userFolderPath, "publication", reward.RewardID, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create submission directory"})
		return
	}

	var uploadedFiles []models.PublicationDocument
	now := time.Now()

	// Process all files
	for fieldName, files := range form.File {
		// Extract document type from field name (e.g., "doc_1", "doc_11_0", "doc_11_1")
		var documentType string
		if strings.HasPrefix(fieldName, "doc_") {
			parts := strings.Split(fieldName, "_")
			if len(parts) >= 2 {
				documentType = parts[1]
			}
		}

		if documentType == "" {
			continue
		}

		for _, fileHeader := range files {
			// Use original filename with safety checks
			safeOriginalName := utils.SanitizeForFilename(fileHeader.Filename)
			finalFilename := fmt.Sprintf("pub_%s_%s", documentType, safeOriginalName)

			// Generate unique filename in submission folder
			uniqueFilename := utils.GenerateUniqueFilename(submissionFolderPath, finalFilename)
			dst := filepath.Join(submissionFolderPath, uniqueFilename)

			// Save file
			if err := c.SaveUploadedFile(fileHeader, dst); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
				return
			}

			// Create FileUpload record
			fileUpload := models.FileUpload{
				OriginalName: fileHeader.Filename,
				StoredPath:   dst,
				FileSize:     fileHeader.Size,
				MimeType:     fileHeader.Header.Get("Content-Type"),
				FileHash:     "", // ไม่ใช้ hash ในระบบ user-based
				IsPublic:     false,
				UploadedBy:   userID.(int),
				UploadedAt:   now,
				CreateAt:     now,
				UpdateAt:     now,
			}

			if err := config.DB.Create(&fileUpload).Error; err != nil {
				// Delete uploaded file if database save fails
				os.Remove(dst)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file info"})
				return
			}

			// Get file extension
			fileType := strings.TrimPrefix(filepath.Ext(fileHeader.Filename), ".")

			// Create document record - ใช้ fields ที่มีจริง
			doc := models.PublicationDocument{
				RewardID:         reward.RewardID,
				DocumentType:     documentType,
				OriginalFilename: fileHeader.Filename,
				StoredFilename:   uniqueFilename,
				FileType:         fileType,
				UploadedBy:       userID.(int),
				UploadedAt:       &now,
				CreateAt:         &now,
			}

			if err := config.DB.Create(&doc).Error; err != nil {
				// Delete uploaded file and FileUpload record if document creation fails
				os.Remove(dst)
				config.DB.Delete(&fileUpload)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save document record"})
				return
			}

			uploadedFiles = append(uploadedFiles, doc)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Documents uploaded successfully",
		"documents": uploadedFiles,
		"folder":    submissionFolderPath,
	})
}

// GetPublicationDocuments returns documents for a publication reward
func GetPublicationDocuments(c *gin.Context) {
	id := c.Param("id")

	var reward models.PublicationReward
	if err := config.DB.Where("reward_id = ? AND delete_at IS NULL", id).
		First(&reward).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Publication reward not found"})
		return
	}

	var documents []models.PublicationDocument
	if err := config.DB.Where("reward_id = ?", id).Find(&documents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch documents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"documents": documents})
}

// RejectPublicationReward rejects a publication reward (admin only)
func RejectPublicationReward(c *gin.Context) {
	id := c.Param("id")

	var reward models.PublicationReward
	if err := config.DB.Where("reward_id = ? AND delete_at IS NULL", id).
		First(&reward).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Publication reward not found"})
		return
	}

	// Check if already processed
	if reward.Status != "submitted" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reward not in submitted status"})
		return
	}

	// Update status
	now := time.Now()
	reward.Status = "rejected"
	reward.UpdateAt = &now

	if err := config.DB.Save(&reward).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject publication reward"})
		return
	}

	// Add comment if provided
	var requestBody struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&requestBody); err == nil && requestBody.Comment != "" {
		userID, _ := c.Get("userID")
		comment := models.PublicationComment{
			RewardID:      reward.RewardID,
			CommentBy:     userID.(int),
			CommentText:   requestBody.Comment,
			CommentStatus: "rejected",
			CreateAt:      &now,
		}
		config.DB.Create(&comment)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Publication reward rejected",
		"reward":  reward,
	})
}

// UpdatePublicationRewardRates updates reward rates configuration (admin only)
func UpdatePublicationRewardRates(c *gin.Context) {
	var rates []models.PublicationRewardRate
	if err := c.ShouldBindJSON(&rates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update rates in transaction
	tx := config.DB.Begin()

	for _, rate := range rates {
		if err := tx.Model(&models.PublicationRewardRate{}).
			Where("rate_id = ?", rate.RateID).
			Updates(&rate).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update rates"})
			return
		}
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"message": "Reward rates updated successfully",
		"rates":   rates,
	})
}

// GetPublicationRewardRateLookup returns specific reward amount for calculation
func GetPublicationRewardRateLookup(c *gin.Context) {
	year := c.Query("year")
	authorStatus := c.Query("author_status")
	quartile := c.Query("quartile")

	// Validate required parameters
	if year == "" || authorStatus == "" || quartile == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Missing required parameters: year, author_status, quartile",
		})
		return
	}

	var rate models.PublicationRewardRate
	if err := config.DB.Where("year = ? AND author_status = ? AND journal_quartile = ? AND is_active = ?",
		year, authorStatus, quartile, true).First(&rate).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Reward rate not found for the specified parameters",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"reward_amount": rate.RewardAmount,
		"year":          rate.Year,
		"author_status": rate.AuthorStatus,
		"quartile":      rate.JournalQuartile,
	})
}

// GetAllPublicationRewardRates returns all active rates (no year filter)
func GetAllPublicationRewardRates(c *gin.Context) {
	var rates []models.PublicationRewardRate
	if err := config.DB.Where("is_active = ?", true).
		Order("year DESC, author_status, journal_quartile").
		Find(&rates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reward rates"})
		return
	}

	// Group by year for easy frontend consumption
	ratesByYear := make(map[string][]models.PublicationRewardRate)
	for _, rate := range rates {
		ratesByYear[rate.Year] = append(ratesByYear[rate.Year], rate)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"rates":         rates,
		"rates_by_year": ratesByYear,
		"total":         len(rates),
	})
}

// CreatePublicationRewardRate creates new reward rate (admin only)
func CreatePublicationRewardRate(c *gin.Context) {
	var newRate models.PublicationRewardRate
	if err := c.ShouldBindJSON(&newRate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set defaults
	newRate.IsActive = true
	now := time.Now()
	newRate.CreateAt = &now
	newRate.UpdateAt = &now

	if err := config.DB.Create(&newRate).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create reward rate"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Reward rate created successfully",
		"rate":    newRate,
	})
}

// UpdatePublicationRewardRate updates single reward rate (admin only)
func UpdatePublicationRewardRate(c *gin.Context) {
	id := c.Param("id")

	var existingRate models.PublicationRewardRate
	if err := config.DB.Where("rate_id = ?", id).First(&existingRate).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reward rate not found"})
		return
	}

	var updateData models.PublicationRewardRate
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update timestamp
	now := time.Now()
	updateData.UpdateAt = &now

	if err := config.DB.Model(&existingRate).Updates(&updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update reward rate"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Reward rate updated successfully",
		"rate":    existingRate,
	})
}

// DeletePublicationRewardRate deletes reward rate (admin only)
func DeletePublicationRewardRate(c *gin.Context) {
	id := c.Param("id")

	var rate models.PublicationRewardRate
	if err := config.DB.Where("rate_id = ?", id).First(&rate).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reward rate not found"})
		return
	}

	if err := config.DB.Delete(&rate).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete reward rate"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Reward rate deleted successfully",
	})
}

// TogglePublicationRewardRateStatus toggles active status (admin only)
func TogglePublicationRewardRateStatus(c *gin.Context) {
	id := c.Param("id")

	var rate models.PublicationRewardRate
	if err := config.DB.Where("rate_id = ?", id).First(&rate).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reward rate not found"})
		return
	}

	// Toggle active status
	rate.IsActive = !rate.IsActive
	now := time.Now()
	rate.UpdateAt = &now

	if err := config.DB.Save(&rate).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle reward rate status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "Reward rate status updated successfully",
		"is_active": rate.IsActive,
	})
}

// GetAvailableYears returns list of years that have reward rates
func GetAvailableYears(c *gin.Context) {
	var years []string
	if err := config.DB.Model(&models.PublicationRewardRate{}).
		Distinct("year").
		Where("is_active = ?", true).
		Order("year DESC").
		Pluck("year", &years).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch available years"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"years":   years,
		"total":   len(years),
	})
}
