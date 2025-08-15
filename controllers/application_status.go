// controllers/application_status.go
package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetApplicationStatuses returns all application statuses
func GetApplicationStatuses(c *gin.Context) {
	var statuses []models.ApplicationStatus

	if err := config.DB.Find(&statuses).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch application statuses",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"statuses": statuses,
		"total":    len(statuses),
	})
}
