package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GetDashboardStats returns dashboard statistics
func GetDashboardStats(c *gin.Context) {
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	stats := make(map[string]interface{})

	// Common stats for all users
	stats["current_date"] = time.Now().Format("2006-01-02")

	if roleID.(int) == 3 { // Admin dashboard
		stats = getAdminDashboard()
	} else { // Teacher/Staff dashboard
		stats = getUserDashboard(userID.(int))
	}

	c.JSON(http.StatusOK, gin.H{
		"stats": stats,
	})
}

// getUserDashboard returns dashboard for regular users
func getUserDashboard(userID int) map[string]interface{} {
	stats := make(map[string]interface{})

	// My applications summary
	var applicationStats struct {
		Total          int64   `json:"total"`
		Pending        int64   `json:"pending"`
		Approved       int64   `json:"approved"`
		Rejected       int64   `json:"rejected"`
		TotalAmount    float64 `json:"total_requested"`
		ApprovedAmount float64 `json:"total_approved"`
	}

	// Total applications
	config.DB.Table("fund_applications").
		Where("user_id = ? AND delete_at IS NULL", userID).
		Count(&applicationStats.Total)

	// By status
	config.DB.Table("fund_applications").
		Where("user_id = ? AND application_status_id = 1 AND delete_at IS NULL", userID).
		Count(&applicationStats.Pending)

	config.DB.Table("fund_applications").
		Where("user_id = ? AND application_status_id = 2 AND delete_at IS NULL", userID).
		Count(&applicationStats.Approved)

	config.DB.Table("fund_applications").
		Where("user_id = ? AND application_status_id = 3 AND delete_at IS NULL", userID).
		Count(&applicationStats.Rejected)

	// Total amounts
	config.DB.Table("fund_applications").
		Where("user_id = ? AND delete_at IS NULL", userID).
		Select("COALESCE(SUM(requested_amount), 0)").
		Scan(&applicationStats.TotalAmount)

	config.DB.Table("fund_applications").
		Where("user_id = ? AND application_status_id = 2 AND delete_at IS NULL", userID).
		Select("COALESCE(SUM(approved_amount), 0)").
		Scan(&applicationStats.ApprovedAmount)

	stats["my_applications"] = applicationStats

	// Recent applications
	var recentApplications []map[string]interface{}
	config.DB.Table("fund_applications").
		Select(`application_id, application_number, project_title, 
			requested_amount, application_status_id, submitted_at,
			(SELECT status_name FROM application_status WHERE application_status_id = fund_applications.application_status_id) as status_name`).
		Where("user_id = ? AND delete_at IS NULL", userID).
		Order("submitted_at DESC").
		Limit(5).
		Scan(&recentApplications)

	stats["recent_applications"] = recentApplications

	// Monthly statistics (last 6 months)
	monthlyStats := getMonthlyStats(userID, 6)
	stats["monthly_stats"] = monthlyStats

	// Budget usage for current year
	var budgetUsage struct {
		YearBudget      float64 `json:"year_budget"`
		UsedBudget      float64 `json:"used_budget"`
		RemainingBudget float64 `json:"remaining_budget"`
	}

	currentYear := time.Now().Format("2006")
	config.DB.Table("fund_applications fa").
		Joins("JOIN years y ON fa.year_id = y.year_id").
		Where("fa.user_id = ? AND y.year = ? AND fa.application_status_id = 2", userID, currentYear).
		Select("COALESCE(SUM(fa.approved_amount), 0)").
		Scan(&budgetUsage.UsedBudget)

	stats["budget_usage"] = budgetUsage

	return stats
}

// getAdminDashboard returns dashboard for admin users
func getAdminDashboard() map[string]interface{} {
	stats := make(map[string]interface{})

	// Overall statistics
	var overallStats struct {
		TotalApplications int64   `json:"total_applications"`
		TotalUsers        int64   `json:"total_users"`
		TotalBudget       float64 `json:"total_budget"`
		UsedBudget        float64 `json:"used_budget"`
		PendingCount      int64   `json:"pending_count"`
	}

	config.DB.Table("fund_applications").Where("delete_at IS NULL").Count(&overallStats.TotalApplications)
	config.DB.Table("users").Where("delete_at IS NULL").Count(&overallStats.TotalUsers)
	config.DB.Table("fund_applications").
		Where("application_status_id = 1 AND delete_at IS NULL").
		Count(&overallStats.PendingCount)

	// Current year budget
	currentYear := time.Now().Format("2006")
	config.DB.Table("years").
		Where("year = ?", currentYear).
		Select("COALESCE(budget, 0)").
		Scan(&overallStats.TotalBudget)

	config.DB.Table("fund_applications fa").
		Joins("JOIN years y ON fa.year_id = y.year_id").
		Where("y.year = ? AND fa.application_status_id = 2", currentYear).
		Select("COALESCE(SUM(fa.approved_amount), 0)").
		Scan(&overallStats.UsedBudget)

	stats["overview"] = overallStats

	// Budget by category
	var categoryBudgets []map[string]interface{}
	config.DB.Table("fund_categories fc").
		Select(`fc.category_name, 
			COUNT(DISTINCT fa.application_id) as total_applications,
			COALESCE(SUM(CASE WHEN fa.application_status_id = 2 THEN fa.approved_amount ELSE 0 END), 0) as approved_amount,
			COALESCE(SUM(sb.allocated_amount), 0) as allocated_budget`).
		Joins("LEFT JOIN fund_subcategorie fs ON fc.category_id = fs.category_id").
		Joins("LEFT JOIN subcategorie_budgets sb ON fs.subcategorie_budget_id = sb.subcategorie_budget_id").
		Joins("LEFT JOIN fund_applications fa ON fs.subcategorie_id = fa.subcategory_id AND fa.delete_at IS NULL").
		Where("fc.delete_at IS NULL").
		Group("fc.category_id, fc.category_name").
		Scan(&categoryBudgets)

	stats["category_budgets"] = categoryBudgets

	// Recent pending applications
	var pendingApplications []map[string]interface{}
	config.DB.Table("fund_applications fa").
		Select(`fa.application_id, fa.application_number, fa.project_title,
			fa.requested_amount, fa.submitted_at,
			CONCAT(u.user_fname, ' ', u.user_lname) as applicant_name,
			fc.category_name`).
		Joins("JOIN users u ON fa.user_id = u.user_id").
		Joins("JOIN fund_subcategorie fs ON fa.subcategory_id = fs.subcategorie_id").
		Joins("JOIN fund_categories fc ON fs.category_id = fc.category_id").
		Where("fa.application_status_id = 1 AND fa.delete_at IS NULL").
		Order("fa.submitted_at DESC").
		Limit(10).
		Scan(&pendingApplications)

	stats["pending_applications"] = pendingApplications

	// Monthly trends
	monthlyTrends := getSystemMonthlyTrends(12)
	stats["monthly_trends"] = monthlyTrends

	return stats
}

