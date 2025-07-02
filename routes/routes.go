package routes

import (
	"fund-management-api/controllers"
	"fund-management-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine) {
	// API v1 group
	v1 := router.Group("/api/v1")
	{
		// Public routes
		public := v1.Group("")
		{
			// Authentication
			public.POST("/login", controllers.Login)
			public.POST("/refresh", controllers.RefreshToken)

			// Health check
			public.GET("/health", func(c *gin.Context) {
				c.JSON(200, gin.H{
					"status":  "ok",
					"message": "Fund Management API is running",
				})
			})
		}

		// Protected routes (require authentication)
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			// Auth management
			protected.POST("/logout", controllers.Logout)
			protected.POST("/logout-all", controllers.LogoutAllDevices)
			protected.GET("/sessions", controllers.GetActiveSessions)
			protected.DELETE("/sessions/:session_id", controllers.RevokeSession)

			// User profile
			protected.GET("/profile", controllers.GetProfile)
			protected.PUT("/change-password", controllers.ChangePassword)

			// Common endpoints (all authenticated users)
			protected.GET("/years", controllers.GetYears)
			protected.GET("/categories", controllers.GetCategories)
			protected.GET("/subcategories", controllers.GetSubcategories)

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
				// All authenticated users can view their publication rewards
				publications.GET("", controllers.GetPublicationRewards)
				publications.GET("/:id", controllers.GetPublicationReward)
				publications.GET("/rates", controllers.GetPublicationRewardRates)

				// Only teachers can create/update/delete
				publications.POST("", middleware.RequireRole(1), controllers.CreatePublicationReward)
				publications.PUT("/:id", middleware.RequireRole(1), controllers.UpdatePublicationReward)
				publications.DELETE("/:id", middleware.RequireRole(1), controllers.DeletePublicationReward)

				// Only admin can approve
				publications.POST("/:id/approve", middleware.RequireRole(3), controllers.ApprovePublicationReward)

				// Document upload for publication rewards
				publications.POST("/:id/documents", controllers.UploadPublicationDocument)
				publications.GET("/:id/documents", controllers.GetPublicationDocuments)
			}
		}
	}
}
