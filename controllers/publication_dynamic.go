// Dynamic publication reward helper endpoints
package controllers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// matchesFund determines if a fund description matches a given quartile bucket.
// NOTE: budgets are NOT split by author status, so we intentionally ignore authorStatus here.
func matchesFund(desc, authorStatus, quartile string) bool {
	if desc == "" || quartile == "" {
		return false
	}

	normalizedQuartile := strings.ToUpper(strings.TrimSpace(quartile))
	d := strings.ToLower(desc)

	// normalize some Thai/typographical variants and spacing
	replacer := strings.NewReplacer(
		"ลําดับ", "ลำดับ",
		"％", "%",
		"  ", " ",
		"กลุ่มที่", "กลุ่ม",
	)
	d = replacer.Replace(d)

	switch normalizedQuartile {
	case "T5":
		// วารสาร ... (ลำดับ 5% แรก)
		return strings.Contains(d, "5%") || strings.Contains(d, "5 %") || strings.Contains(d, "top 5")
	case "T10":
		// วารสาร ... (ลำดับ 10% แรก)
		return strings.Contains(d, "10%") || strings.Contains(d, "10 %") || strings.Contains(d, "top 10")
	case "Q1", "Q2", "Q3", "Q4":
		return strings.Contains(d, "ควอร์ไทล์") && strings.Contains(d, strings.ToLower(normalizedQuartile)) ||
			strings.Contains(d, strings.ToLower(normalizedQuartile))
	}

	if strings.HasPrefix(normalizedQuartile, "TCI") || normalizedQuartile == "TCI" {
		if !strings.Contains(d, "tci") {
			return false
		}

		group1 := strings.Contains(d, "กลุ่ม 1") || strings.Contains(d, "group 1") || strings.Contains(d, "tci 1")
		group2 := strings.Contains(d, "กลุ่ม 2") || strings.Contains(d, "group 2") || strings.Contains(d, "tci 2")
		hasGroup := group1 || group2

		// accept both science/tech and social/humanities wording
		sci := strings.Contains(d, "วิทยาศาสตร์") || strings.Contains(d, "เทคโนโลยี") || strings.Contains(d, "science") || strings.Contains(d, "technology")
		social := strings.Contains(d, "สังคม") || strings.Contains(d, "มนุษยศาสตร์") || strings.Contains(d, "social") || strings.Contains(d, "human")

		return hasGroup && (sci || social)
	}

	return false
}

// GetEnabledYearsForCategory returns years that have budgets for a category
func GetEnabledYearsForCategory(c *gin.Context) {
	categoryID := c.Query("category_id")
	if categoryID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category_id is required"})
		return
	}

	var years []models.Year
	err := config.DB.Table("years y").
		Joins(`
			JOIN fund_subcategories fs 
			  ON fs.year_id = y.year_id
			 AND fs.category_id = ?
			 AND fs.status = 'active'
			 AND fs.delete_at IS NULL
			 AND (fs.form_type = 'publication_reward' OR fs.form_type IS NULL)
		`, categoryID).
		Joins(`
			JOIN subcategory_budgets sb
			  ON sb.subcategory_id = fs.subcategory_id
			 AND sb.status = 'active'
			 AND sb.delete_at IS NULL
		`).
		Where("y.delete_at IS NULL").
		Distinct().
		Find(&years).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch years"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"years": years, "total": len(years)})
}

