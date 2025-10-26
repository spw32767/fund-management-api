package controllers

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"

	"github.com/gin-gonic/gin"
)

// GetDashboardStats returns dashboard statistics
func GetDashboardStats(c *gin.Context) {
	userIDVal, userExists := c.Get("userID")
	roleIDVal, roleExists := c.Get("roleID")
	if !userExists || !roleExists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication context missing",
		})
		return
	}

	userID, okUser := userIDVal.(int)
	roleID, okRole := roleIDVal.(int)
	if !okUser || !okRole {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "invalid user or role id",
		})
		return
	}

	var stats map[string]interface{}
	if roleID == 3 || roleID == 4 { // Admin dashboard (includes dept heads)
		stats = getAdminDashboard()
	} else { // Teacher/Staff dashboard
		stats = getUserDashboard(userID)
	}

	if stats == nil {
		stats = make(map[string]interface{})
	}

	t := time.Now().AddDate(543, 0, 0) // convert to Buddhist year to match database
	stats["current_date"] = t.Format("2006-01-02")

	c.JSON(http.StatusOK, gin.H{
		"stats": stats,
	})
}

// getUserDashboard returns dashboard for regular users
func getUserDashboard(userID int) map[string]interface{} {
	stats := make(map[string]interface{})

	// My submissions summary (fund applications + publication rewards)
	var submissionStats struct {
		Total          int64   `json:"total"`
		Pending        int64   `json:"pending"`
		Approved       int64   `json:"approved"`
		Rejected       int64   `json:"rejected"`
		TotalAmount    float64 `json:"total_requested"`
		ApprovedAmount float64 `json:"total_approved"`
	}

	pendingStatusIDs := make([]int, 0, 2)
	if pendingID, err := utils.GetStatusIDByCode(utils.StatusCodePending); err == nil && pendingID > 0 {
		pendingStatusIDs = append(pendingStatusIDs, pendingID)
	}
	if deptPendingID, err := utils.GetStatusIDByCode(utils.StatusCodeDeptHeadPending); err == nil && deptPendingID > 0 {
		pendingStatusIDs = append(pendingStatusIDs, deptPendingID)
	}
	if len(pendingStatusIDs) == 0 {
		pendingStatusIDs = []int{-1}
	}

	approvedStatusID, err := utils.GetStatusIDByCode(utils.StatusCodeApproved)
	if err != nil {
		approvedStatusID = -1
	}

	rejectedStatusIDs := make([]int, 0, 2)
	if rejectedID, err := utils.GetStatusIDByCode(utils.StatusCodeRejected); err == nil && rejectedID > 0 {
		rejectedStatusIDs = append(rejectedStatusIDs, rejectedID)
	}
	if deptRejectedID, err := utils.GetStatusIDByCode(utils.StatusCodeDeptHeadNotRecommended); err == nil && deptRejectedID > 0 {
		rejectedStatusIDs = append(rejectedStatusIDs, deptRejectedID)
	}
	if len(rejectedStatusIDs) == 0 {
		rejectedStatusIDs = []int{-1}
	}

	// Total submissions
	config.DB.Table("submissions").
		Where("user_id = ? AND submission_type IN ? AND deleted_at IS NULL",
			userID, []string{"fund_application", "publication_reward"}).
		Count(&submissionStats.Total)

	// By status
	config.DB.Table("submissions").
		Where("user_id = ? AND submission_type IN ? AND status_id IN ? AND deleted_at IS NULL",
			userID, []string{"fund_application", "publication_reward"}, pendingStatusIDs).
		Count(&submissionStats.Pending)

	if approvedStatusID > 0 {
		config.DB.Table("submissions").
			Where("user_id = ? AND submission_type IN ? AND status_id = ? AND deleted_at IS NULL",
				userID, []string{"fund_application", "publication_reward"}, approvedStatusID).
			Count(&submissionStats.Approved)
	}

	config.DB.Table("submissions").
		Where("user_id = ? AND submission_type IN ? AND status_id IN ? AND deleted_at IS NULL",
			userID, []string{"fund_application", "publication_reward"}, rejectedStatusIDs).
		Count(&submissionStats.Rejected)

	// Total requested and approved amounts
	var fundAmounts struct {
		Requested float64
		Approved  float64
	}
	config.DB.Table("fund_application_details fad").
		Joins("JOIN submissions s ON fad.submission_id = s.submission_id").
		Where("s.user_id = ? AND s.deleted_at IS NULL", userID).
		Select("COALESCE(SUM(fad.requested_amount),0) AS requested, COALESCE(SUM(CASE WHEN s.status_id = ? THEN fad.approved_amount ELSE 0 END),0) AS approved", approvedStatusID).
		Scan(&fundAmounts)

	var rewardAmounts struct {
		Requested float64
		Approved  float64
	}
	config.DB.Table("publication_reward_details prd").
		Joins("JOIN submissions s ON prd.submission_id = s.submission_id").
		Where("s.user_id = ? AND s.deleted_at IS NULL", userID).
		Select("COALESCE(SUM(prd.reward_amount),0) AS requested, COALESCE(SUM(CASE WHEN s.status_id = ? THEN prd.reward_approve_amount ELSE 0 END),0) AS approved", approvedStatusID).
		Scan(&rewardAmounts)

	submissionStats.TotalAmount = fundAmounts.Requested + rewardAmounts.Requested
	submissionStats.ApprovedAmount = fundAmounts.Approved + rewardAmounts.Approved

	stats["my_applications"] = submissionStats

	// Budget summary metrics
	var budgetSummary struct {
		TotalRequested  float64 `json:"total_requested"`
		TotalApproved   float64 `json:"total_approved"`
		Remaining       float64 `json:"remaining"`
		SubmissionCount int64   `json:"submission_count"`
	}

	budgetSummary.TotalRequested = submissionStats.TotalAmount
	budgetSummary.TotalApproved = submissionStats.ApprovedAmount
	budgetSummary.SubmissionCount = submissionStats.Total

	config.DB.Table("user_fund_eligibilities").
		Where("user_id = ? AND delete_at IS NULL", userID).
		Select("COALESCE(SUM(remaining_quota),0)").
		Scan(&budgetSummary.Remaining)

	stats["budget_summary"] = budgetSummary

	// Recent submissions
	var recentSubmissions []map[string]interface{}
	config.DB.Table("submissions s").
		Select(`s.submission_id, s.submission_number, s.submission_type,
                        COALESCE(fad.project_title, prd.paper_title) as title,
                        COALESCE(fad.requested_amount, prd.reward_amount) as amount,
                        s.status_id, s.submitted_at,
                        (SELECT status_name FROM application_status WHERE application_status_id = s.status_id) as status_name`).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Where("s.user_id = ? AND s.deleted_at IS NULL", userID).
		Order("s.submitted_at DESC").
		Limit(5).
		Scan(&recentSubmissions)

	stats["recent_applications"] = recentSubmissions

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
	// Approved fund application amounts
	config.DB.Table("fund_application_details fad").
		Joins("JOIN submissions s ON fad.submission_id = s.submission_id").
		Joins("JOIN years y ON s.year_id = y.year_id").
		Where("s.user_id = ? AND y.year = ? AND s.status_id = 2", userID, currentYear).
		Select("COALESCE(SUM(fad.approved_amount), 0)").
		Scan(&budgetUsage.UsedBudget)

	// Approved publication reward amounts
	var rewardUsed float64
	config.DB.Table("publication_reward_details prd").
		Joins("JOIN submissions s ON prd.submission_id = s.submission_id").
		Joins("JOIN years y ON s.year_id = y.year_id").
		Where("s.user_id = ? AND y.year = ? AND s.status_id = 2", userID, currentYear).
		Select("COALESCE(SUM(prd.reward_approve_amount), 0)").
		Scan(&rewardUsed)
	budgetUsage.UsedBudget += rewardUsed

	// Query total budget for the current year
	config.DB.Table("years").
		Where("year = ?", currentYear).
		Select("COALESCE(budget, 0)").
		Scan(&budgetUsage.YearBudget)

	// Calculate remaining budget
	budgetUsage.RemainingBudget = budgetUsage.YearBudget - budgetUsage.UsedBudget

	stats["budget_usage"] = budgetUsage

	return stats
}

