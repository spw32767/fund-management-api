// controllers/user.go
package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetUsers(c *gin.Context) {
	roleID, _ := c.Get("roleID")

	var users []models.User
	query := config.DB.Preload("Role").Preload("Position").
		Select("user_id, user_fname, user_lname, email, role_id, position_id, prefix").
		Where("delete_at IS NULL").
		Where("is_test = ?", 0) 

	if role := c.Query("role"); role != "" {
		query = query.Where("role_id = ?", role)
	}

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

