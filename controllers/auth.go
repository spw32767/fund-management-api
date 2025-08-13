package controllers

import (
	"crypto/rand"
	"encoding/base64"
	"fund-management-api/config"
	"fund-management-api/middleware"
	"fund-management-api/models"
	"fund-management-api/utils"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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
	User         UserProfile `json:"user"`
	SessionID    int         `json:"session_id"`
	Message      string      `json:"message"`

	// เก็บ field เดิมไว้เพื่อ backward compatibility
	Token string `json:"token"` // จะเป็นค่าเดียวกับ access_token
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

// DeviceInfo structure for client information
type DeviceInfo struct {
	DeviceName string
	DeviceType string
	IPAddress  string
	UserAgent  string
}

// Login handles user authentication with session management
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
	// เพิ่ม debug log
	log.Printf("User loaded: %+v", user)
	log.Printf("Role loaded: %+v", user.Role)
	log.Printf("Position loaded: %+v", user.Position)

	// Manual load Role
	if user.RoleID > 0 {
		config.DB.Where("role_id = ?", user.RoleID).First(&user.Role)
	}

	// Manual load Position
	if user.PositionID > 0 {
		config.DB.Where("position_id = ?", user.PositionID).First(&user.Position)
	}

	// Check password using bcrypt
	if !utils.CheckPasswordHash(req.Password, user.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid email or password",
		})
		return
	}

	// Generate JTI (JWT ID) for tracking this specific token
	jti := uuid.New().String()

	// Generate access token with JTI
	accessToken, expiresIn, err := generateAccessToken(user, jti)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate authentication token",
		})
		return
	}

	// Generate refresh token
	refreshToken, err := generateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate refresh token",
		})
		return
	}

	// Get device and client information
	deviceInfo := extractDeviceInfo(c)

	// Create session record
	session := models.UserSession{
		UserID:         user.UserID,
		AccessTokenJTI: jti,
		RefreshToken:   refreshToken,
		DeviceName:     deviceInfo.DeviceName,
		DeviceType:     deviceInfo.DeviceType,
		IPAddress:      deviceInfo.IPAddress,
		UserAgent:      deviceInfo.UserAgent,
		LastActivity:   &time.Time{},
		ExpiresAt:      time.Now().Add(time.Duration(getRefreshTokenExpireHours()) * time.Hour),
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Update last activity to current time
	now := time.Now()
	session.LastActivity = &now

	// Save session to database
	if err := config.DB.Create(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create session",
		})
		return
	}

	// Store refresh token in user_tokens table
	userToken := models.UserToken{
		UserID:     user.UserID,
		TokenType:  "refresh",
		Token:      refreshToken,
		ExpiresAt:  session.ExpiresAt,
		IsRevoked:  false,
		DeviceInfo: deviceInfo.DeviceName + " / " + deviceInfo.DeviceType,
		IPAddress:  deviceInfo.IPAddress,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := config.DB.Create(&userToken).Error; err != nil {
		// Rollback session if token creation fails
		config.DB.Delete(&session)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to store refresh token",
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

	// Response with backward compatibility
	c.JSON(http.StatusOK, LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		User:         userProfile,
		SessionID:    session.SessionID,
		Message:      "Login successful",
		Token:        accessToken, // backward compatibility
	})
}

// GetProfile returns current user profile
func GetProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	var user models.User
	if err := config.DB.Where("user_id = ? AND delete_at IS NULL", userID).
		First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "User not found",
		})
		return
	}

	// Manual load Role
	if user.RoleID > 0 {
		config.DB.Where("role_id = ?", user.RoleID).First(&user.Role)
	}

	// Manual load Position
	if user.PositionID > 0 {
		config.DB.Where("position_id = ?", user.PositionID).First(&user.Position)
	}

	// Debug log
	log.Printf("Profile - User loaded: %+v", user)
	log.Printf("Profile - Role loaded: %+v", user.Role)
	log.Printf("Profile - Position loaded: %+v", user.Position)

	// Update session activity
	updateSessionActivity(c)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"user": gin.H{
			"user_id":       user.UserID,
			"user_fname":    user.UserFname,
			"user_lname":    user.UserLname,
			"email":         user.Email,
			"role_id":       user.RoleID,
			"position_id":   user.PositionID,
			"role":          user.Role.Role,
			"position_name": user.Position.PositionName,
		},
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