// getAdminDashboard returns dashboard for admin users
func getAdminDashboard() map[string]interface{} {
	stats := make(map[string]interface{})

	pendingStatusIDs := make([]int, 0, 2)
	if pendingID, err := utils.GetStatusIDByCode(utils.StatusCodePending); err == nil && pendingID > 0 {
		pendingStatusIDs = append(pendingStatusIDs, pendingID)
	}
	if deptPendingID, err := utils.GetStatusIDByCode(utils.StatusCodeDeptHeadPending); err == nil && deptPendingID > 0 {
		pendingStatusIDs = append(pendingStatusIDs, deptPendingID)
	}
	if len(pendingStatusIDs) == 0 {
		pendingStatusIDs = []int{-1}
	}

	approvedStatusID, err := utils.GetStatusIDByCode(utils.StatusCodeApproved)
	if err != nil || approvedStatusID <= 0 {
		approvedStatusID = -1
	}

	rejectedStatusIDs := make([]int, 0, 2)
	if rejectedID, err := utils.GetStatusIDByCode(utils.StatusCodeRejected); err == nil && rejectedID > 0 {
		rejectedStatusIDs = append(rejectedStatusIDs, rejectedID)
	}
	if deptRejectedID, err := utils.GetStatusIDByCode(utils.StatusCodeDeptHeadNotRecommended); err == nil && deptRejectedID > 0 {
		rejectedStatusIDs = append(rejectedStatusIDs, deptRejectedID)
	}
	if len(rejectedStatusIDs) == 0 {
		rejectedStatusIDs = []int{-1}
	}

	// Determine current Buddhist Era year to align with budgeting tables
	currentYear := time.Now().AddDate(543, 0, 0).Format("2006")

	stats["overview"] = buildAdminOverview(currentYear, pendingStatusIDs, approvedStatusID, rejectedStatusIDs)
	stats["category_budgets"] = buildAdminCategoryBudgets(currentYear)
	stats["pending_applications"] = buildAdminPendingApplications(pendingStatusIDs)
	stats["quota_summary"] = buildAdminQuotaSummary(currentYear)

	if statusBreakdown := buildAdminStatusBreakdown(); len(statusBreakdown) > 0 {
		stats["status_breakdown"] = statusBreakdown
	}

	if financialOverview := buildAdminFinancialOverview(pendingStatusIDs, approvedStatusID, rejectedStatusIDs); len(financialOverview) > 0 {
		stats["financial_overview"] = financialOverview
	}

	if upcoming := buildAdminUpcomingInstallments(currentYear); len(upcoming) > 0 {
		stats["upcoming_periods"] = upcoming
	}

	if activities := buildAdminActivityFeed(); len(activities) > 0 {
		stats["activity_feed"] = activities
	}

	if topUsers := buildAdminTopUsers(); len(topUsers) > 0 {
		stats["top_users"] = topUsers
	}

	trendBreakdown := buildSystemTrendBreakdown(currentYear, approvedStatusID)
	if len(trendBreakdown) > 0 {
		stats["trend_breakdown"] = trendBreakdown
		if monthly, ok := trendBreakdown["monthly"]; ok {
			stats["monthly_trends"] = monthly
		}
	}

	return stats
}

