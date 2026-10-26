package controllers

import (
	"fund-management-api/middleware"
	"github.com/gin-gonic/gin"
)

func requestHasPermission(c *gin.Context, codes ...string) bool {
	return middleware.HasEffectivePermission(c, codes...)
}

// Projects expose both settings metadata and project records through shared
// handlers. Match their area and distinguish reading from changing records.
func canAccessProjectArea(c *gin.Context) bool {
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
		return requestHasPermission(c, "projects.manage")
	}
	return requestHasPermission(c, "ui.page.admin.projects.view", "fund.settings.read")
}
