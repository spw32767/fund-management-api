package controllers

import (
	"crypto/rand"
	"encoding/base64"
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    int         `json:"expires_in"`
	User         models.User `json:"user"`
	Message      string      `json:"message"`
}

// Login handles user authentication with session management
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

	// Check password (TODO: ใช้ bcrypt ในระบบจริง)
	if user.Password != req.Password {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Generate tokens
	accessToken, accessExp, jti, err := generateAccessToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	refreshToken, refreshExp, err := generateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	// Save session
	session := models.UserSession{
		UserID:         user.UserID,
		AccessTokenJTI: jti,
		RefreshToken:   refreshToken,
		DeviceName:     c.Request.Header.Get("X-Device-Name"),
		DeviceType:     detectDeviceType(c.Request.UserAgent()),
		IPAddress:      c.ClientIP(),
		UserAgent:      c.Request.UserAgent(),
		ExpiresAt:      refreshExp,
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Deactivate old sessions if needed (optional: limit to 5 active sessions)
	var activeCount int64
	config.DB.Model(&models.UserSession{}).
		Where("user_id = ? AND is_active = true", user.UserID).
		Count(&activeCount)

	if activeCount >= 5 {
		// Deactivate oldest session
		var oldestSession models.UserSession
		config.DB.Where("user_id = ? AND is_active = true", user.UserID).
			Order("created_at ASC").
			First(&oldestSession)
		oldestSession.IsActive = false
		config.DB.Save(&oldestSession)
	}

	// Create new session
	if err := config.DB.Create(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	// Response
	c.JSON(http.StatusOK, LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessExp.Sub(time.Now()).Seconds()),
		User:         user,
		Message:      "Login successful",
	})
}

// RefreshToken generates new access token using refresh token
func RefreshToken(c *gin.Context) {
	type RefreshRequest struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find session by refresh token
	var session models.UserSession
	if err := config.DB.Preload("User.Role").Preload("User.Position").
		Where("refresh_token = ? AND is_active = true AND expires_at > ?", req.RefreshToken, time.Now()).
		First(&session).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired refresh token"})
		return
	}

	// Generate new access token
	accessToken, accessExp, jti, err := generateAccessToken(session.User)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	// Update session
	now := time.Now()
	session.AccessTokenJTI = jti
	session.LastActivity = &now
	session.UpdatedAt = now

	if err := config.DB.Save(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   int(accessExp.Sub(time.Now()).Seconds()),
	})
}

// Logout invalidates user session
func Logout(c *gin.Context) {
	// Get token from header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No token provided"})
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Parse token to get JTI
	token, _ := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if jti, ok := claims["jti"].(string); ok {
			// Deactivate session
			result := config.DB.Model(&models.UserSession{}).
				Where("access_token_jti = ?", jti).
				Update("is_active", false)

			if result.Error != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to logout"})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
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

	// Logout all sessions after password change
	config.DB.Model(&models.UserSession{}).
		Where("user_id = ?", userID).
		Update("is_active", false)

	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully. Please login again."})
}

// GetActiveSessions returns user's active sessions
func GetActiveSessions(c *gin.Context) {
	userID, _ := c.Get("userID")

	var sessions []models.UserSession
	if err := config.DB.
		Where("user_id = ? AND is_active = true AND expires_at > ?", userID, time.Now()).
		Order("last_activity DESC").
		Find(&sessions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sessions"})
		return
	}

	// Hide sensitive data
	for i := range sessions {
		sessions[i].RefreshToken = ""
		sessions[i].AccessTokenJTI = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
		"total":    len(sessions),
	})
}

// RevokeSession revokes a specific session
func RevokeSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	userID, _ := c.Get("userID")

	result := config.DB.Model(&models.UserSession{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Update("is_active", false)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke session"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Session revoked successfully"})
}

// LogoutAllDevices logs out from all devices
func LogoutAllDevices(c *gin.Context) {
	userID, _ := c.Get("userID")

	result := config.DB.Model(&models.UserSession{}).
		Where("user_id = ?", userID).
		Update("is_active", false)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to logout from all devices"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":           "Logged out from all devices successfully",
		"affected_sessions": result.RowsAffected,
	})
}

// Helper functions
func generateAccessToken(user models.User) (string, time.Time, string, error) {
	expireHours, err := strconv.Atoi(os.Getenv("JWT_EXPIRE_HOURS"))
	if err != nil {
		expireHours = 24
	}

	expiresAt := time.Now().Add(time.Duration(expireHours) * time.Hour)
	jti := uuid.New().String()

	claims := jwt.MapClaims{
		"user_id": user.UserID,
		"email":   user.Email,
		"role_id": user.RoleID,
		"jti":     jti,
		"exp":     expiresAt.Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

	return tokenString, expiresAt, jti, err
}

func generateRefreshToken() (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}

	token := base64.URLEncoding.EncodeToString(b)
	expiresAt := time.Now().Add(30 * 24 * time.Hour) // 30 days

	return token, expiresAt, nil
}

func detectDeviceType(userAgent string) string {
	ua := strings.ToLower(userAgent)
	if strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone") {
		return "mobile"
	} else if strings.Contains(ua, "tablet") || strings.Contains(ua, "ipad") {
		return "tablet"
	}
	return "web"
}

// Utility functions for bcrypt (ยังไม่ได้ใช้)
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
