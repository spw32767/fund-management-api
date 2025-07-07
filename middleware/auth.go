package middleware

import (
	"errors"
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
	RoleID int    `json:"role_id"`
	jwt.RegisteredClaims
}

// AuthMiddleware validates JWT token and session (updated version)
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization header is required",
				"code":    "MISSING_AUTH_HEADER",
			})
			c.Abort()
			return
		}

		// Check Bearer prefix
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid authorization header format. Use 'Bearer <token>'",
				"code":    "INVALID_AUTH_FORMAT",
			})
			c.Abort()
			return
		}

		// Get JWT secret
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = "default-secret-change-this-in-production"
		}

		// Parse token with enhanced validation
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("invalid signing method")
			}
			return []byte(jwtSecret), nil
		})

		if err != nil {
			var errorCode string
			var errorMessage string

			// JWT v5 error handling
			switch {
			case errors.Is(err, jwt.ErrTokenExpired):
				errorCode = "TOKEN_EXPIRED"
				errorMessage = "Access token has expired. Please refresh your token."
			case errors.Is(err, jwt.ErrTokenSignatureInvalid):
				errorCode = "INVALID_SIGNATURE"
				errorMessage = "Invalid token signature"
			case errors.Is(err, jwt.ErrTokenMalformed):
				errorCode = "MALFORMED_TOKEN"
				errorMessage = "Malformed token"
			case errors.Is(err, jwt.ErrTokenNotValidYet):
				errorCode = "TOKEN_NOT_VALID_YET"
				errorMessage = "Token not valid yet"
			case errors.Is(err, jwt.ErrTokenInvalidClaims):
				errorCode = "INVALID_CLAIMS"
				errorMessage = "Invalid token claims"
			default:
				errorCode = "INVALID_TOKEN"
				errorMessage = "Invalid token: " + err.Error()
			}

			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   errorMessage,
				"code":    errorCode,
			})
			c.Abort()
			return
		}

		if !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid token",
				"code":    "INVALID_TOKEN",
			})
			c.Abort()
			return
		}

		// Get claims
		claims, ok := token.Claims.(*Claims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid token claims",
				"code":    "INVALID_CLAIMS",
			})
			c.Abort()
			return
		}

		// Additional validation - check if token is too old (optional)
		if time.Until(claims.ExpiresAt.Time) < 0 {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Token has expired",
				"code":    "TOKEN_EXPIRED",
			})
			c.Abort()
			return
		}

		// NEW: Session validation - only if JTI exists
		if claims.ID != "" {
			// Check if session exists and is active
			var session models.UserSession
			if err := config.DB.Where("user_id = ? AND access_token_jti = ? AND is_active = ?",
				claims.UserID, claims.ID, true).First(&session).Error; err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error":   "Session not found or has been terminated",
					"code":    "SESSION_NOT_FOUND",
				})
				c.Abort()
				return
			}

			// Check if session has expired
			if time.Now().After(session.ExpiresAt) {
				// Mark session as inactive
				config.DB.Model(&session).Update("is_active", false)

				c.JSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error":   "Session has expired",
					"code":    "SESSION_EXPIRED",
				})
				c.Abort()
				return
			}

			// Set session ID in context
			c.Set("sessionID", session.SessionID)

			// Update session last activity in background (non-blocking)
			go func() {
				now := time.Now()
				config.DB.Model(&models.UserSession{}).
					Where("session_id = ?", session.SessionID).
					Update("last_activity", now)
			}()
		}

		// Check if user still exists and is active
		var user models.User
		if err := config.DB.Where("user_id = ? AND delete_at IS NULL", claims.UserID).First(&user).Error; err != nil {
			// Mark session as inactive if user doesn't exist and session exists
			if claims.ID != "" {
				config.DB.Model(&models.UserSession{}).
					Where("access_token_jti = ?", claims.ID).
					Update("is_active", false)
			}

			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "User account not found or has been deactivated",
				"code":    "USER_NOT_FOUND",
			})
			c.Abort()
			return
		}

		// Check if user's email matches token email (additional security)
		if user.Email != claims.Email {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Token user mismatch",
				"code":    "USER_MISMATCH",
			})
			c.Abort()
			return
		}

		// Set user info in context for use in handlers
		c.Set("userID", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("roleID", claims.RoleID)
		c.Set("user", user)     // Set full user object for convenience
		c.Set("jti", claims.ID) // Add JTI to context

		// Add token expiry warning header if token expires soon (within 30 minutes)
		if time.Until(claims.ExpiresAt.Time) < 30*time.Minute {
			c.Header("X-Token-Expires-Soon", "true")
			c.Header("X-Token-Expires-At", claims.ExpiresAt.Time.Format(time.RFC3339))
		}

		c.Next()
	}
}

// RequireRole checks if user has specific role(s) - existing function unchanged
func RequireRole(roleIDs ...int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRoleID, exists := c.Get("roleID")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Role information not found",
				"code":    "ROLE_NOT_FOUND",
			})
			c.Abort()
			return
		}

		// Check if user's role is in allowed roles
		userRole := userRoleID.(int)
		for _, allowedRole := range roleIDs {
			if userRole == allowedRole {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "Insufficient permissions for this resource",
			"code":    "INSUFFICIENT_PERMISSIONS",
		})
		c.Abort()
	}
}
