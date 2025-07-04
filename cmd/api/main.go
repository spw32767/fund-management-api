package main

import (
	"fund-management-api/config"
	"fund-management-api/middleware"
	"fund-management-api/routes"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize database
	config.InitDB()

	// Set Gin mode
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	router := gin.New()

	// Add logging middleware
	router.Use(gin.Logger())

	// Add recovery middleware
	router.Use(gin.Recovery())

	// Add security headers middleware
	router.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	})

	// Add CORS middleware
	router.Use(middleware.CORSMiddleware())

	// Optional: Add rate limiting (uncomment in production)
	// router.Use(middleware.RateLimitMiddleware())

	// Register Log route
	// Register /logs route early (before 404 catch-all in SetupRoutes)
	router.GET("/logs", func(c *gin.Context) {
		const accessToken = "secret-token"
		if c.Query("token") != accessToken {
			c.JSON(401, gin.H{"error": "Unauthorized"})
			return
		}

		logData, err := os.ReadFile("fund-api.log")
		if err != nil {
			c.JSON(500, gin.H{"error": "Unable to read log"})
			return
		}

		c.Data(200, "text/plain; charset=utf-8", logData)
	})

	// Setup routes
	routes.SetupRoutes(router)

	// Create upload directory if not exists
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}
	if err := os.MkdirAll(uploadPath, os.ModePerm); err != nil {
		log.Printf("Warning: Failed to create upload directory: %v", err)
	}

	// Create logs directory if not exists
	logPath := "./logs"
	if err := os.MkdirAll(logPath, os.ModePerm); err != nil {
		log.Printf("Warning: Failed to create logs directory: %v", err)
	}

	// Start server
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Server starting on port %s", port)
	log.Printf("📊 Database connected successfully")
	log.Printf("🔒 Security middlewares enabled")
	log.Printf("🌐 CORS configured for allowed origins")

	if ginMode == "release" {
		log.Printf("🏭 Running in production mode")
	} else {
		log.Printf("🔧 Running in development mode")
		log.Printf("📝 API documentation available at http://localhost:%s/api/v1/info", port)
	}

	if err := router.Run(":" + port); err != nil {
		log.Fatal("❌ Failed to start server:", err)
	}
}
