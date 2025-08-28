// Dynamic publication reward helper endpoints
package controllers

import (
	"net/http"
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
)

// matchesFund checks if a budget description matches author status and quartile
func matchesFund(desc, authorStatus, quartile string) bool {
	if desc == "" {
		return false
	}
	d := strings.ToLower(desc)
	// Quartile keywords
	switch quartile {
	case "T5":
		if !strings.Contains(d, "5%") {
			return false
		}
	case "T10":
		if !strings.Contains(d, "10%") {
			return false
		}
	case "Q1":
		if !strings.Contains(d, "q1") && !strings.Contains(d, "ควอร์ไทล์ 1") {
			return false
		}
	case "Q2":
		if !strings.Contains(d, "q2") && !strings.Contains(d, "ควอร์ไทล์ 2") {
			return false
		}
	case "Q3":
		if !strings.Contains(d, "q3") && !strings.Contains(d, "ควอร์ไทล์ 3") {
			return false
		}
	case "Q4":
		if !strings.Contains(d, "q4") && !strings.Contains(d, "ควอร์ไทล์ 4") {
			return false
		}
	case "TCI":
		if !strings.Contains(d, "tci") {
			return false
		}
	}
	switch authorStatus {
	case "first_author":
		if strings.Contains(d, "ผู้แต่ง") || strings.Contains(d, "first") {
			return true
		}
	case "corresponding_author":
		if strings.Contains(d, "ประพันธ์") || strings.Contains(d, "corresponding") {
			return true
		}
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
	err := config.DB.Table("years").
		Joins("JOIN fund_categories fc ON fc.year_id = years.year_id AND fc.category_id = ? AND fc.delete_at IS NULL", categoryID).
		Joins("JOIN fund_subcategories fs ON fs.category_id = fc.category_id AND fs.status = 'active' AND fs.delete_at IS NULL").
		Joins("JOIN subcategory_budgets sb ON sb.subcategory_id = fs.subcategory_id AND sb.status = 'active' AND sb.delete_at IS NULL").
		Where("years.delete_at IS NULL").
		Distinct().Find(&years).Error
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

	var year models.Year
	if err := config.DB.First(&year, "year_id = ? AND delete_at IS NULL", yearID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year_id"})
		return
	}

	var rates []models.PublicationRewardRate
	if err := config.DB.Where("year = ? AND is_active = ?", year.Year, true).Find(&rates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch rates"})
		return
	}

	type budgetRow struct {
		SubcategoryID   int
		BudgetID        int
		FundDescription string
	}
	var budgets []budgetRow
	err := config.DB.Table("fund_subcategories fs").
		Select("fs.subcategory_id, sb.subcategory_budget_id as budget_id, sb.fund_description").
		Joins("JOIN fund_categories fc ON fs.category_id = fc.category_id AND fc.category_id = ? AND fc.year_id = ?", categoryID, yearID).
		Joins("JOIN subcategory_budgets sb ON sb.subcategory_id = fs.subcategory_id AND sb.status = 'active' AND sb.delete_at IS NULL").
		Where("fs.delete_at IS NULL AND fs.status = 'active'").
		Find(&budgets).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch budgets"})
		return
	}

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

	var year models.Year
	if err := config.DB.First(&year, "year_id = ? AND delete_at IS NULL", yearID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year_id"})
		return
	}

	var rate models.PublicationRewardRate
	if err := config.DB.Where("year = ? AND author_status = ? AND journal_quartile = ? AND is_active = ?", year.Year, authorStatus, quartile, true).
		First(&rate).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rate not found"})
		return
	}

	type budgetRow struct {
		SubcategoryID   int
		BudgetID        int
		FundDescription string
		RemainingBudget float64
	}
	var budgets []budgetRow
	err := config.DB.Table("fund_subcategories fs").
		Select("fs.subcategory_id, sb.subcategory_budget_id as budget_id, sb.fund_description, sb.remaining_budget").
		Joins("JOIN fund_categories fc ON fs.category_id = fc.category_id AND fc.category_id = ? AND fc.year_id = ?", categoryID, yearID).
		Joins("JOIN subcategory_budgets sb ON sb.subcategory_id = fs.subcategory_id AND sb.status = 'active' AND sb.delete_at IS NULL").
		Where("fs.delete_at IS NULL AND fs.status = 'active'").
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
