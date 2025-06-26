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
		}
	}
}