// getMonthlyStats returns monthly statistics for a user
func getMonthlyStats(userID int, months int) []map[string]interface{} {
	var monthlyData []map[string]interface{}

	for i := months - 1; i >= 0; i-- {
		monthStart := time.Now().AddDate(0, -i, 0).Format("2006-01")
		monthEnd := time.Now().AddDate(0, -i+1, 0).Format("2006-01")

		stats := make(map[string]interface{})
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

func buildAdminOverview(currentYear string, pendingStatusIDs []int, approvedStatusID int, rejectedStatusIDs []int) map[string]interface{} {
	overview := make(map[string]interface{})

	// Total submissions across supported submission types
	submissionTypes := []string{"fund_application", "publication_reward"}

	var totalApplications int64
	config.DB.Table("submissions").
		Where("submission_type IN ? AND deleted_at IS NULL", submissionTypes).
		Count(&totalApplications)

	overview["total_applications"] = totalApplications

	// Breakdown by submission type
	typeCounts := make(map[string]int64)
	var typeRows []struct {
		SubmissionType string
		Total          int64
	}

	config.DB.Table("submissions").
		Select("submission_type, COUNT(*) AS total").
		Where("submission_type IN ? AND deleted_at IS NULL", submissionTypes).
		Group("submission_type").
		Scan(&typeRows)

	for _, row := range typeRows {
		typeCounts[row.SubmissionType] = row.Total
	}

	overview["fund_applications"] = typeCounts["fund_application"]
	overview["publication_rewards"] = typeCounts["publication_reward"]

	// Pending / approved / rejected counts
	var pendingCount int64
	config.DB.Table("submissions").
		Where("submission_type IN ? AND status_id IN ? AND deleted_at IS NULL", submissionTypes, pendingStatusIDs).
		Count(&pendingCount)
	overview["pending_count"] = pendingCount

	var approvedCount int64
	if approvedStatusID > 0 {
		config.DB.Table("submissions").
			Where("submission_type IN ? AND status_id = ? AND deleted_at IS NULL", submissionTypes, approvedStatusID).
			Count(&approvedCount)
	}
	overview["approved_count"] = approvedCount

	var rejectedCount int64
	config.DB.Table("submissions").
		Where("submission_type IN ? AND status_id IN ? AND deleted_at IS NULL", submissionTypes, rejectedStatusIDs).
		Count(&rejectedCount)
	overview["rejected_count"] = rejectedCount

	if totalApplications > 0 {
		overview["approval_rate"] = (float64(approvedCount) / float64(totalApplications)) * 100
	} else {
		overview["approval_rate"] = 0.0
	}

	// Total users in the system
	var totalUsers int64
	config.DB.Table("users").
		Where("delete_at IS NULL").
		Count(&totalUsers)
	overview["total_users"] = totalUsers

	// Budget totals using the consolidated budget summary view
	var budget struct {
		Total     float64
		Used      float64
		Remaining float64
	}

	config.DB.Table("view_budget_summary").
		Select("COALESCE(SUM(allocated_amount),0) AS total, COALESCE(SUM(used_amount),0) AS used, COALESCE(SUM(remaining_budget),0) AS remaining").
		Where("year = ?", currentYear).
		Scan(&budget)

	overview["total_budget"] = budget.Total
	overview["used_budget"] = budget.Used
	overview["remaining_budget"] = budget.Remaining
	overview["current_year"] = currentYear

	return overview
}

func buildAdminCategoryBudgets(currentYear string) []map[string]interface{} {
	var rows []struct {
		CategoryName         string
		TotalApplications    int64
		ApprovedApplications int64
		AllocatedAmount      float64
		UsedAmount           float64
		RemainingBudget      float64
	}

	config.DB.Table("view_budget_summary").
		Select(`category_name,
                    COALESCE(SUM(total_applications),0) AS total_applications,
                    COALESCE(SUM(approved_applications),0) AS approved_applications,
                    COALESCE(SUM(allocated_amount),0) AS allocated_amount,
                    COALESCE(SUM(used_amount),0) AS used_amount,
                    COALESCE(SUM(remaining_budget),0) AS remaining_budget`).
		Where("year = ?", currentYear).
		Group("category_name").
		Order("category_name ASC").
		Scan(&rows)

	budgets := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		budgets = append(budgets, map[string]interface{}{
			"category_name":         row.CategoryName,
			"total_applications":    row.TotalApplications,
			"approved_applications": row.ApprovedApplications,
			"approved_amount":       row.UsedAmount,
			"allocated_budget":      row.AllocatedAmount,
			"used_amount":           row.UsedAmount,
			"remaining_budget":      row.RemainingBudget,
		})
	}

	return budgets
}

func buildAdminPendingApplications(pendingStatusIDs []int) []map[string]interface{} {
	var pendingApplications []map[string]interface{}

	submissionTypes := []string{"fund_application", "publication_reward"}

	config.DB.Table("submissions s").
		Select(`s.submission_id,
                    s.submission_number,
                    s.submission_type,
                    COALESCE(fad.project_title, prd.paper_title) AS title,
                    CASE WHEN s.submission_type = 'fund_application' THEN fad.requested_amount ELSE prd.reward_amount END AS requested_amount,
                    s.submitted_at,
                    s.status_id,
                    ast.status_name,
                    CONCAT(u.user_fname, ' ', u.user_lname) AS applicant_name,
                    fc.category_name,
                    fsc.subcategory_name`).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Joins("LEFT JOIN users u ON s.user_id = u.user_id").
		Joins("LEFT JOIN fund_categories fc ON s.category_id = fc.category_id").
		Joins("LEFT JOIN fund_subcategories fsc ON s.subcategory_id = fsc.subcategory_id").
		Joins("LEFT JOIN application_status ast ON s.status_id = ast.application_status_id").
		Where("s.submission_type IN ? AND s.status_id IN ? AND s.deleted_at IS NULL", submissionTypes, pendingStatusIDs).
		Order("s.submitted_at DESC").
		Limit(10).
		Scan(&pendingApplications)

	return pendingApplications
}

func buildAdminQuotaSummary(currentYear string) []map[string]interface{} {
	var rows []struct {
		CategoryName    string
		SubcategoryName string
		AllocatedAmount float64
		RemainingBudget float64
		MaxGrants       float64
		RemainingGrant  float64
		UsedGrantsTotal float64
		UsedAmountTotal float64
	}

	config.DB.Table("subcategory_budgets sb").
		Select(`fc.category_name AS category_name,
                    fsc.subcategory_name AS subcategory_name,
                    COALESCE(sb.allocated_amount,0) AS allocated_amount,
                    COALESCE(sb.remaining_budget,0) AS remaining_budget,
                    COALESCE(sb.max_grants,0) AS max_grants,
                    COALESCE(sb.remaining_grant,0) AS remaining_grant,
                    COALESCE(SUM(usage.used_grants_total),0) AS used_grants_total,
                    COALESCE(SUM(usage.used_amount_total),0) AS used_amount_total`).
		Joins("JOIN fund_subcategories fsc ON sb.subcategory_id = fsc.subcategory_id").
		Joins("JOIN fund_categories fc ON fsc.category_id = fc.category_id").
		Joins("JOIN years y ON fc.year_id = y.year_id").
		Joins("LEFT JOIN v_subcategory_user_usage_total usage ON usage.subcategory_id = sb.subcategory_id AND usage.year_id = y.year_id").
		Where("sb.record_scope = 'overall' AND sb.delete_at IS NULL").
		Where("y.year = ?", currentYear).
		Group("sb.subcategory_id, fc.category_name, fsc.subcategory_name, sb.allocated_amount, sb.remaining_budget, sb.max_grants, sb.remaining_grant").
		Order("fc.category_name ASC, fsc.subcategory_name ASC").
		Scan(&rows)

	summaries := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		usedAmount := row.UsedAmountTotal
		remainingBudget := row.RemainingBudget
		if remainingBudget < 0 {
			remainingBudget = 0
		}

		summaries = append(summaries, map[string]interface{}{
			"category_name":    row.CategoryName,
			"subcategory_name": row.SubcategoryName,
			"allocated_amount": row.AllocatedAmount,
			"used_amount":      usedAmount,
			"remaining_budget": remainingBudget,
			"max_grants":       row.MaxGrants,
			"used_grants":      row.UsedGrantsTotal,
			"remaining_grants": row.RemainingGrant,
		})
	}

	return summaries
}