// GetPublicationOptions returns valid author status and quartile combinations
func GetPublicationOptions(c *gin.Context) {
	categoryID := c.Query("category_id")
	yearID := c.Query("year_id")
	if categoryID == "" || yearID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category_id and year_id are required"})
		return
	}

	// Resolve year string from year_id
	var year models.Year
	if err := config.DB.First(&year, "year_id = ? AND delete_at IS NULL", yearID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year_id"})
		return
	}

	// Load active rates for the given year, fallback to reward_config when missing
	var rates []models.PublicationRewardRate
	rateYear := year.Year
	if err := config.DB.
		Where("year = ? AND is_active = ?", rateYear, true).
		Find(&rates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch rates"})
		return
	}
	if len(rates) == 0 {
		type configRow struct {
			Year            string
			JournalQuartile string
			MaxAmount       float64
		}
		var configs []configRow
		if err := config.DB.Table(models.RewardConfig{}.TableName()).
			Select("year, journal_quartile, max_amount").
			Where("year = ? AND is_active = ? AND delete_at IS NULL", rateYear, true).
			Find(&configs).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reward config"})
			return
		}

		if len(configs) == 0 {
			if err := config.DB.Table(models.RewardConfig{}.TableName()).
				Select("year, journal_quartile, max_amount").
				Where("is_active = ? AND delete_at IS NULL", true).
				Order("year DESC").
				Limit(1).
				Find(&configs).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch fallback reward config"})
				return
			}
		}

		if len(configs) > 0 {
			defaultStatuses := []string{"first_author", "corresponding_author"}
			for _, cfg := range configs {
				for _, status := range defaultStatuses {
					rates = append(rates, models.PublicationRewardRate{
						Year:            cfg.Year,
						AuthorStatus:    status,
						JournalQuartile: strings.ToUpper(cfg.JournalQuartile),
						RewardAmount:    cfg.MaxAmount,
						IsActive:        true,
					})
				}
			}
		}
	}

	// Load active budgets for the category/year
	type budgetRow struct {
		SubcategoryID   int
		BudgetID        int
		FundDescription string
	}
	var budgets []budgetRow
	if err := config.DB.Table("fund_subcategories fs").
		Select("fs.subcategory_id, sb.subcategory_budget_id AS budget_id, sb.fund_description").
		Joins(`
                        JOIN subcategory_budgets sb
                          ON sb.subcategory_id = fs.subcategory_id
                         AND sb.status = 'active'
                         AND sb.delete_at IS NULL
                `).
		Where(`
                        fs.delete_at IS NULL
                        AND fs.status = 'active'
                        AND fs.category_id = ?
                        AND fs.year_id = ?
                        AND (fs.form_type = 'publication_reward' OR fs.form_type IS NULL)
                `, categoryID, yearID).
		Find(&budgets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch budgets"})
		return
	}

	// Match budgets to rate rows by fund_description bucket
	options := []gin.H{}
	for _, rate := range rates {
		for _, b := range budgets {
			if matchesFund(b.FundDescription, rate.AuthorStatus, rate.JournalQuartile) {
				options = append(options, gin.H{
					"author_status":         rate.AuthorStatus,
					"journal_quartile":      rate.JournalQuartile,
					"reward_amount":         rate.RewardAmount,
					"subcategory_id":        b.SubcategoryID,
					"subcategory_budget_id": b.BudgetID,
					"fund_description":      b.FundDescription,
				})
				break
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"options": options, "total": len(options)})
}

