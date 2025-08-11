package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware handles Cross-Origin Resource Sharing with security considerations
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Get allowed origins from environment
		allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
		if allowedOrigins == "" {
			// Default allowed origins for development
			allowedOrigins = "http://localhost:3000,http://127.0.0.1:3000"
		}

		// Check if origin is allowed
		origins := strings.Split(allowedOrigins, ",")
		allowedOrigin := ""
		for _, allowedOrig := range origins {
			allowedOrig = strings.TrimSpace(allowedOrig)
			if origin == allowedOrig {
				allowedOrigin = origin
				break
			}
		}

		// Set CORS headers
		if allowedOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
		} else {
			// Don't set the header if origin is not allowed
			// This prevents unauthorized origins from making requests
		}

		// Get allowed methods from environment
		allowedMethods := os.Getenv("ALLOWED_METHODS")
		if allowedMethods == "" {
			allowedMethods = "GET,POST,PUT,DELETE,PATCH,OPTIONS"
		}

		// Get allowed headers from environment
		allowedHeaders := os.Getenv("ALLOWED_HEADERS")
		if allowedHeaders == "" {
			allowedHeaders = "Content-Type,Authorization,X-Requested-With"
		}

		c.Header("Access-Control-Allow-Methods", allowedMethods)
		c.Header("Access-Control-Allow-Headers", allowedHeaders)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400") // 24 hours

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
