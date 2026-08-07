package controllers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ===== SSO cross-app handoff =====
//
// This subsystem lets a sibling application (e.g. the "academic" system on another
// subdomain of the same VM) reuse our KKU SSO login WITHOUT registering its own SSO
// app. Our app stays the single SSO integration point and acts as a lightweight
// identity broker:
//
//  1. The sibling app redirects the unauthenticated user to
//     GET /api/auth/sso/login?return_to=<sibling callback URL>.
//  2. We remember return_to in a short-lived cookie, then run the normal SSO flow.
//  3. After SSO succeeds (and the email passes our users allowlist), we mint a
//     one-time, short-lived "handoff ticket" and redirect the browser back to
//     return_to?ticket=<ticket> instead of "/".
//  4. The sibling app's backend exchanges the ticket server-to-server at
//     POST /api/auth/sso/handoff/verify and learns who the user is. It then creates
//     ITS OWN session cookie on ITS OWN subdomain.
//
// The ticket is an opaque 256-bit random string (like an OAuth authorization code):
// single-use, ~60s TTL, no shared signing secret required. Our auth cookie remains
// host-only to fs.* — identity crosses the subdomain boundary only through the ticket.
//
// The whole feature is OPT-IN: when SSO_HANDOFF_ALLOWED_ORIGINS is unset, return_to is
// ignored and SSO behaves exactly as before.

const (
	// handoffTokenType is the user_tokens.token_type used to persist one-time handoff
	// tickets. Reusing user_tokens means no schema migration is needed.
	handoffTokenType = "sso_handoff"
	// returnToCookieName remembers the sibling app's callback URL across the SSO round trip.
	returnToCookieName = "sso_return_to"
	// returnToCookieMaxAgeSeconds — long enough to complete an SSO sign-in, short otherwise.
	returnToCookieMaxAgeSeconds = 300
	// handoffTicketTTL is how long a minted ticket stays valid before it must be discarded.
	handoffTicketTTL = 60 * time.Second
)

// handoffAllowedOrigins returns the allowlisted origins (scheme://host[:port]) that may
// be used as SSO handoff targets, configured via SSO_HANDOFF_ALLOWED_ORIGINS (comma
// separated). An empty result means the handoff feature is disabled.
func handoffAllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("SSO_HANDOFF_ALLOWED_ORIGINS"))
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		o := strings.ToLower(strings.TrimRight(strings.TrimSpace(p), "/"))
		if o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

// validateReturnTo returns the original return_to URL when it targets an allowlisted
// origin over HTTPS, or "" when it is missing, malformed, non-HTTPS, or not allowed.
// This is the open-redirect / ticket-leak guard: we only ever hand a ticket to an
// origin we explicitly trust.
func validateReturnTo(returnTo string) string {
	returnTo = strings.TrimSpace(returnTo)
	if returnTo == "" {
		return ""
	}

	allowed := handoffAllowedOrigins()
	if len(allowed) == 0 {
		return ""
	}

	parsed, err := url.Parse(returnTo)
	if err != nil {
		return ""
	}
	if !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
		return ""
	}

	origin := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	for _, a := range allowed {
		if origin == a {
			return returnTo
		}
	}
	return ""
}

// appendTicketToURL returns returnTo with the ticket added as a query parameter,
// preserving any existing query the sibling app included in its callback URL.
func appendTicketToURL(returnTo, ticket string) string {
	parsed, err := url.Parse(returnTo)
	if err != nil {
		// validateReturnTo already parsed this successfully; fall back defensively.
		sep := "?"
		if strings.Contains(returnTo, "?") {
			sep = "&"
		}
		return returnTo + sep + "ticket=" + url.QueryEscape(ticket)
	}

	q := parsed.Query()
	q.Set("ticket", ticket)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func setReturnToCookie(c *gin.Context, value string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(returnToCookieName, value, returnToCookieMaxAgeSeconds, "/", "", true, true)
}

func clearReturnToCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(returnToCookieName, "", -1, "/", "", true, true)
}

// consumeReturnToCookie reads and immediately clears the return_to cookie, returning the
// validated target (or "" when absent/invalid). It is safe to call unconditionally.
func consumeReturnToCookie(c *gin.Context) string {
	raw, err := c.Cookie(returnToCookieName)
	if err != nil || strings.TrimSpace(raw) == "" {
		return ""
	}
	clearReturnToCookie(c)
	return validateReturnTo(raw)
}

// issueHandoffTicket generates a one-time opaque ticket, persists it in user_tokens with
// a short TTL, and returns it. The sibling app exchanges it at VerifyHandoffTicket.
func issueHandoffTicket(userID int, ipAddress string) (string, error) {
	buf := make([]byte, 32) // 256 bits
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	ticket := base64.RawURLEncoding.EncodeToString(buf)

	now := time.Now()
	record := models.UserToken{
		UserID:     userID,
		TokenType:  handoffTokenType,
		Token:      ticket,
		ExpiresAt:  now.Add(handoffTicketTTL),
		IsRevoked:  false,
		DeviceInfo: "sso_handoff",
		IPAddress:  ipAddress,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := config.DB.Create(&record).Error; err != nil {
		return "", err
	}
	return ticket, nil
}

type handoffVerifyRequest struct {
	Ticket string `json:"ticket"`
}

type handoffIdentityResponse struct {
	OK        bool   `json:"ok"`
	UserID    int    `json:"user_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// VerifyHandoffTicket is the server-to-server endpoint the sibling app calls to exchange
// a one-time handoff ticket for the authenticated user's identity. The ticket is
// single-use and short-lived. When SSO_HANDOFF_CLIENT_SECRET is configured, the caller
// must present the same value in the X-Handoff-Client-Secret header.
func VerifyHandoffTicket(c *gin.Context) {
	if secret := strings.TrimSpace(os.Getenv("SSO_HANDOFF_CLIENT_SECRET")); secret != "" {
		provided := strings.TrimSpace(c.GetHeader("X-Handoff-Client-Secret"))
		if subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "invalid client secret"})
			return
		}
	}

	var req handoffVerifyRequest
	_ = c.ShouldBindJSON(&req)
	ticket := strings.TrimSpace(req.Ticket)
	if ticket == "" {
		// Allow the ticket to be passed as a query parameter as a fallback.
		ticket = strings.TrimSpace(c.Query("ticket"))
	}
	if ticket == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing ticket"})
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "server error"})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var record models.UserToken
	err := tx.Where("token = ? AND token_type = ? AND is_revoked = ?", ticket, handoffTokenType, false).
		First(&record).Error
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "invalid or used ticket"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "server error"})
		return
	}

	// Consume the ticket first (one-time use), regardless of expiry, so a leaked ticket
	// cannot be replayed even within the TTL window.
	if err := tx.Model(&models.UserToken{}).
		Where("token_id = ?", record.TokenID).
		Update("is_revoked", true).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "server error"})
		return
	}

	if time.Now().After(record.ExpiresAt) {
		tx.Commit()
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "expired ticket"})
		return
	}

	var user models.User
	if err := tx.Where("user_id = ? AND delete_at IS NULL", record.UserID).First(&user).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "user not allowed"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "server error"})
		return
	}

	c.JSON(http.StatusOK, handoffIdentityResponse{
		OK:        true,
		UserID:    user.UserID,
		Email:     user.Email,
		FirstName: user.UserFname,
		LastName:  user.UserLname,
	})
}
