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

// AuthMiddleware validates JWT token with enhanced security
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
				errorMessage = "Token has expired"
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

		// Check if user still exists and is active
		var user models.User
		if err := config.DB.Where("user_id = ? AND delete_at IS NULL", claims.UserID).First(&user).Error; err != nil {
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
		c.Set("user", user) // Set full user object for convenience

		// Add token expiry warning header if token expires soon (within 1 hour)
		if time.Until(claims.ExpiresAt.Time) < time.Hour {
			c.Header("X-Token-Expires-Soon", "true")
			c.Header("X-Token-Expires-At", claims.ExpiresAt.Time.Format(time.RFC3339))
		}

		c.Next()
	}
}

// RequireRole checks if user has specific role(s)
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
		allowed := false
		for _, roleID := range roleIDs {
			if userRole == roleID {
				allowed = true
				break
			}
		}

		if !allowed {
			// Get role names for better error message
			var allowedRoles []string
			for _, roleID := range roleIDs {
				switch roleID {
				case 1:
					allowedRoles = append(allowedRoles, "teacher")
				case 2:
					allowedRoles = append(allowedRoles, "staff")
				case 3:
					allowedRoles = append(allowedRoles, "admin")
				default:
					allowedRoles = append(allowedRoles, "unknown")
				}
			}

			c.JSON(http.StatusForbidden, gin.H{
				"success":        false,
				"error":          "Insufficient permissions",
				"code":           "INSUFFICIENT_PERMISSIONS",
				"required_roles": allowedRoles,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequirePermission is a more granular permission check (future enhancement)
func RequirePermission(permission string) gin.HandlerFunc {
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

		// Simple permission mapping (can be enhanced with database-driven permissions)
		rolePermissions := map[int][]string{
			1: {"view_own_applications", "create_application", "edit_own_application"}, // teacher
			2: {"view_applications", "process_applications"},                           // staff
			3: {"view_all", "edit_all", "delete_all", "admin_functions"},               // admin
		}

		userRole := userRoleID.(int)
		permissions, exists := rolePermissions[userRole]
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Unknown role",
				"code":    "UNKNOWN_ROLE",
			})
			c.Abort()
			return
		}

		// Check if user has required permission
		hasPermission := false
		for _, perm := range permissions {
			if perm == permission {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{
				"success":             false,
				"error":               "Permission denied",
				"code":                "PERMISSION_DENIED",
				"required_permission": permission,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RateLimitMiddleware implements basic rate limiting (optional)
func RateLimitMiddleware() gin.HandlerFunc {
	// This is a simple implementation - in production, use Redis or similar
	requestCounts := make(map[string]int)
	lastReset := time.Now()

	return func(c *gin.Context) {
		// Reset counts every hour
		if time.Since(lastReset) > time.Hour {
			requestCounts = make(map[string]int)
			lastReset = time.Now()
		}

		clientIP := c.ClientIP()
		requestCounts[clientIP]++

		// Allow 1000 requests per hour per IP
		if requestCounts[clientIP] > 1000 {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success":     false,
				"error":       "Rate limit exceeded",
				"code":        "RATE_LIMIT_EXCEEDED",
				"retry_after": "1 hour",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
