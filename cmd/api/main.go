package main

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/controllers"
	"fund-management-api/middleware"
	"fund-management-api/monitor"
	"fund-management-api/routes"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

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

	godotenv.Load()

	// SECURITY (Phase 0): refuse to start without a JWT secret. The code previously fell
	// back to a public default secret when JWT_SECRET was unset, which would allow anyone
	// to forge valid tokens (including admin). Fail fast instead of running insecurely.
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET is not set. Refusing to start; set JWT_SECRET in the environment or .env file.")
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

	uploadPath := os.Getenv("UPLOAD_PATH")
	if uploadPath == "" {
		uploadPath = "./uploads"
	}

	// Setup routes
	routes.SetupRoutes(router)

	// SECURITY (Phase 0-C): the broad `router.Static("/uploads", "./uploads")` mount was
	// removed — it exposed EVERY uploaded file (submissions, user documents, personal data)
	// anonymously by URL. All file access now goes through authenticated routes or a
	// short-lived signed URL (GET /api/v1/files/sign -> /api/v1/view). The folders below
	// hold PUBLIC documents and stay openly readable (no login):
	//   - email_assets     : logos embedded in outgoing email (external recipients)
	//   - announcements    : public announcements everyone may read
	//   - fund_forms       : blank fund application forms for download
	//   - import_templates : import templates for download
	// Personal-data folders (users, merge_submissions) are NOT here -> signed URL only.
	router.Static("/uploads/email_assets", filepath.Join(uploadPath, "email_assets"))
	router.Static("/uploads/announcements", filepath.Join(uploadPath, "announcements"))
	router.Static("/uploads/fund_forms", filepath.Join(uploadPath, "fund_forms"))
	router.Static("/uploads/import_templates", filepath.Join(uploadPath, "import_templates"))

	// Create upload directory if not exists
	if err := os.MkdirAll(uploadPath, os.ModePerm); err != nil {
		log.Printf("Warning: Failed to create upload directory: %v", err)
	}

	// MOU: background scheduler ส่งอีเมลแจ้งเตือน MOU ใกล้หมดอายุ ทำงานทุก NOTIFICATION_INTERVAL_MINUTES (default 1440)
	go func() {
		interval := 1440
		if v := os.Getenv("NOTIFICATION_INTERVAL_MINUTES"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				interval = n
			}
		}
		ticker := time.NewTicker(time.Duration(interval) * time.Minute)
		defer ticker.Stop()

		// Run once on startup
		log.Printf("[Scheduler] Starting MOU notification sender (interval: %d min)", interval)
		sent, failed, msg := controllers.SendPendingMouNotifications()
		if sent > 0 || failed > 0 {
			log.Printf("[Scheduler] %s", msg)
		}

		for range ticker.C {
			sent, failed, msg := controllers.SendPendingMouNotifications()
			if sent > 0 || failed > 0 {
				log.Printf("[Scheduler] %s", msg)
			}
		}
	}()

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