func buildAdminStatusBreakdown() map[string]map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}

	var rows []struct {
		SubmissionType string
		StatusCode     string
		Total          int64
	}

	config.DB.Table("submissions s").
		Select("s.submission_type, ast.status_code, COUNT(*) AS total").
		Joins("LEFT JOIN application_status ast ON s.status_id = ast.application_status_id").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL", submissionTypes).
		Group("s.submission_type, ast.status_code").
		Scan(&rows)

	stageDefinitions := []struct {
		Key   string
		Label string
	}{
		{Key: "draft", Label: "ร่างคำร้อง"},
		{Key: "dept_review", Label: "รอหัวหน้าสาขา"},
		{Key: "admin_review", Label: "รอผู้ดูแล"},
		{Key: "needs_revision", Label: "ขอข้อมูลเพิ่มเติม"},
		{Key: "approved", Label: "อนุมัติแล้ว"},
		{Key: "rejected", Label: "ไม่อนุมัติ"},
		{Key: "closed", Label: "ปิดคำร้อง"},
	}

	totals := map[string]int64{
		"overall": 0,
	}
	counts := map[string]map[string]int64{
		"overall": {},
	}

	for _, submissionType := range submissionTypes {
		totals[submissionType] = 0
		counts[submissionType] = make(map[string]int64)
	}

	for _, row := range rows {
		stage := stageKeyFromStatusCode(row.StatusCode)
		if stage == "" {
			stage = "other"
		}

		totals["overall"] += row.Total
		totals[row.SubmissionType] += row.Total

		counts["overall"][stage] += row.Total
		counts[row.SubmissionType][stage] += row.Total
	}

	result := make(map[string]map[string]interface{})
	targetKeys := append([]string{"overall"}, submissionTypes...)

	for _, target := range targetKeys {
		total := totals[target]
		stages := make([]map[string]interface{}, 0, len(stageDefinitions)+1)

		for _, def := range stageDefinitions {
			count := counts[target][def.Key]
			percentage := 0.0
			if total > 0 {
				percentage = (float64(count) / float64(total)) * 100
			}
			stages = append(stages, map[string]interface{}{
				"stage":      def.Key,
				"label":      def.Label,
				"count":      count,
				"percentage": percentage,
			})
		}

		if otherCount := counts[target]["other"]; otherCount > 0 {
			percentage := 0.0
			if total > 0 {
				percentage = (float64(otherCount) / float64(total)) * 100
			}
			stages = append(stages, map[string]interface{}{
				"stage":      "other",
				"label":      "สถานะอื่น ๆ",
				"count":      otherCount,
				"percentage": percentage,
			})
		}

		result[target] = map[string]interface{}{
			"total":  total,
			"stages": stages,
		}
	}

	return result
}

