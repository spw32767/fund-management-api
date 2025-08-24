// controllers/application_status.go
package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetApplicationStatuses - ดึงสถานะทั้งหมดจาก application_status table
func GetApplicationStatuses(c *gin.Context) {
	var statuses []models.ApplicationStatus

	// ดึงสถานะที่ไม่ถูกลบ
	if err := config.DB.Where("delete_at IS NULL").
		Order("application_status_id ASC").
		Find(&statuses).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch application statuses",
			"debug": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"statuses": statuses,
		"total":    len(statuses),
	})
}
