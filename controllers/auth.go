package controllers

import (
	"fund-management-api/config"
	"fund-management-api/middleware"
	"fund-management-api/models"
	"fund-management-api/utils"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token   string      `json:"token"`
	User    UserProfile `json:"user"`
	Message string      `json:"message"`
}

type UserProfile struct {
	UserID       int    `json:"user_id"`
	UserFname    string `json:"user_fname"`
	UserLname    string `json:"user_lname"`
	Email        string `json:"email"`
	RoleID       int    `json:"role_id"`
	PositionID   int    `json:"position_id"`
	Role         string `json:"role"`
	PositionName string `json:"position_name"`
}

// Login handles user authentication with bcrypt password verification
func Login(c *gin.Context) {
	var req LoginRequest

	// Bind request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	// Sanitize input
	req.Email = utils.SanitizeInput(req.Email)
	req.Password = utils.SanitizeInput(req.Password)

	// Validate email format
	if !utils.ValidateEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid email format",
		})
		return
	}

	// Find user by email
	var user models.User
	if err := config.DB.Preload("Role").Preload("Position").
		Where("email = ? AND delete_at IS NULL", req.Email).
		First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid email or password",
		})
		return
	}

	// Check password using bcrypt
	if !utils.CheckPasswordHash(req.Password, user.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid email or password",
		})
		return
	}

	// Generate JWT token
	token, err := generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate authentication token",
		})
		return
	}

	// Create user profile response
	userProfile := UserProfile{
		UserID:       user.UserID,
		UserFname:    user.UserFname,
		UserLname:    user.UserLname,
		Email:        user.Email,
		RoleID:       user.RoleID,
		PositionID:   user.PositionID,
		Role:         user.Role.Role,
		PositionName: user.Position.PositionName,
	}

	// Response
	c.JSON(http.StatusOK, LoginResponse{
		Token:   token,
		User:    userProfile,
		Message: "Login successful",
	})
}

// GetProfile returns current user profile
func GetProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	var user models.User
	if err := config.DB.Preload("Role").Preload("Position").
		Where("user_id = ? AND delete_at IS NULL", userID).
		First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "User not found",
		})
		return
	}

	userProfile := UserProfile{
		UserID:       user.UserID,
		UserFname:    user.UserFname,
		UserLname:    user.UserLname,
		Email:        user.Email,
		RoleID:       user.RoleID,
		PositionID:   user.PositionID,
		Role:         user.Role.Role,
		PositionName: user.Position.PositionName,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"user":    userProfile,
	})
}

// ChangePassword handles password change with proper validation
func ChangePassword(c *gin.Context) {
	type PasswordChangeRequest struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=8"`
		ConfirmPassword string `json:"confirm_password" binding:"required"`
	}

	var req PasswordChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	// Validate new password
	if valid, message := utils.ValidatePassword(req.NewPassword); !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   message,
		})
		return
	}

	// Check password confirmation
	if req.NewPassword != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "New password and confirmation do not match",
		})
		return
	}

	userID, _ := c.Get("userID")

	// Get current user
	var user models.User
	if err := config.DB.Where("user_id = ? AND delete_at IS NULL", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "User not found",
		})
		return
	}

	// Verify current password
	if !utils.CheckPasswordHash(req.CurrentPassword, user.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Current password is incorrect",
		})
		return
	}

	// Hash new password
	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to process new password",
		})
		return
	}

	// Update password
	now := time.Now()
	user.Password = hashedPassword
	user.UpdateAt = &now

	if err := config.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update password",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Password changed successfully",
	})
}

// generateToken creates JWT token with proper claims
func generateToken(user models.User) (string, error) {
	// Get expiration hours from env
	expireHours, err := strconv.Atoi(os.Getenv("JWT_EXPIRE_HOURS"))
	if err != nil {
		expireHours = 24 // default 24 hours
	}

	// Create claims
	claims := middleware.Claims{
		UserID: user.UserID,
		Email:  user.Email,
		RoleID: user.RoleID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "fund-management-api",
			Subject:   user.Email,
		},
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default-secret-change-this-in-production"
	}

	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// RefreshToken handles token refresh (optional endpoint)
func RefreshToken(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid token",
		})
		return
	}

	// Get user
	var user models.User
	if err := config.DB.Preload("Role").Preload("Position").
		Where("user_id = ? AND delete_at IS NULL", userID).
		First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "User not found",
		})
		return
	}

	// Generate new token
	token, err := generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to refresh token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"token":   token,
		"message": "Token refreshed successfully",
	})
}
