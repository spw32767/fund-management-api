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

/* -------------------- Normalizers & Inference -------------------- */

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
		return c
	}
}

// เดารหัสจากคำอธิบายไทย (รองรับข้อความที่พบใน DB)
func inferQuartileFromDescription(desc string) string {
	d := strings.ToUpper(strings.TrimSpace(desc))
	if d == "" {
		return ""
	}
	// Top 5% / Top 10%
	if strings.Contains(d, "5%") {
		return "T5"
	}
	if strings.Contains(d, "10%") {
		return "T10"
	}
	// ควอร์ไทล์ 1-4
	if strings.Contains(d, "ควอร์ไทล์ 1") || strings.Contains(d, "QUARTILE 1") || strings.Contains(d, " Q1") {
		return "Q1"
	}
	if strings.Contains(d, "ควอร์ไทล์ 2") || strings.Contains(d, "QUARTILE 2") || strings.Contains(d, " Q2") {
		return "Q2"
	}
	if strings.Contains(d, "ควอร์ไทล์ 3") || strings.Contains(d, "QUARTILE 3") || strings.Contains(d, " Q3") {
		return "Q3"
	}
	if strings.Contains(d, "ควอร์ไทล์ 4") || strings.Contains(d, "QUARTILE 4") || strings.Contains(d, " Q4") {
		return "Q4"
	}
	// TCI
	if strings.Contains(d, "TCI") {
		return "TCI"
	}
	return ""
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

/* -------------------- Main Helpers -------------------- */

// GetBudgetQuartileMapping - ดึงการ mapping ระหว่าง quartile และ budget
// ใช้ rc เป็น expected set และประกอบ budget แบบ normalize (อาศัย level ถ้าไม่มีให้เดาจาก fund_description)
func GetBudgetQuartileMapping(subcategoryID int) ([]BudgetQuartileMapping, error) {
	// 1) expected set จาก reward_config
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

	// 2) budgets ของ subcategory นี้ทั้งหมด
	type sbRow struct {
		Level           *string `gorm:"column:level"` // อาจเป็น NULL
		BudgetID        int     `gorm:"column:subcategory_budget_id"`
		FundDescription string  `gorm:"column:fund_description"`
		RemainingBudget float64 `gorm:"column:remaining_budget"`
		Status          string  `gorm:"column:status"`
	}
	var sbRows []sbRow
	sbQuery := `
		SELECT level, subcategory_budget_id, COALESCE(fund_description,'') AS fund_description,
		       COALESCE(remaining_budget,0) AS remaining_budget, COALESCE(status,'') AS status
		FROM subcategory_budgets
		WHERE subcategory_id = ? AND delete_at IS NULL
	`
	if err := config.DB.Raw(sbQuery, subcategoryID).Scan(&sbRows).Error; err != nil {
		return nil, err
	}

	// 3) ทำ map: canonical -> best sbRow
	sbMap := make(map[string]sbRow)
	for _, r := range sbRows {
		canon := ""
		if r.Level != nil && strings.TrimSpace(*r.Level) != "" {
			canon = normalizeQuartileCode(*r.Level)
		}
		if canon == "" || canon == "UNKNOWN" {
			// เดาจากคำอธิบาย (กรณี level ไม่เก็บ quartile)
			canon = inferQuartileFromDescription(r.FundDescription)
		}
		if canon == "" {
			continue // ระงับแถวที่เดาไม่ได้
		}
		// เก็บตัวที่ active/งบมากกว่า
		if existing, ok := sbMap[canon]; ok {
			exIsActive := strings.EqualFold(existing.Status, "active")
			curIsActive := strings.EqualFold(r.Status, "active")
			if (curIsActive && !exIsActive) || (curIsActive == exIsActive && r.RemainingBudget > existing.RemainingBudget) {
				sbMap[canon] = r
			}
		} else {
			sbMap[canon] = r
		}
	}

	// 4) ประกอบผลลัพธ์ตาม expected set
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

// ValidateBudgetSelection - ตรวจสอบการเลือก budget (normalize + รองรับเดาจาก description)
func ValidateBudgetSelection(subcategoryID int, quartileCode string) (*BudgetQuartileMapping, error) {
	canon := normalizeQuartileCode(quartileCode)

	// โหลด budgets ของ subcategory ก่อน แล้วหาแถวที่แมตรหัส
	type sbRow struct {
		Level           *string `gorm:"column:level"`
		BudgetID        int     `gorm:"column:subcategory_budget_id"`
		FundDescription string  `gorm:"column:fund_description"`
		RemainingBudget float64 `gorm:"column:remaining_budget"`
		Status          string  `gorm:"column:status"`
	}
	var rows []sbRow
	sbQuery := `
		SELECT level, subcategory_budget_id, COALESCE(fund_description,'') AS fund_description,
		       COALESCE(remaining_budget,0) AS remaining_budget, COALESCE(status,'') AS status
		FROM subcategory_budgets
		WHERE subcategory_id = ? AND delete_at IS NULL
	`
	if err := config.DB.Raw(sbQuery, subcategoryID).Scan(&rows).Error; err != nil {
		return nil, err
	}

	// หาแถวที่ตรงกับ canon
	var picked *sbRow
	for _, r := range rows {
		code := ""
		if r.Level != nil && strings.TrimSpace(*r.Level) != "" {
			code = normalizeQuartileCode(*r.Level)
		}
		if code == "" || code == "UNKNOWN" {
			code = inferQuartileFromDescription(r.FundDescription)
		}
		if code == canon {
			if picked == nil {
				picked = &r
			} else {
				// เลือก active/งบมากกว่า
				exIsActive := strings.EqualFold(picked.Status, "active")
				curIsActive := strings.EqualFold(r.Status, "active")
				if (curIsActive && !exIsActive) || (curIsActive == exIsActive && r.RemainingBudget > picked.RemainingBudget) {
					tmp := r
					picked = &tmp
				}
			}
		}
	}

	if picked == nil || picked.BudgetID == 0 {
		return nil, fmt.Errorf("budget not found for quartile %s (subcategory %d)", canon, subcategoryID)
	}

	// ดึงจำนวนเงินจาก reward_config (optional)
	type rcRow struct {
		MaxAmount float64 `gorm:"column:max_amount"`
	}
	var rc rcRow
	rcQuery := `
		SELECT COALESCE(max_amount,0) AS max_amount
		FROM reward_config
		WHERE is_active = 1 AND delete_at IS NULL AND UPPER(journal_quartile) = UPPER(?)
		LIMIT 1
	`
	_ = config.DB.Raw(rcQuery, canon).Scan(&rc).Error // ไม่ critical

	return &BudgetQuartileMapping{
		QuartileCode:    canon,
		BudgetID:        picked.BudgetID,
		Description:     picked.FundDescription,
		RewardAmount:    rc.MaxAmount,
		RemainingBudget: picked.RemainingBudget,
		IsAvailable:     strings.EqualFold(picked.Status, "active") && picked.RemainingBudget > 0,
	}, nil
}

// GetQuartileFromFormData - normalize ค่าที่มาจากฟอร์ม
func GetQuartileFromFormData(authorStatus string, journalQuartile string, journalTier string) string {
	if strings.TrimSpace(journalQuartile) != "" {
		return normalizeQuartileCode(journalQuartile)
	}
	if strings.TrimSpace(journalTier) != "" {
		switch strings.ToLower(strings.TrimSpace(journalTier)) {
		case "top_5_percent", "top5", "top 5%", "top-5%":
			return "T5"
		case "top_10_percent", "top10", "top 10%", "top-10%":
			return "T10"
		case "tci_1", "tci":
			return "TCI"
		case "na", "n/a":
			return "N/A"
		}
	}
	return normalizeQuartileCode("UNKNOWN")
}

// CalculateSubcategoryBudgetID - คำนวณหา subcategory_budget_id จากข้อมูลฟอร์ม
func CalculateSubcategoryBudgetID(categoryID int, subcategoryID int, formData map[string]interface{}) (int, error) {
	authorStatus := getStringFromMap(formData, "author_status")
	journalQuartile := getStringFromMap(formData, "journal_quartile")
	journalTier := getStringFromMap(formData, "journal_tier")

	// map author -> subcategory id
	finalSubcategoryID := subcategoryID
	if authorStatus == "first_author" {
		finalSubcategoryID = 14
	} else if authorStatus == "corresponding_author" {
		finalSubcategoryID = 15
	}

	quartileCode := GetQuartileFromFormData(authorStatus, journalQuartile, journalTier)

	mapping, err := ValidateBudgetSelection(finalSubcategoryID, quartileCode)
	if err != nil {
		return 0, fmt.Errorf("ไม่พบงบประมาณสำหรับ %s ใน subcategory %d: %v", quartileCode, finalSubcategoryID, err)
	}
	if !mapping.IsAvailable {
		return 0, fmt.Errorf("งบประมาณสำหรับ %s ไม่พร้อมใช้งาน", quartileCode)
	}
	return mapping.BudgetID, nil
}

// Helper
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if s, ok2 := val.(string); ok2 {
			return s
		}
	}
	return ""
}
