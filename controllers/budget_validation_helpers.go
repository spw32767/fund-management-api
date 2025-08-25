// controllers/budget_validation_helpers.go
package controllers

import (
	"fmt"
	"fund-management-api/config"
)

// BudgetQuartileMapping - โครงสร้างสำหรับ mapping ระหว่าง quartile และ budget
type BudgetQuartileMapping struct {
	QuartileCode    string  `json:"quartile_code"`
	BudgetID        int     `json:"budget_id"`
	Description     string  `json:"description"`
	RewardAmount    float64 `json:"reward_amount"`
	RemainingBudget float64 `json:"remaining_budget"`
	IsAvailable     bool    `json:"is_available"`
}

// GetBudgetQuartileMapping - ดึงการ mapping ระหว่าง quartile และ budget
func GetBudgetQuartileMapping(subcategoryID int) ([]BudgetQuartileMapping, error) {
	var mappings []BudgetQuartileMapping

	query := `
		SELECT 
			COALESCE(sb.level, rc.journal_quartile) as quartile_code,
			sb.subcategory_budget_id as budget_id,
			COALESCE(sb.fund_description, CONCAT('รางวัล ', rc.journal_quartile)) as description,
			COALESCE(rc.max_amount, 0) as reward_amount,
			COALESCE(sb.remaining_budget, 0) as remaining_budget,
			CASE 
				WHEN sb.subcategory_budget_id IS NOT NULL 
					AND sb.status = 'active' 
					AND sb.remaining_budget > 0 
				THEN 1 
				ELSE 0 
			END as is_available
		FROM reward_config rc
		LEFT JOIN subcategory_budgets sb ON rc.journal_quartile = sb.level 
			AND sb.subcategory_id = ? 
			AND sb.delete_at IS NULL
		WHERE rc.is_active = 1 
			AND rc.delete_at IS NULL
		ORDER BY 
			CASE rc.journal_quartile 
				WHEN 'Q1' THEN 1 
				WHEN 'Q2' THEN 2 
				WHEN 'Q3' THEN 3 
				WHEN 'Q4' THEN 4 
				ELSE 5 
			END`

	err := config.DB.Raw(query, subcategoryID).Scan(&mappings).Error
	return mappings, err
}

// ValidateBudgetSelection - ตรวจสอบการเลือก budget
func ValidateBudgetSelection(subcategoryID int, quartileCode string) (*BudgetQuartileMapping, error) {
	var mapping BudgetQuartileMapping

	query := `
		SELECT 
			sb.level as quartile_code,
			sb.subcategory_budget_id as budget_id,
			sb.fund_description as description,
			COALESCE(rc.max_amount, 0) as reward_amount,
			sb.remaining_budget,
			CASE 
				WHEN sb.status = 'active' AND sb.remaining_budget > 0 
				THEN 1 
				ELSE 0 
			END as is_available
		FROM subcategory_budgets sb
		LEFT JOIN reward_config rc ON sb.level = rc.journal_quartile 
			AND rc.is_active = 1
		WHERE sb.subcategory_id = ? 
			AND sb.level = ?
			AND sb.delete_at IS NULL`

	err := config.DB.Raw(query, subcategoryID, quartileCode).Scan(&mapping).Error
	if err != nil {
		return nil, err
	}

	if mapping.BudgetID == 0 {
		return nil, fmt.Errorf("budget not found for quartile %s", quartileCode)
	}

	return &mapping, nil
}

// GetQuartileFromFormData - แปลงข้อมูลจากฟอร์มเป็น quartile code
func GetQuartileFromFormData(authorStatus string, journalQuartile string, journalTier string) string {
	// Logic สำหรับแปลงข้อมูลฟอร์มเป็น quartile code
	// ขึ้นอยู่กับ business logic ของระบบ

	if journalQuartile != "" {
		return journalQuartile // Q1, Q2, Q3, Q4
	}

	if journalTier != "" {
		switch journalTier {
		case "top_5_percent":
			return "TOP_5_PERCENT"
		case "top_10_percent":
			return "TOP_10_PERCENT"
		case "tci_1":
			return "TCI"
		}
	}

	return "UNKNOWN"
}

// CalculateSubcategoryBudgetID - คำนวณหา subcategory_budget_id จากข้อมูลฟอร์ม
func CalculateSubcategoryBudgetID(categoryID int, subcategoryID int, formData map[string]interface{}) (int, error) {
	// ดึงข้อมูลจากฟอร์ม
	authorStatus := getStringFromMap(formData, "author_status")
	journalQuartile := getStringFromMap(formData, "journal_quartile")
	journalTier := getStringFromMap(formData, "journal_tier")

	// กำหนด subcategory_id ตาม author_status
	finalSubcategoryID := subcategoryID
	if authorStatus == "first_author" {
		finalSubcategoryID = 14
	} else if authorStatus == "corresponding_author" {
		finalSubcategoryID = 15
	}

	// หา quartile code
	quartileCode := GetQuartileFromFormData(authorStatus, journalQuartile, journalTier)

	// ตรวจสอบและดึง budget_id
	mapping, err := ValidateBudgetSelection(finalSubcategoryID, quartileCode)
	if err != nil {
		return 0, fmt.Errorf("ไม่พบงบประมาณสำหรับ %s ใน subcategory %d: %v", quartileCode, finalSubcategoryID, err)
	}

	if !mapping.IsAvailable {
		return 0, fmt.Errorf("งบประมาণสำหรับ %s ไม่พร้อมใช้งาน", quartileCode)
	}

	return mapping.BudgetID, nil
}

// Helper function
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, exists := m[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