// ResolvePublicationBudget resolves IDs for given parameters
func ResolvePublicationBudget(c *gin.Context) {
	categoryID := c.Query("category_id")
	yearID := c.Query("year_id")
	authorStatus := c.Query("author_status")
	quartile := c.Query("journal_quartile")
	if categoryID == "" || yearID == "" || authorStatus == "" || quartile == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing parameters"})
		return
	}

	userIDVal, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user context missing"})
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user context"})
		return
	}

	// Resolve year string from year_id
	var year models.Year
	if err := config.DB.First(&year, "year_id = ? AND delete_at IS NULL", yearID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year_id"})
		return
	}

	// Find the active rate row for (year, authorStatus, quartile) with fallback to reward_config/latest year
	var rate models.PublicationRewardRate
	err := config.DB.
		Where("year = ? AND author_status = ? AND journal_quartile = ? AND is_active = ?", year.Year, authorStatus, quartile, true).
		First(&rate).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch rate"})
			return
		}

		fallback := func() error {
			if err := config.DB.
				Where("author_status = ? AND journal_quartile = ? AND is_active = ?", authorStatus, quartile, true).
				Order("year DESC").
				First(&rate).Error; err == nil {
				return nil
			}

			type cfgRow struct {
				Year            string
				JournalQuartile string
				MaxAmount       float64
			}
			var cfg cfgRow
			cfgErr := config.DB.Table(models.RewardConfig{}.TableName()).
				Select("year, journal_quartile, max_amount").
				Where("year = ? AND journal_quartile = ? AND is_active = ? AND delete_at IS NULL", year.Year, quartile, true).
				First(&cfg).Error
			if errors.Is(cfgErr, gorm.ErrRecordNotFound) {
				cfgErr = config.DB.Table(models.RewardConfig{}.TableName()).
					Select("year, journal_quartile, max_amount").
					Where("journal_quartile = ? AND is_active = ? AND delete_at IS NULL", quartile, true).
					Order("year DESC").
					First(&cfg).Error
			}
			if cfgErr != nil {
				return cfgErr
			}

			rate = models.PublicationRewardRate{
				Year:            cfg.Year,
				AuthorStatus:    authorStatus,
				JournalQuartile: strings.ToUpper(cfg.JournalQuartile),
				RewardAmount:    cfg.MaxAmount,
				IsActive:        true,
			}
			return nil
		}

		if err := fallback(); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "rate not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch fallback rate"})
			return
		}
	}

	// Load budgets for the category/year
	type budgetRow struct {
		SubcategoryID     int
		BudgetID          int
		FundDescription   string
		RecordScope       string
		AllocatedAmount   sql.NullFloat64
		MaxAmountPerGrant sql.NullFloat64
		MaxAmountPerYear  sql.NullFloat64
		MaxGrants         sql.NullInt64
	}
	var budgets []budgetRow
	if err := config.DB.Table("fund_subcategories fs").
		Select(`
                        fs.subcategory_id,
                        sb.subcategory_budget_id AS budget_id,
                        sb.fund_description,
                        sb.record_scope,
                        sb.allocated_amount,
                        sb.max_amount_per_grant,
                        sb.max_amount_per_year,
                        sb.max_grants
                `).
		Joins(`
                        JOIN subcategory_budgets sb
                          ON sb.subcategory_id = fs.subcategory_id
                         AND sb.status = 'active'
                         AND sb.delete_at IS NULL
                `).
		Where(`
                        fs.delete_at IS NULL
                        AND fs.status = 'active'
                        AND fs.category_id = ?
                        AND fs.year_id = ?
                        AND (fs.form_type = 'publication_reward' OR fs.form_type IS NULL)
                `, categoryID, yearID).
		Find(&budgets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch budgets"})
		return
	}

	var overallRow *budgetRow
	var ruleRow *budgetRow
	var fallbackRow *budgetRow
	for i, b := range budgets {
		if b.RecordScope == "overall" && overallRow == nil {
			overallRow = &budgets[i]
		}
		if b.RecordScope == "rule" && matchesFund(b.FundDescription, authorStatus, quartile) {
			ruleRow = &budgets[i]
		}
		if fallbackRow == nil && matchesFund(b.FundDescription, authorStatus, quartile) {
			fallbackRow = &budgets[i]
		}
	}

	chosenRow := overallRow
	if ruleRow != nil {
		chosenRow = ruleRow
	}
	if chosenRow == nil {
		if fallbackRow != nil {
			chosenRow = fallbackRow
		} else if len(budgets) > 0 {
			chosenRow = &budgets[0]
		}
		overallRow = chosenRow
	}

	if chosenRow == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active budget found"})
		return
	}

	approvedStatusID, err := utils.GetStatusIDByCode(utils.StatusCodeApproved)
	if err != nil || approvedStatusID <= 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to resolve approved status"})
		return
	}

	// Budget usage for the pool (all submission types)
	type poolUsage struct {
		Used float64
	}
	var pool poolUsage
	if err := config.DB.Table("submissions s").
		Select(`COALESCE(SUM(
                        CASE
                                WHEN s.submission_type = 'fund_application' THEN fad.approved_amount
                                WHEN s.submission_type = 'publication_reward' THEN prd.total_approve_amount
                                ELSE 0
                        END
                ), 0) AS used`).
		Joins("LEFT JOIN fund_application_details fad ON fad.submission_id = s.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON prd.submission_id = s.submission_id").
		Where("s.year_id = ? AND s.subcategory_id = ? AND s.status_id = ? AND s.deleted_at IS NULL", yearID, overallRow.SubcategoryID, approvedStatusID).
		Scan(&pool).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute budget usage"})
		return
	}

	allocated := 0.0
	if overallRow.AllocatedAmount.Valid {
		allocated = overallRow.AllocatedAmount.Float64
	}
	remainingAmount := allocated - pool.Used
	if remainingAmount < 0 {
		remainingAmount = 0
	}

	type userTotals struct {
		TotalGrants int64
		TotalAmount float64
	}
	var totals userTotals
	if err := config.DB.Table("submissions s").
		Select(`COUNT(*) AS total_grants, COALESCE(SUM(
                        CASE
                                WHEN s.submission_type = 'fund_application' THEN fad.approved_amount
                                WHEN s.submission_type = 'publication_reward' THEN prd.total_approve_amount
                                ELSE 0
                        END
                ), 0) AS total_amount`).
		Joins("LEFT JOIN fund_application_details fad ON fad.submission_id = s.submission_id").
		Joins("LEFT JOIN publication_reward_details prd ON prd.submission_id = s.submission_id").
		Where("s.user_id = ? AND s.year_id = ? AND s.subcategory_id = ? AND s.status_id = ? AND s.deleted_at IS NULL", userID, yearID, overallRow.SubcategoryID, approvedStatusID).
		Scan(&totals).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute user totals"})
		return
	}

	type publicationUsage struct {
		Grants int64
		Amount float64
	}
	var pubUsage publicationUsage
	if err := config.DB.Table("submissions s").
		Select("COUNT(*) AS grants, COALESCE(SUM(prd.total_approve_amount), 0) AS amount").
		Joins("JOIN publication_reward_details prd ON prd.submission_id = s.submission_id").
		Where("s.user_id = ? AND s.year_id = ? AND s.subcategory_id = ? AND s.submission_type = 'publication_reward' AND s.status_id = ? AND s.deleted_at IS NULL", userID, yearID, overallRow.SubcategoryID, approvedStatusID).
		Scan(&pubUsage).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute publication usage"})
		return
	}

	var remainingGrants interface{}
	if overallRow.MaxGrants.Valid {
		rem := overallRow.MaxGrants.Int64 - totals.TotalGrants
		if rem < 0 {
			rem = 0
		}
		remainingGrants = rem
	}

	var remainingUserAmount interface{}
	if overallRow.MaxAmountPerYear.Valid {
		rem := overallRow.MaxAmountPerYear.Float64 - totals.TotalAmount
		if rem < 0 {
			rem = 0
		}
		remainingUserAmount = rem
	}

	response := gin.H{
		"subcategory_id":        chosenRow.SubcategoryID,
		"subcategory_budget_id": chosenRow.BudgetID,
		"fund_description":      chosenRow.FundDescription,
		"reward_amount":         rate.RewardAmount,
		"policy": gin.H{
			"overall": gin.H{
				"subcategory_budget_id": overallRow.BudgetID,
				"allocated_amount":      allocated,
				"used_amount":           pool.Used,
				"remaining_amount":      remainingAmount,
				"max_amount_per_year": func() interface{} {
					if overallRow.MaxAmountPerYear.Valid {
						return overallRow.MaxAmountPerYear.Float64
					}
					return nil
				}(),
				"max_grants": func() interface{} {
					if overallRow.MaxGrants.Valid {
						return overallRow.MaxGrants.Int64
					}
					return nil
				}(),
				"max_amount_per_grant": func() interface{} {
					if overallRow.MaxAmountPerGrant.Valid {
						return overallRow.MaxAmountPerGrant.Float64
					}
					return nil
				}(),
			},
			"rule": gin.H{
				"subcategory_budget_id": chosenRow.BudgetID,
				"fund_description":      chosenRow.FundDescription,
				"max_amount_per_grant": func() interface{} {
					if chosenRow.MaxAmountPerGrant.Valid {
						return chosenRow.MaxAmountPerGrant.Float64
					}
					return nil
				}(),
			},
			"user_usage": gin.H{
				"total_grants":       totals.TotalGrants,
				"total_amount":       totals.TotalAmount,
				"publication_grants": pubUsage.Grants,
				"publication_amount": pubUsage.Amount,
			},
			"user_remaining": gin.H{
				"grants": remainingGrants,
				"amount": remainingUserAmount,
			},
		},
	}

	c.JSON(http.StatusOK, response)
}

// CheckBudgetAvailability returns basic availability info
func CheckBudgetAvailability(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	var budget models.SubcategoryBudget
	if err := config.DB.Where("subcategory_budget_id = ? AND delete_at IS NULL", id).First(&budget).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "budget not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"subcategory_budget_id": budget.SubcategoryBudgetID,
		"is_active":             budget.Status == "active",
		"has_budget":            budget.RemainingBudget > 0,
		"remaining_amount":      budget.RemainingBudget,
	})
}
