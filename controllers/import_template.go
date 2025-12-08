package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetImportTemplatesAdmin lists active import templates for admin panel
func GetImportTemplatesAdmin(c *gin.Context) {
	var templates []models.ImportTemplate
	query := config.DB.Model(&models.ImportTemplate{}).
		Preload("Creator").
		Preload("Year").
		Where("status = ? AND delete_at IS NULL", "active").
		Order("display_order IS NULL, display_order ASC").
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
