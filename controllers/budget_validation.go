// controllers/budget_validation.go
package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

// BudgetValidationResponse - Response structure for budget validation
type BudgetValidationResponse struct {
	SubcategoryID    int                   `json:"subcategory_id"`
	SubcategoryName  string                `json:"subcategory_name"`
	ExpectedCount    int                   `json:"expected_count"`
	BudgetCount      int                   `json:"budget_count"`
	AvailableBudgets []AvailableBudgetInfo `json:"available_budgets"`
	MissingBudgets   []string              `json:"missing_budgets"`
	IsFullyAvailable bool                  `json:"is_fully_available"`
}

// AvailableBudgetInfo - Information about available budget
type AvailableBudgetInfo struct {
	BudgetID        int     `json:"budget_id"`
	Level           string  `json:"level"`
	Description     string  `json:"description"`
	AllocatedAmount float64 `json:"allocated_amount"`
	RemainingBudget float64 `json:"remaining_budget"`
	QuartileCode    string  `json:"quartile_code"`
}

// ValidateSubcategoryBudgets - ตรวจสอบ budget availability สำหรับ subcategory
func ValidateSubcategoryBudgets(c *gin.Context) {
	subcategoryIDStr := c.Query("subcategory_id")
	if subcategoryIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subcategory_id is required"})
		return
	}

	subcategoryID, err := strconv.Atoi(subcategoryIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_id"})
		return
	}

	// ดึงข้อมูล subcategory
	var subcategory struct {
		SubcategoryID      int    `json:"subcategory_id"`
		SubcategoryName    string `json:"subcategory_name"`
		BudgetCount        int    `json:"budget_count"`
		BudgetDescriptions string `json:"budget_descriptions"`
	}

	query := `
		SELECT 
			fs.subcategory_id,
			fs.subcategory_name,
			COUNT(DISTINCT sb.subcategory_budget_id) as budget_count,
			GROUP_CONCAT(DISTINCT sb.fund_description) as budget_descriptions
		FROM fund_subcategories fs
		LEFT JOIN subcategory_budgets sb ON fs.subcategory_id = sb.subcategory_id 
			AND sb.delete_at IS NULL 
			AND sb.status = 'active'
		WHERE fs.subcategory_id = ? 
			AND fs.delete_at IS NULL
		GROUP BY fs.subcategory_id, fs.subcategory_name`

	err = config.DB.Raw(query, subcategoryID).Scan(&subcategory).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subcategory info"})
		return
	}

	if subcategory.SubcategoryID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subcategory not found"})
		return
	}

	// ดึงข้อมูล available budgets
	var availableBudgets []AvailableBudgetInfo
	budgetQuery := `
		SELECT 
			sb.subcategory_budget_id as budget_id,
			sb.level,
			sb.fund_description as description,
			sb.allocated_amount,
			sb.remaining_budget
		FROM subcategory_budgets sb
		WHERE sb.subcategory_id = ? 
			AND sb.delete_at IS NULL 
			AND sb.status = 'active'
			AND sb.remaining_budget > 0
		ORDER BY sb.subcategory_budget_id`

	err = config.DB.Raw(budgetQuery, subcategoryID).Scan(&availableBudgets).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch available budgets"})
		return
	}

	// เพิ่ม quartile_code สำหรับแต่ละ budget
	for i := range availableBudgets {
		availableBudgets[i].QuartileCode = generateQuartileCode(availableBudgets[i].Description)
	}

	// ดึงรายการ quartiles ทั้งหมดที่ควรมี (จาก reward_config)
	expectedQuartiles := getExpectedQuartiles(subcategoryID)

	// หา missing budgets
	missingBudgets := findMissingBudgets(expectedQuartiles, availableBudgets)

	response := BudgetValidationResponse{
		SubcategoryID:    subcategoryID,
		SubcategoryName:  subcategory.SubcategoryName,
		ExpectedCount:    len(expectedQuartiles),
		BudgetCount:      len(availableBudgets),
		AvailableBudgets: availableBudgets,
		MissingBudgets:   missingBudgets,
		IsFullyAvailable: len(missingBudgets) == 0 && len(availableBudgets) >= len(expectedQuartiles),
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}

