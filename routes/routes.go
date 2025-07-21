package routes

import (
	"fmt"
	"fund-management-api/controllers"
	"fund-management-api/middleware"
	"log"
	"net/http"
	"net/url"
	"os"
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

			// Teacher-specific endpoints
			teacher := protected.Group("/teacher")
			{
				// ไม่ต้องใส่ RequireRole(1) เพราะ GetSubcategoryForRole จะ check role เอง
				teacher.GET("/subcategories", controllers.GetSubcategoryForRole)
			}

			// Staff-specific endpoints
			staff := protected.Group("/staff")
			{
				// ใช้ function เดียวกัน
				staff.GET("/subcategories", controllers.GetSubcategoryForRole)
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

				// Reward rates configuration (admin only)
				publications.GET("/rates", controllers.GetPublicationRewardRates)
				publications.PUT("/rates", middleware.RequireRole(3), controllers.UpdatePublicationRewardRates)
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

				// ========== USER MANAGEMENT (if needed in future) ==========
				// users := admin.Group("/users")
				// {
				//     users.GET("", controllers.GetAllUsers)
				//     users.PUT("/:id/role", controllers.UpdateUserRole)
				// }
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
	// List uploaded files
	rg.GET("/files", func(c *gin.Context) {
		files, err := os.ReadDir("./uploads")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to read uploads folder"})
			return
		}

		var fileList []gin.H
		for _, f := range files {
			if !f.IsDir() {
				info, err := os.Stat("./uploads/" + f.Name())
				if err != nil {
					continue // ข้ามไฟล์ที่มีปัญหา
				}

				fileList = append(fileList, gin.H{
					"name": f.Name(),
					"url":  "/uploads/" + f.Name(),
					"size": info.Size(), // size เป็น byte
				})
			}
		}
		// จัดเรียงชื่อไฟล์ A-Z
		sort.Slice(fileList, func(i, j int) bool {
			return fileList[i]["name"].(string) < fileList[j]["name"].(string)
		})

		c.JSON(http.StatusOK, gin.H{
			"files": fileList,
		})
	})

	// (Optional) Delete file
	rg.DELETE("/files/:name", func(c *gin.Context) {
		rawName := c.Param("name")

		// Decode URL เช่น %20 เป็นช่องว่าง
		filename, err := url.QueryUnescape(rawName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file name"})
			return
		}

		// ป้องกันลบไฟล์ HTML
		if strings.HasSuffix(filename, ".html") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete HTML files"})
			return
		}

		path := "./uploads/" + filename
		if err := os.Remove(path); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to delete file"})
			return
		}

		log.Printf("✅ Deleted file: %s", filename) // << Log file deletion
		c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
	})
}
