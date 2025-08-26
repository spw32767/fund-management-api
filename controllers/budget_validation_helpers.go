// controllers/budget_validation_helpers.go
package controllers

import (
	"fmt"
	"strings"

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

// ---- Helpers: Normalize / Synonyms for quartile codes ----

func normalizeQuartileCode(s string) string {
	c := strings.ToUpper(strings.TrimSpace(s))
	switch c {
	case "Q1", "Q2", "Q3", "Q4", "T5", "T10", "TCI", "N/A":
		return c
	case "TOP_5_PERCENT", "TOP5", "TOP 5%", "TOP-5%", "TOP5PERCENT", "TOP 5 PERCENT":
		return "T5"
	case "TOP_10_PERCENT", "TOP10", "TOP 10%", "TOP-10%", "TOP10PERCENT", "TOP 10 PERCENT":
		return "T10"
	case "NA", "N-A", "N A":
		return "N/A"
	default:
		// กรณีค่าอื่น ๆ ให้คงไว้ตามที่เป็น แต่เป็นตัวพิมพ์ใหญ่เพื่อให้เทียบได้ง่าย
		return c
	}
}

func synonymsForQuartile(canon string) []string {
	switch canon {
	case "T5":
		return []string{"T5", "TOP_5_PERCENT", "TOP5", "TOP 5%", "TOP-5%", "TOP5PERCENT", "TOP 5 PERCENT"}
	case "T10":
		return []string{"T10", "TOP_10_PERCENT", "TOP10", "TOP 10%", "TOP-10%", "TOP10PERCENT", "TOP 10 PERCENT"}
	case "N/A":
		return []string{"N/A", "NA", "N-A", "N A"}
	default:
		return []string{canon}
	}
}

// GetBudgetQuartileMapping - ดึงการ mapping ระหว่าง quartile และ budget
// ปรับเป็น: ดึง reward_config และ subcategory_budgets แยกกัน แล้วประกอบผลแบบ normalize ใน Go
func GetBudgetQuartileMapping(subcategoryID int) ([]BudgetQuartileMapping, error) {
	// 1) ดึงชุด quartile ที่ระบบรองรับจาก reward_config (source of truth)
	type rcRow struct {
		JournalQuartile string  `gorm:"column:journal_quartile"`
		MaxAmount       float64 `gorm:"column:max_amount"`
	}

	var rcRows []rcRow
	rcQuery := `
		SELECT journal_quartile, COALESCE(max_amount, 0) AS max_amount
		FROM reward_config
		WHERE is_active = 1 AND delete_at IS NULL
		ORDER BY 
			CASE UPPER(journal_quartile)
				WHEN 'Q1' THEN 1
				WHEN 'Q2' THEN 2
				WHEN 'Q3' THEN 3
				WHEN 'Q4' THEN 4
				WHEN 'T5' THEN 5
				WHEN 'T10' THEN 6
				WHEN 'TCI' THEN 7
				WHEN 'N/A' THEN 8
				ELSE 99
			END
	`
	if err := config.DB.Raw(rcQuery).Scan(&rcRows).Error; err != nil {
		return nil, err
	}

	// 2) ดึง budget ของ subcategory นี้ทั้งหมด
	type sbRow struct {
		Level           string  `gorm:"column:level"`
		BudgetID        int     `gorm:"column:subcategory_budget_id"`
		FundDescription string  `gorm:"column:fund_description"`
		RemainingBudget float64 `gorm:"column:remaining_budget"`
		Status          string  `gorm:"column:status"`
	}

	var sbRows []sbRow
	sbQuery := `
		SELECT level, subcategory_budget_id, fund_description, COALESCE(remaining_budget, 0) AS remaining_budget, status
		FROM subcategory_budgets
		WHERE subcategory_id = ? AND delete_at IS NULL
	`
	if err := config.DB.Raw(sbQuery, subcategoryID).Scan(&sbRows).Error; err != nil {
		return nil, err
	}

	// 3) ทำแผนที่ budget ตาม "canonical quartile code"
	sbMap := make(map[string]sbRow)
	for _, r := range sbRows {
		canon := normalizeQuartileCode(r.Level)
		// ถ้ามีหลายแถวที่ normalize ซ้ำกัน เลือกตัวที่เหลืองบมากสุด/active ก่อน
		if existing, ok := sbMap[canon]; ok {
			// heuristic เล็ก ๆ: เลือกอันที่ active ก่อน แล้วค่อยเทียบ remaining_budget
			exIsActive := strings.EqualFold(existing.Status, "active")
			curIsActive := strings.EqualFold(r.Status, "active")
			if (curIsActive && !exIsActive) || (curIsActive == exIsActive && r.RemainingBudget > existing.RemainingBudget) {
				sbMap[canon] = r
			}
		} else {
			sbMap[canon] = r
		}
	}

	// 4) ประกอบผลลัพธ์ตามชุด quartile ของ reward_config
	mappings := make([]BudgetQuartileMapping, 0, len(rcRows))
	for _, rc := range rcRows {
		canon := normalizeQuartileCode(rc.JournalQuartile)
		sb, ok := sbMap[canon]

		desc := fmt.Sprintf("รางวัล %s", canon)
		if ok && strings.TrimSpace(sb.FundDescription) != "" {
			desc = sb.FundDescription
		}

		isAvailable := false
		remaining := 0.0
		budgetID := 0

		if ok {
			remaining = sb.RemainingBudget
			budgetID = sb.BudgetID
			if strings.EqualFold(sb.Status, "active") && sb.RemainingBudget > 0 {
				isAvailable = true
			}
		}

		mappings = append(mappings, BudgetQuartileMapping{
			QuartileCode:    canon,
			BudgetID:        budgetID,
			Description:     desc,
			RewardAmount:    rc.MaxAmount,
			RemainingBudget: remaining,
			IsAvailable:     isAvailable,
		})
	}

	return mappings, nil
}

// ValidateBudgetSelection - ตรวจสอบการเลือก budget
// ปรับให้รับ quartile แบบใดมาก็ normalize แล้วค้นหาในกลุ่มคำพ้อง (IN (?))
func ValidateBudgetSelection(subcategoryID int, quartileCode string) (*BudgetQuartileMapping, error) {
	var mapping BudgetQuartileMapping

	canon := normalizeQuartileCode(quartileCode)
	candidates := synonymsForQuartile(canon)

	query := `
		SELECT 
			sb.level AS quartile_code,
			sb.subcategory_budget_id AS budget_id,
			sb.fund_description AS description,
			COALESCE(rc.max_amount, 0) AS reward_amount,
			COALESCE(sb.remaining_budget, 0) AS remaining_budget,
			CASE 
				WHEN sb.status = 'active' AND COALESCE(sb.remaining_budget, 0) > 0 
				THEN 1 
				ELSE 0 
			END AS is_available
		FROM subcategory_budgets sb
		LEFT JOIN reward_config rc 
			ON UPPER(rc.journal_quartile) = UPPER(?)
			AND rc.is_active = 1
			AND rc.delete_at IS NULL
		WHERE sb.subcategory_id = ? 
			AND UPPER(sb.level) IN (?)
			AND sb.delete_at IS NULL
		LIMIT 1
	`

	// NOTE: GORM จะขยาย slice ใน IN (?) ให้เองเมื่อใช้ Raw
	if err := config.DB.Raw(query, canon, subcategoryID, candidates).Scan(&mapping).Error; err != nil {
		return nil, err
	}

	// normalize ฟิลด์สำคัญก่อนคืน
	mapping.QuartileCode = normalizeQuartileCode(mapping.QuartileCode)

	if mapping.BudgetID == 0 {
		return nil, fmt.Errorf("budget not found for quartile %s (subcategory %d)", canon, subcategoryID)
	}

	return &mapping, nil
}

// GetQuartileFromFormData - แปลงข้อมูลจากฟอร์มเป็น quartile code (คืนค่าแบบ normalize)
func GetQuartileFromFormData(authorStatus string, journalQuartile string, journalTier string) string {
	if strings.TrimSpace(journalQuartile) != "" {
		// Q1, Q2, Q3, Q4, T5, T10, TCI, N/A (หรือค่าที่กรอกมา) -> normalize
		return normalizeQuartileCode(journalQuartile)
	}

	if strings.TrimSpace(journalTier) != "" {
		switch strings.ToLower(strings.TrimSpace(journalTier)) {
		case "5%", "top5", "top 5%", "top-5%":
			return "T5"
		case "10%", "top10", "top 10%", "top-10%":
			return "T10"
		case "tci1", "tci":
			return "TCI"
		case "na", "n/a":
			return "N/A"
		}
	}

	return normalizeQuartileCode("UNKNOWN")
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

	// หา quartile code (normalize แล้ว)
	quartileCode := GetQuartileFromFormData(authorStatus, journalQuartile, journalTier)

	// ตรวจสอบและดึง budget_id
	mapping, err := ValidateBudgetSelection(finalSubcategoryID, quartileCode)
	if err != nil {
		return 0, fmt.Errorf("ไม่พบงบประมาณสำหรับ %s ใน subcategory %d: %v", quartileCode, finalSubcategoryID, err)
	}

	if !mapping.IsAvailable {
		return 0, fmt.Errorf("งบประมาณสำหรับ %s ไม่พร้อมใช้งาน", quartileCode)
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
