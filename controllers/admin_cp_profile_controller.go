package controllers

import (
	"net/http"
	"strconv"

	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// POST /admin/trigger/cp-profile?debug=1
func AdminTriggerCpProfile(c *gin.Context) {
	debug := false
	if q := c.Query("debug"); q != "" {
		if b, err := strconv.ParseBool(q); err == nil {
			debug = b
		}
	}

	svc := services.NewCpProfileService(nil)
	sum, err := svc.Import(c.Request.Context(), debug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"summary": sum,
	})
}