func stageKeyFromStatusCode(code string) string {
	normalized := strings.ToLower(strings.TrimSpace(code))
	switch normalized {
	case strings.ToLower(utils.StatusCodeDraft), "4", "draft":
		return "draft"
	case strings.ToLower(utils.StatusCodeDeptHeadPending), "5", "dept_head_pending", "department_pending":
		return "dept_review"
	case strings.ToLower(utils.StatusCodeNeedsMoreInfo), "3", "needs_more_info", "revision":
		return "needs_revision"
	case strings.ToLower(utils.StatusCodePending), "0", "pending":
		return "admin_review"
	case strings.ToLower(utils.StatusCodeApproved), "1", "approved":
		return "approved"
	case strings.ToLower(utils.StatusCodeRejected), "2", "rejected", "dept_head_not_recommended":
		return "rejected"
	case strings.ToLower(utils.StatusCodeAdminClosed), "6", "closed", "admin_closed":
		return "closed"
	default:
		return ""
	}
}

func buildAdminFinancialOverview(pendingStatusIDs []int, approvedStatusID int, rejectedStatusIDs []int) map[string]interface{} {
	type amountSummary struct {
		Requested float64
		Approved  float64
		Pending   float64
		Rejected  float64
	}

	var fundAmounts amountSummary
	config.DB.Table("fund_application_details fad").
		Joins("JOIN submissions s ON fad.submission_id = s.submission_id").
		Where("s.submission_type = ? AND s.deleted_at IS NULL", "fund_application").
		Select("COALESCE(SUM(fad.requested_amount),0) AS requested, COALESCE(SUM(CASE WHEN s.status_id = ? THEN fad.approved_amount ELSE 0 END),0) AS approved, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN fad.requested_amount ELSE 0 END),0) AS pending, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN fad.requested_amount ELSE 0 END),0) AS rejected", approvedStatusID, pendingStatusIDs, rejectedStatusIDs).
		Scan(&fundAmounts)

	var rewardAmounts amountSummary
	config.DB.Table("publication_reward_details prd").
		Joins("JOIN submissions s ON prd.submission_id = s.submission_id").
		Where("s.submission_type = ? AND s.deleted_at IS NULL", "publication_reward").
		Select("COALESCE(SUM(prd.reward_amount),0) AS requested, COALESCE(SUM(CASE WHEN s.status_id = ? THEN prd.reward_approve_amount ELSE 0 END),0) AS approved, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN prd.reward_amount ELSE 0 END),0) AS pending, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN prd.reward_amount ELSE 0 END),0) AS rejected", approvedStatusID, pendingStatusIDs, rejectedStatusIDs).
		Scan(&rewardAmounts)

	var fundCount, fundApprovedCount, fundPendingCount, fundRejectedCount int64
	config.DB.Table("submissions").
		Where("submission_type = ? AND deleted_at IS NULL", "fund_application").
		Count(&fundCount)

	if approvedStatusID > 0 {
		config.DB.Table("submissions").
			Where("submission_type = ? AND status_id = ? AND deleted_at IS NULL", "fund_application", approvedStatusID).
			Count(&fundApprovedCount)
	}

	config.DB.Table("submissions").
		Where("submission_type = ? AND status_id IN ? AND deleted_at IS NULL", "fund_application", pendingStatusIDs).
		Count(&fundPendingCount)

	config.DB.Table("submissions").
		Where("submission_type = ? AND status_id IN ? AND deleted_at IS NULL", "fund_application", rejectedStatusIDs).
		Count(&fundRejectedCount)

	var rewardCount, rewardApprovedCount, rewardPendingCount, rewardRejectedCount int64
	config.DB.Table("submissions").
		Where("submission_type = ? AND deleted_at IS NULL", "publication_reward").
		Count(&rewardCount)

	if approvedStatusID > 0 {
		config.DB.Table("submissions").
			Where("submission_type = ? AND status_id = ? AND deleted_at IS NULL", "publication_reward", approvedStatusID).
			Count(&rewardApprovedCount)
	}

	config.DB.Table("submissions").
		Where("submission_type = ? AND status_id IN ? AND deleted_at IS NULL", "publication_reward", pendingStatusIDs).
		Count(&rewardPendingCount)

	config.DB.Table("submissions").
		Where("submission_type = ? AND status_id IN ? AND deleted_at IS NULL", "publication_reward", rejectedStatusIDs).
		Count(&rewardRejectedCount)

	totalRequested := fundAmounts.Requested + rewardAmounts.Requested
	totalApproved := fundAmounts.Approved + rewardAmounts.Approved
	totalPending := fundAmounts.Pending + rewardAmounts.Pending
	totalRejected := fundAmounts.Rejected + rewardAmounts.Rejected

	totalCount := fundCount + rewardCount
	totalApprovedCount := fundApprovedCount + rewardApprovedCount
	totalPendingCount := fundPendingCount + rewardPendingCount
	totalRejectedCount := fundRejectedCount + rewardRejectedCount

	approvalRate := 0.0
	if totalCount > 0 {
		approvalRate = (float64(totalApprovedCount) / float64(totalCount)) * 100
	}

	fundApprovalRate := 0.0
	if fundCount > 0 {
		fundApprovalRate = (float64(fundApprovedCount) / float64(fundCount)) * 100
	}

	rewardApprovalRate := 0.0
	if rewardCount > 0 {
		rewardApprovalRate = (float64(rewardApprovedCount) / float64(rewardCount)) * 100
	}

	return map[string]interface{}{
		"total_requested": totalRequested,
		"total_approved":  totalApproved,
		"total_pending":   totalPending,
		"total_rejected":  totalRejected,
		"approval_rate":   approvalRate,
		"total_count":     totalCount,
		"approved_count":  totalApprovedCount,
		"pending_count":   totalPendingCount,
		"rejected_count":  totalRejectedCount,
		"fund_application": map[string]interface{}{
			"requested":      fundAmounts.Requested,
			"approved":       fundAmounts.Approved,
			"pending":        fundAmounts.Pending,
			"rejected":       fundAmounts.Rejected,
			"total_count":    fundCount,
			"approved_count": fundApprovedCount,
			"pending_count":  fundPendingCount,
			"rejected_count": fundRejectedCount,
			"approval_rate":  fundApprovalRate,
		},
		"publication_reward": map[string]interface{}{
			"requested":      rewardAmounts.Requested,
			"approved":       rewardAmounts.Approved,
			"pending":        rewardAmounts.Pending,
			"rejected":       rewardAmounts.Rejected,
			"total_count":    rewardCount,
			"approved_count": rewardApprovedCount,
			"pending_count":  rewardPendingCount,
			"rejected_count": rewardRejectedCount,
			"approval_rate":  rewardApprovalRate,
		},
	}
}

