package controllers

import (
	"context"
	"errors"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var ssoClientFactory = func() services.SSOCodeExchanger {
	return services.NewSSOClientFromEnv()
}

func getAuthCookieName() string {
	name := strings.TrimSpace(os.Getenv("AUTH_COOKIE_NAME"))
	if name == "" {
		return "auth_token"
	}
	return name
}

func setAuthTokenCookie(c *gin.Context, token string, maxAgeSeconds int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(getAuthCookieName(), token, maxAgeSeconds, "/", "", true, true)
}

func clearAuthTokenCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(getAuthCookieName(), "", -1, "/", "", true, true)
}

func SSOLoginRedirect(c *gin.Context) {
	if strings.TrimSpace(os.Getenv("SSO_APP_ID")) == "" {
		c.Redirect(http.StatusFound, "/login?error=sso_not_configured")
		return
	}

	c.Redirect(http.StatusFound, services.BuildSSOLoginURLFromEnv())
}

func SSOCallback(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.Redirect(http.StatusFound, "/login?error=sso_missing_code")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	result, err := ssoClientFactory().ExchangeCodeForToken(ctx, code)
	if err != nil || result == nil {
		c.Redirect(http.StatusFound, "/login?error=sso_failed")
		return
	}

	email := strings.ToLower(strings.TrimSpace(result.Email))
	if !result.OK || email == "" {
		c.Redirect(http.StatusFound, "/login?error=sso_failed")
		return
	}

	now := time.Now()

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.Redirect(http.StatusFound, "/login?error=sso_failed")
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var user models.User
	err = tx.Where("email = ?", email).First(&user).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			tx.Rollback()
			c.Redirect(http.StatusFound, "/login?error=sso_failed")
			return
		}

		insertData := map[string]any{
			"email":     email,
			"role_id":   1,
			"password":  nil,
			"create_at": now,
		}
		if name := strings.TrimSpace(result.FirstName); name != "" {
			insertData["user_fname"] = name
		} else {
			insertData["user_fname"] = nil
		}
		if name := strings.TrimSpace(result.LastName); name != "" {
			insertData["user_lname"] = name
		} else {
			insertData["user_lname"] = nil
		}

		if err := tx.Table("users").Create(insertData).Error; err != nil {
			tx.Rollback()
			c.Redirect(http.StatusFound, "/login?error=sso_failed")
			return
		}

		if err := tx.Where("email = ?", email).First(&user).Error; err != nil {
			tx.Rollback()
			c.Redirect(http.StatusFound, "/login?error=sso_failed")
			return
		}
	}

	if user.DeleteAt != nil {
		if err := tx.Model(&models.User{}).
			Where("user_id = ?", user.UserID).
			Update("delete_at", nil).Error; err != nil {
			tx.Rollback()
			c.Redirect(http.StatusFound, "/login?error=sso_failed")
			return
		}
		user.DeleteAt = nil
	}

	if err := upsertSSOIdentity(tx, user.UserID, email, result.ProviderSubject, result.RawClaims, now); err != nil {
		tx.Rollback()
		c.Redirect(http.StatusFound, "/login?error=sso_failed")
		return
	}

	if err := tx.Model(&models.User{}).
		Where("user_id = ?", user.UserID).
		Updates(map[string]any{"last_login_at": now, "update_at": now}).Error; err != nil {
		tx.Rollback()
		c.Redirect(http.StatusFound, "/login?error=sso_failed")
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.Redirect(http.StatusFound, "/login?error=sso_failed")
		return
	}

	user.LastLoginAt = &now
	user.UpdateAt = &now

	token, expiresIn, err := generateAccessToken(user, "")
	if err != nil {
		c.Redirect(http.StatusFound, "/login?error=sso_failed")
		return
	}

	setAuthTokenCookie(c, token, expiresIn)
	c.Redirect(http.StatusFound, "/")
}

func LogoutWithSSORedirect(c *gin.Context) {
	clearAuthTokenCookie(c)
	logoutRedirect := strings.TrimSpace(os.Getenv("SSO_LOGOUT_REDIRECT_URL"))

	if strings.TrimSpace(os.Getenv("SSO_APP_ID")) == "" {
		if logoutRedirect != "" {
			c.Redirect(http.StatusFound, logoutRedirect)
			return
		}
		c.Redirect(http.StatusFound, "/login")
		return
	}

	c.Redirect(http.StatusFound, services.BuildSSOLogoutURLFromEnv())
}

func upsertSSOIdentity(tx *gorm.DB, userID int, email string, providerSubject string, rawClaims []byte, now time.Time) error {
	provider := services.DefaultSSOProvider
	providerSubject = strings.TrimSpace(providerSubject)

	var identity models.AuthIdentity
	var err error

	if providerSubject != "" {
		err = tx.Where("provider = ? AND provider_subject = ?", provider, providerSubject).
			First(&identity).Error
	} else {
		err = tx.Where("user_id = ? AND provider = ?", userID, provider).
			Order("identity_id ASC").
			First(&identity).Error
	}

	providerSubjectPtr := nullableString(providerSubject)
	emailPtr := nullableString(email)

	if errors.Is(err, gorm.ErrRecordNotFound) {
		newIdentity := models.AuthIdentity{
			UserID:          userID,
			Provider:        provider,
			ProviderSubject: providerSubjectPtr,
			EmailAtProvider: emailPtr,
			RawClaims:       rawClaims,
			LastLoginAt:     &now,
		}
		return tx.Create(&newIdentity).Error
	}

	if err != nil {
		return err
	}

	updates := map[string]any{
		"user_id":           userID,
		"email_at_provider": emailPtr,
		"raw_claims":        rawClaims,
		"last_login_at":     now,
		"update_at":         now,
	}
	if providerSubjectPtr != nil {
		updates["provider_subject"] = providerSubjectPtr
	}

	return tx.Model(&models.AuthIdentity{}).
		Where("identity_id = ?", identity.IdentityID).
		Updates(updates).Error
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
