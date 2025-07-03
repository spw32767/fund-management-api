package controllers

import (
	"fmt"
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
			"error":   "รูปแบบข้อมูลไม่ถูกต้อง",
			"details": err.Error(),
		})
		return
	}

	// Sanitize input (ถ้ามี utils.SanitizeInput)
	if utils.SanitizeInput != nil {
		req.Email = utils.SanitizeInput(req.Email)
		req.Password = utils.SanitizeInput(req.Password)
	}

	// Validate email format (ถ้ามี utils.ValidateEmail)
	if utils.ValidateEmail != nil && !utils.ValidateEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "รูปแบบอีเมลไม่ถูกต้อง",
		})
		return
	}

	// Find user by email
	var user models.User
	if err := config.DB.Preload("Role").Preload("Position").
		Where("email = ? AND delete_at IS NULL", req.Email).
		First(&user).Error; err != nil {
		fmt.Printf("User not found for email: %s\n", req.Email)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "อีเมลหรือรหัสผ่านไม่ถูกต้อง",
		})
		return
	}

	// Check password using bcrypt (ถ้ามี utils.CheckPasswordHash)
	var passwordValid bool
	if utils.CheckPasswordHash != nil {
		passwordValid = utils.CheckPasswordHash(req.Password, user.Password)
	} else {
		// Fallback สำหรับกรณีที่ยังไม่มี bcrypt
		passwordValid = (req.Password == user.Password)
		fmt.Println("Warning: Using plain text password comparison. Please implement bcrypt.")
	}

	if !passwordValid {
		fmt.Printf("Invalid password for user: %s\n", req.Email)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "อีเมลหรือรหัสผ่านไม่ถูกต้อง",
		})
		return
	}

	// ตรวจสอบให้แน่ใจว่า role และ position มีข้อมูล
	if user.RoleID == 0 {
		fmt.Printf("User %s has invalid role_id: %d\n", req.Email, user.RoleID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "ข้อมูลสิทธิ์ผู้ใช้ไม่ถูกต้อง",
		})
		return
	}

	// Generate JWT token
	token, err := generateToken(user)
	if err != nil {
		fmt.Printf("Failed to generate token for user %s: %v\n", req.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "ไม่สามารถสร้าง token ได้",
		})
		return
	}

	// Create user profile response - ตรวจสอบข้อมูล Role และ Position
	roleName := ""
	positionName := ""

	if user.Role.Role != "" {
		roleName = user.Role.Role
	}

	if user.Position.PositionName != "" {
		positionName = user.Position.PositionName
	}

	userProfile := UserProfile{
		UserID:       user.UserID,
		UserFname:    user.UserFname,
		UserLname:    user.UserLname,
		Email:        user.Email,
		RoleID:       user.RoleID,
		PositionID:   user.PositionID,
		Role:         roleName,
		PositionName: positionName,
	}

	// Log สำหรับ debug
	fmt.Printf("User login successful: ID=%d, Email=%s, RoleID=%d, Role=%s\n",
		user.UserID, user.Email, user.RoleID, roleName)

	// Response
	c.JSON(http.StatusOK, LoginResponse{
		Token:   token,
		User:    userProfile,
		Message: "เข้าสู่ระบบสำเร็จ",
	})
}

// GetProfile returns current user profile
func GetProfile(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "ไม่พบข้อมูลผู้ใช้",
		})
		return
	}

	var user models.User
	if err := config.DB.Preload("Role").Preload("Position").
		Where("user_id = ? AND delete_at IS NULL", userID).
		First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "ไม่พบข้อมูลผู้ใช้",
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
			"error":   "รูปแบบข้อมูลไม่ถูกต้อง",
			"details": err.Error(),
		})
		return
	}

	// Validate new password (ถ้ามี utils.ValidatePassword)
	if utils.ValidatePassword != nil {
		if valid, message := utils.ValidatePassword(req.NewPassword); !valid {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   message,
			})
			return
		}
	}

	// Check password confirmation
	if req.NewPassword != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "รหัสผ่านใหม่และการยืนยันไม่ตรงกัน",
		})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "ไม่พบข้อมูลผู้ใช้",
		})
		return
	}

	// Get current user
	var user models.User
	if err := config.DB.Where("user_id = ? AND delete_at IS NULL", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "ไม่พบข้อมูลผู้ใช้",
		})
		return
	}

	// Verify current password
	var passwordValid bool
	if utils.CheckPasswordHash != nil {
		passwordValid = utils.CheckPasswordHash(req.CurrentPassword, user.Password)
	} else {
		passwordValid = (req.CurrentPassword == user.Password)
	}

	if !passwordValid {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "รหัสผ่านปัจจุบันไม่ถูกต้อง",
		})
		return
	}

	// Hash new password
	var newPasswordHash string
	if utils.HashPassword != nil {
		hashedPassword, err := utils.HashPassword(req.NewPassword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "ไม่สามารถประมวลผลรหัสผ่านใหม่ได้",
			})
			return
		}
		newPasswordHash = hashedPassword
	} else {
		// Fallback สำหรับกรณีที่ยังไม่มี bcrypt
		newPasswordHash = req.NewPassword
		fmt.Println("Warning: Storing plain text password. Please implement bcrypt.")
	}

	// Update password
	now := time.Now()
	user.Password = newPasswordHash
	user.UpdateAt = &now

	if err := config.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "ไม่สามารถอัพเดทรหัสผ่านได้",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "เปลี่ยนรหัสผ่านสำเร็จ",
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
			"error":   "Token ไม่ถูกต้อง",
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
			"error":   "ไม่พบข้อมูลผู้ใช้",
		})
		return
	}

	// Generate new token
	token, err := generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "ไม่สามารถสร้าง token ใหม่ได้",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"token":   token,
		"message": "สร้าง token ใหม่สำเร็จ",
	})
}

// Logout handles user logout
func Logout(c *gin.Context) {
	// สำหรับ JWT ไม่จำเป็นต้องทำอะไรใน server side
	// แต่ถ้าต้องการ blacklist token ก็สามารถเพิ่มได้

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "ออกจากระบบสำเร็จ",
	})
}
