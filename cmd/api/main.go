package main

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/middleware"
	"fund-management-api/monitor"
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

	config.ReloadMailerConfig()

	logFile, logWriter := config.InitLogging()
	if logFile != nil {
		defer logFile.Close()
	}
	gin.DefaultWriter = logWriter
	gin.DefaultErrorWriter = logWriter

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
	httpAddr := fmt.Sprintf(":%s", port)

	log.Printf("Server starting on port %s (HTTP)", port)
	log.Printf("📊 Database connected successfully")
	log.Printf("🔒 Security middlewares enabled")
	log.Printf("🌐 CORS configured for allowed origins")

	if ginMode == "release" {
		log.Printf("🏭 Running in production mode")
	} else {
		log.Printf("🔧 Running in development mode")
		log.Printf("📝 API documentation available at http://localhost:%s/api/v1/info", port)
	}

	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")
	httpsPort := os.Getenv("SERVER_HTTPS_PORT")
	if httpsPort == "" {
		httpsPort = "8443"
	}

	if certFile == "" || keyFile == "" {
		log.Printf("TLS_CERT_FILE or TLS_KEY_FILE not set. HTTPS disabled; running HTTP only.")
		if err := router.Run(httpAddr); err != nil {
			log.Fatal("Failed to start HTTP server:", err)
		}
		return
	}

	httpsAddr := fmt.Sprintf(":%s", httpsPort)
	go func() {
		if err := router.Run(httpAddr); err != nil {
			log.Printf("HTTP server stopped: %v", err)
		}
	}()

	log.Printf("HTTPS API starting on port %s", httpsPort)
	if err := router.RunTLS(httpsAddr, certFile, keyFile); err != nil {
		log.Fatal("Failed to start HTTPS server:", err)
	}
}
