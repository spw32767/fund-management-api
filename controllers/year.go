package controllers

import (
	"net/http"

	"fund-management-api/models"

	"github.com/gin-gonic/gin"
)

// GetCurrentYear returns the current fiscal year from system configuration
func GetCurrentYear(c *gin.Context) {
	year, err := models.GetCurrentYear()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch current year"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"year": year})
}
