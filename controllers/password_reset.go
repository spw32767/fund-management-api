package controllers

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	passwordResetTokenGenerator = func() (string, error) {
		return generateRefreshToken()
	}

	sendMailFunc                              = config.SendMail
	passwordResetRepo passwordResetRepository = &gormPasswordResetRepository{}
)

type passwordResetRepository interface {
	FindUserByEmail(email string) (*models.User, error)
	RevokePasswordResetTokens(userID int, now time.Time) error
	CreateUserToken(token *models.UserToken) error
	FindActivePasswordResetTokens(now time.Time) ([]models.UserToken, error)
	UpdateUserPassword(userID int, hashedPassword string, now time.Time) error
	RevokeToken(tokenID int, now time.Time) error
}

type gormPasswordResetRepository struct{}

func (r *gormPasswordResetRepository) FindUserByEmail(email string) (*models.User, error) {
	var user models.User
	if err := config.DB.Where("email = ? AND delete_at IS NULL", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *gormPasswordResetRepository) RevokePasswordResetTokens(userID int, now time.Time) error {
	if userID == 0 {
		return nil
	}

	return config.DB.Model(&models.UserToken{}).
		Where("user_id = ? AND token_type = ? AND is_revoked = ?", userID, "password_reset", false).
		Updates(map[string]interface{}{
			"is_revoked": true,
			"updated_at": now,
			"expires_at": now,
		}).Error
}

func (r *gormPasswordResetRepository) CreateUserToken(token *models.UserToken) error {
	return config.DB.Create(token).Error
}

func (r *gormPasswordResetRepository) FindActivePasswordResetTokens(now time.Time) ([]models.UserToken, error) {
	var tokens []models.UserToken
	err := config.DB.Where("token_type = ? AND is_revoked = ? AND expires_at > ?", "password_reset", false, now).
		Order("created_at DESC").
		Find(&tokens).Error
	return tokens, err
}

func (r *gormPasswordResetRepository) UpdateUserPassword(userID int, hashedPassword string, now time.Time) error {
	return config.DB.Model(&models.User{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"password":  hashedPassword,
			"update_at": now,
		}).Error
}

func (r *gormPasswordResetRepository) RevokeToken(tokenID int, now time.Time) error {
	return config.DB.Model(&models.UserToken{}).
		Where("token_id = ?", tokenID).
		Updates(map[string]interface{}{
			"is_revoked": true,
			"updated_at": now,
			"expires_at": now,
		}).Error
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token           string `json:"token"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

// ForgotPassword handles password reset token generation and email dispatch.
func ForgotPassword(c *gin.Context) {
	var req forgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": err.Error(),
		})
		return
	}

	req.Email = utils.SanitizeInput(req.Email)
	if !utils.ValidateEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid email format",
		})
		return
	}

	user, err := passwordResetRepo.FindUserByEmail(req.Email)
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to process request",
			})
			return
		}

		// Always return success for non-existing users to avoid email enumeration.
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "If the email exists, a reset link has been sent.",
		})
		return
	}

	rawToken, err := passwordResetTokenGenerator()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create reset token",
		})
		return
	}

	hashedToken, err := utils.HashPassword(rawToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to secure reset token",
		})
		return
	}

	clientIP := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	now := time.Now()
	expiresAt := now.Add(10 * time.Minute)

	if err := passwordResetRepo.RevokePasswordResetTokens(user.UserID, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to prepare reset token",
		})
		return
	}

	token := models.UserToken{
		UserID:     user.UserID,
		TokenType:  "password_reset",
		Token:      hashedToken,
		ExpiresAt:  expiresAt,
		IsRevoked:  false,
		DeviceInfo: "password_reset",
		IPAddress:  clientIP,
		UserAgent:  userAgent,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := passwordResetRepo.CreateUserToken(&token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to store reset token",
		})
		return
	}

	if err := sendPasswordResetEmail(*user, rawToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to send reset email",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "If the email exists, a reset link has been sent.",
	})
}

// ResetPassword handles password reset using a previously generated token.
func ResetPassword(c *gin.Context) {
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": err.Error(),
		})
		return
	}

	req.Token = utils.SanitizeInput(req.Token)
	req.NewPassword = utils.SanitizeInput(req.NewPassword)
	req.ConfirmPassword = utils.SanitizeInput(req.ConfirmPassword)

	if req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Token is required",
		})
		return
	}

	if req.NewPassword != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Passwords do not match",
		})
		return
	}

	if valid, message := utils.ValidatePassword(req.NewPassword); !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   message,
		})
		return
	}

	now := time.Now()
	tokenRecord, err := findActivePasswordResetToken(passwordResetRepo, req.Token, now)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid or expired token",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to verify token",
		})
		return
	}

	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to hash password",
		})
		return
	}

	if err := passwordResetRepo.UpdateUserPassword(tokenRecord.UserID, hashedPassword, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update password",
		})
		return
	}

	if err := passwordResetRepo.RevokeToken(tokenRecord.TokenID, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to revoke token",
		})
		return
	}

	if err := passwordResetRepo.RevokePasswordResetTokens(tokenRecord.UserID, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to finalize reset",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Password updated successfully",
	})
}

func findActivePasswordResetToken(repo passwordResetRepository, rawToken string, now time.Time) (*models.UserToken, error) {
	tokens, err := repo.FindActivePasswordResetTokens(now)
	if err != nil {
		return nil, err
	}

	for i := range tokens {
		if utils.CheckPasswordHash(rawToken, tokens[i].Token) {
			return &tokens[i], nil
		}
	}

	return nil, gorm.ErrRecordNotFound
}

func sendPasswordResetEmail(user models.User, rawToken string) error {
	baseURL := strings.TrimSpace(os.Getenv("APP_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	resetURL, err := buildResetURL(baseURL, rawToken)
	if err != nil {
		return err
	}

	fullName := strings.TrimSpace(fmt.Sprintf("%s %s", user.UserFname, user.UserLname))
	if fullName == "" {
		fullName = "ผู้ใช้งาน"
	}

	subject := "คำแนะนำการรีเซ็ตรหัสผ่าน"
	expiresIn := "10 นาที"
	paragraphs := []string{
		fmt.Sprintf("เรียนคุณ %s", fullName),
		"เราได้รับคำขอให้รีเซ็ตรหัสผ่านสำหรับระบบบริหารจัดการทุนวิจัย",
	}

	paragraphs = append(paragraphs,
		fmt.Sprintf("โปรดคลิกปุ่มด้านล่างเพื่อสร้างรหัสผ่านใหม่ ลิงก์นี้จะหมดอายุใน %s.", expiresIn),
		"หากคุณไม่ได้ส่งคำขอนี้ คุณสามารถเพิกเฉยต่ออีเมลฉบับนี้ได้",
	)

	meta := []emailMetaItem{{
		Label: "ลิงก์หมดอายุภายใน",
		Value: expiresIn,
	}}

	escapedResetURL := template.HTMLEscapeString(resetURL)
	footerHTML := fmt.Sprintf(
		"หากปุ่มไม่ทำงาน คุณสามารถคัดลอกลิงก์นี้แล้วเปิดในเบราว์เซอร์ของคุณ:<br /><a href=\"%s\" style=\"color:#2563eb;\">%s</a>",
		escapedResetURL,
		escapedResetURL,
	)

	html := buildEmailTemplate(subject, paragraphs, meta, "รีเซ็ตรหัสผ่าน", resetURL, footerHTML)
	return sendMailFunc([]string{user.Email}, subject, html)
}

func buildResetURL(baseURL, token string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/reset-password"
	query := parsed.Query()
	query.Set("token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