func buildAdminUpcomingInstallments(currentYear string) []map[string]interface{} {
	now := time.Now()

	var rows []struct {
		InstallmentNumber int
		Name              string
		CutoffDate        time.Time
		Year              string
	}

	config.DB.Table("fund_installment_periods fip").
		Select("fip.installment_number, fip.name, fip.cutoff_date, y.year").
		Joins("JOIN years y ON fip.year_id = y.year_id").
		Where("fip.deleted_at IS NULL AND fip.status = ?", "active").
		Where("y.year >= ?", currentYear).
		Order("fip.cutoff_date ASC").
		Limit(10).
		Scan(&rows)

	if len(rows) == 0 {
		config.DB.Table("fund_installment_periods fip").
			Select("fip.installment_number, fip.name, fip.cutoff_date, y.year").
			Joins("JOIN years y ON fip.year_id = y.year_id").
			Where("fip.deleted_at IS NULL AND fip.status = ?", "active").
			Order("fip.cutoff_date DESC").
			Limit(5).
			Scan(&rows)
	}

	periods := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		cutoff := row.CutoffDate
		cutoffStr := cutoff.Format("2006-01-02")
		periodLabel := strings.TrimSpace(row.Name)
		if periodLabel == "" {
			periodLabel = fmt.Sprintf("รอบที่ %d", row.InstallmentNumber)
		}

		diff := cutoff.Sub(now)
		remainingDays := int(math.Ceil(diff.Hours() / 24))
		status := "upcoming"
		if remainingDays < 0 {
			status = "overdue"
		} else if remainingDays <= 7 {
			status = "due_soon"
		}

		periods = append(periods, map[string]interface{}{
			"installment":     row.InstallmentNumber,
			"name":            periodLabel,
			"cutoff_date":     cutoffStr,
			"year":            row.Year,
			"days_remaining":  remainingDays,
			"status":          status,
			"is_overdue":      remainingDays < 0,
			"cutoff_datetime": cutoff.Format(time.RFC3339),
		})
	}

	return periods
}

func buildAdminActivityFeed() []map[string]interface{} {
	var rows []struct {
		LogID        int
		CreatedAt    time.Time
		UserName     string
		Action       string
		EntityType   string
		EntityNumber string
		Description  string
		IPAddress    string
	}

	config.DB.Table("v_recent_audit_logs").
		Select("log_id, created_at, user_name, action, entity_type, entity_number, description, ip_address").
		Order("created_at DESC").
		Limit(12).
		Scan(&rows)

	feed := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		feed = append(feed, map[string]interface{}{
			"log_id":        row.LogID,
			"created_at":    row.CreatedAt.Format(time.RFC3339),
			"user_name":     row.UserName,
			"action":        row.Action,
			"entity_type":   row.EntityType,
			"entity_number": row.EntityNumber,
			"description":   row.Description,
			"ip_address":    row.IPAddress,
		})
	}

	return feed
}

func buildAdminTopUsers() []map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}

	submissionsSubQuery := config.DB.Table("submissions").
		Select("user_id, COUNT(*) AS submission_count").
		Where("submission_type IN ? AND deleted_at IS NULL", submissionTypes).
		Group("user_id")

	var rows []struct {
		UserID          int
		UserName        string
		LoginCount      int64
		CreateCount     int64
		UpdateCount     int64
		DownloadCount   int64
		TotalActions    int64
		LastLogin       *time.Time
		SubmissionCount int64
	}

	config.DB.Table("v_user_activity_summary vus").
		Select(`vus.user_id,
            vus.user_name,
            COALESCE(vus.login_count,0) AS login_count,
            COALESCE(vus.create_count,0) AS create_count,
            COALESCE(vus.update_count,0) AS update_count,
            COALESCE(vus.download_count,0) AS download_count,
            COALESCE(vus.total_actions,0) AS total_actions,
            vus.last_login,
            COALESCE(subs.submission_count,0) AS submission_count`).
		Joins("LEFT JOIN (?) AS subs ON subs.user_id = vus.user_id", submissionsSubQuery).
		Order("total_actions DESC").
		Limit(8).
		Scan(&rows)

	summaries := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		lastLogin := ""
		if row.LastLogin != nil {
			lastLogin = row.LastLogin.Format(time.RFC3339)
		}

		summaries = append(summaries, map[string]interface{}{
			"user_id":          row.UserID,
			"user_name":        row.UserName,
			"login_count":      row.LoginCount,
			"create_count":     row.CreateCount,
			"update_count":     row.UpdateCount,
			"download_count":   row.DownloadCount,
			"total_actions":    row.TotalActions,
			"submission_count": row.SubmissionCount,
			"last_login":       lastLogin,
		})
	}

	return summaries
}

func buildSystemTrendBreakdown(currentYear string, approvedStatusID int) map[string][]map[string]interface{} {
	breakdown := make(map[string][]map[string]interface{})

	breakdown["monthly"] = buildMonthlyTrend(approvedStatusID)
	breakdown["yearly"] = buildYearlyTrend(approvedStatusID)
	breakdown["quarterly"] = buildQuarterlyTrend(approvedStatusID)
	breakdown["installment"] = buildInstallmentTrend(currentYear, approvedStatusID)

	return breakdown
}

