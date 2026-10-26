package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"fund-management-api/config"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var errBudgetYearNotConfigured = errors.New("ยังไม่ได้เปิดใช้ปีงบประมาณ หรือปีที่ตั้งค่าไม่ได้เปิดใช้งาน")

type budgetYear struct {
	YearID int    `json:"year_id"`
	Year   string `json:"year"`
	Status string `json:"status"`
}

// Keep a single persisted setting for compatibility. Resolve its year_id once
// here rather than letting individual screens guess the latest/calendar year.
func currentBudgetYear(db *gorm.DB) (budgetYear, error) {
	var year budgetYear
	err := db.Raw(`SELECT y.year_id, y.year, y.status FROM years y
		WHERE y.year = (SELECT current_year FROM system_config ORDER BY config_id DESC LIMIT 1)
		AND y.status = 'active' AND y.delete_at IS NULL
		ORDER BY y.year_id DESC LIMIT 1`).Scan(&year).Error
	if err == nil && year.YearID == 0 {
		err = errBudgetYearNotConfigured
	}
	return year, err
}

type budgetYearCheck struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Passed bool   `json:"passed"`
}

type budgetYearReadiness struct {
	YearID int               `json:"year_id"`
	Year   string            `json:"year"`
	Ready  bool              `json:"ready"`
	Checks []budgetYearCheck `json:"checks"`
}

func inspectBudgetYear(db *gorm.DB, year budgetYear) (budgetYearReadiness, error) {
	result := budgetYearReadiness{YearID: year.YearID, Year: year.Year, Ready: true}
	var totals struct{ OpenFunds, MissingBudgets, PublicationFunds, RewardRates, FeeLimits, Installments int64 }
	err := db.Raw(`SELECT
		(SELECT COUNT(*) FROM fund_subcategories fs JOIN fund_categories fc ON fc.category_id = fs.category_id
		 WHERE fs.year_id = ? AND fc.year_id = fs.year_id AND fs.status = 'active' AND fc.status = 'active'
		 AND fs.delete_at IS NULL AND fc.delete_at IS NULL AND COALESCE(fs.form_type, '') <> 'download') AS open_funds,
		(SELECT COUNT(*) FROM fund_subcategories fs JOIN fund_categories fc ON fc.category_id = fs.category_id
		 WHERE fs.year_id = ? AND fc.year_id = fs.year_id AND fs.status = 'active' AND fc.status = 'active'
		 AND fs.delete_at IS NULL AND fc.delete_at IS NULL AND COALESCE(fs.form_type, '') <> 'download'
		 AND NOT EXISTS (SELECT 1 FROM subcategory_budgets sb WHERE sb.subcategory_id = fs.subcategory_id
		 AND sb.record_scope = 'overall' AND sb.status = 'active' AND sb.delete_at IS NULL AND sb.allocated_amount > 0)) AS missing_budgets,
		(SELECT COUNT(*) FROM fund_subcategories fs JOIN fund_categories fc ON fc.category_id = fs.category_id
		 WHERE fs.year_id = ? AND fc.year_id = fs.year_id AND fs.status = 'active' AND fc.status = 'active'
		 AND fs.delete_at IS NULL AND fc.delete_at IS NULL AND fs.form_type = 'publication_reward') AS publication_funds,
		(SELECT COUNT(*) FROM publication_reward_rates WHERE year = ? AND is_active = 1) AS reward_rates,
		(SELECT COUNT(*) FROM reward_config WHERE year = ? AND is_active = 1 AND delete_at IS NULL) AS fee_limits,
		(SELECT COUNT(*) FROM fund_installment_periods WHERE year_id = ?
		 AND (status IS NULL OR LOWER(TRIM(status)) IN ('active', 'enabled', 'open', 'current')) AND deleted_at IS NULL) AS installments`,
		year.YearID, year.YearID, year.YearID, year.Year, year.Year, year.YearID).Scan(&totals).Error
	if err != nil {
		return result, err
	}
	result.Checks = []budgetYearCheck{
		{"year", "ปีงบประมาณเปิดใช้งาน", year.Status == "active"},
		{"funds", "มีโครงสร้างทุนที่เปิดรับคำขอ", totals.OpenFunds > 0},
		{"budgets", "ทุนที่เปิดรับมีงบประมาณรวมมากกว่า 0 ครบทุกทุน", totals.MissingBudgets == 0},
		{"installments", "มีรอบพิจารณาที่เปิดใช้งานของปีนี้", totals.Installments > 0},
	}
	if totals.PublicationFunds > 0 {
		result.Checks = append(result.Checks,
			budgetYearCheck{"reward_rates", "มีอัตรารางวัลตีพิมพ์ของปีนี้", totals.RewardRates > 0},
			budgetYearCheck{"fee_limits", "มีเพดานค่าใช้จ่ายตีพิมพ์ของปีนี้", totals.FeeLimits > 0})
	}
	for _, check := range result.Checks {
		result.Ready = result.Ready && check.Passed
	}
	return result, nil
}

func GetBudgetYearReadiness(c *gin.Context) {
	var year budgetYear
	query := config.DB.Table("years").Select("year_id, year, status").Where("delete_at IS NULL")
	if raw := c.Query("year_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			c.JSON(400, gin.H{"error": "invalid year_id"})
			return
		}
		query = query.Where("year_id = ?", id)
	} else {
		query = query.Where("year = ?", strings.TrimSpace(c.Query("year")))
	}
	if err := query.Order("year_id DESC").Limit(1).Scan(&year).Error; err != nil {
		c.JSON(500, gin.H{"error": "failed to resolve budget year"})
		return
	}
	if year.YearID == 0 {
		c.JSON(404, gin.H{"error": "ไม่พบปีงบประมาณที่เลือก"})
		return
	}
	result, err := inspectBudgetYear(config.DB, year)
	if err != nil {
		c.JSON(500, gin.H{"error": "ไม่สามารถตรวจความพร้อมปีงบประมาณได้"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func validateNewSubmissionFundYear(db *gorm.DB, yearID int, categoryID, subcategoryID, budgetID *int) error {
	if categoryID != nil {
		var count int64
		if err := db.Table("fund_categories").Where("category_id = ? AND year_id = ? AND delete_at IS NULL", *categoryID, yearID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return errors.New("category year mismatch")
		}
	}
	if subcategoryID != nil {
		var sub struct{ CategoryID, YearID int }
		if err := db.Table("fund_subcategories").Select("category_id, year_id").Where("subcategory_id = ? AND delete_at IS NULL", *subcategoryID).Scan(&sub).Error; err != nil {
			return err
		}
		if sub.YearID != yearID || (categoryID != nil && sub.CategoryID != *categoryID) {
			return errors.New("subcategory year mismatch")
		}
	}
	if budgetID != nil {
		if subcategoryID == nil {
			return errors.New("budget needs subcategory")
		}
		var count int64
		if err := db.Table("subcategory_budgets").Where("subcategory_budget_id = ? AND subcategory_id = ? AND delete_at IS NULL", *budgetID, *subcategoryID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return errors.New("budget subcategory mismatch")
		}
	}
	return nil
}