// getMonthlyStats returns monthly statistics for a user
func getMonthlyStats(userID int, months int) []map[string]interface{} {
	var monthlyData []map[string]interface{}

	for i := months - 1; i >= 0; i-- {
		monthStart := time.Now().AddDate(0, -i, 0).Format("2006-01")
		monthEnd := time.Now().AddDate(0, -i+1, 0).Format("2006-01")

		var stats map[string]interface{}
		config.DB.Table("fund_applications").
			Select(`COUNT(*) as applications,
				COUNT(CASE WHEN application_status_id = 2 THEN 1 END) as approved,
				COUNT(CASE WHEN application_status_id = 3 THEN 1 END) as rejected,
				COALESCE(SUM(CASE WHEN application_status_id = 2 THEN approved_amount ELSE 0 END), 0) as approved_amount`).
			Where("user_id = ? AND submitted_at >= ? AND submitted_at < ? AND delete_at IS NULL",
				userID, monthStart+"-01", monthEnd+"-01").
			Scan(&stats)

		stats["month"] = monthStart
		monthlyData = append(monthlyData, stats)
	}

	return monthlyData
}

// getSystemMonthlyTrends returns system-wide monthly trends
func getSystemMonthlyTrends(months int) []map[string]interface{} {
	var trends []map[string]interface{}

	for i := months - 1; i >= 0; i-- {
		monthStart := time.Now().AddDate(0, -i, 0).Format("2006-01")
		monthEnd := time.Now().AddDate(0, -i+1, 0).Format("2006-01")

		var trend map[string]interface{}
		config.DB.Table("fund_applications").
			Select(`COUNT(*) as total_applications,
				COUNT(CASE WHEN application_status_id = 2 THEN 1 END) as approved,
				COUNT(CASE WHEN application_status_id = 3 THEN 1 END) as rejected,
				COALESCE(SUM(requested_amount), 0) as total_requested,
				COALESCE(SUM(CASE WHEN application_status_id = 2 THEN approved_amount ELSE 0 END), 0) as total_approved`).
			Where("submitted_at >= ? AND submitted_at < ? AND delete_at IS NULL",
				monthStart+"-01", monthEnd+"-01").
			Scan(&trend)

		trend["month"] = monthStart
		trends = append(trends, trend)
	}

	return trends
}

// GetBudgetSummary returns budget summary using the view
func GetBudgetSummary(c *gin.Context) {
	yearID := c.Query("year_id")

	var budgetSummary []map[string]interface{}
	query := config.DB.Table("view_budget_summary")

	if yearID != "" {
		// Need to join with years table to filter by year_id
		query = config.DB.Table("view_budget_summary vbs").
			Joins("JOIN years y ON vbs.year = y.year").
			Where("y.year_id = ?", yearID)
	}

	if err := query.Scan(&budgetSummary).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch budget summary"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"budget_summary": budgetSummary,
	})
}

// GetApplicationsSummary returns applications summary using the view
func GetApplicationsSummary(c *gin.Context) {
	// Get query parameters
	status := c.Query("status")
	year := c.Query("year")
	userID := c.Query("user_id")

	// For non-admin users, force filter by their user_id
	currentUserID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var applicationsSummary []map[string]interface{}
	query := config.DB.Table("view_fund_applications_summary")

	// Apply filters
	if roleID.(int) != 3 { // Not admin
		// TODO: This is a temporary solution. The view should include user_id column
		// For now, we'll filter by exact match on email or use a different approach
		// Option 1: Get user info and filter by name (not ideal)
		var user models.User
		config.DB.First(&user, currentUserID)
		userName := user.UserFname + " " + user.UserLname
		query = query.Where("applicant_name = ?", userName)

		// Option 2: Better solution - join with original table
		// query = query.Joins("JOIN fund_applications fa ON fa.application_number = view_fund_applications_summary.application_number").
		//              Where("fa.user_id = ?", currentUserID)
	}

	if status != "" {
		query = query.Where("status_name = ?", status)
	}

	if year != "" {
		query = query.Where("year = ?", year)
	}

	if userID != "" && roleID.(int) == 3 { // Admin can filter by user
		// Need to modify view to include user_id for proper filtering
	}

	if err := query.Order("submitted_at DESC").Scan(&applicationsSummary).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch applications summary"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"applications_summary": applicationsSummary,
		"total":                len(applicationsSummary),
	})
}