// RefreshToken handles token refresh (existing endpoint - updated)
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

	// Generate new JTI and access token
	newJTI := uuid.New().String()
	newAccessToken, expiresIn, err := generateAccessToken(user, newJTI)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to refresh token",
		})
		return
	}

	// Update session with new JTI if session exists
	sessionID, hasSession := c.Get("sessionID")
	if hasSession {
		now := time.Now()
		config.DB.Model(&models.UserSession{}).
			Where("session_id = ?", sessionID).
			Updates(map[string]interface{}{
				"access_token_jti": newJTI,
				"last_activity":    &now,
				"updated_at":       now,
			})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"token":        newAccessToken, // backward compatibility
		"access_token": newAccessToken,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
		"message":      "Token refreshed successfully",
	})
}

// ======= NEW SESSION MANAGEMENT ENDPOINTS =======

// Logout handles user logout and session cleanup
func Logout(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid session",
		})
		return
	}

	sessionID, exists := c.Get("sessionID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Session information not found",
		})
		return
	}

	// Begin transaction
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Mark session as inactive
	if err := tx.Model(&models.UserSession{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Update("is_active", false).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to invalidate session",
		})
		return
	}

	// Revoke refresh token associated with this session
	var session models.UserSession
	if err := tx.Where("session_id = ?", sessionID).First(&session).Error; err == nil {
		// Mark refresh token as revoked
		tx.Model(&models.UserToken{}).
			Where("user_id = ? AND token = ? AND token_type = ?", userID, session.RefreshToken, "refresh").
			Update("is_revoked", true)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to complete logout",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfully",
	})
}

// GetActiveSessions returns user's active sessions
func GetActiveSessions(c *gin.Context) {
	userID, _ := c.Get("userID")

	var sessions []models.UserSession
	if err := config.DB.Where("user_id = ? AND is_active = ?", userID, true).
		Order("last_activity DESC").
		Find(&sessions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to fetch sessions",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"sessions": sessions,
	})
}

// RevokeOtherSessions revokes all sessions except the current one
func RevokeOtherSessions(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid session",
		})
		return
	}

	currentSessionID, exists := c.Get("sessionID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Current session information not found",
		})
		return
	}

	// Begin transaction
	tx := config.DB.Begin()

	// Get refresh tokens from other sessions before revoking
	var otherSessions []models.UserSession
	if err := tx.Where("user_id = ? AND session_id != ? AND is_active = ?",
		userID, currentSessionID, true).Find(&otherSessions).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to fetch other sessions",
		})
		return
	}

	// Mark other sessions as inactive
	if err := tx.Model(&models.UserSession{}).
		Where("user_id = ? AND session_id != ? AND is_active = ?", userID, currentSessionID, true).
		Update("is_active", false).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to invalidate other sessions",
		})
		return
	}

	// Revoke refresh tokens from other sessions
	for _, session := range otherSessions {
		if session.RefreshToken != "" {
			tx.Model(&models.UserToken{}).
				Where("user_id = ? AND token = ? AND token_type = ?", userID, session.RefreshToken, "refresh").
				Update("is_revoked", true)
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to complete session revocation",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "Other sessions revoked successfully",
		"revoked_count": len(otherSessions),
	})
}

// RefreshTokenWithRefreshToken handles token refresh using refresh token
func RefreshTokenWithRefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Refresh token is required",
		})
		return
	}

	// Find refresh token in database
	var userToken models.UserToken
	if err := config.DB.Where("token = ? AND token_type = ? AND is_revoked = ?",
		req.RefreshToken, "refresh", false).First(&userToken).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid or expired refresh token",
		})
		return
	}

	// Check if token has expired
	if time.Now().After(userToken.ExpiresAt) {
		// Mark token as revoked
		config.DB.Model(&userToken).Update("is_revoked", true)

		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Refresh token has expired",
		})
		return
	}

	// Get user information
	var user models.User
	if err := config.DB.Preload("Role").Preload("Position").
		Where("user_id = ? AND delete_at IS NULL", userToken.UserID).
		First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "User not found",
		})
		return
	}

	// Generate new JTI and access token
	newJTI := uuid.New().String()
	newAccessToken, expiresIn, err := generateAccessToken(user, newJTI)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate new access token",
		})
		return
	}

	// Update session with new JTI
	var session models.UserSession
	if err := config.DB.Where("user_id = ? AND refresh_token = ? AND is_active = ?",
		userToken.UserID, req.RefreshToken, true).First(&session).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Session not found",
		})
		return
	}

	// Update session with new access token JTI
	now := time.Now()
	session.AccessTokenJTI = newJTI
	session.LastActivity = &now
	session.UpdatedAt = now

	if err := config.DB.Save(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update session",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"access_token": newAccessToken,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
		"message":      "Token refreshed successfully",
	})
}

