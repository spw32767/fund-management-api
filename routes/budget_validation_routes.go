// routes/budget_validation_routes.go
package routes

import (
	"fund-management-api/controllers"
	"fund-management-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetupBudgetValidationRoutes mounts subcategory budget validation endpoints
func SetupBudgetValidationRoutes(router *gin.Engine) {
	v1 := router.Group("/api/v1")
	budgetRoutes := v1.Group("/subcategory-budgets")
	budgetRoutes.Use(middleware.AuthMiddleware())
	{
		budgetRoutes.GET("/validate", controllers.ValidateSubcategoryBudgets)
		budgetRoutes.GET("/available-quartiles", controllers.GetAvailableQuartiles)
	}
}
