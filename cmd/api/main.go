package main

import (
	"fund-management-api/config"
	"fund-management-api/middleware"
	"fund-management-api/monitor"
	"fund-management-api/routes"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	config.ReloadMailerConfig()

	// Initialize database
	config.InitDB()

	// Set Gin mode
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create logs directory if not exists
	logPath := filepath.Dir(config.LogFilePath())
	if err := os.MkdirAll(logPath, os.ModePerm); err != nil {
		log.Printf("Warning: Failed to create logs directory: %v", err)
	}

	logWriter := io.Writer(os.Stdout)
	logFile, err := os.OpenFile(config.LogFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("Warning: Failed to open log file: %v", err)
	} else {
		defer logFile.Close()
		logWriter = io.MultiWriter(os.Stdout, logFile)
	}
	log.SetOutput(logWriter)
	gin.DefaultWriter = logWriter
	gin.DefaultErrorWriter = logWriter

	// Create Gin router
	router := gin.New()

	// Add logging middleware
	router.Use(gin.LoggerWithWriter(logWriter))

	// Add recovery middleware
	router.Use(gin.Recovery())

	// Add security headers middleware
	router.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:;")
		c.Next()
	})

	// Add CORS middleware
	router.Use(middleware.CORSMiddleware())

	// Optional: Add rate limiting (uncomment in production)
	// router.Use(middleware.RateLimitMiddleware())

	// Register monitoring routes
	monitor.RegisterMonitorPage(router)
	monitor.RegisterLogsRoute(router)

	// Setup routes
	routes.SetupRoutes(router)

	// Serve static files
	router.Static("/uploads", "./uploads")

	// Create upload directory if not exists
	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}
	if err := os.MkdirAll(uploadPath, os.ModePerm); err != nil {
		log.Printf("Warning: Failed to create upload directory: %v", err)
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
