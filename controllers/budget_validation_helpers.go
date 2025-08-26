// controllers/budget_validation.go (เฉพาะ 2 ฟังก์ชันนี้)

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

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

	mappings, err := GetBudgetQuartileMapping(subID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

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

	// expected = FixedQuartiles
	missing := make([]string, 0, len(FixedQuartiles))
	for _, exp := range FixedQuartiles {
		if _, ok := availableSet[exp]; !ok {
			missing = append(missing, exp)
		}
	}

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
			"available_quartiles": availCodes,
		},
	})
}
