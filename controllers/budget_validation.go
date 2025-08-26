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

// NOTE: ใช้ชุด expected เดียวกันทั้งระบบ (7 รายการตามที่คุณใช้อยู่)
var expectedQuartiles = []string{"Q1", "Q2", "Q3", "Q4", "T5", "T10", "TCI"}

// small helper สำหรับเซ็ต
func toSet(items []string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, it := range items {
		m[strings.ToUpper(strings.TrimSpace(it))] = struct{}{}
	}
	return m
}

func uniqueUpper(in []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(in))
	for _, s := range in {
		k := strings.ToUpper(strings.TrimSpace(s))
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}

// GET /api/v1/subcategory-budgets/validate?subcategory_id=X
func ValidateSubcategoryBudgets(c *gin.Context) {
	idStr := c.Query("subcategory_id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing subcategory_id"})
		return
	}
	subID, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid subcategory_id"})
		return
	}

	// ใช้ helper ที่ normalize แล้ว
	mappings, err := GetBudgetQuartileMapping(subID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// สร้าง available จาก mapping ที่ IsAvailable = true
	type availItem struct {
		QuartileCode    string  `json:"quartile_code"`
		BudgetID        int     `json:"budget_id"`
		Description     string  `json:"description"`
		RewardAmount    float64 `json:"reward_amount"`
		RemainingBudget float64 `json:"remaining_budget"`
	}

	available := make([]availItem, 0, len(mappings))
	availableSet := make(map[string]struct{}, len(mappings))
	for _, m := range mappings {
		code := strings.ToUpper(strings.TrimSpace(m.QuartileCode))
		if m.IsAvailable && m.BudgetID > 0 {
			available = append(available, availItem{
				QuartileCode:    code,
				BudgetID:        m.BudgetID,
				Description:     m.Description,
				RewardAmount:    m.RewardAmount,
				RemainingBudget: m.RemainingBudget,
			})
			availableSet[code] = struct{}{}
		}
	}

	// ลบบรรทัดนี้ทิ้ง (ตัวแปรไม่ถูกใช้)
	// expSet := toSet(expectedQuartiles)

	// คำนวณ missing = expected - available (อิง canonical codes เท่านั้น)
	missing := make([]string, 0, len(expectedQuartiles))
	for _, exp := range expectedQuartiles {
		if _, ok := availableSet[exp]; !ok {
			missing = append(missing, exp)
		}
	}
	missing = uniqueUpper(missing)

	isFullyAvailable := len(missing) == 0

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"subcategory_id":     subID,
			"is_fully_available": isFullyAvailable,
			"available_budgets":  available,
			"missing_budgets":    missing,
		},
	})
}

// GET /api/v1/subcategory-budgets/available-quartiles?subcategory_id=X
func GetAvailableQuartiles(c *gin.Context) {
	idStr := c.Query("subcategory_id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing subcategory_id"})
		return
	}
	subID, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid subcategory_id"})
		return
	}

	mappings, err := GetBudgetQuartileMapping(subID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	availCodes := make([]string, 0, len(mappings))
	seen := make(map[string]struct{})
	for _, m := range mappings {
		if m.IsAvailable && m.BudgetID > 0 {
			code := strings.ToUpper(strings.TrimSpace(m.QuartileCode))
			if _, ok := seen[code]; !ok {
				seen[code] = struct{}{}
				availCodes = append(availCodes, code)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"subcategory_id":      subID,
			"available_quartiles": availCodes, // คืนเฉพาะโค้ด canonical
		},
	})
}
