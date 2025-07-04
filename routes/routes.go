package routes

import (
	"fund-management-api/controllers"
	"fund-management-api/middleware"
	"net/http"

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
			// Authentication
			public.POST("/login", controllers.Login)

			// Health check
			public.GET("/health", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"status":  "ok",
					"message": "Fund Management API is running",
					"timestamp": gin.H{
						"server": "2025-07-02T10:00:00Z",
					},
					"version": "1.0.0",
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
							"refresh":         "POST /api/v1/refresh-token",
							"profile":         "GET /api/v1/profile",
							"change_password": "PUT /api/v1/change-password",
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
			protected.POST("/refresh-token", controllers.RefreshToken)

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
