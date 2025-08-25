// routes/budget_validation_routes.go
package routes

import (
	"fund-management-api/controllers"
	"fund-management-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetupBudgetValidationRoutes(router *gin.Engine) {
	// Budget validation routes - ต้อง login
	budgetRoutes := router.Group("/api/subcategory-budgets")
	budgetRoutes.Use(middleware.AuthMiddleware())
	{
		// ตรวจสอบ budget availability สำหรับ subcategory
		budgetRoutes.GET("/validate", controllers.ValidateSubcategoryBudgets)

		// ดึงรายการ quartiles ที่มี budget พร้อมใช้งาน
		budgetRoutes.GET("/available-quartiles", controllers.GetAvailableQuartiles)
	}
}
