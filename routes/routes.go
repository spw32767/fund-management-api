package routes

import (
	"fmt"
	"fund-management-api/controllers"
	"fund-management-api/middleware"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine) {
	// Add security headers middleware
	router.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	})

	// API v1 group
	v1 := router.Group("/api/v1")
	{
		// Public routes
		public := v1.Group("")
		{

			RegisterUploadRoutes(public) // สำหรับ POST /upload
			RegisterFileRoutes(public)   // สำหรับ GET /files, DELETE /files/:name

			// Authentication
			public.POST("/login", controllers.Login)

			// NEW: Refresh token endpoint (public)
			public.POST("/refresh", controllers.RefreshTokenWithRefreshToken)

			// Health check
			public.GET("/health", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"status":  "ok",
					"message": "Fund Management API is running",
					"timestamp": gin.H{
						"server": "2025-07-02T10:00:00Z",
					},
					"version": "1.0.0",
					"success": true,
				})
			})

			// API Info
			public.GET("/info", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"name":        "Fund Management API",
					"version":     "1.0.0",
					"description": "API for managing research fund applications",
					"endpoints": gin.H{
						"auth": gin.H{
							"login":           "POST /api/v1/login",
							"refresh":         "POST /api/v1/refresh",
							"profile":         "GET /api/v1/profile",
							"change_password": "PUT /api/v1/change-password",
							"logout":          "POST /api/v1/logout",
							"sessions":        "GET /api/v1/sessions",
						},
						"applications": gin.H{
							"list":   "GET /api/v1/applications",
							"create": "POST /api/v1/applications",
							"detail": "GET /api/v1/applications/:id",
						},
						"dashboard": gin.H{
							"stats": "GET /api/v1/dashboard/stats",
						},
						"role_based": gin.H{
							"teacher_subcategories": "GET /api/v1/teacher/subcategories",
							"staff_subcategories":   "GET /api/v1/staff/subcategories",
							"admin_manage_roles":    "PUT /api/v1/admin/subcategories/:id/roles",
						},
					},
				})
			})
		}

		// Protected routes (require authentication)
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			// Authentication routes
			protected.GET("/profile", controllers.GetProfile)
			protected.PUT("/change-password", controllers.ChangePassword)
			protected.POST("/refresh-token", controllers.RefreshToken) // Legacy endpoint

			// NEW: Session management endpoints
			protected.POST("/logout", controllers.Logout)
			protected.GET("/sessions", controllers.GetActiveSessions)
			protected.POST("/sessions/revoke-others", controllers.RevokeOtherSessions)

			// Common endpoints (all authenticated users)
			protected.GET("/years", controllers.GetYears)
			protected.GET("/categories", controllers.GetCategories)
			protected.GET("/subcategories", controllers.GetSubcategories)

			// General submissions listing (all users)
			protected.GET("/submissions", controllers.GetAllSubmissions)        // ดูรายการ submissions (filtered by role)
			protected.GET("/submissions/search", controllers.SearchSubmissions) // ค้นหา submissions

			// Teacher-specific endpoints
			teacher := protected.Group("/teacher")
			{
				// ไม่ต้องใส่ RequireRole(1) เพราะ GetSubcategoryForRole จะ check role เอง
				teacher.GET("/subcategories", controllers.GetSubcategoryForRole)
				teacher.GET("/submissions", controllers.GetTeacherSubmissions) // Teacher ดู submissions ของตัวเอง
			}

			// Staff-specific endpoints
			staff := protected.Group("/staff")
			{
				// ใช้ function เดียวกัน
				staff.GET("/subcategories", controllers.GetSubcategoryForRole)
				staff.GET("/submissions", controllers.GetStaffSubmissions) // Staff ดู submissions ของตัวเอง
			}

			// Fund Applications
			applications := protected.Group("/applications")
			{
				// Teacher & Admin can view their applications
				applications.GET("", controllers.GetApplications)
				applications.GET("/:id", controllers.GetApplication)

				// Only teachers can create/update/delete applications
				applications.POST("", middleware.RequireRole(1), controllers.CreateApplication) // 1 = teacher
				applications.PUT("/:id", middleware.RequireRole(1), controllers.UpdateApplication)
				applications.DELETE("/:id", middleware.RequireRole(1), controllers.DeleteApplication)

				// Only admin can approve/reject
				applications.POST("/:id/approve", middleware.RequireRole(3), controllers.ApproveApplication) // 3 = admin
				applications.POST("/:id/reject", middleware.RequireRole(3), controllers.RejectApplication)
			}

			submissions := protected.Group("/submissions")
			{
				// Basic CRUD
				submissions.POST("", controllers.CreateSubmission)
				submissions.GET("/:id", controllers.GetSubmission)
				submissions.PUT("/:id", controllers.UpdateSubmission)
				submissions.DELETE("/:id", controllers.DeleteSubmission)

				// Submit submission
				submissions.POST("/:id/submit", controllers.SubmitSubmission)

				// Add specific details
				submissions.POST("/:id/publication-details", controllers.AddPublicationDetails)
				submissions.POST("/:id/fund-details", controllers.AddFundDetails)

				// Documents management
				submissions.POST("/:id/documents", controllers.AttachDocument)
				submissions.GET("/:id/documents", controllers.GetSubmissionDocuments)

				// === Co-authors Management (ใหม่) ===
				// submissions.POST("/:id/coauthors", controllers.AddCoauthor)               // เพิ่ม co-author
				// submissions.GET("/:id/coauthors", controllers.GetCoauthors)               // ดู co-authors
				// submissions.PUT("/:id/coauthors/:user_id", controllers.UpdateCoauthor)    // แก้ไข co-author
				// submissions.DELETE("/:id/coauthors/:user_id", controllers.RemoveCoauthor) // ลบ co-author

				// === NEW: Submission Users Management (ให้ตรงกับ Frontend) ===
				submissions.POST("/:id/users", controllers.AddSubmissionUser)               // เพิ่ม user ลงใน submission
				submissions.GET("/:id/users", controllers.GetSubmissionUsers)               // ดู users ใน submission
				submissions.PUT("/:id/users/:user_id", controllers.UpdateSubmissionUser)    // แก้ไข user ใน submission
				submissions.DELETE("/:id/users/:user_id", controllers.RemoveSubmissionUser) // ลบ user จาก submission

				// === NEW: Batch Operations for Frontend ===
				submissions.POST("/:id/users/batch", controllers.AddMultipleUsers)     // เพิ่ม users หลายคนพร้อมกัน
				submissions.POST("/:id/users/set-coauthors", controllers.SetCoauthors) // ตั้งค่า co-authors ทั้งหมด (replace existing)

				// Enhanced submission details with co-authors
				//submissions.GET("/:id/full", controllers.GetSubmissionWithCoauthors) // ดู submission พร้อม co-authors

				// เพิ่ม route ใหม่สำหรับแนบไฟล์
				submissions.POST("/:id/attach-document", controllers.AttachDocumentToSubmission) // แนบไฟล์กับ submission
			}

			// Files management
			files := protected.Group("/files")
			{
				files.POST("/upload", controllers.UploadFile)
				files.GET("/managed/:id", controllers.GetFile)               // เปลี่ยนเป็น /managed/:id
				files.GET("/managed/:id/download", controllers.DownloadFile) // เปลี่ยนเป็น /managed/:id/download
				files.DELETE("/managed/:id", controllers.DeleteFile)         // เปลี่ยนเป็น /managed/:id
			}

			// Documents
			documents := protected.Group("/documents")
			{
				documents.POST("/upload/:id", controllers.UploadDocument)
				documents.GET("/application/:id", controllers.GetDocuments)
				documents.GET("/download/:document_id", controllers.DownloadDocument)
				documents.DELETE("/:document_id", controllers.DeleteDocument)
				documents.GET("/types", controllers.GetDocumentTypes)
			}

			// Dashboard
			dashboard := protected.Group("/dashboard")
			{
				dashboard.GET("/stats", controllers.GetDashboardStats)
				dashboard.GET("/budget-summary", controllers.GetBudgetSummary)
				dashboard.GET("/applications-summary", controllers.GetApplicationsSummary)
			}

			// Publication Rewards
			publications := protected.Group("/publication-rewards")
			{
				// List and view (teachers can see their own, admin can see all)
				publications.GET("", controllers.GetPublicationRewards)
				publications.GET("/:id", controllers.GetPublicationReward)

				// Only teachers can create/update/delete
				publications.POST("", middleware.RequireRole(1), controllers.CreatePublicationReward)
				publications.PUT("/:id", middleware.RequireRole(1), controllers.UpdatePublicationReward)
				publications.DELETE("/:id", middleware.RequireRole(1), controllers.DeletePublicationReward)

				// Only admin can approve/reject
				publications.POST("/:id/approve", middleware.RequireRole(3), controllers.ApprovePublicationReward)
				publications.POST("/:id/reject", middleware.RequireRole(3), controllers.RejectPublicationReward)

				// Documents
				publications.POST("/:id/documents", controllers.UploadPublicationDocument)
				publications.GET("/:id/documents", controllers.GetPublicationDocuments)

				// === REWARD RATES API ===
				rates := publications.Group("/rates")
				{
					// Public endpoints (สำหรับ calculation)
					rates.GET("", controllers.GetPublicationRewardRates)             // GET /api/v1/publication-rewards/rates
					rates.GET("/all", controllers.GetAllPublicationRewardRates)      // GET /api/v1/publication-rewards/rates/all
					rates.GET("/lookup", controllers.GetPublicationRewardRateLookup) // GET /api/v1/publication-rewards/rates/lookup
					rates.GET("/years", controllers.GetAvailableYears)               // GET /api/v1/publication-rewards/rates/years

					// Admin only endpoints
					rates.GET("/admin", middleware.RequireRole(3), controllers.GetPublicationRewardRatesAdmin)           // GET /api/v1/publication-rewards/rates/admin (ดูทั้งหมด ไม่ filter is_active)
					rates.POST("", middleware.RequireRole(3), controllers.CreatePublicationRewardRate)                   // POST /api/v1/publication-rewards/rates
					rates.PUT("/bulk", middleware.RequireRole(3), controllers.UpdatePublicationRewardRates)              // PUT /api/v1/publication-rewards/rates/bulk (existing)
					rates.PUT("/:id", middleware.RequireRole(3), controllers.UpdatePublicationRewardRate)                // PUT /api/v1/publication-rewards/rates/:id
					rates.DELETE("/:id", middleware.RequireRole(3), controllers.DeletePublicationRewardRate)             // DELETE /api/v1/publication-rewards/rates/:id
					rates.PATCH("/:id/toggle", middleware.RequireRole(3), controllers.TogglePublicationRewardRateStatus) // PATCH /api/v1/publication-rewards/rates/:id/toggle
					rates.POST("/:id/toggle", middleware.RequireRole(3), controllers.TogglePublicationRewardRateStatus)  // PATCH /api/v1/publication-rewards/rates/:id/toggle
				}
			}

			rewardConfig := v1.Group("/reward-config")
			rewardConfig.Use(middleware.AuthMiddleware())
			{
				// Public endpoints (สำหรับ teacher และ staff)
				rewardConfig.GET("", controllers.GetRewardConfig)              // GET /api/v1/reward-config
				rewardConfig.GET("/lookup", controllers.GetRewardConfigLookup) // GET /api/v1/reward-config/lookup
			}

			// Users endpoint for form dropdown
			protected.GET("/users", controllers.GetUsers)

			// Document types with category filter
			protected.GET("/document-types", controllers.GetDocumentTypes)

			// เพิ่มส่วนนี้ใน admin group หลังจาก middleware.RequireRole(3)
			admin := protected.Group("/admin")
			admin.Use(middleware.RequireRole(3)) // Require admin role
			{
				// Dashboard
				admin.GET("/dashboard/stats", controllers.GetDashboardStats)
				admin.GET("/submissions", controllers.GetAdminSubmissions) // Admin ดู submissions ทั้งหมด

				// ========== YEAR MANAGEMENT ==========
				years := admin.Group("/years")
				{
					years.GET("", controllers.GetAllYears)                   // GET /api/v1/admin/years
					years.POST("", controllers.CreateYear)                   // POST /api/v1/admin/years
					years.PUT("/:id", controllers.UpdateYear)                // PUT /api/v1/admin/years/:id
					years.DELETE("/:id", controllers.DeleteYear)             // DELETE /api/v1/admin/years/:id
					years.PATCH("/:id/toggle", controllers.ToggleYearStatus) // PATCH /api/v1/admin/years/:id/toggle
					years.GET("/:id/stats", controllers.GetYearStats)        // GET /api/v1/admin/years/:id/stats
				}

				// ========== FUND CATEGORIES MANAGEMENT ==========
				categories := admin.Group("/categories")
				{
					categories.GET("", controllers.GetAllCategories)                  // GET /api/v1/admin/categories
					categories.POST("", controllers.CreateCategory)                   // POST /api/v1/admin/categories
					categories.PUT("/:id", controllers.UpdateCategory)                // PUT /api/v1/admin/categories/:id
					categories.DELETE("/:id", controllers.DeleteCategory)             // DELETE /api/v1/admin/categories/:id
					categories.PATCH("/:id/toggle", controllers.ToggleCategoryStatus) // PATCH /api/v1/admin/categories/:id/toggle
				}

				// ========== FUND SUBCATEGORIES MANAGEMENT ==========
				subcategories := admin.Group("/subcategories")
				{
					subcategories.GET("", controllers.GetAllSubcategories)                  // GET /api/v1/admin/subcategories
					subcategories.POST("", controllers.CreateSubcategory)                   // POST /api/v1/admin/subcategories
					subcategories.PUT("/:id", controllers.UpdateSubcategory)                // PUT /api/v1/admin/subcategories/:id
					subcategories.DELETE("/:id", controllers.DeleteSubcategory)             // DELETE /api/v1/admin/subcategories/:id
					subcategories.PATCH("/:id/toggle", controllers.ToggleSubcategoryStatus) // PATCH /api/v1/admin/subcategories/:id/toggle

					// Target roles management (existing functionality)
					subcategories.PUT("/:id/roles", controllers.UpdateSubcategoryTargetRoles) // PUT /api/v1/admin/subcategories/:id/roles
					subcategories.POST("/bulk-roles", controllers.BulkUpdateSubcategoryRoles) // POST /api/v1/admin/subcategories/bulk-roles
				}

				// ========== SUBCATEGORY BUDGETS MANAGEMENT ==========
				budgets := admin.Group("/budgets")
				{
					budgets.GET("", controllers.GetAllSubcategoryBudgets)                   // GET /api/v1/admin/budgets
					budgets.GET("/:id", controllers.GetSubcategoryBudget)                   // GET /api/v1/admin/budgets/:id
					budgets.POST("", controllers.CreateSubcategoryBudget)                   // POST /api/v1/admin/budgets
					budgets.PUT("/:id", controllers.UpdateSubcategoryBudget)                // PUT /api/v1/admin/budgets/:id
					budgets.DELETE("/:id", controllers.DeleteSubcategoryBudget)             // DELETE /api/v1/admin/budgets/:id
					budgets.PATCH("/:id/toggle", controllers.ToggleSubcategoryBudgetStatus) // PATCH /api/v1/admin/budgets/:id/toggle
				}

				// ========== STATISTICS AND REPORTING ==========
				reports := admin.Group("/reports")
				{
					reports.GET("/categories", controllers.GetCategoryStats) // GET /api/v1/admin/reports/categories
				}

				// ========== APPLICATION MANAGEMENT (existing) ==========
				applications := admin.Group("/applications")
				{
					applications.GET("", controllers.GetApplications)                 // GET /api/v1/admin/applications
					applications.GET("/:id", controllers.GetApplication)              // GET /api/v1/admin/applications/:id
					applications.POST("/:id/approve", controllers.ApproveApplication) // POST /api/v1/admin/applications/:id/approve
					applications.POST("/:id/reject", controllers.RejectApplication)   // POST /api/v1/admin/applications/:id/reject
				}

				rewardConfigAdmin := admin.Group("/reward-config")
				{
					rewardConfigAdmin.GET("", controllers.GetRewardConfigAdmin) // GET /api/v1/admin/reward-config (ดูทั้งหมด ไม่ filter is_active)

					rewardConfigAdmin.POST("", controllers.CreateRewardConfig)                   // POST /api/v1/admin/reward-config
					rewardConfigAdmin.PUT("/:id", controllers.UpdateRewardConfig)                // PUT /api/v1/admin/reward-config/:id
					rewardConfigAdmin.DELETE("/:id", controllers.DeleteRewardConfig)             // DELETE /api/v1/admin/reward-config/:id
					rewardConfigAdmin.PATCH("/:id/toggle", controllers.ToggleRewardConfigStatus) // PATCH /api/v1/admin/reward-config/:id/toggle
					rewardConfigAdmin.POST("/:id/toggle", controllers.ToggleRewardConfigStatus)  // alias
				}

				// ========== USER MANAGEMENT (if needed in future) ==========
				// users := admin.Group("/users")
				// {
				//     users.GET("", controllers.GetAllUsers)
				//     users.PUT("/:id/role", controllers.UpdateUserRole)
				// }

				// User folders management
				admin.GET("/files/users", controllers.ListUserFolders)   // ดู user folders ทั้งหมด
				admin.GET("/files/users/:id", controllers.ListUserFiles) // ดูไฟล์ของ user
				admin.GET("/files/stats", controllers.GetFileStats)      // สถิติการใช้งานไฟล์

				// File cleanup and maintenance
				admin.DELETE("/files/cleanup", controllers.CleanupTempFiles) // ลบไฟล์ temp เก่า
				admin.POST("/files/backup/:id", controllers.BackupUserData)  // backup ข้อมูล user

				// File system utilities (เพิ่มเติมในอนาคต)
				// admin.GET("/files/orphaned", controllers.FindOrphanedFiles)     // หาไฟล์ที่ไม่มีใน DB
				// admin.DELETE("/files/orphaned", controllers.DeleteOrphanedFiles) // ลบไฟล์ที่ไม่มีใน DB
			}
		}
	}

	// Catch-all route for 404
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Endpoint not found",
			"path":    c.Request.URL.Path,
			"method":  c.Request.Method,
		})
	})
}