func buildMonthlyTrend(approvedStatusID int) []map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -11, 0)

	var rows []struct {
		Period         string
		FundTotal      float64
		RewardTotal    float64
		FundApproved   float64
		RewardApproved float64
		TotalRequested float64
		TotalApproved  float64
	}

	config.DB.Table("submissions s").
		Select(`DATE_FORMAT(s.submitted_at, '%Y-%m') AS period,
                    SUM(CASE WHEN s.submission_type = 'fund_application' THEN 1 ELSE 0 END) AS fund_total,
                    SUM(CASE WHEN s.submission_type = 'publication_reward' THEN 1 ELSE 0 END) AS reward_total,
                    SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id = ? THEN 1 ELSE 0 END) AS fund_approved,
                    SUM(CASE WHEN s.submission_type = 'publication_reward' AND s.status_id = ? THEN 1 ELSE 0 END) AS reward_approved,
                    SUM(CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.requested_amount,0) ELSE COALESCE(prd.reward_amount,0) END) AS total_requested,
                    SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id = ? THEN COALESCE(fad.approved_amount,0)
                             WHEN s.submission_type = 'publication_reward' AND s.status_id = ? THEN COALESCE(prd.total_approve_amount, COALESCE(prd.reward_approve_amount,0))
                             ELSE 0 END) AS total_approved`, approvedStatusID, approvedStatusID, approvedStatusID, approvedStatusID).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL AND s.submitted_at IS NOT NULL", submissionTypes).
		Where("s.submitted_at >= ?", start.Format("2006-01-02")).
		Group("DATE_FORMAT(s.submitted_at, '%Y-%m')").
		Order("period ASC").
		Scan(&rows)

	dataByPeriod := make(map[string]struct {
		FundTotal      float64
		RewardTotal    float64
		FundApproved   float64
		RewardApproved float64
		TotalRequested float64
		TotalApproved  float64
	})

	for _, row := range rows {
		dataByPeriod[row.Period] = struct {
			FundTotal      float64
			RewardTotal    float64
			FundApproved   float64
			RewardApproved float64
			TotalRequested float64
			TotalApproved  float64
		}{
			FundTotal:      row.FundTotal,
			RewardTotal:    row.RewardTotal,
			FundApproved:   row.FundApproved,
			RewardApproved: row.RewardApproved,
			TotalRequested: row.TotalRequested,
			TotalApproved:  row.TotalApproved,
		}
	}

	results := make([]map[string]interface{}, 0, 12)
	for i := 0; i < 12; i++ {
		current := start.AddDate(0, i, 0)
		period := current.Format("2006-01")

		values, ok := dataByPeriod[period]
		if !ok {
			values = struct {
				FundTotal      float64
				RewardTotal    float64
				FundApproved   float64
				RewardApproved float64
				TotalRequested float64
				TotalApproved  float64
			}{}
		}

		totalApplications := values.FundTotal + values.RewardTotal
		approvedApplications := values.FundApproved + values.RewardApproved

		results = append(results, map[string]interface{}{
			"period":             period,
			"month":              period,
			"total_applications": totalApplications,
			"approved":           approvedApplications,
			"fund_total":         values.FundTotal,
			"reward_total":       values.RewardTotal,
			"fund_approved":      values.FundApproved,
			"reward_approved":    values.RewardApproved,
			"total_requested":    values.TotalRequested,
			"total_approved":     values.TotalApproved,
		})
	}

	return results
}

func buildYearlyTrend(approvedStatusID int) []map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}

	now := time.Now()
	start := time.Date(now.Year()-4, 1, 1, 0, 0, 0, 0, now.Location())

	var rows []struct {
		Year           int
		FundTotal      float64
		RewardTotal    float64
		FundApproved   float64
		RewardApproved float64
		TotalRequested float64
		TotalApproved  float64
	}

	config.DB.Table("submissions s").
		Select(`YEAR(s.submitted_at) AS year,
                    SUM(CASE WHEN s.submission_type = 'fund_application' THEN 1 ELSE 0 END) AS fund_total,
                    SUM(CASE WHEN s.submission_type = 'publication_reward' THEN 1 ELSE 0 END) AS reward_total,
                    SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id = ? THEN 1 ELSE 0 END) AS fund_approved,
                    SUM(CASE WHEN s.submission_type = 'publication_reward' AND s.status_id = ? THEN 1 ELSE 0 END) AS reward_approved,
                    SUM(CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.requested_amount,0) ELSE COALESCE(prd.reward_amount,0) END) AS total_requested,
                    SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id = ? THEN COALESCE(fad.approved_amount,0)
                             WHEN s.submission_type = 'publication_reward' AND s.status_id = ? THEN COALESCE(prd.total_approve_amount, COALESCE(prd.reward_approve_amount,0))
                             ELSE 0 END) AS total_approved`, approvedStatusID, approvedStatusID, approvedStatusID, approvedStatusID).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL AND s.submitted_at IS NOT NULL", submissionTypes).
		Where("s.submitted_at >= ?", start.Format("2006-01-02")).
		Group("YEAR(s.submitted_at)").
		Order("year ASC").
		Scan(&rows)

	results := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		totalApplications := row.FundTotal + row.RewardTotal
		approvedApplications := row.FundApproved + row.RewardApproved

		results = append(results, map[string]interface{}{
			"period":             row.Year,
			"year":               row.Year,
			"total_applications": totalApplications,
			"approved":           approvedApplications,
			"fund_total":         row.FundTotal,
			"reward_total":       row.RewardTotal,
			"fund_approved":      row.FundApproved,
			"reward_approved":    row.RewardApproved,
			"total_requested":    row.TotalRequested,
			"total_approved":     row.TotalApproved,
		})
	}

	return results
}

