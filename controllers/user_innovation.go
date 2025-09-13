package controllers

import (
	"net/http"
	"strconv"

	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/teacher/user-innovations?limit=50&offset=0
func GetUserInnovations(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		q := c.Query("user_id")
		if q == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "user_id not found"})
			return
		}
		id64, err := strconv.ParseUint(q, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
			return
		}
		userID = uint(id64)
	}

	limit := parseIntOrDefault(c.Query("limit"), 50)
	offset := parseIntOrDefault(c.Query("offset"), 0)

	svc := services.NewInnovationService(nil)
	items, total, err := svc.ListByUser(userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"paging": gin.H{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}
