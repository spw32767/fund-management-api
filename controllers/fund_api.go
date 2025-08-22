// controllers/fund_api.go
package controllers

import (
	"encoding/json"
	"fmt"
	"fund-management-api/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetFundStructure - API ใหม่ที่จัดกลุ่มข้อมูลให้พร้อมใช้งาน
func GetFundStructure(c *gin.Context) {
	yearID := c.Query("year_id")
	categoryID := c.Query("category_id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Build query สำหรับดึง categories และ subcategories
	categoriesQuery := `
        SELECT DISTINCT
            fc.category_id,
            fc.category_name,
            fc.status as category_status,
            fc.year_id
        FROM fund_categories fc
        WHERE fc.delete_at IS NULL 
            AND fc.status = 'active'`

	var categoryArgs []interface{}

	if yearID != "" {
		categoriesQuery += " AND fc.year_id = ?"
		categoryArgs = append(categoryArgs, yearID)
	}

	if categoryID != "" {
		categoriesQuery += " AND fc.category_id = ?"
		categoryArgs = append(categoryArgs, categoryID)
	}

	categoriesQuery += " ORDER BY fc.category_id"

	// Execute categories query
	categoryRows, err := config.DB.Raw(categoriesQuery, categoryArgs...).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch categories",
		})
		return
	}
	defer categoryRows.Close()

	var categories []map[string]interface{}

	for categoryRows.Next() {
		var (
			catID     int
			catName   string
			catStatus string
			yearID    *int
		)

		err := categoryRows.Scan(&catID, &catName, &catStatus, &yearID)
		if err != nil {
			continue
		}

		// Get subcategories for this category (grouped)
		subcategories := getGroupedSubcategories(catID, roleID.(int))

		// Only add category if it has visible subcategories
		if len(subcategories) > 0 {
			category := map[string]interface{}{
				"category_id":   catID,
				"category_name": catName,
				"status":        catStatus,
				"year_id":       yearID,
				"subcategories": subcategories,
			}
			categories = append(categories, category)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"categories": categories,
		"total":      len(categories),
		"user_id":    userID,
		"role_id":    roleID,
	})
}