func buildQuarterlyTrend(approvedStatusID int) []map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}

	now := time.Now()
	start := time.Date(now.Year()-1, 1, 1, 0, 0, 0, 0, now.Location())

	var rows []struct {
		Year           int
		Quarter        int
		FundTotal      float64
		RewardTotal    float64
		FundApproved   float64
		RewardApproved float64
		TotalRequested float64
		TotalApproved  float64
	}

	config.DB.Table("submissions s").
		Select(`YEAR(s.submitted_at) AS year,
                    QUARTER(s.submitted_at) AS quarter,
                    SUM(CASE WHEN s.submission_type = 'fund_application' THEN 1 ELSE 0 END) AS fund_total,
                    SUM(CASE WHEN s.submission_type = 'publication_reward' THEN 1 ELSE 0 END) AS reward_total,
                    SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id = ? THEN 1 ELSE 0 END) AS fund_approved,
                    SUM(CASE WHEN s.submission_type = 'publication_reward' AND s.status_id = ? THEN 1 ELSE 0 END) AS reward_approved,
                    SUM(CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.requested_amount,0) ELSE COALESCE(prd.reward_amount,0) END) AS total_requested,
                    SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id = ? THEN COALESCE(fad.approved_amount,0)
                             WHEN s.submission_type = 'publication_reward' AND s.status_id = ? THEN COALESCE(prd.total_approve_amount, COALESCE(prd.reward_approve_amount,0))
                             ELSE 0 END) AS total_approved`, approvedStatusID, approvedStatusID, approvedStatusID, approvedStatusID).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL AND s.submitted_at IS NOT NULL", submissionTypes).
		Where("s.submitted_at >= ?", start.Format("2006-01-02")).
		Group("YEAR(s.submitted_at), QUARTER(s.submitted_at)").
		Order("year ASC, quarter ASC").
		Scan(&rows)

	results := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		totalApplications := row.FundTotal + row.RewardTotal
		approvedApplications := row.FundApproved + row.RewardApproved

		results = append(results, map[string]interface{}{
			"period":             map[string]int{"year": row.Year, "quarter": row.Quarter},
			"year":               row.Year,
			"quarter":            row.Quarter,
			"total_applications": totalApplications,
			"approved":           approvedApplications,
			"fund_total":         row.FundTotal,
			"reward_total":       row.RewardTotal,
			"fund_approved":      row.FundApproved,
			"reward_approved":    row.RewardApproved,
			"total_requested":    row.TotalRequested,
			"total_approved":     row.TotalApproved,
		})
	}

	return results
}

func buildInstallmentTrend(currentYear string, approvedStatusID int) []map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}

	var rows []struct {
		Year           string
		Installment    *int
		PeriodName     *string
		FundTotal      float64
		RewardTotal    float64
		FundApproved   float64
		RewardApproved float64
		TotalRequested float64
		TotalApproved  float64
	}

	config.DB.Table("submissions s").
		Select(`y.year AS year,
                    s.installment_number_at_submit AS installment,
                    COALESCE(fip.name, CONCAT('รอบที่ ', s.installment_number_at_submit)) AS period_name,
                    SUM(CASE WHEN s.submission_type = 'fund_application' THEN 1 ELSE 0 END) AS fund_total,
                    SUM(CASE WHEN s.submission_type = 'publication_reward' THEN 1 ELSE 0 END) AS reward_total,
                    SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id = ? THEN 1 ELSE 0 END) AS fund_approved,
                    SUM(CASE WHEN s.submission_type = 'publication_reward' AND s.status_id = ? THEN 1 ELSE 0 END) AS reward_approved,
                    SUM(CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.requested_amount,0) ELSE COALESCE(prd.reward_amount,0) END) AS total_requested,
                    SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id = ? THEN COALESCE(fad.approved_amount,0)
                             WHEN s.submission_type = 'publication_reward' AND s.status_id = ? THEN COALESCE(prd.total_approve_amount, COALESCE(prd.reward_approve_amount,0))
                             ELSE 0 END) AS total_approved`, approvedStatusID, approvedStatusID, approvedStatusID, approvedStatusID).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Joins("JOIN years y ON s.year_id = y.year_id").
		Joins("LEFT JOIN fund_installment_periods fip ON fip.year_id = s.year_id AND fip.installment_number = s.installment_number_at_submit AND fip.deleted_at IS NULL").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL AND s.installment_number_at_submit IS NOT NULL", submissionTypes).
		Where("y.year = ?", currentYear).
		Group("y.year, s.installment_number_at_submit, period_name").
		Order("s.installment_number_at_submit ASC").
		Scan(&rows)

	results := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		installmentNumber := 0
		if row.Installment != nil {
			installmentNumber = *row.Installment
		}
		totalApplications := row.FundTotal + row.RewardTotal
		approvedApplications := row.FundApproved + row.RewardApproved

		periodLabel := row.PeriodName
		if periodLabel == nil || *periodLabel == "" {
			fallback := "รอบที่ "
			if installmentNumber > 0 {
				fallback = fallback + fmt.Sprintf("%d", installmentNumber)
			} else {
				fallback = "ไม่ระบุรอบ"
			}
			periodLabel = &fallback
		}

		results = append(results, map[string]interface{}{
			"year":               row.Year,
			"installment":        installmentNumber,
			"period_label":       *periodLabel,
			"total_applications": totalApplications,
			"approved":           approvedApplications,
			"fund_total":         row.FundTotal,
			"reward_total":       row.RewardTotal,
			"fund_approved":      row.FundApproved,
			"reward_approved":    row.RewardApproved,
			"total_requested":    row.TotalRequested,
			"total_approved":     row.TotalApproved,
		})
	}

	return results
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
