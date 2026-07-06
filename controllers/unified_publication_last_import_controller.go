package controllers

import (
	"net/http"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

func GetLastImportDates(c *gin.Context) {
	var rows []map[string]interface{}
	err := config.DB.Raw(`
		(SELECT 'Scopus' AS source, finished_at FROM scopus_batch_import_runs WHERE status = 'success' ORDER BY finished_at DESC LIMIT 1)
		UNION ALL
		(SELECT 'TCI-ThaiJO' AS source, finished_at FROM thaijo_batch_import_runs WHERE status = 'success' ORDER BY finished_at DESC LIMIT 1)
		UNION ALL
		(SELECT 'AI Showcase' AS source, MAX(updated_at) AS finished_at FROM ai_showcase_projects)
	`).Scan(&rows).Error

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}

	if rows == nil {
		rows = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{"data": rows})
}
