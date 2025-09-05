package controllers

import (
	"fund-management-api/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetCurrentYear returns the current year from system configuration
func GetCurrentYear(c *gin.Context) {
	var result struct {
		CurrentYear int `json:"current_year"`
	}

	if err := config.DB.Table("system_config").
		Select("current_year").
		Order("config_id DESC").
		Limit(1).
		Scan(&result).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch current year"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"current_year": result.CurrentYear})
}