// getGroupedSubcategories - Helper function to get subcategories grouped by subcategory_id
func getGroupedSubcategories(categoryID int, roleID int) []map[string]interface{} {
	// Query ที่ดึงข้อมูล subcategories และใช้ budget จากรายการแรกที่พบ (ไม่รวม)
	// เพราะ budget ที่มี subcategory_id เดียวกันใช้เงินก้อนเดียวกัน
	query := `
        SELECT 
            fs.subcategory_id,
            fs.subcategory_name,
            fs.fund_condition,
            fs.target_roles,
            fs.form_type,
            fs.form_url,
            fs.status,
            -- ใช้ค่าแรกที่พบ ไม่ใช้ SUM เพราะเป็นเงินก้อนเดียวกัน
            (SELECT sb1.allocated_amount 
             FROM subcategory_budgets sb1 
             WHERE sb1.subcategory_id = fs.subcategory_id 
                AND sb1.delete_at IS NULL 
                AND sb1.status = 'active' 
             LIMIT 1) as allocated_amount,
            (SELECT sb2.remaining_budget 
             FROM subcategory_budgets sb2 
             WHERE sb2.subcategory_id = fs.subcategory_id 
                AND sb2.delete_at IS NULL 
                AND sb2.status = 'active' 
             LIMIT 1) as remaining_budget,
            COUNT(DISTINCT sb.subcategory_budget_id) as budget_count,
            GROUP_CONCAT(DISTINCT sb.level) as levels,
            GROUP_CONCAT(DISTINCT sb.fund_description) as descriptions
        FROM fund_subcategories fs
        LEFT JOIN subcategory_budgets sb ON fs.subcategory_id = sb.subcategory_id
            AND sb.delete_at IS NULL
            AND sb.status = 'active'
        WHERE fs.delete_at IS NULL 
            AND fs.status = 'active'
            AND fs.category_id = ?`

	var args []interface{}
	args = append(args, categoryID)

	// Role-based filtering
	if roleID != 3 { // Not admin
		roleIDStr := fmt.Sprintf("%d", roleID)
		query += " AND (fs.target_roles IS NULL OR fs.target_roles = '' OR JSON_CONTAINS(fs.target_roles, ?))"
		args = append(args, fmt.Sprintf(`"%s"`, roleIDStr))
	}

	query += ` GROUP BY fs.subcategory_id, fs.subcategory_name, 
               fs.fund_condition, fs.target_roles, fs.form_type, 
               fs.form_url, fs.status
               ORDER BY fs.subcategory_id`

	rows, err := config.DB.Raw(query, args...).Rows()
	if err != nil {
		return []map[string]interface{}{}
	}
	defer rows.Close()

	var subcategories []map[string]interface{}

	for rows.Next() {
		var (
			subID           int
			subName         string
			fundCondition   *string
			targetRoles     *string
			formType        *string
			formURL         *string
			status          string
			allocatedAmount *float64 // เปลี่ยนเป็น pointer เพราะอาจเป็น NULL
			remainingBudget *float64 // เปลี่ยนเป็น pointer เพราะอาจเป็น NULL
			budgetCount     int
			levels          *string
			descriptions    *string
		)

		err := rows.Scan(
			&subID,
			&subName,
			&fundCondition,
			&targetRoles,
			&formType,
			&formURL,
			&status,
			&allocatedAmount,
			&remainingBudget,
			&budgetCount,
			&levels,
			&descriptions,
		)
		if err != nil {
			continue
		}

		// Parse target roles
		var targetRolesList []string
		if targetRoles != nil && *targetRoles != "" {
			json.Unmarshal([]byte(*targetRoles), &targetRolesList)
		}

		// ใช้ค่า default ถ้า budget เป็น NULL
		allocatedVal := float64(0)
		remainingVal := float64(0)
		if allocatedAmount != nil {
			allocatedVal = *allocatedAmount
		}
		if remainingBudget != nil {
			remainingVal = *remainingBudget
		}

		subcategory := map[string]interface{}{
			"subcategory_id":      subID,
			"subcategory_name":    subName,
			"fund_condition":      fundCondition,
			"target_roles":        targetRolesList,
			"form_type":           formType,
			"form_url":            formURL,
			"status":              status,
			"allocated_amount":    allocatedVal, // ใช้เงินก้อนเดียวกัน ไม่รวม
			"remaining_budget":    remainingVal, // ใช้เงินก้อนเดียวกัน ไม่รวม
			"budget_count":        budgetCount,
			"has_multiple_levels": budgetCount > 1,
		}

		// Add budget breakdown info if needed
		if budgetCount > 1 && (levels != nil || descriptions != nil) {
			subcategory["budget_levels"] = levels
			subcategory["budget_descriptions"] = descriptions
		}

		subcategories = append(subcategories, subcategory)
	}

	return subcategories
}

