// Dynamic publication reward helper endpoints
package controllers

import (
	"net/http"
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
)

// matchesFund determines if a fund description matches a given quartile bucket.
// NOTE: budgets are NOT split by author status, so we intentionally ignore authorStatus here.
func matchesFund(desc, authorStatus, quartile string) bool {
	if desc == "" || quartile == "" {
		return false
	}
	d := strings.ToLower(desc)

	// normalize some Thai/typographical variants
	d = strings.ReplaceAll(d, "ลําดับ", "ลำดับ") // normalize variants
	d = strings.ReplaceAll(d, "％", "%")
	d = strings.ReplaceAll(d, "  ", " ")

	switch quartile {
	case "T5":
		// วารสาร ... (ลำดับ 5% แรก)
		return strings.Contains(d, "5%") || strings.Contains(d, "5 %")
	case "T10":
		// วารสาร ... (ลำดับ 10% แรก)
		return strings.Contains(d, "10%") || strings.Contains(d, "10 %")
	case "Q1":
		return strings.Contains(d, "ควอร์ไทล์ 1") || strings.Contains(d, "q1")
	case "Q2":
		return strings.Contains(d, "ควอร์ไทล์ 2") || strings.Contains(d, "q2")
	case "Q3":
		return strings.Contains(d, "ควอร์ไทล์ 3") || strings.Contains(d, "q3")
	case "Q4":
		return strings.Contains(d, "ควอร์ไทล์ 4") || strings.Contains(d, "q4")
	case "TCI":
		// TCI กลุ่มที่ 1 สาขาวิทยาศาสตร์เทคโนโลยี
		return strings.Contains(d, "tci") &&
			(strings.Contains(d, "กลุ่มที่ 1") || strings.Contains(d, "group 1")) &&
			(strings.Contains(d, "วิทยาศาสตร์") || strings.Contains(d, "เทคโนโลยี") ||
				strings.Contains(d, "science") || strings.Contains(d, "technology"))
	default:
		return false
	}
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

	// Load active rates for the given year
	var rates []models.PublicationRewardRate
	if err := config.DB.
		Where("year = ? AND is_active = ?", year.Year, true).
		Find(&rates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch rates"})
		return
	}

	// Load active budgets for the category/year
	type budgetRow struct {
		SubcategoryID   int
		BudgetID        int
		FundDescription string
	}
	var budgets []budgetRow
	err := config.DB.Table("fund_subcategories fs").
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
		Find(&budgets).Error
	if err != nil {
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

	// Resolve year string from year_id
	var year models.Year
	if err := config.DB.First(&year, "year_id = ? AND delete_at IS NULL", yearID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year_id"})
		return
	}

	// Find the active rate row for (year, authorStatus, quartile)
	var rate models.PublicationRewardRate
	if err := config.DB.
		Where("year = ? AND author_status = ? AND journal_quartile = ? AND is_active = ?", year.Year, authorStatus, quartile, true).
		First(&rate).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rate not found"})
		return
	}

	// Load budgets for the category/year
	type budgetRow struct {
		SubcategoryID   int
		BudgetID        int
		FundDescription string
		RemainingBudget float64
	}
	var budgets []budgetRow
	err := config.DB.Table("fund_subcategories fs").
		Select("fs.subcategory_id, sb.subcategory_budget_id AS budget_id, sb.fund_description, sb.remaining_budget").
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
		Find(&budgets).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch budgets"})
		return
	}

	for _, b := range budgets {
		if matchesFund(b.FundDescription, authorStatus, quartile) {
			c.JSON(http.StatusOK, gin.H{
				"subcategory_id":        b.SubcategoryID,
				"subcategory_budget_id": b.BudgetID,
				"fund_description":      b.FundDescription,
				"reward_amount":         rate.RewardAmount,
				"remaining_budget":      b.RemainingBudget,
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "no matching fund"})
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