// GetAvailableQuartiles - ดึงรายการ quartiles ที่มี budget พร้อมใช้งาน
func GetAvailableQuartiles(c *gin.Context) {
	subcategoryIDStr := c.Query("subcategory_id")
	if subcategoryIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subcategory_id is required"})
		return
	}

	subcategoryID, err := strconv.Atoi(subcategoryIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_id"})
		return
	}

	// ดึงรายการ quartiles ที่มี budget พร้อมใช้งาน
	var availableQuartiles []struct {
		BudgetID        int     `json:"budget_id"`
		QuartileCode    string  `json:"quartile_code"`
		Description     string  `json:"description"`
		RewardAmount    float64 `json:"reward_amount"`
		RemainingBudget float64 `json:"remaining_budget"`
	}

	query := `
		SELECT 
			sb.subcategory_budget_id as budget_id,
			sb.level as quartile_code,
			sb.fund_description as description,
			COALESCE(rc.max_amount, 0) as reward_amount,
			sb.remaining_budget
		FROM subcategory_budgets sb
		LEFT JOIN reward_config rc ON sb.level = rc.journal_quartile AND rc.is_active = 1
		WHERE sb.subcategory_id = ? 
			AND sb.delete_at IS NULL 
			AND sb.status = 'active'
			AND sb.remaining_budget > 0
		ORDER BY 
			CASE sb.level 
				WHEN 'Q1' THEN 1 
				WHEN 'Q2' THEN 2 
				WHEN 'Q3' THEN 3 
				WHEN 'Q4' THEN 4 
				ELSE 5 
			END`

	err = config.DB.Raw(query, subcategoryID).Scan(&availableQuartiles).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch available quartiles"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":             true,
		"available_quartiles": availableQuartiles,
		"count":               len(availableQuartiles),
	})
}

// Helper Functions

// getExpectedQuartiles - ดึงรายการ quartiles ทั้งหมดที่ควรมี
func getExpectedQuartiles(subcategoryID int) []string {
	var quartiles []string

	// ดึงจาก reward_config ที่ active
	query := `
		SELECT DISTINCT journal_quartile 
		FROM reward_config 
		WHERE is_active = 1 AND delete_at IS NULL
		ORDER BY 
			CASE journal_quartile 
				WHEN 'Q1' THEN 1 
				WHEN 'Q2' THEN 2 
				WHEN 'Q3' THEN 3 
				WHEN 'Q4' THEN 4 
				ELSE 5 
			END`

	config.DB.Raw(query).Pluck("journal_quartile", &quartiles)

	return quartiles
}

// findMissingBudgets - หา budgets ที่ขาดหายไป
func findMissingBudgets(expectedQuartiles []string, availableBudgets []AvailableBudgetInfo) []string {
	availableMap := make(map[string]bool)
	for _, budget := range availableBudgets {
		availableMap[budget.QuartileCode] = true
	}

	var missing []string
	for _, quartile := range expectedQuartiles {
		if !availableMap[quartile] {
			missing = append(missing, quartile)
		}
	}

	return missing
}

// generateQuartileCode - สร้าง quartile code จาก description
func generateQuartileCode(description string) string {
	desc := strings.ToUpper(description)

	if strings.Contains(desc, "ควอร์ไทล์ 1") || strings.Contains(desc, "QUARTILE 1") {
		return "Q1"
	}
	if strings.Contains(desc, "ควอร์ไทล์ 2") || strings.Contains(desc, "QUARTILE 2") {
		return "Q2"
	}
	if strings.Contains(desc, "ควอร์ไทล์ 3") || strings.Contains(desc, "QUARTILE 3") {
		return "Q3"
	}
	if strings.Contains(desc, "ควอร์ไทล์ 4") || strings.Contains(desc, "QUARTILE 4") {
		return "Q4"
	}
	if strings.Contains(desc, "TCI") {
		return "TCI"
	}
	if strings.Contains(desc, "5%") {
		return "TOP_5_PERCENT"
	}
	if strings.Contains(desc, "10%") {
		return "TOP_10_PERCENT"
	}

	// Default fallback - ใช้ส่วนแรกของ description
	parts := strings.Split(description, " ")
	if len(parts) > 0 {
		return strings.ToUpper(parts[0])
	}

	return "UNKNOWN"
}
