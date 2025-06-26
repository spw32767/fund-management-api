package controllers

import (
	"fund-management-api/config"
	"fund-management-api/middleware"
	"fund-management-api/models"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token   string      `json:"token"`
	User    models.User `json:"user"`
	Message string      `json:"message"`
}

// Login handles user authentication
func Login(c *gin.Context) {
	var req LoginRequest

	// Bind request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user by email
	var user models.User
	if err := config.DB.Preload("Role").Preload("Position").
		Where("email = ? AND delete_at IS NULL", req.Email).
		First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Check password (ในระบบจริงควรใช้ bcrypt)
	// TODO: ในระบบจริงต้องใช้ bcrypt.CompareHashAndPassword
	if user.Password != req.Password {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Generate JWT token
	token, err := generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// Response
	c.JSON(http.StatusOK, LoginResponse{
		Token:   token,
		User:    user,
		Message: "Login successful",
	})
}

// GetProfile returns current user profile
func GetProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	var user models.User
	if err := config.DB.Preload("Role").Preload("Position").
		Where("user_id = ?", userID).
		First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// ChangePassword handles password change
func ChangePassword(c *gin.Context) {
	type PasswordChangeRequest struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=6"`
	}

	var req PasswordChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("userID")

	// Get current user
	var user models.User
	if err := config.DB.Where("user_id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Verify current password
	// TODO: ในระบบจริงต้องใช้ bcrypt
	if user.Password != req.CurrentPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	// Update password
	// TODO: ในระบบจริงต้อง hash password ด้วย bcrypt
	now := time.Now()
	user.Password = req.NewPassword
	user.UpdateAt = &now

	if err := config.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}

// generateToken creates JWT token
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
		},
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token
	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// HashPassword hashes password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash compares password with hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