// GetFundStructureAlternative - Alternative approach using single query
func GetFundStructureAlternative(c *gin.Context) {
	yearID := c.Query("year_id")
	categoryID := c.Query("category_id")
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	// Single comprehensive query
	query := `
        SELECT DISTINCT
            fc.category_id,
            fc.category_name,
            fc.status as category_status,
            fc.year_id,
            fs.subcategory_id,
            fs.subcategory_name,
            fs.fund_condition,
            fs.target_roles,
            fs.form_type,
            fs.form_url,
            fs.status as subcategory_status,
            -- ใช้ MIN หรือ MAX เพื่อเอาค่าเดียว (เพราะทุก budget ของ subcategory_id เดียวกันใช้เงินก้อนเดียวกัน)
            MIN(sb.allocated_amount) as allocated_amount,
            MIN(sb.remaining_budget) as remaining_budget,
            COUNT(DISTINCT sb.subcategory_budget_id) as budget_count,
            GROUP_CONCAT(DISTINCT sb.level) as levels
        FROM fund_categories fc
        INNER JOIN fund_subcategories fs ON fc.category_id = fs.category_id
            AND fs.delete_at IS NULL
            AND fs.status = 'active'
        LEFT JOIN subcategory_budgets sb ON fs.subcategory_id = sb.subcategory_id
            AND sb.delete_at IS NULL
            AND sb.status = 'active'
        WHERE fc.delete_at IS NULL 
            AND fc.status = 'active'`

	var args []interface{}

	if yearID != "" {
		query += " AND fc.year_id = ?"
		args = append(args, yearID)
	}

	if categoryID != "" {
		query += " AND fc.category_id = ?"
		args = append(args, categoryID)
	}

	// Role-based filtering
	if roleID != 3 {
		roleIDStr := fmt.Sprintf("%d", roleID)
		query += " AND (fs.target_roles IS NULL OR fs.target_roles = '' OR JSON_CONTAINS(fs.target_roles, ?))"
		args = append(args, fmt.Sprintf(`"%s"`, roleIDStr))
	}

	query += ` GROUP BY fc.category_id, fc.category_name, fc.status, fc.year_id,
               fs.subcategory_id, fs.subcategory_name, fs.fund_condition, 
               fs.target_roles, fs.form_type, fs.form_url, fs.status
               ORDER BY fc.category_id, fs.subcategory_id`

	rows, err := config.DB.Raw(query, args...).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch fund structure",
			"debug": err.Error(),
		})
		return
	}
	defer rows.Close()

	// Process results into structured format
	categoriesMap := make(map[int]map[string]interface{})

	for rows.Next() {
		var (
			catID           int
			catName         string
			catStatus       string
			yearID          *int
			subID           int
			subName         string
			fundCondition   *string
			targetRoles     *string
			formType        *string
			formURL         *string
			subStatus       string
			allocatedAmount *float64
			remainingBudget *float64
			budgetCount     int
			levels          *string
		)

		err := rows.Scan(
			&catID, &catName, &catStatus, &yearID,
			&subID, &subName, &fundCondition, &targetRoles,
			&formType, &formURL, &subStatus,
			&allocatedAmount, &remainingBudget,
			&budgetCount, &levels,
		)
		if err != nil {
			continue
		}

		// Create category if not exists
		if _, exists := categoriesMap[catID]; !exists {
			categoriesMap[catID] = map[string]interface{}{
				"category_id":   catID,
				"category_name": catName,
				"status":        catStatus,
				"year_id":       yearID,
				"subcategories": []map[string]interface{}{},
			}
		}

		// Parse target roles
		var targetRolesList []string
		if targetRoles != nil && *targetRoles != "" {
			json.Unmarshal([]byte(*targetRoles), &targetRolesList)
		}

		// Default values for NULL budgets
		allocatedVal := float64(0)
		remainingVal := float64(0)
		if allocatedAmount != nil {
			allocatedVal = *allocatedAmount
		}
		if remainingBudget != nil {
			remainingVal = *remainingBudget
		}

		// Add subcategory
		subcategory := map[string]interface{}{
			"subcategory_id":      subID,
			"subcategory_name":    subName,
			"fund_condition":      fundCondition,
			"target_roles":        targetRolesList,
			"form_type":           formType,
			"form_url":            formURL,
			"status":              subStatus,
			"allocated_amount":    allocatedVal,
			"remaining_budget":    remainingVal,
			"budget_count":        budgetCount,
			"has_multiple_levels": budgetCount > 1,
		}

		if budgetCount > 1 && levels != nil {
			subcategory["budget_levels"] = levels
		}

		// Append subcategory to category
		subs := categoriesMap[catID]["subcategories"].([]map[string]interface{})
		categoriesMap[catID]["subcategories"] = append(subs, subcategory)
	}

	// Convert map to slice
	var categories []map[string]interface{}
	for _, cat := range categoriesMap {
		categories = append(categories, cat)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"categories": categories,
		"total":      len(categories),
		"user_id":    userID,
		"role_id":    roleID,
	})
}
