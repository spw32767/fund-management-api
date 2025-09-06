package controllers

import (
	"net/http"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

// GetActiveYears returns active years (sorted desc by year)
func GetActiveYears(c *gin.Context) {
	type row struct {
		YearID int     `json:"year_id"`
		Year   string  `json:"year"`
		Budget float64 `json:"budget"`
		Status string  `json:"status"`
	}
	var years []row

	err := config.DB.Table("years").
		Select("year_id, year, budget, status").
		Where("status = 'active' AND (delete_at IS NULL OR delete_at = '')").
		Order("year DESC").
		Scan(&years).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to fetch years"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"years":   years,
		"data":    years, // compatibility
	})
}
