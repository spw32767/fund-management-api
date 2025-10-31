package controllers

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
		scopeParam := c.Query("scope")
		yearParam := c.Query("year")
		installmentParam := c.Query("installment")

		filter, options := resolveDashboardFilter(scopeParam, yearParam, installmentParam)
		stats = getAdminDashboard(filter, options)
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

type dashboardFilter struct {
	Scope               string
	Years               []string
	YearIDs             []int
	YearIDMap           map[string]int
	Installments        []int
	SelectedInstallment *int
	SelectedYear        string
	IncludeAll          bool
	CurrentYear         string
	ActiveInstallment   *int
	ExcludedStatusIDs   []int
}

type dashboardStatusSets struct {
	Pending  []int
	Approved []int
	Rejected []int
	Excluded []int
}

const submissionDateExpression = "COALESCE(s.submitted_at, s.created_at)"

func uniqueInts(values []int) []int {
	if len(values) == 0 {
		return values
	}

	seen := make(map[int]struct{}, len(values))
	unique := make([]int, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func ensureIDs(values []int) []int {
	if len(values) == 0 {
		return []int{-1}
	}
	return values
}

type yearOption struct {
	YearID int
	Year   string
}

type installmentOption struct {
	Year        string
	Installment int
	Name        string
	CutoffDate  *time.Time
}

type dashboardFilterOptions struct {
	CurrentYear  string
	Years        []yearOption
	Installments map[string][]installmentOption
	Scopes       []string
}

func (o dashboardFilterOptions) toMap() map[string]interface{} {
	years := make([]map[string]interface{}, 0, len(o.Years))
	for _, year := range o.Years {
		years = append(years, map[string]interface{}{
			"year_id": year.YearID,
			"year":    year.Year,
		})
	}

	installMap := make(map[string][]map[string]interface{}, len(o.Installments))
	for year, list := range o.Installments {
		items := make([]map[string]interface{}, 0, len(list))
		for _, inst := range list {
			cutoff := ""
			if inst.CutoffDate != nil {
				cutoff = inst.CutoffDate.Format("2006-01-02")
			}
			name := strings.TrimSpace(inst.Name)
			items = append(items, map[string]interface{}{
				"installment": inst.Installment,
				"name":        name,
				"cutoff_date": cutoff,
			})
		}
		installMap[year] = items
	}

	return map[string]interface{}{
		"current_year": o.CurrentYear,
		"years":        years,
		"scopes":       o.Scopes,
		"installments": installMap,
	}
}

func (f dashboardFilter) toMap() map[string]interface{} {
	result := map[string]interface{}{
		"scope": f.Scope,
	}
	if f.SelectedYear != "" {
		result["year"] = f.SelectedYear
	}
	if f.SelectedInstallment != nil {
		result["installment"] = *f.SelectedInstallment
	}
	return result
}

func (f dashboardFilter) yearStrings() []string {
	if len(f.Years) == 0 {
		return nil
	}
	values := make([]string, len(f.Years))
	copy(values, f.Years)
	return values
}

func applyFilterToSubmissions(query *gorm.DB, alias string, filter dashboardFilter) *gorm.DB {
	if alias == "" {
		alias = "s"
	}
	if !filter.IncludeAll && len(filter.YearIDs) > 0 {
		query = query.Where(fmt.Sprintf("%s.year_id IN ?", alias), filter.YearIDs)
	}
	if filter.Scope == "installment" && len(filter.Installments) > 0 {
		query = query.Where(fmt.Sprintf("%s.installment_number_at_submit IN ?", alias), filter.Installments)
	}
	if len(filter.ExcludedStatusIDs) > 0 {
		query = query.Where(fmt.Sprintf("%s.status_id NOT IN ?", alias), filter.ExcludedStatusIDs)
	}
	return query
}

func applyFilterToYearColumn(query *gorm.DB, column string, filter dashboardFilter) *gorm.DB {
	if !filter.IncludeAll {
		if years := filter.yearStrings(); len(years) > 0 {
			query = query.Where(fmt.Sprintf("%s IN ?", column), years)
		}
	}
	return query
}

func resolveDashboardFilter(scopeParam, yearParam, installmentParam string) (dashboardFilter, dashboardFilterOptions) {
	filter := dashboardFilter{}
	options := dashboardFilterOptions{
		Scopes: []string{"all", "current_year", "year", "installment"},
	}

	var yearRows []struct {
		YearID int
		Year   string
	}

	config.DB.Table("years").
		Select("year_id, year").
		Order("year DESC").
		Scan(&yearRows)

	options.Years = make([]yearOption, 0, len(yearRows))
	yearMap := make(map[string]int, len(yearRows))
	for _, row := range yearRows {
		options.Years = append(options.Years, yearOption{YearID: row.YearID, Year: row.Year})
		yearMap[row.Year] = row.YearID
	}

	filter.YearIDMap = yearMap

	var cfg struct {
		CurrentYear *string
		Installment *int
	}

	config.DB.Table("system_config").
		Select("current_year, installment").
		Order("config_id DESC").
		Limit(1).
		Scan(&cfg)

	if cfg.CurrentYear != nil && strings.TrimSpace(*cfg.CurrentYear) != "" {
		filter.CurrentYear = strings.TrimSpace(*cfg.CurrentYear)
	} else if len(options.Years) > 0 {
		filter.CurrentYear = options.Years[0].Year
	} else {
		filter.CurrentYear = time.Now().AddDate(543, 0, 0).Format("2006")
	}

	if cfg.Installment != nil && *cfg.Installment > 0 {
		filter.ActiveInstallment = cfg.Installment
	}

	options.CurrentYear = filter.CurrentYear

	var installmentRows []struct {
		YearID      int
		Year        string
		Installment int
		Name        *string
		CutoffDate  *time.Time
	}

	config.DB.Table("fund_installment_periods fip").
		Select("fip.year_id, y.year, fip.installment_number AS installment, fip.name, fip.cutoff_date").
		Joins("JOIN years y ON fip.year_id = y.year_id").
		Where("fip.deleted_at IS NULL").
		Order("y.year DESC, fip.installment_number ASC").
		Scan(&installmentRows)

	options.Installments = make(map[string][]installmentOption)
	for _, row := range installmentRows {
		name := ""
		if row.Name != nil {
			name = strings.TrimSpace(*row.Name)
		}
		options.Installments[row.Year] = append(options.Installments[row.Year], installmentOption{
			Year:        row.Year,
			Installment: row.Installment,
			Name:        name,
			CutoffDate:  row.CutoffDate,
		})
	}

	for year := range options.Installments {
		sort.Slice(options.Installments[year], func(i, j int) bool {
			return options.Installments[year][i].Installment < options.Installments[year][j].Installment
		})
	}

	scope := strings.TrimSpace(strings.ToLower(scopeParam))
	switch scope {
	case "", "default":
		scope = "current_year"
	case "all", "current_year", "year", "installment":
		// valid scopes
	default:
		scope = "current_year"
	}

	filter.Scope = scope

	cleanedYear := strings.TrimSpace(yearParam)
	if cleanedYear == "" {
		cleanedYear = filter.CurrentYear
	}

	switch scope {
	case "all":
		filter.IncludeAll = true
	case "current_year":
		if id, ok := yearMap[filter.CurrentYear]; ok {
			filter.YearIDs = []int{id}
			filter.Years = []string{filter.CurrentYear}
			filter.SelectedYear = filter.CurrentYear
		}
	case "year", "installment":
		targetYear := cleanedYear
		if id, ok := yearMap[targetYear]; ok {
			filter.YearIDs = []int{id}
			filter.Years = []string{targetYear}
			filter.SelectedYear = targetYear
		} else if id, ok := yearMap[filter.CurrentYear]; ok {
			filter.YearIDs = []int{id}
			filter.Years = []string{filter.CurrentYear}
			filter.SelectedYear = filter.CurrentYear
		} else if len(options.Years) > 0 {
			filter.YearIDs = []int{options.Years[0].YearID}
			filter.Years = []string{options.Years[0].Year}
			filter.SelectedYear = options.Years[0].Year
		}
	}

	if scope == "installment" {
		cleanedInstallment := strings.TrimSpace(installmentParam)
		if cleanedInstallment != "" {
			parts := strings.Split(cleanedInstallment, ",")
			for _, part := range parts {
				trimmed := strings.TrimSpace(part)
				if trimmed == "" {
					continue
				}
				if val, err := strconv.Atoi(trimmed); err == nil {
					filter.Installments = append(filter.Installments, val)
				}
			}
		}

		if len(filter.Installments) == 0 {
			if filter.SelectedYear == filter.CurrentYear && filter.ActiveInstallment != nil {
				filter.Installments = []int{*filter.ActiveInstallment}
			}
		}

		if len(filter.Installments) > 0 {
			first := filter.Installments[0]
			filter.SelectedInstallment = &first
		}
	}

	return filter, options
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
func getAdminDashboard(filter dashboardFilter, options dashboardFilterOptions) map[string]interface{} {
	stats := make(map[string]interface{})

	statusSets := dashboardStatusSets{}

	if pendingIDs, err := utils.GetStatusIDsByCodes(utils.StatusCodePending, utils.StatusCodeDeptHeadPending, utils.StatusCodeNeedsMoreInfo); err == nil {
		statusSets.Pending = append(statusSets.Pending, pendingIDs...)
	}

	if approvedIDs, err := utils.GetStatusIDsByCodes(utils.StatusCodeApproved, utils.StatusCodeAdminClosed); err == nil {
		statusSets.Approved = append(statusSets.Approved, approvedIDs...)
	}

	if rejectedIDs, err := utils.GetStatusIDsByCodes(utils.StatusCodeRejected, utils.StatusCodeDeptHeadNotRecommended); err == nil {
		statusSets.Rejected = append(statusSets.Rejected, rejectedIDs...)
	}

	if draftIDs, err := utils.GetStatusIDsByCodes(utils.StatusCodeDraft); err == nil {
		filter.ExcludedStatusIDs = append(filter.ExcludedStatusIDs, draftIDs...)
		statusSets.Excluded = append(statusSets.Excluded, draftIDs...)
	}

	statusSets.Pending = uniqueInts(statusSets.Pending)
	statusSets.Approved = uniqueInts(statusSets.Approved)
	statusSets.Rejected = uniqueInts(statusSets.Rejected)
	statusSets.Excluded = uniqueInts(statusSets.Excluded)
	filter.ExcludedStatusIDs = uniqueInts(filter.ExcludedStatusIDs)

	stats["overview"] = buildAdminOverview(filter, statusSets)
	stats["category_budgets"] = buildAdminCategoryBudgets(filter, statusSets)
	stats["pending_applications"] = buildAdminPendingApplications(filter, statusSets)

	quotaUsageRows := collectQuotaUsageViewRows(filter)
	stats["quota_summary"] = buildAdminQuotaSummary(filter, statusSets, quotaUsageRows)
	if len(quotaUsageRows) > 0 {
		stats["quota_usage_view_rows"] = quotaUsageRows
	}

	if statusBreakdown := buildAdminStatusBreakdown(filter); len(statusBreakdown) > 0 {
		stats["status_breakdown"] = statusBreakdown
	}

	if financialOverview := buildAdminFinancialOverview(filter, statusSets); len(financialOverview) > 0 {
		stats["financial_overview"] = financialOverview
	}

	if upcoming := buildAdminUpcomingInstallments(filter); len(upcoming) > 0 {
		stats["upcoming_periods"] = upcoming
	}

	trendBreakdown := buildSystemTrendBreakdown(filter, statusSets)
	if len(trendBreakdown) > 0 {
		stats["trend_breakdown"] = trendBreakdown
		if monthly, ok := trendBreakdown["monthly"]; ok {
			stats["monthly_trends"] = monthly
		}
	}

	stats["filter_options"] = options.toMap()
	stats["selected_filter"] = filter.toMap()
	stats["filters"] = options.toMap()
	stats["applied_filter"] = filter.toMap()

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

func buildAdminOverview(filter dashboardFilter, statuses dashboardStatusSets) map[string]interface{} {
	overview := make(map[string]interface{})

	submissionTypes := []string{"fund_application", "publication_reward"}
	pendingIDs := ensureIDs(statuses.Pending)
	approvedIDs := ensureIDs(statuses.Approved)
	rejectedIDs := ensureIDs(statuses.Rejected)

	var totalApplications int64
	submissionQuery := config.DB.Table("submissions s").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL", submissionTypes)
	submissionQuery = applyFilterToSubmissions(submissionQuery, "s", filter)
	submissionQuery.Count(&totalApplications)
	overview["total_applications"] = totalApplications

	typeCounts := make(map[string]int64)
	var typeRows []struct {
		SubmissionType string
		Total          int64
	}

	typeQuery := config.DB.Table("submissions s").
		Select("s.submission_type, COUNT(*) AS total").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL", submissionTypes)
	typeQuery = applyFilterToSubmissions(typeQuery, "s", filter)
	typeQuery.Group("s.submission_type").
		Scan(&typeRows)

	for _, row := range typeRows {
		typeCounts[row.SubmissionType] = row.Total
	}

	overview["fund_applications"] = typeCounts["fund_application"]
	overview["publication_rewards"] = typeCounts["publication_reward"]

	var pendingCount int64
	pendingQuery := config.DB.Table("submissions s").
		Where("s.submission_type IN ? AND s.status_id IN ? AND s.deleted_at IS NULL", submissionTypes, pendingIDs)
	pendingQuery = applyFilterToSubmissions(pendingQuery, "s", filter)
	pendingQuery.Count(&pendingCount)
	overview["pending_count"] = pendingCount

	var approvedCount int64
	if len(statuses.Approved) > 0 {
		approvedQuery := config.DB.Table("submissions s").
			Where("s.submission_type IN ? AND s.status_id IN ? AND s.deleted_at IS NULL", submissionTypes, approvedIDs)
		approvedQuery = applyFilterToSubmissions(approvedQuery, "s", filter)
		approvedQuery.Count(&approvedCount)
	}
	overview["approved_count"] = approvedCount

	var rejectedCount int64
	rejectedQuery := config.DB.Table("submissions s").
		Where("s.submission_type IN ? AND s.status_id IN ? AND s.deleted_at IS NULL", submissionTypes, rejectedIDs)
	rejectedQuery = applyFilterToSubmissions(rejectedQuery, "s", filter)
	rejectedQuery.Count(&rejectedCount)
	overview["rejected_count"] = rejectedCount

	if totalApplications > 0 {
		overview["approval_rate"] = (float64(approvedCount) / float64(totalApplications)) * 100
	} else {
		overview["approval_rate"] = 0.0
	}

	var totalUsers int64
	config.DB.Table("users").
		Where("delete_at IS NULL").
		Count(&totalUsers)
	overview["total_users"] = totalUsers

	var budget struct {
		Allocated float64
		Used      float64
		Remaining float64
	}

	budgetQuery := config.DB.Table("view_budget_summary").
		Select("COALESCE(SUM(allocated_amount),0) AS allocated, COALESCE(SUM(used_amount),0) AS used, COALESCE(SUM(remaining_budget),0) AS remaining")
	budgetQuery = applyFilterToYearColumn(budgetQuery, "year", filter)
	budgetQuery.Scan(&budget)

	overview["total_budget"] = budget.Allocated
	overview["allocated_budget"] = budget.Allocated
	overview["used_budget"] = budget.Used
	overview["approved_amount_total"] = budget.Used
	overview["remaining_budget"] = budget.Remaining

	type amountSummary struct {
		Requested float64
		Approved  float64
	}

	var fundAmounts amountSummary
	fundQuery := config.DB.Table("fund_application_details fad").
		Joins("JOIN submissions s ON fad.submission_id = s.submission_id").
		Where("s.submission_type = ? AND s.deleted_at IS NULL", "fund_application")
	fundQuery = applyFilterToSubmissions(fundQuery, "s", filter)
	fundQuery.Select("COALESCE(SUM(fad.requested_amount),0) AS requested, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN fad.approved_amount ELSE 0 END),0) AS approved", approvedIDs).
		Scan(&fundAmounts)

	var rewardAmounts amountSummary
	rewardQuery := config.DB.Table("publication_reward_details prd").
		Joins("JOIN submissions s ON prd.submission_id = s.submission_id").
		Where("s.submission_type = ? AND s.deleted_at IS NULL", "publication_reward")
	rewardQuery = applyFilterToSubmissions(rewardQuery, "s", filter)
	rewardQuery.Select("COALESCE(SUM(prd.reward_amount),0) AS requested, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN COALESCE(prd.total_approve_amount, prd.reward_approve_amount, prd.reward_amount) ELSE 0 END),0) AS approved", approvedIDs).
		Scan(&rewardAmounts)

	overview["total_requested_amount"] = fundAmounts.Requested + rewardAmounts.Requested
	overview["total_approved_amount"] = fundAmounts.Approved + rewardAmounts.Approved

	if filter.SelectedYear != "" {
		overview["current_year"] = filter.SelectedYear
	} else {
		overview["current_year"] = filter.CurrentYear
	}
	overview["scope"] = filter.Scope

	return overview
}

func buildAdminCategoryBudgets(filter dashboardFilter, statuses dashboardStatusSets) []map[string]interface{} {
	type categoryLookupEntry struct {
		CategoryID   int
		CategoryName string
		Year         string
		YearID       int
	}

	type subcategoryLookupEntry struct {
		SubcategoryID   int
		CategoryID      int
		SubcategoryName string
	}

	type subcategoryAggregate struct {
		SubcategoryID        int
		SubcategoryName      string
		AllocatedAmount      float64
		RemainingBudget      float64
		MaxGrants            float64
		RemainingGrants      float64
		RequestedAmount      float64
		ApprovedAmount       float64
		TotalApplications    int64
		ApprovedApplications int64
		UsedGrants           float64
	}

	type categoryAggregate struct {
		CategoryID           int
		CategoryName         string
		Year                 string
		YearID               int
		TotalApplications    int64
		ApprovedApplications int64
		RequestedAmount      float64
		ApprovedAmount       float64
		BudgetAllocated      float64
		BudgetRemaining      float64
		BudgetMaxGrants      float64
		BudgetRemainingGrant float64
		Subcategories        map[int]*subcategoryAggregate
	}

	categoryInfo := make(map[int]categoryLookupEntry)
	var categoryRows []categoryLookupEntry
	categoryQuery := config.DB.Table("fund_categories fc").
		Select("fc.category_id, fc.category_name, y.year, y.year_id").
		Joins("JOIN years y ON fc.year_id = y.year_id").
		Where("fc.delete_at IS NULL")

	if !filter.IncludeAll && len(filter.YearIDs) > 0 {
		categoryQuery = categoryQuery.Where("fc.year_id IN ?", filter.YearIDs)
	}

	categoryQuery.Scan(&categoryRows)
	for _, row := range categoryRows {
		categoryInfo[row.CategoryID] = row
	}

	subcategoryInfo := make(map[int]subcategoryLookupEntry)
	var subcategoryRows []subcategoryLookupEntry
	subcategoryQuery := config.DB.Table("fund_subcategories fsc").
		Select("fsc.subcategory_id, fsc.subcategory_name, fsc.category_id").
		Joins("JOIN fund_categories fc ON fsc.category_id = fc.category_id").
		Where("fsc.delete_at IS NULL")

	if !filter.IncludeAll && len(filter.YearIDs) > 0 {
		subcategoryQuery = subcategoryQuery.Where("fc.year_id IN ?", filter.YearIDs)
	}

	subcategoryQuery.Scan(&subcategoryRows)
	for _, row := range subcategoryRows {
		subcategoryInfo[row.SubcategoryID] = row
	}

	categories := make(map[string]*categoryAggregate)
	order := make([]string, 0)

	getCategoryAggregate := func(yearID, categoryID int) *categoryAggregate {
		key := fmt.Sprintf("%d:%d", yearID, categoryID)
		if aggregate, exists := categories[key]; exists {
			return aggregate
		}

		info, ok := categoryInfo[categoryID]
		if !ok {
			yearValue := strings.TrimSpace(filter.SelectedYear)
			if yearValue == "" {
				yearValue = strings.TrimSpace(filter.CurrentYear)
			}
			info = categoryLookupEntry{
				CategoryID:   categoryID,
				CategoryName: "ไม่ระบุ",
				Year:         yearValue,
				YearID:       yearID,
			}
			categoryInfo[categoryID] = info
		}

		aggregate := &categoryAggregate{
			CategoryID:    info.CategoryID,
			CategoryName:  info.CategoryName,
			Year:          info.Year,
			YearID:        info.YearID,
			Subcategories: make(map[int]*subcategoryAggregate),
		}
		categories[key] = aggregate
		order = append(order, key)
		return aggregate
	}

	type budgetRow struct {
		CategoryID        int
		YearID            int
		AllocatedAmount   float64
		RemainingBudget   float64
		MaxGrants         float64
		RemainingGrant    float64
		MaxAmountPerYear  float64
		MaxAmountPerGrant float64
		SubcategoryID     *int
	}

	var budgetRows []budgetRow

	budgetQuery := config.DB.Table("fund_categories fc").
		Select("fc.category_id, y.year_id, sb.allocated_amount, sb.remaining_budget, sb.max_grants, sb.remaining_grant, sb.max_amount_per_year, sb.max_amount_per_grant, fsc.subcategory_id").
		Joins("JOIN years y ON fc.year_id = y.year_id").
		Joins("LEFT JOIN fund_subcategories fsc ON fsc.category_id = fc.category_id AND fsc.delete_at IS NULL").
		Joins("LEFT JOIN subcategory_budgets sb ON sb.subcategory_id = fsc.subcategory_id AND sb.record_scope = 'overall' AND sb.delete_at IS NULL").
		Where("fc.delete_at IS NULL")

	if !filter.IncludeAll && len(filter.YearIDs) > 0 {
		budgetQuery = budgetQuery.Where("fc.year_id IN ?", filter.YearIDs)
	}

	budgetQuery.Scan(&budgetRows)

	for _, row := range budgetRows {
		aggregate := getCategoryAggregate(row.YearID, row.CategoryID)

		allocated := row.AllocatedAmount
		if allocated == 0 {
			if row.MaxAmountPerYear > 0 {
				allocated = row.MaxAmountPerYear
			} else if row.MaxAmountPerGrant > 0 && row.MaxGrants > 0 {
				allocated = row.MaxAmountPerGrant * row.MaxGrants
			}
		}

		remaining := row.RemainingBudget
		if remaining <= 0 && allocated > 0 {
			remaining = math.Max(allocated, 0)
		}

		if row.SubcategoryID != nil && *row.SubcategoryID != 0 {
			subID := *row.SubcategoryID
			subAgg, exists := aggregate.Subcategories[subID]
			if !exists {
				name := "ไม่ระบุ"
				if info, ok := subcategoryInfo[subID]; ok && strings.TrimSpace(info.SubcategoryName) != "" {
					name = info.SubcategoryName
				}
				subAgg = &subcategoryAggregate{
					SubcategoryID:   subID,
					SubcategoryName: name,
				}
				aggregate.Subcategories[subID] = subAgg
			}

			subAgg.AllocatedAmount += allocated
			subAgg.RemainingBudget += remaining
			if row.MaxGrants > 0 {
				subAgg.MaxGrants += row.MaxGrants
			}
			if row.RemainingGrant > 0 {
				subAgg.RemainingGrants += row.RemainingGrant
			}
		} else {
			aggregate.BudgetAllocated += allocated
			aggregate.BudgetRemaining += remaining
			if row.MaxGrants > 0 {
				aggregate.BudgetMaxGrants += row.MaxGrants
			}
			if row.RemainingGrant > 0 {
				aggregate.BudgetRemainingGrant += row.RemainingGrant
			}
		}
	}

	approvedIDs := ensureIDs(statuses.Approved)

	type submissionRow struct {
		YearID               int
		CategoryID           int
		SubcategoryID        *int
		TotalApplications    int64
		ApprovedApplications int64
		RequestedAmount      float64
		ApprovedAmount       float64
	}

	var submissionRows []submissionRow
	submissionTypes := []string{"fund_application", "publication_reward"}

	submissionQuery := config.DB.Table("submissions s").
		Select(`s.year_id,
            s.category_id,
            s.subcategory_id,
            COUNT(*) AS total_applications,
            SUM(CASE WHEN s.status_id IN ? THEN 1 ELSE 0 END) AS approved_applications,
            SUM(CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.requested_amount,0)
                     WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.reward_amount,0)
                     ELSE 0 END) AS requested_amount,
            SUM(CASE WHEN s.status_id IN ? THEN CASE
                     WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.approved_amount,0)
                     WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.total_approve_amount, prd.reward_approve_amount, prd.reward_amount, 0)
                     ELSE 0 END ELSE 0 END) AS approved_amount`, approvedIDs, approvedIDs).
		Joins("LEFT JOIN fund_application_details fad ON fad.submission_id = s.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON prd.submission_id = s.submission_id").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL", submissionTypes)

	submissionQuery = applyFilterToSubmissions(submissionQuery, "s", filter)

	submissionQuery.Group("s.year_id, s.category_id, s.subcategory_id").
		Scan(&submissionRows)

	for _, row := range submissionRows {
		aggregate := getCategoryAggregate(row.YearID, row.CategoryID)
		aggregate.TotalApplications += row.TotalApplications
		aggregate.ApprovedApplications += row.ApprovedApplications
		aggregate.RequestedAmount += row.RequestedAmount
		aggregate.ApprovedAmount += row.ApprovedAmount

		if row.SubcategoryID != nil && *row.SubcategoryID != 0 {
			subID := *row.SubcategoryID
			subAgg, exists := aggregate.Subcategories[subID]
			if !exists {
				name := "ไม่ระบุ"
				if info, ok := subcategoryInfo[subID]; ok && strings.TrimSpace(info.SubcategoryName) != "" {
					name = info.SubcategoryName
				}
				subAgg = &subcategoryAggregate{
					SubcategoryID:   subID,
					SubcategoryName: name,
				}
				aggregate.Subcategories[subID] = subAgg
			}

			subAgg.TotalApplications += row.TotalApplications
			subAgg.ApprovedApplications += row.ApprovedApplications
			subAgg.RequestedAmount += row.RequestedAmount
			subAgg.ApprovedAmount += row.ApprovedAmount
			subAgg.UsedGrants += float64(row.ApprovedApplications)
		} else {
			aggregate.BudgetMaxGrants += float64(row.TotalApplications)
		}
	}

	results := make([]map[string]interface{}, 0, len(order))

	sort.Slice(order, func(i, j int) bool {
		left := categories[order[i]]
		right := categories[order[j]]
		if left.ApprovedAmount == right.ApprovedAmount {
			return left.CategoryName < right.CategoryName
		}
		return left.ApprovedAmount > right.ApprovedAmount
	})

	for _, key := range order {
		aggregate := categories[key]

		subcategories := make([]map[string]interface{}, 0, len(aggregate.Subcategories))
		categoryAllocated := aggregate.BudgetAllocated
		categoryRemaining := aggregate.BudgetRemaining
		categoryMaxGrants := aggregate.BudgetMaxGrants
		categoryRemainingGrant := aggregate.BudgetRemainingGrant

		for _, subAgg := range aggregate.Subcategories {
			usedAmount := subAgg.ApprovedAmount
			if subAgg.RemainingBudget <= 0 && subAgg.AllocatedAmount > 0 {
				subAgg.RemainingBudget = math.Max(subAgg.AllocatedAmount-usedAmount, 0)
			}
			if subAgg.RemainingGrants <= 0 && subAgg.MaxGrants > 0 {
				subAgg.RemainingGrants = math.Max(subAgg.MaxGrants-subAgg.UsedGrants, 0)
			}

			subcategories = append(subcategories, map[string]interface{}{
				"subcategory_id":        subAgg.SubcategoryID,
				"subcategory_name":      subAgg.SubcategoryName,
				"total_applications":    subAgg.TotalApplications,
				"approved_applications": subAgg.ApprovedApplications,
				"requested_amount":      subAgg.RequestedAmount,
				"approved_amount":       usedAmount,
				"used_amount":           usedAmount,
				"allocated_amount":      subAgg.AllocatedAmount,
				"remaining_budget":      subAgg.RemainingBudget,
				"max_grants":            subAgg.MaxGrants,
				"remaining_grant":       subAgg.RemainingGrants,
			})

			categoryAllocated += subAgg.AllocatedAmount
			categoryRemaining += subAgg.RemainingBudget
			categoryMaxGrants += subAgg.MaxGrants
			categoryRemainingGrant += subAgg.RemainingGrants
		}

		sort.Slice(subcategories, func(i, j int) bool {
			left := subcategories[i]["approved_amount"].(float64)
			right := subcategories[j]["approved_amount"].(float64)
			if left == right {
				leftName, _ := subcategories[i]["subcategory_name"].(string)
				rightName, _ := subcategories[j]["subcategory_name"].(string)
				return leftName < rightName
			}
			return left > right
		})

		usedAmount := aggregate.ApprovedAmount
		if categoryRemaining <= 0 && categoryAllocated > 0 {
			categoryRemaining = math.Max(categoryAllocated-usedAmount, 0)
		}

		results = append(results, map[string]interface{}{
			"category_id":           aggregate.CategoryID,
			"category_name":         aggregate.CategoryName,
			"year":                  aggregate.Year,
			"year_id":               aggregate.YearID,
			"total_applications":    aggregate.TotalApplications,
			"approved_applications": aggregate.ApprovedApplications,
			"requested_amount":      aggregate.RequestedAmount,
			"approved_amount":       usedAmount,
			"used_amount":           usedAmount,
			"allocated_budget":      categoryAllocated,
			"remaining_budget":      categoryRemaining,
			"max_grants":            categoryMaxGrants,
			"remaining_grant":       categoryRemainingGrant,
			"subcategory_count":     len(subcategories),
			"subcategories":         subcategories,
		})
	}

	return results
}

func buildAdminPendingApplications(filter dashboardFilter, statuses dashboardStatusSets) []map[string]interface{} {
	var pendingApplications []map[string]interface{}

	submissionTypes := []string{"fund_application", "publication_reward"}
	pendingIDs := ensureIDs(statuses.Pending)

	query := config.DB.Table("submissions s").
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
		Where("s.submission_type IN ? AND s.status_id IN ? AND s.deleted_at IS NULL", submissionTypes, pendingIDs)

	query = applyFilterToSubmissions(query, "s", filter)

	query.Order("s.submitted_at DESC").
		Limit(10).
		Scan(&pendingApplications)

	return pendingApplications
}

func buildAdminQuotaSummary(filter dashboardFilter, statuses dashboardStatusSets, rawViewRows []map[string]interface{}) []map[string]interface{} {
	logQuotaUsageViewData(filter, rawViewRows)

	viewUsage := fetchUsageAggregatesFromView(filter)
	if len(viewUsage) == 0 && len(rawViewRows) > 0 {
		if fallback := convertUsageRowsToAggregates(rawViewRows); len(fallback) > 0 {
			fmt.Printf("[dashboard] using raw usage view rows as fallback aggregates for quota summary\n")
			viewUsage = fallback
		}
	}

	approvedUsage := fetchApprovedUsageByUser(filter, statuses)

	usageByKey := make(map[string]usageAggregate, len(viewUsage))
	for _, row := range viewUsage {
		if row.YearID == 0 || row.SubcategoryID == 0 || row.UserID == 0 {
			continue
		}
		key := usageKey(row.YearID, row.SubcategoryID, row.UserID)
		usageByKey[key] = row
	}

	if len(usageByKey) == 0 && len(approvedUsage) > 0 {
		fmt.Printf("[dashboard] falling back to aggregate usage derived from submissions for filter: %+v\n", filter.toMap())
		for key, agg := range approvedUsage {
			if agg.YearID == 0 || agg.SubcategoryID == 0 || agg.UserID == 0 {
				continue
			}
			usageByKey[key] = agg
		}
	} else {
		for key, agg := range approvedUsage {
			if existing, ok := usageByKey[key]; ok {
				if agg.UsedGrants > existing.UsedGrants {
					existing.UsedGrants = agg.UsedGrants
				}
				if agg.UsedAmount > existing.UsedAmount {
					existing.UsedAmount = agg.UsedAmount
				}
				usageByKey[key] = existing
			} else if agg.UsedGrants > 0 || agg.UsedAmount > 0 {
				usageByKey[key] = agg
			}
		}
	}

	if len(usageByKey) == 0 {
		fmt.Printf("[dashboard] no quota usage data available after combining view and fallback results for filter: %+v\n", filter.toMap())
		return nil
	}

	collectIDs := func(values []int) []int {
		filtered := make([]int, 0, len(values))
		for _, v := range values {
			if v > 0 {
				filtered = append(filtered, v)
			}
		}
		return uniqueInts(filtered)
	}

	var (
		subcategoryIDs []int
		userIDs        []int
		yearIDs        []int
	)

	for _, agg := range usageByKey {
		subcategoryIDs = append(subcategoryIDs, agg.SubcategoryID)
		userIDs = append(userIDs, agg.UserID)
		yearIDs = append(yearIDs, agg.YearID)
	}

	subcategoryIDs = collectIDs(subcategoryIDs)
	userIDs = collectIDs(userIDs)
	yearIDs = collectIDs(yearIDs)

	type subcategoryMetadata struct {
		YearID            int
		Year              string
		SubcategoryID     int
		SubcategoryName   string
		CategoryID        int
		CategoryName      string
		AllocatedAmount   float64
		MaxAmountPerYear  float64
		MaxAmountPerGrant float64
		MaxGrants         float64
		RemainingGrant    float64
	}

	subcategoryMeta := make(map[string]subcategoryMetadata, len(subcategoryIDs))
	if len(subcategoryIDs) > 0 {
		metaQuery := config.DB.Table("fund_subcategories fsc").
			Select(`y.year AS year,
                y.year_id AS year_id,
                fsc.subcategory_id AS subcategory_id,
                fsc.subcategory_name AS subcategory_name,
                fc.category_id AS category_id,
                fc.category_name AS category_name,
                COALESCE(sb.allocated_amount,0) AS allocated_amount,
                COALESCE(sb.max_amount_per_year,0) AS max_amount_per_year,
                COALESCE(sb.max_amount_per_grant,0) AS max_amount_per_grant,
                COALESCE(sb.max_grants,0) AS max_grants,
                COALESCE(sb.remaining_grant,0) AS remaining_grant`).
			Joins("JOIN fund_categories fc ON fsc.category_id = fc.category_id").
			Joins("JOIN years y ON fc.year_id = y.year_id").
			Joins("LEFT JOIN subcategory_budgets sb ON sb.subcategory_id = fsc.subcategory_id AND sb.record_scope = 'overall' AND sb.deleted_at IS NULL").
			Where("fsc.deleted_at IS NULL AND fc.deleted_at IS NULL").
			Where("fsc.subcategory_id IN ?", subcategoryIDs)

		if len(yearIDs) > 0 {
			metaQuery = metaQuery.Where("y.year_id IN ?", yearIDs)
		}

		var rows []subcategoryMetadata
		if err := metaQuery.Scan(&rows).Error; err != nil {
			fmt.Printf("[dashboard] failed to load subcategory metadata for quota summary: %v\n", err)
			return nil
		}

		for _, row := range rows {
			key := usageKey(row.YearID, row.SubcategoryID, 0)
			subcategoryMeta[key] = row
		}
	}

	userNames := make(map[int]string, len(userIDs))
	if len(userIDs) > 0 {
		var rows []struct {
			UserID int
			Name   string
		}
		if err := config.DB.Table("users").
			Select("user_id, TRIM(CONCAT(COALESCE(user_fname,''),' ',COALESCE(user_lname,''))) AS name").
			Where("user_id IN ?", userIDs).
			Scan(&rows).Error; err != nil {
			fmt.Printf("[dashboard] failed to load user names for quota summary: %v\n", err)
			return nil
		}

		for _, row := range rows {
			trimmed := strings.TrimSpace(row.Name)
			if trimmed != "" {
				userNames[row.UserID] = trimmed
			}
		}
	}

	type quotaSummaryRow struct {
		Year              string
		YearID            int
		UserID            int
		UserName          string
		CategoryID        int
		CategoryName      string
		SubcategoryID     int
		SubcategoryName   string
		AllocatedAmount   float64
		UsedAmount        float64
		RemainingBudget   float64
		MaxGrants         float64
		UsedGrants        float64
		RemainingGrants   float64
		MaxAmountPerYear  float64
		MaxAmountPerGrant float64
	}

	summaryRows := make([]quotaSummaryRow, 0, len(usageByKey))
	for key, usage := range usageByKey {
		if usage.YearID == 0 || usage.SubcategoryID == 0 || usage.UserID == 0 {
			continue
		}

		meta, ok := subcategoryMeta[usageKey(usage.YearID, usage.SubcategoryID, 0)]
		if !ok || strings.TrimSpace(meta.Year) == "" {
			fmt.Printf("[dashboard] missing subcategory metadata for usage key %s\n", key)
			continue
		}

		userName := userNames[usage.UserID]
		if strings.TrimSpace(userName) == "" {
			userName = "ไม่ระบุชื่อ"
		}

		maxGrants := math.Max(meta.MaxGrants, 0)
		usedGrants := math.Max(usage.UsedGrants, 0)

		usedAmount := usage.UsedAmount
		if usedAmount <= 0 {
			if fallback, ok := approvedUsage[usageKey(usage.YearID, usage.SubcategoryID, usage.UserID)]; ok && fallback.UsedAmount > usedAmount {
				usedAmount = fallback.UsedAmount
			}
		}

		budgetLimit := meta.AllocatedAmount
		if budgetLimit <= 0 && meta.MaxAmountPerYear > 0 {
			budgetLimit = meta.MaxAmountPerYear
		}
		if budgetLimit <= 0 && meta.MaxAmountPerGrant > 0 && maxGrants > 0 {
			budgetLimit = meta.MaxAmountPerGrant * maxGrants
		}

		if usedAmount <= 0 && meta.MaxAmountPerGrant > 0 && usedGrants > 0 {
			usedAmount = meta.MaxAmountPerGrant * usedGrants
		}

		remainingBudget := 0.0
		if budgetLimit > 0 {
			remainingBudget = math.Max(budgetLimit-usedAmount, 0)
		}

		remainingGrants := 0.0
		if maxGrants > 0 {
			remainingGrants = math.Max(maxGrants-usedGrants, 0)
		}

		summaryRows = append(summaryRows, quotaSummaryRow{
			Year:              meta.Year,
			YearID:            usage.YearID,
			UserID:            usage.UserID,
			UserName:          userName,
			CategoryID:        meta.CategoryID,
			CategoryName:      meta.CategoryName,
			SubcategoryID:     usage.SubcategoryID,
			SubcategoryName:   meta.SubcategoryName,
			AllocatedAmount:   budgetLimit,
			UsedAmount:        usedAmount,
			RemainingBudget:   remainingBudget,
			MaxGrants:         maxGrants,
			UsedGrants:        usedGrants,
			RemainingGrants:   remainingGrants,
			MaxAmountPerYear:  meta.MaxAmountPerYear,
			MaxAmountPerGrant: meta.MaxAmountPerGrant,
		})
	}

	sort.Slice(summaryRows, func(i, j int) bool {
		if summaryRows[i].UsedAmount == summaryRows[j].UsedAmount {
			return summaryRows[i].UsedGrants > summaryRows[j].UsedGrants
		}
		return summaryRows[i].UsedAmount > summaryRows[j].UsedAmount
	})

	if len(summaryRows) > 100 {
		summaryRows = summaryRows[:100]
	}

	summaries := make([]map[string]interface{}, 0, len(summaryRows))
	for _, row := range summaryRows {
		summaries = append(summaries, map[string]interface{}{
			"year":                 row.Year,
			"year_id":              row.YearID,
			"user_id":              row.UserID,
			"user_name":            row.UserName,
			"category_id":          row.CategoryID,
			"category_name":        row.CategoryName,
			"subcategory_id":       row.SubcategoryID,
			"subcategory_name":     row.SubcategoryName,
			"allocated_amount":     row.AllocatedAmount,
			"used_amount":          row.UsedAmount,
			"remaining_budget":     row.RemainingBudget,
			"max_grants":           row.MaxGrants,
			"used_grants":          row.UsedGrants,
			"remaining_grants":     row.RemainingGrants,
			"max_amount_per_year":  row.MaxAmountPerYear,
			"max_amount_per_grant": row.MaxAmountPerGrant,
		})
	}

	return summaries
}

func fetchUsageAggregatesFromView(filter dashboardFilter) []usageAggregate {
	query := config.DB.Table("v_subcategory_user_usage_total usage_view").
		Select("usage_view.year_id, usage_view.subcategory_id, usage_view.user_id, SUM(usage_view.used_grants) AS used_grants, SUM(usage_view.used_amount) AS used_amount").
		Group("usage_view.year_id, usage_view.subcategory_id, usage_view.user_id")

	if !filter.IncludeAll && len(filter.YearIDs) > 0 {
		query = query.Where("usage_view.year_id IN ?", filter.YearIDs)
	}

	var rows []usageAggregate
	if err := query.Scan(&rows).Error; err != nil {
		fmt.Printf("[dashboard] failed to query v_subcategory_user_usage_total aggregates: %v\n", err)
		return nil
	}

	if len(rows) == 0 {
		fmt.Printf("[dashboard] v_subcategory_user_usage_total returned no aggregate rows for filter: %+v\n", filter.toMap())
	}

	return rows
}

func collectQuotaUsageViewRows(filter dashboardFilter) []map[string]interface{} {
	query := config.DB.Table("v_subcategory_user_usage_total usage_view")

	if !filter.IncludeAll && len(filter.YearIDs) > 0 {
		query = query.Where("usage_view.year_id IN ?", filter.YearIDs)
	}

	var rows []map[string]interface{}
	if err := query.Limit(50).Find(&rows).Error; err != nil {
		fmt.Printf("[dashboard] failed to query v_subcategory_user_usage_total: %v\n", err)
		return []map[string]interface{}{}
	}

	return rows
}

func logQuotaUsageViewData(filter dashboardFilter, rows []map[string]interface{}) {
	if len(rows) == 0 {
		fmt.Printf("[dashboard] v_subcategory_user_usage_total returned no rows for filter: %+v\n", filter.toMap())
		return
	}

	fmt.Printf("[dashboard] logging first %d rows from v_subcategory_user_usage_total\n", len(rows))
	for idx, row := range rows {
		if idx >= 25 {
			break
		}
		fmt.Printf("[dashboard] usage_view_row[%d]: %v\n", idx, row)
	}
}

func convertUsageRowsToAggregates(rows []map[string]interface{}) []usageAggregate {
	aggregates := make([]usageAggregate, 0, len(rows))
	for _, row := range rows {
		yearID := parseIntValue(row["year_id"])
		subcategoryID := parseIntValue(row["subcategory_id"])
		userID := parseIntValue(row["user_id"])
		usedGrants := parseFloatValue(row["used_grants"])
		usedAmount := parseFloatValue(row["used_amount"])

		if yearID == 0 || subcategoryID == 0 || userID == 0 {
			continue
		}

		aggregates = append(aggregates, usageAggregate{
			YearID:        yearID,
			SubcategoryID: subcategoryID,
			UserID:        userID,
			UsedGrants:    usedGrants,
			UsedAmount:    usedAmount,
		})
	}
	return aggregates
}

func parseIntValue(value interface{}) int {
	switch v := value.(type) {
	case nil:
		return 0
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	case []byte:
		if parsed, err := strconv.Atoi(string(v)); err == nil {
			return parsed
		}
		if parsed, err := strconv.ParseFloat(string(v), 64); err == nil {
			return int(parsed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return parsed
		}
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return int(parsed)
		}
	}
	return 0
}

func parseFloatValue(value interface{}) float64 {
	switch v := value.(type) {
	case nil:
		return 0
	case float32:
		return float64(v)
	case float64:
		return v
	case int:
		return float64(v)
	case int8:
		return float64(v)
	case int16:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case uint:
		return float64(v)
	case uint8:
		return float64(v)
	case uint16:
		return float64(v)
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case []byte:
		if parsed, err := strconv.ParseFloat(string(v), 64); err == nil {
			return parsed
		}
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return parsed
		}
	}
	return 0
}

type usageAggregate struct {
	YearID        int
	SubcategoryID int
	UserID        int
	UsedGrants    float64
	UsedAmount    float64
}

func usageKey(yearID, subcategoryID, userID int) string {
	return fmt.Sprintf("%d:%d:%d", yearID, subcategoryID, userID)
}

func fetchApprovedUsageByUser(filter dashboardFilter, statuses dashboardStatusSets) map[string]usageAggregate {
	if len(statuses.Approved) == 0 {
		return map[string]usageAggregate{}
	}

	approvedIDs := ensureIDs(statuses.Approved)
	submissionTypes := []string{"fund_application", "publication_reward"}

	var rows []struct {
		YearID        int
		SubcategoryID *int
		UserID        *int
		UsedGrants    float64
		UsedAmount    float64
	}

	query := config.DB.Table("submissions s").
		Select(`s.year_id,
            s.subcategory_id,
            s.user_id,
            COUNT(*) AS used_grants,
            SUM(CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.approved_amount,0)
                     WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.total_approve_amount, prd.reward_approve_amount, prd.reward_amount)
                     ELSE 0 END) AS used_amount`).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL", submissionTypes).
		Where("s.status_id IN ?", approvedIDs).
		Where("s.subcategory_id IS NOT NULL")

	query = applyFilterToSubmissions(query, "s", filter)

	query.Group("s.year_id, s.subcategory_id, s.user_id").
		Scan(&rows)

	usage := make(map[string]usageAggregate, len(rows))
	for _, row := range rows {
		if row.SubcategoryID == nil || row.UserID == nil {
			continue
		}
		key := usageKey(row.YearID, *row.SubcategoryID, *row.UserID)
		usage[key] = usageAggregate{
			YearID:        row.YearID,
			SubcategoryID: *row.SubcategoryID,
			UserID:        *row.UserID,
			UsedGrants:    row.UsedGrants,
			UsedAmount:    row.UsedAmount,
		}
	}

	return usage
}

func buildAdminStatusBreakdown(filter dashboardFilter) map[string]map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}

	var rows []struct {
		SubmissionType string
		StatusCode     string
		Total          int64
	}

	query := config.DB.Table("submissions s").
		Select("s.submission_type, ast.status_code, COUNT(*) AS total").
		Joins("LEFT JOIN application_status ast ON s.status_id = ast.application_status_id").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL", submissionTypes)

	query = applyFilterToSubmissions(query, "s", filter)

	query.Group("s.submission_type, ast.status_code").
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

func buildAdminFinancialOverview(filter dashboardFilter, statuses dashboardStatusSets) map[string]interface{} {
	type amountSummary struct {
		Requested float64
		Approved  float64
		Pending   float64
		Rejected  float64
	}

	pendingIDs := ensureIDs(statuses.Pending)
	approvedIDs := ensureIDs(statuses.Approved)
	rejectedIDs := ensureIDs(statuses.Rejected)

	var fundAmounts amountSummary
	fundQuery := config.DB.Table("fund_application_details fad").
		Joins("JOIN submissions s ON fad.submission_id = s.submission_id").
		Where("s.submission_type = ? AND s.deleted_at IS NULL", "fund_application")
	fundQuery = applyFilterToSubmissions(fundQuery, "s", filter)
	fundQuery.Select("COALESCE(SUM(fad.requested_amount),0) AS requested, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN fad.approved_amount ELSE 0 END),0) AS approved, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN fad.requested_amount ELSE 0 END),0) AS pending, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN fad.requested_amount ELSE 0 END),0) AS rejected", approvedIDs, pendingIDs, rejectedIDs).
		Scan(&fundAmounts)

	var rewardAmounts amountSummary
	rewardQuery := config.DB.Table("publication_reward_details prd").
		Joins("JOIN submissions s ON prd.submission_id = s.submission_id").
		Where("s.submission_type = ? AND s.deleted_at IS NULL", "publication_reward")
	rewardQuery = applyFilterToSubmissions(rewardQuery, "s", filter)
	rewardQuery.Select("COALESCE(SUM(prd.reward_amount),0) AS requested, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN COALESCE(prd.total_approve_amount, prd.reward_approve_amount, prd.reward_amount) ELSE 0 END),0) AS approved, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN prd.reward_amount ELSE 0 END),0) AS pending, COALESCE(SUM(CASE WHEN s.status_id IN ? THEN prd.reward_amount ELSE 0 END),0) AS rejected", approvedIDs, pendingIDs, rejectedIDs).
		Scan(&rewardAmounts)

	var fundCount, fundApprovedCount, fundPendingCount, fundRejectedCount int64
	fundCountQuery := config.DB.Table("submissions s").
		Where("s.submission_type = ? AND s.deleted_at IS NULL", "fund_application")
	fundCountQuery = applyFilterToSubmissions(fundCountQuery, "s", filter)
	fundCountQuery.Count(&fundCount)

	if len(statuses.Approved) > 0 {
		approvedQuery := config.DB.Table("submissions s").
			Where("s.submission_type = ? AND s.status_id IN ? AND s.deleted_at IS NULL", "fund_application", approvedIDs)
		approvedQuery = applyFilterToSubmissions(approvedQuery, "s", filter)
		approvedQuery.Count(&fundApprovedCount)
	}

	pendingQuery := config.DB.Table("submissions s").
		Where("s.submission_type = ? AND s.status_id IN ? AND s.deleted_at IS NULL", "fund_application", pendingIDs)
	pendingQuery = applyFilterToSubmissions(pendingQuery, "s", filter)
	pendingQuery.Count(&fundPendingCount)

	rejectedQuery := config.DB.Table("submissions s").
		Where("s.submission_type = ? AND s.status_id IN ? AND s.deleted_at IS NULL", "fund_application", rejectedIDs)
	rejectedQuery = applyFilterToSubmissions(rejectedQuery, "s", filter)
	rejectedQuery.Count(&fundRejectedCount)

	var rewardCount, rewardApprovedCount, rewardPendingCount, rewardRejectedCount int64
	rewardCountQuery := config.DB.Table("submissions s").
		Where("s.submission_type = ? AND s.deleted_at IS NULL", "publication_reward")
	rewardCountQuery = applyFilterToSubmissions(rewardCountQuery, "s", filter)
	rewardCountQuery.Count(&rewardCount)

	if len(statuses.Approved) > 0 {
		rewardApprovedQuery := config.DB.Table("submissions s").
			Where("s.submission_type = ? AND s.status_id IN ? AND s.deleted_at IS NULL", "publication_reward", approvedIDs)
		rewardApprovedQuery = applyFilterToSubmissions(rewardApprovedQuery, "s", filter)
		rewardApprovedQuery.Count(&rewardApprovedCount)
	}

	rewardPendingQuery := config.DB.Table("submissions s").
		Where("s.submission_type = ? AND s.status_id IN ? AND s.deleted_at IS NULL", "publication_reward", pendingIDs)
	rewardPendingQuery = applyFilterToSubmissions(rewardPendingQuery, "s", filter)
	rewardPendingQuery.Count(&rewardPendingCount)

	rewardRejectedQuery := config.DB.Table("submissions s").
		Where("s.submission_type = ? AND s.status_id IN ? AND s.deleted_at IS NULL", "publication_reward", rejectedIDs)
	rewardRejectedQuery = applyFilterToSubmissions(rewardRejectedQuery, "s", filter)
	rewardRejectedQuery.Count(&rewardRejectedCount)

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

func buildAdminUpcomingInstallments(filter dashboardFilter) []map[string]interface{} {
	targetYear := filter.SelectedYear
	if targetYear == "" {
		targetYear = filter.CurrentYear
	}

	yearID := 0
	if filter.YearIDMap != nil {
		if id, ok := filter.YearIDMap[targetYear]; ok {
			yearID = id
		} else if filter.CurrentYear != "" {
			if id, ok := filter.YearIDMap[filter.CurrentYear]; ok {
				yearID = id
				targetYear = filter.CurrentYear
			}
		}
	}

	if yearID == 0 && len(filter.YearIDs) > 0 {
		yearID = filter.YearIDs[0]
	}

	var rows []struct {
		InstallmentNumber int
		Name              string
		CutoffDate        time.Time
		Year              string
	}

	query := config.DB.Table("fund_installment_periods fip").
		Select("fip.installment_number, fip.name, fip.cutoff_date, y.year").
		Joins("JOIN years y ON fip.year_id = y.year_id").
		Where("fip.deleted_at IS NULL")

	if yearID > 0 {
		query = query.Where("y.year_id = ?", yearID)
	} else {
		query = query.Order("y.year DESC")
	}

	query.Order("fip.cutoff_date ASC").
		Scan(&rows)

	if len(rows) == 0 {
		return []map[string]interface{}{}
	}

	now := time.Now()
	openIndex := -1
	for idx, row := range rows {
		if !row.CutoffDate.Before(now) {
			openIndex = idx
			break
		}
	}

	periods := make([]map[string]interface{}, 0, len(rows))
	for idx, row := range rows {
		cutoff := row.CutoffDate
		periodLabel := strings.TrimSpace(row.Name)
		if periodLabel == "" {
			periodLabel = fmt.Sprintf("รอบที่ %d", row.InstallmentNumber)
		}

		remainingDays := int(math.Ceil(cutoff.Sub(now).Hours() / 24))
		status := "closed"

		if openIndex == -1 {
			status = "closed"
		} else {
			switch {
			case idx < openIndex:
				status = "closed"
			case idx == openIndex:
				status = "open"
			default:
				status = "not_yet"
			}
		}

		periods = append(periods, map[string]interface{}{
			"installment":     row.InstallmentNumber,
			"name":            periodLabel,
			"cutoff_date":     row.CutoffDate.Format("2006-01-02"),
			"year":            row.Year,
			"days_remaining":  remainingDays,
			"status":          status,
			"cutoff_datetime": row.CutoffDate.Format(time.RFC3339),
		})
	}

	if len(periods) > 5 {
		periods = periods[:5]
	}

	return periods
}

func buildAdminActivityFeed(filter dashboardFilter) []map[string]interface{} {
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

	query := config.DB.Table("audit_logs al").
		Select("al.log_id, al.created_at, CONCAT(u.user_fname, ' ', u.user_lname) AS user_name, al.action, al.entity_type, al.entity_number, al.description, al.ip_address").
		Joins("LEFT JOIN users u ON al.user_id = u.user_id").
		Joins("LEFT JOIN submissions s ON (al.entity_type IN ('submission','fund_application','publication_reward') AND s.submission_number = al.entity_number)")

	if !filter.IncludeAll {
		query = query.Where("s.submission_id IS NOT NULL")
		query = applyFilterToSubmissions(query, "s", filter)
	} else if len(filter.YearIDs) > 0 {
		query = query.Where("s.submission_id IS NULL OR s.year_id IN ?", filter.YearIDs)
	}

	query.Order("al.created_at DESC").
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

func buildAdminTopUsers(filter dashboardFilter, statuses dashboardStatusSets) []map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}
	approvedIDs := ensureIDs(statuses.Approved)

	submissionsSubQuery := config.DB.Table("submissions s").
		Select("s.user_id, COUNT(*) AS submission_count, SUM(CASE WHEN s.status_id IN ? THEN 1 ELSE 0 END) AS approved_count", approvedIDs).
		Where("s.submission_type IN ? AND s.deleted_at IS NULL", submissionTypes)
	submissionsSubQuery = applyFilterToSubmissions(submissionsSubQuery, "s", filter)
	submissionsSubQuery = submissionsSubQuery.Group("s.user_id")

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
		ApprovedCount   int64
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
            COALESCE(subs.submission_count,0) AS submission_count,
            COALESCE(subs.approved_count,0) AS approved_count`).
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
			"approved_count":   row.ApprovedCount,
			"last_login":       lastLogin,
		})
	}

	return summaries
}

func buildSystemTrendBreakdown(filter dashboardFilter, statuses dashboardStatusSets) map[string][]map[string]interface{} {
	breakdown := make(map[string][]map[string]interface{})

	if monthly := buildMonthlyTrend(filter, statuses); len(monthly) > 0 {
		breakdown["monthly"] = monthly
	}

	if yearly := buildYearlyTrend(filter, statuses); len(yearly) > 0 {
		breakdown["yearly"] = yearly
	}

	if quarterly := buildQuarterlyTrend(filter, statuses); len(quarterly) > 0 {
		breakdown["quarterly"] = quarterly
	}

	if installments := buildInstallmentTrend(filter, statuses); len(installments) > 0 {
		breakdown["installment"] = installments
	}

	return breakdown
}

func parseThaiYear(year string) (int, bool) {
	trimmed := strings.TrimSpace(year)
	if trimmed == "" {
		return 0, false
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}
	if value >= 2400 {
		value -= 543
	}
	return value, true
}

func monthPeriodsForFilter(filter dashboardFilter) []string {
	if filter.IncludeAll || len(filter.Years) == 0 {
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -11, 0)
		periods := make([]string, 0, 12)
		for i := 0; i < 12; i++ {
			periods = append(periods, start.AddDate(0, i, 0).Format("2006-01"))
		}
		return periods
	}

	unique := make(map[string]struct{})
	years := make([]int, 0, len(filter.Years))
	for _, yearStr := range filter.Years {
		if parsed, ok := parseThaiYear(yearStr); ok {
			years = append(years, parsed)
		}
	}

	if len(years) == 0 {
		return []string{}
	}

	sort.Ints(years)
	periods := make([]string, 0, len(years)*12)
	for _, year := range years {
		for month := time.January; month <= time.December; month++ {
			key := time.Date(year, month, 1, 0, 0, 0, 0, time.Now().Location()).Format("2006-01")
			if _, exists := unique[key]; exists {
				continue
			}
			unique[key] = struct{}{}
			periods = append(periods, key)
		}
	}

	sort.Strings(periods)
	return periods
}

func buildMonthlyTrend(filter dashboardFilter, statuses dashboardStatusSets) []map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}
	approvedIDs := ensureIDs(statuses.Approved)
	dateExpr := submissionDateExpression

	query := config.DB.Table("submissions s").
		Select(fmt.Sprintf(`DATE_FORMAT(%s, '%%Y-%%m') AS period,
            y.year AS thai_year,
            SUM(CASE WHEN s.submission_type = 'fund_application' THEN 1 ELSE 0 END) AS fund_total,
            SUM(CASE WHEN s.submission_type = 'publication_reward' THEN 1 ELSE 0 END) AS reward_total,
            SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id IN ? THEN 1 ELSE 0 END) AS fund_approved,
            SUM(CASE WHEN s.submission_type = 'publication_reward' AND s.status_id IN ? THEN 1 ELSE 0 END) AS reward_approved,
            SUM(CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.requested_amount,0)
                     WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.reward_amount,0)
                     ELSE 0 END) AS total_requested,
            SUM(CASE WHEN s.status_id IN ? THEN
                        CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.approved_amount,0)
                             WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.total_approve_amount, prd.reward_approve_amount, prd.reward_amount, 0)
                             ELSE 0 END
                     ELSE 0 END) AS total_approved`, dateExpr), approvedIDs, approvedIDs, approvedIDs).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Joins("LEFT JOIN years y ON s.year_id = y.year_id").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL", submissionTypes)

	query = applyFilterToSubmissions(query, "s", filter)

	if filter.IncludeAll || len(filter.YearIDs) == 0 {
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -11, 0)
		query = query.Where(fmt.Sprintf("%s >= ?", dateExpr), start.Format("2006-01-02"))
	}

	var rows []struct {
		Period         string
		ThaiYear       *string
		FundTotal      float64
		RewardTotal    float64
		FundApproved   float64
		RewardApproved float64
		TotalRequested float64
		TotalApproved  float64
	}

	query.Group(fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m'), y.year", dateExpr)).
		Order("period ASC").
		Scan(&rows)

	dataByPeriod := make(map[string]struct {
		ThaiYear       *string
		FundTotal      float64
		RewardTotal    float64
		FundApproved   float64
		RewardApproved float64
		TotalRequested float64
		TotalApproved  float64
	})

	for _, row := range rows {
		dataByPeriod[row.Period] = struct {
			ThaiYear       *string
			FundTotal      float64
			RewardTotal    float64
			FundApproved   float64
			RewardApproved float64
			TotalRequested float64
			TotalApproved  float64
		}{
			ThaiYear:       row.ThaiYear,
			FundTotal:      row.FundTotal,
			RewardTotal:    row.RewardTotal,
			FundApproved:   row.FundApproved,
			RewardApproved: row.RewardApproved,
			TotalRequested: row.TotalRequested,
			TotalApproved:  row.TotalApproved,
		}
	}

	periods := monthPeriodsForFilter(filter)
	if len(periods) == 0 {
		for period := range dataByPeriod {
			periods = append(periods, period)
		}
		sort.Strings(periods)
	}

	results := make([]map[string]interface{}, 0, len(periods))
	for _, period := range periods {
		data := dataByPeriod[period]
		totalApplications := data.FundTotal + data.RewardTotal
		approvedApplications := data.FundApproved + data.RewardApproved

		thaiYear := ""
		if data.ThaiYear != nil {
			thaiYear = *data.ThaiYear
		}

		results = append(results, map[string]interface{}{
			"period":             period,
			"thai_year":          thaiYear,
			"total_applications": totalApplications,
			"approved":           approvedApplications,
			"fund_total":         data.FundTotal,
			"reward_total":       data.RewardTotal,
			"fund_approved":      data.FundApproved,
			"reward_approved":    data.RewardApproved,
			"total_requested":    data.TotalRequested,
			"total_approved":     data.TotalApproved,
		})
	}

	return results
}

func buildYearlyTrend(filter dashboardFilter, statuses dashboardStatusSets) []map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}
	approvedIDs := ensureIDs(statuses.Approved)

	query := config.DB.Table("submissions s").
		Select(`y.year AS year,
            SUM(CASE WHEN s.submission_type = 'fund_application' THEN 1 ELSE 0 END) AS fund_total,
            SUM(CASE WHEN s.submission_type = 'publication_reward' THEN 1 ELSE 0 END) AS reward_total,
            SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id IN ? THEN 1 ELSE 0 END) AS fund_approved,
            SUM(CASE WHEN s.submission_type = 'publication_reward' AND s.status_id IN ? THEN 1 ELSE 0 END) AS reward_approved,
            SUM(CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.requested_amount,0)
                     WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.reward_amount,0)
                     ELSE 0 END) AS total_requested,
            SUM(CASE WHEN s.status_id IN ? THEN
                        CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.approved_amount,0)
                             WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.total_approve_amount, prd.reward_approve_amount, prd.reward_amount, 0)
                             ELSE 0 END
                     ELSE 0 END) AS total_approved`, approvedIDs, approvedIDs, approvedIDs).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Joins("LEFT JOIN years y ON s.year_id = y.year_id").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL", submissionTypes)

	query = applyFilterToSubmissions(query, "s", filter)

	var rows []struct {
		Year           string
		FundTotal      float64
		RewardTotal    float64
		FundApproved   float64
		RewardApproved float64
		TotalRequested float64
		TotalApproved  float64
	}

	query.Group("y.year").
		Order("y.year ASC").
		Scan(&rows)

	results := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		totalApplications := row.FundTotal + row.RewardTotal
		approvedApplications := row.FundApproved + row.RewardApproved

		results = append(results, map[string]interface{}{
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

func buildQuarterlyTrend(filter dashboardFilter, statuses dashboardStatusSets) []map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}
	approvedIDs := ensureIDs(statuses.Approved)
	dateExpr := submissionDateExpression

	query := config.DB.Table("submissions s").
		Select(fmt.Sprintf(`y.year AS year,
            QUARTER(%s) AS quarter,
            SUM(CASE WHEN s.submission_type = 'fund_application' THEN 1 ELSE 0 END) AS fund_total,
            SUM(CASE WHEN s.submission_type = 'publication_reward' THEN 1 ELSE 0 END) AS reward_total,
            SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id IN ? THEN 1 ELSE 0 END) AS fund_approved,
            SUM(CASE WHEN s.submission_type = 'publication_reward' AND s.status_id IN ? THEN 1 ELSE 0 END) AS reward_approved,
            SUM(CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.requested_amount,0)
                     WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.reward_amount,0)
                     ELSE 0 END) AS total_requested,
            SUM(CASE WHEN s.status_id IN ? THEN
                        CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.approved_amount,0)
                             WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.total_approve_amount, prd.reward_approve_amount, prd.reward_amount, 0)
                             ELSE 0 END
                     ELSE 0 END) AS total_approved`, dateExpr), approvedIDs, approvedIDs, approvedIDs).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Joins("LEFT JOIN years y ON s.year_id = y.year_id").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL", submissionTypes)

	query = applyFilterToSubmissions(query, "s", filter)

	var rows []struct {
		Year           string
		Quarter        int
		FundTotal      float64
		RewardTotal    float64
		FundApproved   float64
		RewardApproved float64
		TotalRequested float64
		TotalApproved  float64
	}

	query.Group(fmt.Sprintf("y.year, QUARTER(%s)", dateExpr)).
		Order("y.year ASC, quarter ASC").
		Scan(&rows)

	results := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		totalApplications := row.FundTotal + row.RewardTotal
		approvedApplications := row.FundApproved + row.RewardApproved

		results = append(results, map[string]interface{}{
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

func buildInstallmentTrend(filter dashboardFilter, statuses dashboardStatusSets) []map[string]interface{} {
	submissionTypes := []string{"fund_application", "publication_reward"}
	approvedIDs := ensureIDs(statuses.Approved)

	query := config.DB.Table("submissions s").
		Select(`y.year AS year,
            s.installment_number_at_submit AS installment,
            COALESCE(fip.name, CONCAT('รอบที่ ', s.installment_number_at_submit)) AS period_name,
            SUM(CASE WHEN s.submission_type = 'fund_application' THEN 1 ELSE 0 END) AS fund_total,
            SUM(CASE WHEN s.submission_type = 'publication_reward' THEN 1 ELSE 0 END) AS reward_total,
            SUM(CASE WHEN s.submission_type = 'fund_application' AND s.status_id IN ? THEN 1 ELSE 0 END) AS fund_approved,
            SUM(CASE WHEN s.submission_type = 'publication_reward' AND s.status_id IN ? THEN 1 ELSE 0 END) AS reward_approved,
            SUM(CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.requested_amount,0)
                     WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.reward_amount,0)
                     ELSE 0 END) AS total_requested,
            SUM(CASE WHEN s.status_id IN ? THEN
                        CASE WHEN s.submission_type = 'fund_application' THEN COALESCE(fad.approved_amount,0)
                             WHEN s.submission_type = 'publication_reward' THEN COALESCE(prd.total_approve_amount, prd.reward_approve_amount, prd.reward_amount, 0)
                             ELSE 0 END
                     ELSE 0 END) AS total_approved`, approvedIDs, approvedIDs, approvedIDs).
		Joins("LEFT JOIN fund_application_details fad ON s.submission_id = fad.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON s.submission_id = prd.submission_id").
		Joins("LEFT JOIN years y ON s.year_id = y.year_id").
		Joins("LEFT JOIN fund_installment_periods fip ON fip.year_id = s.year_id AND fip.installment_number = s.installment_number_at_submit AND fip.deleted_at IS NULL").
		Where("s.submission_type IN ? AND s.deleted_at IS NULL AND s.installment_number_at_submit IS NOT NULL", submissionTypes)

	query = applyFilterToSubmissions(query, "s", filter)

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

	query.Group("y.year, s.installment_number_at_submit, period_name").
		Order("y.year ASC, s.installment_number_at_submit ASC").
		Scan(&rows)

	results := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		installmentNumber := 0
		if row.Installment != nil {
			installmentNumber = *row.Installment
		}

		periodLabel := "ไม่ระบุรอบ"
		if row.PeriodName != nil && strings.TrimSpace(*row.PeriodName) != "" {
			periodLabel = strings.TrimSpace(*row.PeriodName)
		} else if installmentNumber > 0 {
			periodLabel = fmt.Sprintf("รอบที่ %d", installmentNumber)
		}

		totalApplications := row.FundTotal + row.RewardTotal
		approvedApplications := row.FundApproved + row.RewardApproved

		results = append(results, map[string]interface{}{
			"year":               row.Year,
			"installment":        installmentNumber,
			"period_label":       periodLabel,
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
