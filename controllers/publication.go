// controllers/publication.go
package controllers

import (
	"encoding/json"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

	// Parse JSON data
	type CreateRewardRequest struct {
		AuthorStatus             string  `json:"author_status"`
		ArticleTitle             string  `json:"article_title"`
		JournalName              string  `json:"journal_name"`
		JournalIssue             string  `json:"journal_issue"`
		JournalPages             string  `json:"journal_pages"`
		JournalMonth             string  `json:"journal_month"`
		JournalYear              string  `json:"journal_year"`
		JournalURL               string  `json:"journal_url"`
		DOI                      string  `json:"doi"`
		ArticleOnlineDB          string  `json:"article_online_db"`
		JournalTier              string  `json:"journal_tier"`
		JournalQuartile          string  `json:"journal_quartile"`
		InISI                    bool    `json:"in_isi"`
		InScopus                 bool    `json:"in_scopus"`
		ArticleType              string  `json:"article_type"`
		JournalType              string  `json:"journal_type"`
		EditorFee                float64 `json:"editor_fee"`
		PublicationFeeUniversity float64 `json:"publication_fee_university"`
		PublicationFeeCollege    float64 `json:"publication_fee_college"`
		UniversityRanking        string  `json:"university_ranking"`
		BankAccount              string  `json:"bank_account"`
		BankName                 string  `json:"bank_name"`
		PhoneNumber              string  `json:"phone_number"`
		HasUniversityFund        string  `json:"has_university_fund"`
		UniversityFundRef        string  `json:"university_fund_ref"`
		Coauthors                []int   `json:"coauthors"`
		Status                   string  `json:"status"`
	}

	var req CreateRewardRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data format"})
		return
	}

	// Validate required fields
	if req.AuthorStatus == "" || req.ArticleTitle == "" || req.JournalName == "" ||
		req.JournalQuartile == "" || req.BankAccount == "" || req.BankName == "" ||
		req.PhoneNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required fields"})
		return
	}

	// Calculate publication reward based on author status and quartile
	publicationReward := calculatePublicationReward(req.AuthorStatus, req.JournalQuartile)

	// Calculate total amount
	totalAmount := publicationReward + req.PublicationFeeUniversity + req.PublicationFeeCollege

	// Generate reward number
	rewardNumber := generateRewardNumber()

	// Create reward
	now := time.Now()
	reward := models.PublicationReward{
		RewardNumber:             rewardNumber,
		UserID:                   userID.(int),
		AuthorStatus:             req.AuthorStatus,
		ArticleTitle:             req.ArticleTitle,
		JournalName:              req.JournalName,
		JournalIssue:             req.JournalIssue,
		JournalPages:             req.JournalPages,
		JournalMonth:             req.JournalMonth,
		JournalYear:              req.JournalYear,
		JournalURL:               req.JournalURL,
		DOI:                      req.DOI,
		ArticleOnlineDB:          req.ArticleOnlineDB,
		JournalTier:              req.JournalTier,
		JournalQuartile:          req.JournalQuartile,
		InISI:                    req.InISI,
		InScopus:                 req.InScopus,
		ArticleType:              req.ArticleType,
		JournalType:              req.JournalType,
		PublicationReward:        publicationReward,
		EditorFee:                req.EditorFee,
		PublicationFeeUniversity: req.PublicationFeeUniversity,
		PublicationFeeCollege:    req.PublicationFeeCollege,
		TotalAmount:              totalAmount,
		UniversityRanking:        req.UniversityRanking,
		BankAccount:              req.BankAccount,
		BankName:                 req.BankName,
		PhoneNumber:              req.PhoneNumber,
		HasUniversityFund:        req.HasUniversityFund == "yes",
		UniversityFundRef:        req.UniversityFundRef,
		Status:                   req.Status,
		CreateAt:                 &now,
		UpdateAt:                 &now,
	}

	// Set submitted time if not draft
	if req.Status == "submitted" {
		reward.SubmittedAt = &now
	}

	// Start transaction
	tx := config.DB.Begin()

	// Create reward
	if err := tx.Create(&reward).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create publication reward"})
		return
	}

	// Add coauthors
	for i, coauthorID := range req.Coauthors {
		coauthor := models.PublicationCoauthor{
			RewardID:    reward.RewardID,
			UserID:      coauthorID,
			AuthorOrder: i + 1,
			CreateAt:    &now,
		}
		if err := tx.Create(&coauthor).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add coauthor"})
			return
		}
	}

	// Handle file uploads
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	// Process all uploaded files
	if c.Request.MultipartForm != nil && c.Request.MultipartForm.File != nil {
		for fieldName, files := range c.Request.MultipartForm.File {
			// Extract document type from field name (e.g., "doc_1", "doc_11_0")
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
				// Generate unique filename
				ext := filepath.Ext(fileHeader.Filename)
				newFilename := fmt.Sprintf("pub_%d_%s_%s%s",
					reward.RewardID,
					documentType,
					uuid.New().String(),
					ext)

				// Save file
				dst := filepath.Join(uploadPath, "publications", newFilename)
				if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
					return
				}

				src, err := fileHeader.Open()
				if err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open uploaded file"})
					return
				}
				defer src.Close()

				out, err := os.Create(dst)
				if err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create file"})
					return
				}
				defer out.Close()

				_, err = io.Copy(out, src)
				if err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
					return
				}

				// Get file type
				fileType := strings.TrimPrefix(ext, ".")

				// Create document record
				doc := models.PublicationDocument{
					RewardID:         reward.RewardID,
					DocumentType:     documentType,
					OriginalFilename: fileHeader.Filename,
					StoredFilename:   newFilename,
					FileType:         fileType,
					UploadedBy:       userID.(int),
					UploadedAt:       &now,
					CreateAt:         &now,
				}

				if err := tx.Create(&doc).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save document record"})
					return
				}
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

	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
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
				// Use document type ID as string for now
				documentType = parts[1]
			}
		}

		if documentType == "" {
			continue
		}

		for _, fileHeader := range files {
			// Generate unique filename
			ext := filepath.Ext(fileHeader.Filename)
			newFilename := fmt.Sprintf("pub_%d_%s_%s%s",
				reward.RewardID,
				documentType,
				uuid.New().String(),
				ext)

			// Save file
			dst := filepath.Join(uploadPath, "publications", newFilename)
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
				return
			}

			if err := c.SaveUploadedFile(fileHeader, dst); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
				return
			}

			// Get file extension
			fileType := strings.TrimPrefix(ext, ".")

			// Create document record using correct field names
			doc := models.PublicationDocument{
				RewardID:         reward.RewardID,
				DocumentType:     documentType,
				OriginalFilename: fileHeader.Filename,
				StoredFilename:   newFilename,
				FileType:         fileType,
				UploadedBy:       userID.(int),
				UploadedAt:       &now,
				CreateAt:         &now,
			}

			if err := config.DB.Create(&doc).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save document record"})
				return
			}

			uploadedFiles = append(uploadedFiles, doc)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Documents uploaded successfully",
		"documents": uploadedFiles,
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