// ======= HELPER FUNCTIONS =======

// extractDeviceInfo extracts device and client information from request
func extractDeviceInfo(c *gin.Context) DeviceInfo {
	userAgent := c.GetHeader("User-Agent")
	clientIP := c.ClientIP()

	// Determine device type from user agent
	deviceType := "unknown"
	deviceName := ""

	if userAgent != "" {
		switch {
		case containsAny(userAgent, []string{"Mobile", "Android", "iPhone"}):
			deviceType = "mobile"
		case containsAny(userAgent, []string{"Tablet", "iPad"}):
			deviceType = "tablet"
		case containsAny(userAgent, []string{"Mozilla", "Chrome", "Safari", "Firefox", "Edge"}):
			deviceType = "web"
		case containsAny(userAgent, []string{"Postman", "Thunder", "Insomnia"}):
			deviceType = "api_client"
		}

		// Extract device name from user agent (simplified)
		if deviceType == "mobile" {
			if containsAny(userAgent, []string{"iPhone"}) {
				deviceName = "iPhone"
			} else if containsAny(userAgent, []string{"Android"}) {
				deviceName = "Android"
			}
		} else if deviceType == "web" {
			if containsAny(userAgent, []string{"Chrome"}) {
				deviceName = "Chrome Browser"
			} else if containsAny(userAgent, []string{"Firefox"}) {
				deviceName = "Firefox Browser"
			} else if containsAny(userAgent, []string{"Safari"}) {
				deviceName = "Safari Browser"
			} else if containsAny(userAgent, []string{"Edge"}) {
				deviceName = "Edge Browser"
			}
		}
	}

	return DeviceInfo{
		DeviceName: deviceName,
		DeviceType: deviceType,
		IPAddress:  clientIP,
		UserAgent:  userAgent,
	}
}

// containsAny checks if string contains any of the substrings
func containsAny(s string, substrings []string) bool {
	for _, substring := range substrings {
		if strings.Contains(s, substring) {
			return true
		}
	}
	return false
}

// generateAccessToken creates JWT access token with JTI
func generateAccessToken(user models.User, jti string) (string, int, error) {
	// Get expiration hours from env
	expireHours, err := strconv.Atoi(os.Getenv("JWT_EXPIRE_HOURS"))
	if err != nil {
		expireHours = 1 // default 1 hour for access token (shorter than before)
	}

	expirationTime := time.Now().Add(time.Duration(expireHours) * time.Hour)

	// Create claims with JTI
	claims := middleware.Claims{
		UserID: user.UserID,
		Email:  user.Email,
		RoleID: user.RoleID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti, // JWT ID for tracking
			ExpiresAt: jwt.NewNumericDate(expirationTime),
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
		return "", 0, err
	}

	return tokenString, int(expirationTime.Sub(time.Now()).Seconds()), nil
}

// generateToken creates JWT token with proper claims (existing function for backward compatibility)
func generateToken(user models.User) (string, error) {
	// Use new function with empty JTI for backward compatibility
	token, _, err := generateAccessToken(user, "")
	return token, err
}

// generateRefreshToken creates a secure random refresh token
func generateRefreshToken() (string, error) {
	bytes := make([]byte, 32) // 32 bytes = 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// getRefreshTokenExpireHours returns refresh token expiration hours
func getRefreshTokenExpireHours() int {
	expireHours, err := strconv.Atoi(os.Getenv("REFRESH_TOKEN_EXPIRE_HOURS"))
	if err != nil {
		return 720 // default 30 days
	}
	return expireHours
}

// updateSessionActivity updates last activity for a session
func updateSessionActivity(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		return // ไม่จำเป็นต้อง error ถ้าไม่มี userID
	}

	// Get JTI from token
	tokenString := c.GetHeader("Authorization")
	if tokenString == "" {
		return
	}

	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	// Parse token to get JTI
	token, err := jwt.ParseWithClaims(tokenString, &middleware.Claims{}, func(token *jwt.Token) (interface{}, error) {
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = "default-secret-change-this-in-production"
		}
		return []byte(jwtSecret), nil
	})

	if err != nil {
		return
	}

	claims, ok := token.Claims.(*middleware.Claims)
	if !ok {
		return
	}

	// Update session last activity
	now := time.Now()
	config.DB.Model(&models.UserSession{}).
		Where("user_id = ? AND access_token_jti = ? AND is_active = ?", userID, claims.ID, true).
		Update("last_activity", now)
}