func RegisterLogRoute(r *gin.Engine) {
	r.GET("/logs", func(c *gin.Context) {
		// Set your access token here
		const accessToken = "secret-token"

		// Validate query token
		if c.Query("token") != accessToken {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// Path to your log file
		logData, err := os.ReadFile("fund-api.log")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to read log"})
			return
		}

		// Return log content
		c.Data(http.StatusOK, "text/plain; charset=utf-8", logData)
	})
}

func RegisterUploadRoutes(rg *gin.RouterGroup) {
	rg.POST("/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No file found"})
			return
		}

		dst := fmt.Sprintf("./uploads/%s", file.Filename)
		if err := c.SaveUploadedFile(file, dst); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "File uploaded successfully",
			"url":     "/uploads/" + file.Filename,
		})
	})
}

func RegisterFileRoutes(rg *gin.RouterGroup) {
	// List files and folders in a directory (supports nested paths)
	rg.GET("/files", func(c *gin.Context) {
		// Get path parameter (empty string means root directory)
		requestPath := c.Query("path")

		// Sanitize path to prevent directory traversal
		if strings.Contains(requestPath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// Build full path
		fullPath := "./uploads"
		if requestPath != "" {
			fullPath = filepath.Join("./uploads", requestPath)
		}

		// Check if directory exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Directory not found"})
			return
		}

		// Read directory contents
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to read directory"})
			return
		}

		var fileList []gin.H
		var folderList []gin.H

		for _, entry := range entries {
			entryPath := filepath.Join(fullPath, entry.Name())
			info, err := os.Stat(entryPath)
			if err != nil {
				continue // Skip problematic entries
			}

			if entry.IsDir() {
				// It's a folder
				folderList = append(folderList, gin.H{
					"name": entry.Name(),
					"type": "folder",
				})
			} else {
				// It's a file
				var fileURL string
				if requestPath != "" {
					fileURL = "/uploads/" + requestPath + "/" + entry.Name()
				} else {
					fileURL = "/uploads/" + entry.Name()
				}

				fileList = append(fileList, gin.H{
					"name": entry.Name(),
					"url":  fileURL,
					"size": info.Size(),
					"type": "file",
				})
			}
		}

		// Sort folders and files alphabetically
		sort.Slice(folderList, func(i, j int) bool {
			return folderList[i]["name"].(string) < folderList[j]["name"].(string)
		})
		sort.Slice(fileList, func(i, j int) bool {
			return fileList[i]["name"].(string) < fileList[j]["name"].(string)
		})

		c.JSON(http.StatusOK, gin.H{
			"folders": folderList,
			"files":   fileList,
		})
	})

	// Create a new folder
	rg.POST("/folders", func(c *gin.Context) {
		var request struct {
			Path string `json:"path" binding:"required"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return
		}

		// Sanitize path
		if strings.Contains(request.Path, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// Build full path
		fullPath := filepath.Join("./uploads", request.Path)

		// Check if folder already exists
		if _, err := os.Stat(fullPath); err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Folder already exists"})
			return
		}

		// Create folder (with parent directories if needed)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to create folder"})
			return
		}

		log.Printf("✅ Created folder: %s", request.Path)
		c.JSON(http.StatusOK, gin.H{"message": "Folder created successfully"})
	})

	// Delete a folder
	rg.DELETE("/folders/:path", func(c *gin.Context) {
		rawPath := c.Param("path")

		// Decode URL
		folderPath, err := url.QueryUnescape(rawPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid folder path"})
			return
		}

		// Sanitize path
		if strings.Contains(folderPath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		fullPath := filepath.Join("./uploads", folderPath)

		// Check if folder exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Folder not found"})
			return
		}

		// Remove folder and all its contents
		if err := os.RemoveAll(fullPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to delete folder"})
			return
		}

		log.Printf("✅ Deleted folder: %s", folderPath)
		c.JSON(http.StatusOK, gin.H{"message": "Folder deleted successfully"})
	})

	// Delete a file (enhanced to support nested paths)
	rg.DELETE("/files/:path", func(c *gin.Context) {
		rawPath := c.Param("path")

		// Decode URL
		filePath, err := url.QueryUnescape(rawPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file path"})
			return
		}

		// Sanitize path
		if strings.Contains(filePath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// Prevent deletion of HTML files
		if strings.HasSuffix(filePath, ".html") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete HTML files"})
			return
		}

		fullPath := filepath.Join("./uploads", filePath)

		// Check if file exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
			return
		}

		if err := os.Remove(fullPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to delete file"})
			return
		}

		log.Printf("✅ Deleted file: %s", filePath)
		c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
	})
}

// You'll also need to update your upload handler to support paths
// Add this to your upload route handler:
func HandleFileUpload(c *gin.Context) {
	// Get the uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	// Get the optional path parameter
	uploadPath := c.PostForm("path")

	// Sanitize path
	if strings.Contains(uploadPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
		return
	}

	// Build destination directory
	destDir := "./uploads"
	if uploadPath != "" {
		destDir = filepath.Join("./uploads", uploadPath)

		// Create directory if it doesn't exist
		if err := os.MkdirAll(destDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to create directory"})
			return
		}
	}

	// Build full file path
	filePath := filepath.Join(destDir, file.Filename)

	// Save the file
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to save file"})
		return
	}

	log.Printf("✅ Uploaded file: %s to %s", file.Filename, uploadPath)
	c.JSON(http.StatusOK, gin.H{"message": "File uploaded successfully"})
}
