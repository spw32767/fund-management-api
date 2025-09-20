// controllers/reward_config.go
package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// GetRewardConfig returns reward configuration data
func GetRewardConfig(c *gin.Context) {
	var configs []models.RewardConfig

	// Build query
	query := config.DB.Where("is_active = ? AND delete_at IS NULL", true)

	// Filter by year
	if year := c.Query("year"); year != "" {
		query = query.Where("year = ?", year)
	} else {
		// Default to current Buddhist year
		currentYear := strconv.Itoa(time.Now().Year() + 543)
		query = query.Where("year = ?", currentYear)
	}

	// Filter by quartile
	if quartile := c.Query("quartile"); quartile != "" {
		query = query.Where("journal_quartile = ?", quartile)
	}

	// Execute query with ordering
	if err := query.Order("journal_quartile").Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reward config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    configs,
		"total":   len(configs),
	})
}

// GetRewardConfigAdmin returns all reward configuration data for admin (no is_active filter)
func GetRewardConfigAdmin(c *gin.Context) {
	var configs []models.RewardConfig

	// Build query - ไม่ filter is_active เพื่อให้ admin เห็นทั้ง active และ inactive
	query := config.DB.Where("delete_at IS NULL")

	// Filter by year
	if year := c.Query("year"); year != "" {
		query = query.Where("year = ?", year)
	}
	// ไม่ default ไปปีปัจจุบัน เพื่อให้ admin เห็นทุกปี

	// Filter by quartile if needed
	if quartile := c.Query("quartile"); quartile != "" {
		query = query.Where("journal_quartile = ?", quartile)
	}

	// Execute query with ordering
	if err := query.Order("year DESC, journal_quartile").Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reward config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    configs,
		"total":   len(configs),
	})
}

// GetRewardConfigLookup returns specific max amount for calculation
func GetRewardConfigLookup(c *gin.Context) {
	year := strings.TrimSpace(c.Query("year"))
	quartile := strings.TrimSpace(c.Query("quartile"))

	// Validate required parameters
	if year == "" || quartile == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Missing required parameters: year, quartile",
		})
		return
	}

	// 1) พยายามหา "แถวที่ active"
	var active models.RewardConfig
	err := config.DB.
		Where("year = ? AND journal_quartile = ? AND is_active = ? AND delete_at IS NULL",
			year, quartile, true).
		First(&active).Error

	if err == nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"found":      true,
			"is_active":  true,
			"max_amount": active.MaxAmount,
			"condition":  active.ConditionDescription,
		})
		return
	}

	// 2) ไม่เจอ active → เช็คว่ามีแถว (แต่ inactive) หรือไม่มีเลย
	var any models.RewardConfig
	err2 := config.DB.
		Where("year = ? AND journal_quartile = ? AND delete_at IS NULL", year, quartile).
		First(&any).Error

	// คืน 200 เสมอ เพื่อ "กลืน" เคส inactive/ไม่มีค่า ให้ FE ปิดช่องเองจากค่า null
	if err2 == nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"found":      true,
			"is_active":  any.IsActive, // คาดว่า false
			"max_amount": nil,          // ไม่มีเพดานใช้งานได้
			"condition":  any.ConditionDescription,
		})
		return
	}

	// 3) ไม่มี record เลย → คืน 200 พร้อม found=false
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"found":      false,
		"is_active":  false,
		"max_amount": nil,
		"condition":  nil,
	})
}

// CreateRewardConfig creates new reward configuration (admin only)
func CreateRewardConfig(c *gin.Context) {
	var newConfig models.RewardConfig
	if err := c.ShouldBindJSON(&newConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set defaults
	newConfig.IsActive = true
	now := time.Now()
	newConfig.CreateAt = &now
	newConfig.UpdateAt = &now

	if err := config.DB.Create(&newConfig).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create reward config"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Reward config created successfully",
		"data":    newConfig,
	})
}

// UpdateRewardConfig updates existing reward configuration (admin only)
func UpdateRewardConfig(c *gin.Context) {
	id := c.Param("id")

	var existingConfig models.RewardConfig
	if err := config.DB.Where("config_id = ? AND delete_at IS NULL", id).First(&existingConfig).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reward config not found"})
		return
	}

	var updateData models.RewardConfig
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update timestamp
	now := time.Now()
	updateData.UpdateAt = &now

	if err := config.DB.Model(&existingConfig).Updates(&updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update reward config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Reward config updated successfully",
		"data":    existingConfig,
	})
}

// DeleteRewardConfig soft deletes reward configuration (admin only)
func DeleteRewardConfig(c *gin.Context) {
	id := c.Param("id")

	var rewardConfig models.RewardConfig
	if err := config.DB.Where("config_id = ? AND delete_at IS NULL", id).First(&rewardConfig).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reward config not found"})
		return
	}

	// Soft delete
	now := time.Now()
	rewardConfig.DeleteAt = &now

	if err := config.DB.Save(&rewardConfig).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete reward config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Reward config deleted successfully",
	})
}

// ToggleRewardConfigStatus toggles active status (admin only)
func ToggleRewardConfigStatus(c *gin.Context) {
	id := c.Param("id")

	var rewardConfig models.RewardConfig
	if err := config.DB.Where("config_id = ? AND delete_at IS NULL", id).First(&rewardConfig).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reward config not found"})
		return
	}

	// Toggle active status
	rewardConfig.IsActive = !rewardConfig.IsActive
	now := time.Now()
	rewardConfig.UpdateAt = &now

	if err := config.DB.Save(&rewardConfig).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle reward config status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "Reward config status updated successfully",
		"is_active": rewardConfig.IsActive,
	})
}
