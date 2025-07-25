// controllers/user.go
package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetUsers returns list of users for dropdown selection
func GetUsers(c *gin.Context) {
	roleID, _ := c.Get("roleID")

	var users []models.User
	// เพิ่ม Preload("Role") และ Preload("Position") เพื่อดึงข้อมูล relationship
	query := config.DB.Preload("Role").Preload("Position").
		Select("user_id, user_fname, user_lname, email, role_id, position_id").
		Where("delete_at IS NULL")

	// Filter by role if specified
	if role := c.Query("role"); role != "" {
		query = query.Where("role_id = ?", role)
	}

	// Non-admins can only see teachers
	if roleID.(int) != 3 {
		query = query.Where("role_id = ?", 1) // Only teachers
	}

	if err := query.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"total": len(users),
	})
}
