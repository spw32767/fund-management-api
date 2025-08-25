// routes/budget_validation_routes.go
package routes

import (
	"fund-management-api/controllers"

	"github.com/gin-gonic/gin"
)

// SetupBudgetValidationRoutes mounts subcategory budget validation endpoints
// under an already-authenticated router group (e.g., /api/v1 with AuthMiddleware).
func SetupBudgetValidationRoutes(rg *gin.RouterGroup) {
	r := rg.Group("/subcategory-budgets")
	{
		// ตรวจสอบ budget availability สำหรับ subcategory
		r.GET("/validate", controllers.ValidateSubcategoryBudgets)
		// ดึงรายการ quartiles ที่มี budget พร้อมใช้งาน
		r.GET("/available-quartiles", controllers.GetAvailableQuartiles)
	}
}
