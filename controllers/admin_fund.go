// controllers/admin_fund.go
package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ===================== FUND CATEGORIES MANAGEMENT =====================

// GetAllCategories - Admin can view all categories
func GetAllCategories(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	yearID := c.Query("year_id")
	var categories []models.FundCategory

	query := config.DB.Where("delete_at IS NULL")
	if yearID != "" {
		query = query.Where("year_id = ?", yearID)
	}

	if err := query.Order("category_id DESC").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch categories"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"categories": categories,
		"total":      len(categories),
	})
}

// CreateCategory - Admin creates new fund category
func CreateCategory(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	type CreateCategoryRequest struct {
		CategoryName string `json:"category_name" binding:"required"`
		YearID       int    `json:"year_id" binding:"required"`
		Comment      string `json:"comment"`
	}

	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if category name already exists for this year
	var existingCategory models.FundCategory
	if err := config.DB.Where("category_name = ? AND year_id = ? AND delete_at IS NULL",
		req.CategoryName, req.YearID).First(&existingCategory).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Category name already exists for this year",
		})
		return
	}

	// Create new category
	now := time.Now()
	category := models.FundCategory{
		CategoryName: req.CategoryName,
		YearID:       req.YearID,
		Status:       "active",
		CreateAt:     &now,
		UpdateAt:     &now,
	}

	if err := config.DB.Create(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create category"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":  true,
		"message":  "Category created successfully",
		"category": category,
	})
}

// UpdateCategory - Admin updates fund category
func UpdateCategory(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	categoryID := c.Param("id")

	type UpdateCategoryRequest struct {
		CategoryName string `json:"category_name"`
		YearID       int    `json:"year_id"`
		Status       string `json:"status"`
		Comment      string `json:"comment"`
	}

	var req UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find category
	var category models.FundCategory
	if err := config.DB.Where("category_id = ? AND delete_at IS NULL", categoryID).
		First(&category).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	// Check if new category name conflicts (if changed)
	if req.CategoryName != "" && req.CategoryName != category.CategoryName {
		var existingCategory models.FundCategory
		if err := config.DB.Where("category_name = ? AND year_id = ? AND category_id != ? AND delete_at IS NULL",
			req.CategoryName, req.YearID, categoryID).First(&existingCategory).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Category name already exists for this year",
			})
			return
		}
	}

	// Update category
	now := time.Now()
	updates := map[string]interface{}{
		"update_at": &now,
	}

	if req.CategoryName != "" {
		updates["category_name"] = req.CategoryName
	}
	if req.YearID != 0 {
		updates["year_id"] = req.YearID
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}

	if err := config.DB.Model(&category).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update category"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Category updated successfully",
		"category": category,
	})
}

// DeleteCategory - Admin soft deletes fund category
func DeleteCategory(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	categoryID := c.Param("id")

	// Find category
	var category models.FundCategory
	if err := config.DB.Where("category_id = ? AND delete_at IS NULL", categoryID).
		First(&category).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	// Check if category has active subcategories
	var subcategoryCount int64
	config.DB.Model(&models.FundSubcategory{}).
		Where("category_id = ? AND delete_at IS NULL", categoryID).
		Count(&subcategoryCount)

	if subcategoryCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Cannot delete category that has subcategories",
			"details": fmt.Sprintf("Category has %d subcategories", subcategoryCount),
		})
		return
	}

	// Soft delete
	now := time.Now()
	category.DeleteAt = &now

	if err := config.DB.Save(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete category"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Category deleted successfully",
	})
}

// ToggleCategoryStatus - Admin toggles category active/disable status
func ToggleCategoryStatus(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	categoryID := c.Param("id")

	// Find category
	var category models.FundCategory
	if err := config.DB.Where("category_id = ? AND delete_at IS NULL", categoryID).
		First(&category).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	// Toggle status
	newStatus := "active"
	if category.Status == "active" {
		newStatus = "disable"
	}

	now := time.Now()
	category.Status = newStatus
	category.UpdateAt = &now

	if err := config.DB.Save(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle category status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    fmt.Sprintf("Category status changed to %s", newStatus),
		"category":   category,
		"new_status": newStatus,
	})
}

// ===================== FUND SUBCATEGORIES MANAGEMENT =====================

// GetAllSubcategories - Admin can view all subcategories
// GetAllSubcategories - Admin can view all subcategories (WITH budgets attached)
func GetAllSubcategories(c *gin.Context) {
	// Admin only
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	categoryID := c.Query("category_id")

	// -------- 1) ดึงรายการทุนย่อย (เหมือนเวอร์ชันเดิม) --------
	baseQuery := `
		SELECT 
			fs.subcategory_id,
			fs.category_id,
			fs.subcategory_name,
			fs.fund_condition,
			fs.target_roles,
			fs.status,
			fs.comment,
			fs.create_at,
			fs.update_at,
			fc.category_name
		FROM fund_subcategories fs
		LEFT JOIN fund_categories fc ON fs.category_id = fc.category_id
		WHERE fs.delete_at IS NULL`

	var args []interface{}
	if categoryID != "" {
		baseQuery += " AND fs.category_id = ?"
		args = append(args, categoryID)
	}
	baseQuery += " ORDER BY fs.subcategory_id DESC"

	rows, err := config.DB.Raw(baseQuery, args...).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch subcategories",
			"debug": err.Error(),
		})
		return
	}
	defer rows.Close()

	type subcatRow struct {
		SubcategoryID   int
		CategoryID      int
		SubcategoryName string
		FundCondition   *string
		TargetRoles     *string
		Status          string
		Comment         *string
		CreateAt        *time.Time
		UpdateAt        *time.Time
		CategoryName    *string
	}
	var rawSubcats []subcatRow
	var subcategoryIDs []int

	for rows.Next() {
		var r subcatRow
		if err := rows.Scan(
			&r.SubcategoryID,
			&r.CategoryID,
			&r.SubcategoryName,
			&r.FundCondition,
			&r.TargetRoles,
			&r.Status,
			&r.Comment,
			&r.CreateAt,
			&r.UpdateAt,
			&r.CategoryName,
		); err != nil {
			continue
		}
		rawSubcats = append(rawSubcats, r)
		subcategoryIDs = append(subcategoryIDs, r.SubcategoryID)
	}

	// ถ้าไม่มีทุนย่อย ก็ส่งกลับตามเดิม
	if len(rawSubcats) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success":       true,
			"subcategories": []any{},
			"total":         0,
		})
		return
	}

	// -------- 2) ดึง budgets ของทุก subcategory ที่ได้ในข้อ 1) ครั้งเดียวแบบรวบยอด --------
	// NOTE: UI ต้องการ key = "budgets" ภายในแต่ละ subcategory
	// ฟิลด์ที่แนบ: record_scope, max_amount_per_year, max_grants, max_amount_per_grant, level, fund_description, comment, status ฯลฯ
	// เรียงลำดับ: overall มาก่อน rule แล้ว fallback ตามลำดับที่มี/ id
	var budgetsBySub = map[int][]map[string]any{}

	// เตรียม IN (?, ?, ...)
	inPlaceholders := make([]string, len(subcategoryIDs))
	inArgs := make([]any, len(subcategoryIDs))
	for i, id := range subcategoryIDs {
		inPlaceholders[i] = "?"
		inArgs[i] = id
	}

	bq := fmt.Sprintf(`
		SELECT
			sb.subcategory_budget_id,
			sb.subcategory_id,
			sb.record_scope,
			sb.allocated_amount,
			sb.used_amount,
			sb.remaining_budget,
			sb.max_amount_per_year,
			sb.max_grants,
			sb.max_amount_per_grant,
			sb.remaining_grant,
			sb.level,
			sb.status,
			sb.fund_description,
			sb.comment,
			sb.create_at,
			sb.update_at,
			-- ฟิลด์ช่วยเรียงลำดับ ถ้ามีคอลัมน์เหล่านี้ในสคีมาบางเวอร์ชัน
			COALESCE(sb.display_order, sb.sort_order, sb.sequence, sb.order, sb.order_index, sb.subcategory_budget_id) AS ord
		FROM subcategory_budgets sb
		WHERE sb.delete_at IS NULL
		  AND sb.subcategory_id IN (%s)
	`, strings.Join(inPlaceholders, ","))

	bRows, err := config.DB.Raw(bq, inArgs...).Rows()
	if err == nil {
		defer bRows.Close()
		for bRows.Next() {
			var (
				budgetID          int
				subcatID          int
				recordScope       string
				allocatedAmount   float64
				usedAmount        float64
				remainingBudget   float64
				maxAmountPerYear  *float64
				maxGrants         *int
				maxAmountPerGrant float64
				remainingGrant    *int
				level             *string
				status            string
				fundDescription   *string
				comment           *string
				createAt          *time.Time
				updateAt          *time.Time
				orderVal          int
			)
			if err := bRows.Scan(
				&budgetID,
				&subcatID,
				&recordScope,
				&allocatedAmount,
				&usedAmount,
				&remainingBudget,
				&maxAmountPerYear,
				&maxGrants,
				&maxAmountPerGrant,
				&remainingGrant,
				&level,
				&status,
				&fundDescription,
				&comment,
				&createAt,
				&updateAt,
				&orderVal,
			); err != nil {
				continue
			}

			item := map[string]any{
				"subcategory_budget_id": budgetID,
				"subcategory_id":        subcatID,
				"record_scope":          recordScope,
				"allocated_amount":      allocatedAmount,
				"used_amount":           usedAmount,
				"remaining_budget":      remainingBudget,
				"max_amount_per_year":   maxAmountPerYear,
				"max_grants":            maxGrants,
				"max_amount_per_grant":  maxAmountPerGrant,
				"remaining_grant":       remainingGrant,
				"level":                 level,
				"status":                status,
				"fund_description":      fundDescription,
				"comment":               comment,
				"create_at":             createAt,
				"update_at":             updateAt,
				"_ord":                  orderVal, // ใช้เรียงภายในโค้ด
			}
			budgetsBySub[subcatID] = append(budgetsBySub[subcatID], item)
		}
	}

	// -------- 3) จัดเรียง budgets: overall ก่อน rule แล้วตาม _ord --------
	for subID, arr := range budgetsBySub {
		sort.SliceStable(arr, func(i, j int) bool {
			// overall มาก่อน
			ri := fmt.Sprintf("%v", arr[i]["record_scope"])
			rj := fmt.Sprintf("%v", arr[j]["record_scope"])
			if ri != rj {
				if ri == "overall" {
					return true
				}
				if rj == "overall" {
					return false
				}
			}
			// จากนั้นตาม _ord (ถ้าไม่มีค่าจะเป็น 0 แต่เรา set จาก id แล้ว)
			oi, _ := arr[i]["_ord"].(int)
			oj, _ := arr[j]["_ord"].(int)
			return oi < oj
		})
		// ลบ key ชั่วคราว
		for _, it := range arr {
			delete(it, "_ord")
		}
		budgetsBySub[subID] = arr
	}

	// -------- 4) สร้าง payload ส่งกลับ (แนบ budgets ลงใน key "budgets") --------
	subcategories := make([]map[string]any, 0, len(rawSubcats))
	for _, r := range rawSubcats {
		var targetRolesList []string
		if r.TargetRoles != nil && *r.TargetRoles != "" {
			_ = json.Unmarshal([]byte(*r.TargetRoles), &targetRolesList)
		}
		sub := map[string]any{
			"subcategory_id":   r.SubcategoryID,
			"category_id":      r.CategoryID,
			"subcategory_name": r.SubcategoryName,
			"fund_condition":   r.FundCondition,
			"target_roles":     targetRolesList,
			"status":           r.Status,
			"comment":          r.Comment,
			"create_at":        r.CreateAt,
			"update_at":        r.UpdateAt,
			"category": map[string]any{
				"category_id":   r.CategoryID,
				"category_name": r.CategoryName,
			},
			// KEY สำคัญที่ UI ใช้
			"budgets": budgetsBySub[r.SubcategoryID],
		}
		subcategories = append(subcategories, sub)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"subcategories": subcategories,
		"total":         len(subcategories),
	})
}

// CreateSubcategory - Admin creates new fund subcategory
func CreateSubcategory(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	type CreateSubcategoryRequest struct {
		CategoryID      int      `json:"category_id" binding:"required"`
		SubcategoryName string   `json:"subcategory_name" binding:"required"`
		FundCondition   string   `json:"fund_condition"`
		TargetRoles     []string `json:"target_roles"`
		Comment         string   `json:"comment"`
	}

	var req CreateSubcategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate category exists
	var category models.FundCategory
	if err := config.DB.Where("category_id = ? AND delete_at IS NULL", req.CategoryID).
		First(&category).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category_id"})
		return
	}

	// Check if subcategory name already exists in this category
	var existingSubcategory models.FundSubcategory
	if err := config.DB.Where("subcategory_name = ? AND category_id = ? AND delete_at IS NULL",
		req.SubcategoryName, req.CategoryID).First(&existingSubcategory).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Subcategory name already exists in this category",
		})
		return
	}

	// Convert target_roles to JSON string
	var targetRolesJSON *string
	if len(req.TargetRoles) > 0 {
		jsonBytes, err := json.Marshal(req.TargetRoles)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target_roles format"})
			return
		}
		jsonStr := string(jsonBytes)
		targetRolesJSON = &jsonStr
	}

	// Create new subcategory
	now := time.Now()
	var fundCondition *string
	if req.FundCondition != "" {
		fundCondition = &req.FundCondition
	}
	var comment *string
	if req.Comment != "" {
		comment = &req.Comment
	}

	subcategory := models.FundSubcategory{
		CategoryID:      req.CategoryID,
		SubcategoryName: req.SubcategoryName,
		FundCondition:   fundCondition,
		TargetRoles:     targetRolesJSON,
		Status:          "active",
		Comment:         comment,
		CreateAt:        &now,
		UpdateAt:        &now,
	}

	if err := config.DB.Create(&subcategory).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create subcategory"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":     true,
		"message":     "Subcategory created successfully",
		"subcategory": subcategory,
	})
}

// UpdateSubcategory - Admin updates fund subcategory
func UpdateSubcategory(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	subcategoryID := c.Param("id")

	type UpdateSubcategoryRequest struct {
		CategoryID      int      `json:"category_id"`
		SubcategoryName string   `json:"subcategory_name"`
		FundCondition   string   `json:"fund_condition"`
		TargetRoles     []string `json:"target_roles"`
		Status          string   `json:"status"`
		Comment         string   `json:"comment"`
	}

	var req UpdateSubcategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find subcategory
	var subcategory models.FundSubcategory
	if err := config.DB.Where("subcategory_id = ? AND delete_at IS NULL", subcategoryID).
		First(&subcategory).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subcategory not found"})
		return
	}

	// Validate category exists if changed
	if req.CategoryID != 0 && req.CategoryID != subcategory.CategoryID {
		var category models.FundCategory
		if err := config.DB.Where("category_id = ? AND delete_at IS NULL", req.CategoryID).
			First(&category).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category_id"})
			return
		}
	}

	// Check name conflict if name changed
	if req.SubcategoryName != "" && req.SubcategoryName != subcategory.SubcategoryName {
		categoryIDToCheck := req.CategoryID
		if categoryIDToCheck == 0 {
			categoryIDToCheck = subcategory.CategoryID
		}

		var existingSubcategory models.FundSubcategory
		if err := config.DB.Where("subcategory_name = ? AND category_id = ? AND subcategory_id != ? AND delete_at IS NULL",
			req.SubcategoryName, categoryIDToCheck, subcategoryID).First(&existingSubcategory).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Subcategory name already exists in this category",
			})
			return
		}
	}

	// Prepare updates
	now := time.Now()
	updates := map[string]interface{}{
		"update_at": &now,
	}

	if req.CategoryID != 0 {
		updates["category_id"] = req.CategoryID
	}
	if req.SubcategoryName != "" {
		updates["subcategory_name"] = req.SubcategoryName
	}
	if req.FundCondition != "" {
		updates["fund_condition"] = req.FundCondition
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.Comment != "" {
		updates["comment"] = req.Comment
	}

	// Handle target_roles
	if req.TargetRoles != nil {
		if len(req.TargetRoles) > 0 {
			jsonBytes, err := json.Marshal(req.TargetRoles)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target_roles format"})
				return
			}
			updates["target_roles"] = string(jsonBytes)
		} else {
			updates["target_roles"] = nil
		}
	}

	if err := config.DB.Model(&subcategory).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update subcategory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "Subcategory updated successfully",
		"subcategory": subcategory,
	})
}

// DeleteSubcategory - Admin soft deletes fund subcategory
func DeleteSubcategory(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	subcategoryID := c.Param("id")

	// Find subcategory
	var subcategory models.FundSubcategory
	if err := config.DB.Where("subcategory_id = ? AND delete_at IS NULL", subcategoryID).
		First(&subcategory).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subcategory not found"})
		return
	}

	// Check if subcategory has active applications
	var applicationCount int64
	config.DB.Model(&models.FundApplication{}).
		Where("subcategory_id = ? AND delete_at IS NULL", subcategoryID).
		Count(&applicationCount)

	if applicationCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Cannot delete subcategory that has applications",
			"details": fmt.Sprintf("Subcategory has %d applications", applicationCount),
		})
		return
	}

	// Check if subcategory has budgets
	var budgetCount int64
	config.DB.Model(&models.SubcategoryBudget{}).
		Where("subcategory_id = ? AND delete_at IS NULL", subcategoryID).
		Count(&budgetCount)

	if budgetCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Cannot delete subcategory that has budget allocations",
			"details": fmt.Sprintf("Subcategory has %d budget records", budgetCount),
		})
		return
	}

	// Soft delete
	now := time.Now()
	subcategory.DeleteAt = &now

	if err := config.DB.Save(&subcategory).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete subcategory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Subcategory deleted successfully",
	})
}

// ToggleSubcategoryStatus - Admin toggles subcategory active/disable status
func ToggleSubcategoryStatus(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	subcategoryID := c.Param("id")

	// Find subcategory
	var subcategory models.FundSubcategory
	if err := config.DB.Where("subcategory_id = ? AND delete_at IS NULL", subcategoryID).
		First(&subcategory).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subcategory not found"})
		return
	}

	// Toggle status
	newStatus := "active"
	if subcategory.Status == "active" {
		newStatus = "disable"
	}

	now := time.Now()
	subcategory.Status = newStatus
	subcategory.UpdateAt = &now

	if err := config.DB.Save(&subcategory).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle subcategory status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     fmt.Sprintf("Subcategory status changed to %s", newStatus),
		"subcategory": subcategory,
		"new_status":  newStatus,
	})
}

// ===================== BULK OPERATIONS =====================

// BulkUpdateSubcategoryRoles - Admin bulk updates target_roles for multiple subcategories
func BulkUpdateSubcategoryRoles(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	type BulkRoleUpdate struct {
		SubcategoryID int      `json:"subcategory_id" binding:"required"`
		TargetRoles   []string `json:"target_roles"`
	}

	type BulkUpdateRequest struct {
		Updates []BulkRoleUpdate `json:"updates" binding:"required"`
	}

	var req BulkUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No updates provided"})
		return
	}

	// Process bulk updates
	successCount := 0
	errorCount := 0
	var errors []string

	for _, update := range req.Updates {
		// Find subcategory
		var subcategory models.FundSubcategory
		if err := config.DB.Where("subcategory_id = ? AND delete_at IS NULL", update.SubcategoryID).
			First(&subcategory).Error; err != nil {
			errorCount++
			errors = append(errors, fmt.Sprintf("Subcategory ID %d not found", update.SubcategoryID))
			continue
		}

		// Convert target_roles to JSON
		var targetRolesJSON *string
		if len(update.TargetRoles) > 0 {
			jsonBytes, err := json.Marshal(update.TargetRoles)
			if err != nil {
				errorCount++
				errors = append(errors, fmt.Sprintf("Invalid target_roles for subcategory ID %d", update.SubcategoryID))
				continue
			}
			jsonStr := string(jsonBytes)
			targetRolesJSON = &jsonStr
		}

		// Update subcategory
		now := time.Now()
		if err := config.DB.Model(&subcategory).Updates(map[string]interface{}{
			"target_roles": targetRolesJSON,
			"update_at":    &now,
		}).Error; err != nil {
			errorCount++
			errors = append(errors, fmt.Sprintf("Failed to update subcategory ID %d", update.SubcategoryID))
			continue
		}

		successCount++
	}

	c.JSON(http.StatusOK, gin.H{
		"success":            true,
		"message":            "Bulk update completed",
		"successful_updates": successCount,
		"failed_updates":     errorCount,
		"errors":             errors,
		"total_processed":    len(req.Updates),
	})
}

// ===================== YEAR MANAGEMENT =====================

// GetAllYears - Admin can view all years
func GetAllYears(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	var years []models.Year

	// Get all years (including inactive ones for admin)
	if err := config.DB.Where("delete_at IS NULL").Order("year_id DESC").Find(&years).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch years"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"years":   years,
		"total":   len(years),
	})
}

// CreateYear - Admin creates new year
func CreateYear(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	type CreateYearRequest struct {
		Year   string  `json:"year" binding:"required"`
		Budget float64 `json:"budget" binding:"required"`
	}

	var req CreateYearRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if year already exists
	var existingYear models.Year
	if err := config.DB.Where("year = ? AND delete_at IS NULL", req.Year).First(&existingYear).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Year already exists",
		})
		return
	}

	// Create new year
	now := time.Now()
	year := models.Year{
		Year:     req.Year,
		Budget:   req.Budget,
		Status:   "active",
		CreateAt: &now,
		UpdateAt: &now,
	}

	if err := config.DB.Create(&year).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create year"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Year created successfully",
		"year":    year,
	})
}

// UpdateYear - Admin updates year
func UpdateYear(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	yearID := c.Param("id")

	type UpdateYearRequest struct {
		Year   string  `json:"year"`
		Budget float64 `json:"budget"`
		Status string  `json:"status"`
	}

	var req UpdateYearRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find year
	var year models.Year
	if err := config.DB.Where("year_id = ? AND delete_at IS NULL", yearID).
		First(&year).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Year not found"})
		return
	}

	// Check if new year conflicts (if changed)
	if req.Year != "" && req.Year != year.Year {
		var existingYear models.Year
		if err := config.DB.Where("year = ? AND year_id != ? AND delete_at IS NULL",
			req.Year, yearID).First(&existingYear).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Year already exists",
			})
			return
		}
	}

	// Update year
	now := time.Now()
	updates := map[string]interface{}{
		"update_at": &now,
	}

	if req.Year != "" {
		updates["year"] = req.Year
	}
	if req.Budget > 0 {
		updates["budget"] = req.Budget
	}
	if req.Status != "" {
		if req.Status != "active" && req.Status != "inactive" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Status must be 'active' or 'inactive'"})
			return
		}
		updates["status"] = req.Status
	}

	if err := config.DB.Model(&year).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update year"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Year updated successfully",
		"year":    year,
	})
}

// DeleteYear - Admin soft deletes year
func DeleteYear(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	yearID := c.Param("id")

	// Find year
	var year models.Year
	if err := config.DB.Where("year_id = ? AND delete_at IS NULL", yearID).
		First(&year).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Year not found"})
		return
	}

	// Check if year has categories
	var categoryCount int64
	config.DB.Model(&models.FundCategory{}).
		Where("year_id = ? AND delete_at IS NULL", yearID).
		Count(&categoryCount)

	if categoryCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Cannot delete year that has categories",
			"details": fmt.Sprintf("Year has %d categories", categoryCount),
		})
		return
	}

	// Check if year has applications (using new database structure)
	var applicationCount int64
	config.DB.Raw(`
		SELECT COUNT(*)
		FROM submissions s
		WHERE s.year_id = ? AND s.deleted_at IS NULL
	`, yearID).Scan(&applicationCount)

	if applicationCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Cannot delete year that has applications",
			"details": fmt.Sprintf("Year has %d applications", applicationCount),
		})
		return
	}

	// Soft delete
	now := time.Now()
	year.DeleteAt = &now

	if err := config.DB.Save(&year).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete year"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Year deleted successfully",
	})
}

// ToggleYearStatus - Admin toggles year active/inactive status
func ToggleYearStatus(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	yearID := c.Param("id")

	// Find year
	var year models.Year
	if err := config.DB.Where("year_id = ? AND delete_at IS NULL", yearID).
		First(&year).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Year not found"})
		return
	}

	// Toggle status
	newStatus := "active"
	if year.Status == "active" {
		newStatus = "inactive"
	}

	now := time.Now()
	year.Status = newStatus
	year.UpdateAt = &now

	if err := config.DB.Save(&year).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle year status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    fmt.Sprintf("Year status changed to %s", newStatus),
		"year":       year,
		"new_status": newStatus,
	})
}

// GetYearStats - Admin gets year statistics
func GetYearStats(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	yearID := c.Param("id")

	// Get year info
	var year models.Year
	if err := config.DB.Where("year_id = ? AND delete_at IS NULL", yearID).
		First(&year).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Year not found"})
		return
	}

	// Get statistics using raw SQL
	type YearStats struct {
		YearID           int     `json:"year_id"`
		Year             string  `json:"year"`
		Budget           float64 `json:"budget"`
		Status           string  `json:"status"`
		CategoryCount    int64   `json:"category_count"`
		SubcategoryCount int64   `json:"subcategory_count"`
		ApplicationCount int64   `json:"application_count"`
		TotalAllocated   float64 `json:"total_allocated"`
		TotalUsed        float64 `json:"total_used"`
		TotalRemaining   float64 `json:"total_remaining"`
		ApprovedApps     int64   `json:"approved_applications"`
		PendingApps      int64   `json:"pending_applications"`
		RejectedApps     int64   `json:"rejected_applications"`
	}

	var stats YearStats

	// Basic year info
	stats.YearID = year.YearID
	stats.Year = year.Year
	stats.Budget = year.Budget
	stats.Status = year.Status

	// Category count
	config.DB.Model(&models.FundCategory{}).
		Where("year_id = ? AND delete_at IS NULL", yearID).
		Count(&stats.CategoryCount)

	// Subcategory count
	config.DB.Raw(`
		SELECT COUNT(*)
		FROM fund_subcategories fs
		JOIN fund_categories fc ON fs.category_id = fc.category_id
		WHERE fc.year_id = ? AND fs.delete_at IS NULL AND fc.delete_at IS NULL
	`, yearID).Scan(&stats.SubcategoryCount)

	// Application count
	config.DB.Raw(`
		SELECT COUNT(*)
		FROM submissions s
		WHERE s.year_id = ? AND s.deleted_at IS NULL AND s.submission_type = 'fund_application'
	`, yearID).Scan(&stats.ApplicationCount)

	// Budget summary
	config.DB.Raw(`
		SELECT 
			COALESCE(SUM(sb.allocated_amount), 0) as total_allocated,
			COALESCE(SUM(sb.used_amount), 0) as total_used,
			COALESCE(SUM(sb.remaining_budget), 0) as total_remaining
		FROM subcategory_budgets sb
		JOIN fund_subcategories fs ON sb.subcategory_id = fs.subcategory_id
		JOIN fund_categories fc ON fs.category_id = fc.category_id
		WHERE fc.year_id = ? AND sb.delete_at IS NULL AND fs.delete_at IS NULL AND fc.delete_at IS NULL
	`, yearID).Scan(&stats)

	// Application status counts
	config.DB.Raw(`
		SELECT COUNT(*)
		FROM submissions s
		WHERE s.year_id = ? AND s.status_id = 2 AND s.deleted_at IS NULL AND s.submission_type = 'fund_application'
	`, yearID).Scan(&stats.ApprovedApps)

	config.DB.Raw(`
		SELECT COUNT(*)
		FROM submissions s
		WHERE s.year_id = ? AND s.status_id = 1 AND s.deleted_at IS NULL AND s.submission_type = 'fund_application'
	`, yearID).Scan(&stats.PendingApps)

	config.DB.Raw(`
		SELECT COUNT(*)
		FROM submissions s
		WHERE s.year_id = ? AND s.status_id = 3 AND s.deleted_at IS NULL AND s.submission_type = 'fund_application'
	`, yearID).Scan(&stats.RejectedApps)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
	})
}

// ===================== SUBCATEGORY BUDGETS MANAGEMENT =====================

// GetAllSubcategoryBudgets - Admin can view all subcategory budgets
func GetAllSubcategoryBudgets(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	subcategoryID := c.Query("subcategory_id")
	recordScope := c.Query("record_scope")
	scopeIsAll := strings.EqualFold(recordScope, "all")
	if recordScope == "" {
		recordScope = "rule"
	}

	// Use raw SQL to get budget data with subcategory info
	baseQuery := `
                SELECT
                        sb.subcategory_budget_id,
                        sb.subcategory_id,
                        sb.record_scope,
                        sb.allocated_amount,
                        sb.used_amount,
                        sb.remaining_budget,
                        sb.max_amount_per_year,
                        sb.max_grants,
                        sb.max_amount_per_grant,
                        sb.remaining_grant,
			sb.level,
			sb.status,
			sb.fund_description,
			sb.comment,
			sb.create_at,
			sb.update_at,
			fs.subcategory_name,
			fc.category_name
		FROM subcategory_budgets sb
		LEFT JOIN fund_subcategories fs ON sb.subcategory_id = fs.subcategory_id
		LEFT JOIN fund_categories fc ON fs.category_id = fc.category_id
		WHERE sb.delete_at IS NULL`

	var args []interface{}

	if subcategoryID != "" {
		baseQuery += " AND sb.subcategory_id = ?"
		args = append(args, subcategoryID)
	}

	if !scopeIsAll {
		baseQuery += " AND sb.record_scope = ?"
		args = append(args, recordScope)
	}

	baseQuery += " ORDER BY sb.subcategory_budget_id DESC"

	// Execute query
	rows, err := config.DB.Raw(baseQuery, args...).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch subcategory budgets",
			"debug": err.Error(),
		})
		return
	}
	defer rows.Close()

	var budgets []map[string]interface{}

	for rows.Next() {
		var (
			budgetID          int
			subcategoryID     int
			recordScope       string
			allocatedAmount   float64
			usedAmount        float64
			remainingBudget   float64
			maxAmountPerYear  *float64
			maxGrants         *int
			maxAmountPerGrant float64
			remainingGrant    *int
			level             *string
			status            string
			fundDescription   *string
			comment           *string
			createAt          *time.Time
			updateAt          *time.Time
			subcategoryName   *string
			categoryName      *string
		)

		err := rows.Scan(
			&budgetID,
			&subcategoryID,
			&recordScope,
			&allocatedAmount,
			&usedAmount,
			&remainingBudget,
			&maxAmountPerYear,
			&maxGrants,
			&maxAmountPerGrant,
			&remainingGrant,
			&level,
			&status,
			&fundDescription,
			&comment,
			&createAt,
			&updateAt,
			&subcategoryName,
			&categoryName,
		)
		if err != nil {
			continue
		}

		budget := map[string]interface{}{
			"subcategory_budget_id": budgetID,
			"subcategory_id":        subcategoryID,
			"record_scope":          recordScope,
			"allocated_amount":      allocatedAmount,
			"used_amount":           usedAmount,
			"remaining_budget":      remainingBudget,
			"max_amount_per_year":   maxAmountPerYear,
			"max_grants":            maxGrants,
			"max_amount_per_grant":  maxAmountPerGrant,
			"remaining_grant":       remainingGrant,
			"level":                 level,
			"status":                status,
			"fund_description":      fundDescription,
			"comment":               comment,
			"create_at":             createAt,
			"update_at":             updateAt,
			"subcategory": map[string]interface{}{
				"subcategory_id":   subcategoryID,
				"subcategory_name": subcategoryName,
				"category_name":    categoryName,
			},
		}

		budgets = append(budgets, budget)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"budgets": budgets,
		"total":   len(budgets),
	})
}

// GetSubcategoryBudget - Admin gets specific subcategory budget
func GetSubcategoryBudget(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	budgetID := c.Param("id")

	// Use raw SQL to get budget data with subcategory info
	query := `
                SELECT
                        sb.subcategory_budget_id,
                        sb.subcategory_id,
                        sb.record_scope,
                        sb.allocated_amount,
                        sb.used_amount,
                        sb.remaining_budget,
                        sb.max_amount_per_year,
                        sb.max_grants,
                        sb.max_amount_per_grant,
                        sb.remaining_grant,
			sb.level,
			sb.status,
			sb.fund_description,
			sb.comment,
			sb.create_at,
			sb.update_at,
			fs.subcategory_name,
			fc.category_name,
			fc.category_id
		FROM subcategory_budgets sb
		LEFT JOIN fund_subcategories fs ON sb.subcategory_id = fs.subcategory_id
		LEFT JOIN fund_categories fc ON fs.category_id = fc.category_id
		WHERE sb.subcategory_budget_id = ? AND sb.delete_at IS NULL`

	var (
		budgetIDInt       int
		subcategoryID     int
		recordScope       string
		allocatedAmount   float64
		usedAmount        float64
		remainingBudget   float64
		maxAmountPerYear  *float64
		maxGrants         *int
		maxAmountPerGrant float64
		remainingGrant    *int
		level             *string
		status            string
		fundDescription   *string
		comment           *string
		createAt          *time.Time
		updateAt          *time.Time
		subcategoryName   *string
		categoryName      *string
		categoryID        *int
	)

	err := config.DB.Raw(query, budgetID).Row().Scan(
		&budgetIDInt,
		&subcategoryID,
		&recordScope,
		&allocatedAmount,
		&usedAmount,
		&remainingBudget,
		&maxAmountPerYear,
		&maxGrants,
		&maxAmountPerGrant,
		&remainingGrant,
		&level,
		&status,
		&fundDescription,
		&comment,
		&createAt,
		&updateAt,
		&subcategoryName,
		&categoryName,
		&categoryID,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subcategory budget not found"})
		return
	}

	budget := map[string]interface{}{
		"subcategory_budget_id": budgetIDInt,
		"subcategory_id":        subcategoryID,
		"record_scope":          recordScope,
		"allocated_amount":      allocatedAmount,
		"used_amount":           usedAmount,
		"remaining_budget":      remainingBudget,
		"max_amount_per_year":   maxAmountPerYear,
		"max_grants":            maxGrants,
		"max_amount_per_grant":  maxAmountPerGrant,
		"remaining_grant":       remainingGrant,
		"level":                 level,
		"status":                status,
		"fund_description":      fundDescription,
		"comment":               comment,
		"create_at":             createAt,
		"update_at":             updateAt,
		"subcategory": map[string]interface{}{
			"subcategory_id":   subcategoryID,
			"subcategory_name": subcategoryName,
			"category_id":      categoryID,
			"category_name":    categoryName,
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"budget":  budget,
	})
}

// CreateSubcategoryBudget - Admin creates new subcategory budget
func CreateSubcategoryBudget(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	type CreateBudgetRequest struct {
		SubcategoryID     int         `json:"subcategory_id" binding:"required"`
		AllocatedAmount   float64     `json:"allocated_amount"`
		MaxAmountPerYear  *float64    `json:"max_amount_per_year"`
		MaxGrants         interface{} `json:"max_grants"`
		MaxAmountPerGrant *float64    `json:"max_amount_per_grant"`
		Level             string      `json:"level"`
		FundDescription   string      `json:"fund_description"`
		Comment           string      `json:"comment"`
		RecordScope       string      `json:"record_scope"`
	}

	var req CreateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Normalize scope
	scope := strings.ToLower(req.RecordScope)
	if scope == "" {
		scope = "rule"
	}
	if scope != "rule" && scope != "overall" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "record_scope must be either 'rule' or 'overall'"})
		return
	}

	if scope == "rule" {
		if req.MaxAmountPerGrant == nil || *req.MaxAmountPerGrant <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_amount_per_grant must be provided for rule scope"})
			return
		}
		// Yearly cap ใช้เฉพาะ overall เท่านั้น
		req.MaxAmountPerYear = nil
	}

	// Validate subcategory exists
	var subcategory models.FundSubcategory
	if err := config.DB.Where("subcategory_id = ? AND delete_at IS NULL", req.SubcategoryID).
		First(&subcategory).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subcategory_id"})
		return
	}

	// Create new budget using raw SQL
	now := time.Now()
	insertQuery := `
                INSERT INTO subcategory_budgets (
                        subcategory_id, record_scope, allocated_amount, used_amount, remaining_budget,
                        max_amount_per_year, max_grants, max_amount_per_grant, remaining_grant, level,
                        status, fund_description, comment, create_at, update_at
                ) VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, ?)`

	var level, fundDescription, comment interface{}
	if scope == "rule" && req.Level != "" {
		level = req.Level
	}
	if req.FundDescription != "" {
		fundDescription = req.FundDescription
	}
	if req.Comment != "" {
		comment = req.Comment
	}

	var maxAmountPerYear interface{}
	if req.MaxAmountPerYear != nil && *req.MaxAmountPerYear > 0 {
		maxAmountPerYear = *req.MaxAmountPerYear
	}

	var maxGrants interface{}
	var remainingGrant interface{}

	switch v := req.MaxGrants.(type) {
	case float64:
		// JSON numbers come as float64
		maxGrantsInt := int(v)
		if maxGrantsInt <= 0 {
			// ถ้าเป็น 0 หรือน้อยกว่า ให้เป็น NULL
			maxGrants = nil
			remainingGrant = nil
		} else {
			maxGrants = maxGrantsInt
			remainingGrant = maxGrantsInt
		}
	case nil:
		// ถ้าเป็น null
		maxGrants = nil
		remainingGrant = nil
	default:
		// ถ้าเป็น type อื่น ให้เป็น NULL
		maxGrants = nil
		remainingGrant = nil
	}

	var maxAmountPerGrant interface{}
	if scope == "rule" {
		maxAmountPerGrant = *req.MaxAmountPerGrant
	}

	result := config.DB.Exec(insertQuery,
		req.SubcategoryID,
		scope,
		req.AllocatedAmount,
		req.AllocatedAmount, // remaining_budget = allocated_amount initially
		maxAmountPerYear,
		maxGrants, // ใช้ตัวแปรที่ process แล้ว
		maxAmountPerGrant,
		remainingGrant, // ใช้ตัวแปรที่ process แล้ว
		level,
		fundDescription,
		comment,
		now,
		now,
	)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create subcategory budget"})
		return
	}

	// Get the created budget ID
	var budgetID int64
	config.DB.Raw("SELECT LAST_INSERT_ID()").Scan(&budgetID)

	c.JSON(http.StatusCreated, gin.H{
		"success":               true,
		"message":               "Subcategory budget created successfully",
		"subcategory_budget_id": budgetID,
	})
}

// UpdateSubcategoryBudget - Admin updates subcategory budget
func UpdateSubcategoryBudget(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	budgetID := c.Param("id")

	jsonData, _ := c.GetRawData()

	// ตรวจสอบว่ามี field สำคัญใน JSON หรือไม่
	hasMaxGrantsField := strings.Contains(string(jsonData), `"max_grants"`)
	hasMaxAmountPerYearField := strings.Contains(string(jsonData), `"max_amount_per_year"`)

	// Parse JSON อีกครั้งเพื่อใช้งาน
	c.Request.Body = io.NopCloser(strings.NewReader(string(jsonData)))

	type UpdateBudgetRequest struct {
		AllocatedAmount   *float64    `json:"allocated_amount"`
		MaxAmountPerYear  *float64    `json:"max_amount_per_year"`
		MaxGrants         interface{} `json:"max_grants"`
		MaxAmountPerGrant *float64    `json:"max_amount_per_grant"`
		Level             string      `json:"level"`
		Status            string      `json:"status"`
		FundDescription   string      `json:"fund_description"`
		Comment           string      `json:"comment"`
		RecordScope       string      `json:"record_scope"`
		HasMaxGrants      bool        `json:"has_max_grants"`
	}

	var req UpdateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scopeValue := strings.ToLower(req.RecordScope)

	// Check if budget exists
	var existingBudget struct {
		SubcategoryBudgetID int
		AllocatedAmount     float64
		UsedAmount          float64
		RecordScope         string
	}

	err := config.DB.Raw("SELECT subcategory_budget_id, allocated_amount, used_amount, record_scope FROM subcategory_budgets WHERE subcategory_budget_id = ? AND delete_at IS NULL", budgetID).
		Scan(&existingBudget).Error

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subcategory budget not found"})
		return
	}

	effectiveScope := strings.ToLower(existingBudget.RecordScope)

	// Build update query dynamically
	setParts := []string{}
	args := []interface{}{}

	if req.AllocatedAmount != nil {
		setParts = append(setParts, "allocated_amount = ?")
		args = append(args, *req.AllocatedAmount)

		// Update remaining_budget when allocated_amount changes
		newRemainingBudget := *req.AllocatedAmount - existingBudget.UsedAmount
		setParts = append(setParts, "remaining_budget = ?")
		args = append(args, newRemainingBudget)
	}

	if req.RecordScope != "" {
		if scopeValue != "rule" && scopeValue != "overall" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "record_scope must be either 'rule' or 'overall'"})
			return
		}
		setParts = append(setParts, "record_scope = ?")
		args = append(args, scopeValue)
		effectiveScope = scopeValue

		if scopeValue == "overall" {
			// สำหรับแถวสรุป ไม่จำเป็นต้องมี level
			setParts = append(setParts, "level = NULL")
			// max_amount_per_grant/remaining_grant จะถูกอัปเดตตามข้อมูลที่ส่งมาภายหลัง (ถ้ามี)
		} else {
			// rule scope ไม่ควรมี yearly cap
			setParts = append(setParts, "max_amount_per_year = NULL")
		}
	}

	if hasMaxGrantsField {
		setParts = append(setParts, "max_grants = ?")
		setParts = append(setParts, "remaining_grant = ?")

		switch v := req.MaxGrants.(type) {
		case float64:
			maxGrants := int(v)
			if maxGrants <= 0 {
				// 0 หรือน้อยกว่า = NULL
				args = append(args, nil, nil)
			} else {
				args = append(args, maxGrants, maxGrants)
			}
		case nil:
			// null = NULL
			args = append(args, nil, nil)
		default:
			// อื่นๆ = NULL
			args = append(args, nil, nil)
		}
	}

	if req.MaxAmountPerGrant != nil {
		setParts = append(setParts, "max_amount_per_grant = ?")
		args = append(args, *req.MaxAmountPerGrant)
	} else if scopeValue == "overall" {
		// หากสลับเป็น overall แล้วไม่มีค่าใหม่ ให้ล้างค่า max_amount_per_grant
		setParts = append(setParts, "max_amount_per_grant = NULL")
	}

	if hasMaxAmountPerYearField {
		setParts = append(setParts, "max_amount_per_year = ?")
		if req.MaxAmountPerYear != nil && *req.MaxAmountPerYear > 0 && effectiveScope == "overall" {
			args = append(args, *req.MaxAmountPerYear)
		} else {
			args = append(args, nil)
		}
	}

	if req.Level != "" && effectiveScope != "overall" {
		setParts = append(setParts, "level = ?")
		args = append(args, req.Level)
	}

	if req.Status != "" {
		setParts = append(setParts, "status = ?")
		args = append(args, req.Status)
	}

	if req.FundDescription != "" {
		setParts = append(setParts, "fund_description = ?")
		args = append(args, req.FundDescription)
	}

	if req.Comment != "" {
		setParts = append(setParts, "comment = ?")
		args = append(args, req.Comment)
	}

	if len(setParts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}

	// Add update timestamp
	setParts = append(setParts, "update_at = ?")
	args = append(args, time.Now())

	// Add WHERE clause
	args = append(args, budgetID)

	updateQuery := fmt.Sprintf("UPDATE subcategory_budgets SET %s WHERE subcategory_budget_id = ?",
		strings.Join(setParts, ", "))

	if err := config.DB.Exec(updateQuery, args...).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update subcategory budget"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Subcategory budget updated successfully",
	})
}

// DeleteSubcategoryBudget - Admin soft deletes subcategory budget
func DeleteSubcategoryBudget(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	budgetID := c.Param("id")

	// Check if budget exists and has any usage
	var budgetInfo struct {
		SubcategoryBudgetID int
		UsedAmount          float64
		SubcategoryID       int
	}

	err := config.DB.Raw("SELECT subcategory_budget_id, used_amount, subcategory_id FROM subcategory_budgets WHERE subcategory_budget_id = ? AND delete_at IS NULL", budgetID).
		Scan(&budgetInfo).Error

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subcategory budget not found"})
		return
	}

	// Check if budget has been used
	if budgetInfo.UsedAmount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Cannot delete budget that has been used",
			"details": fmt.Sprintf("Budget has used amount: ฿%.2f", budgetInfo.UsedAmount),
		})
		return
	}

	// Check if budget has applications (using the new database structure)
	var applicationCount int64
	config.DB.Raw(`
		SELECT COUNT(*)
		FROM fund_application_details fad
		JOIN submissions s ON fad.submission_id = s.submission_id
		WHERE fad.subcategory_id = ? AND s.deleted_at IS NULL
	`, budgetInfo.SubcategoryID).Scan(&applicationCount)

	if applicationCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Cannot delete budget that has applications",
			"details": fmt.Sprintf("Subcategory has %d applications", applicationCount),
		})
		return
	}

	// Soft delete
	now := time.Now()
	if err := config.DB.Exec("UPDATE subcategory_budgets SET delete_at = ? WHERE subcategory_budget_id = ?", now, budgetID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete subcategory budget"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Subcategory budget deleted successfully",
	})
}

// ToggleSubcategoryBudgetStatus - Admin toggles budget active/disable status
func ToggleSubcategoryBudgetStatus(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	budgetID := c.Param("id")

	// Get current status
	var currentStatus string
	err := config.DB.Raw("SELECT status FROM subcategory_budgets WHERE subcategory_budget_id = ? AND delete_at IS NULL", budgetID).
		Scan(&currentStatus).Error

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subcategory budget not found"})
		return
	}

	// Toggle status
	newStatus := "active"
	if currentStatus == "active" {
		newStatus = "disable"
	}

	now := time.Now()
	if err := config.DB.Exec("UPDATE subcategory_budgets SET status = ?, update_at = ? WHERE subcategory_budget_id = ?", newStatus, now, budgetID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle budget status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    fmt.Sprintf("Budget status changed to %s", newStatus),
		"new_status": newStatus,
	})
}

// CopyFundConfigurationToYear - Admin duplicates categories, subcategories, and budgets to a new year
func CopyFundConfigurationToYear(c *gin.Context) {
	// Ensure admin role
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	type CopyRequest struct {
		SourceYearID int      `json:"source_year_id" binding:"required"`
		TargetYear   string   `json:"target_year" binding:"required"`
		TargetBudget *float64 `json:"target_budget"`
	}

	var req CopyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	targetYearValue := strings.TrimSpace(req.TargetYear)
	if targetYearValue == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_year is required"})
		return
	}

	// Validate source year
	var sourceYear models.Year
	if err := config.DB.Where("year_id = ? AND delete_at IS NULL", req.SourceYearID).
		First(&sourceYear).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Source year not found"})
		return
	}

	// Ensure target year does not exist
	var existingYear models.Year
	if err := config.DB.Where("year = ? AND delete_at IS NULL", targetYearValue).
		First(&existingYear).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Target year already exists"})
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	// Helper for rollback with response
	rollbackWithError := func(status int, message string, debug string) {
		tx.Rollback()
		payload := gin.H{"error": message}
		if debug != "" {
			payload["debug"] = debug
		}
		c.JSON(status, payload)
	}

	targetBudget := sourceYear.Budget
	if req.TargetBudget != nil {
		targetBudget = *req.TargetBudget
	}

	now := time.Now()
	newYear := models.Year{
		Year:   targetYearValue,
		Budget: targetBudget,
		Status: sourceYear.Status,
	}
	if newYear.Status == "" {
		newYear.Status = "active"
	}
	newYear.CreateAt = &now
	newYear.UpdateAt = &now

	if err := tx.Create(&newYear).Error; err != nil {
		rollbackWithError(http.StatusInternalServerError, "Failed to create target year", err.Error())
		return
	}

	// Copy categories
	var categories []models.FundCategory
	if err := tx.Where("year_id = ? AND delete_at IS NULL", req.SourceYearID).
		Find(&categories).Error; err != nil {
		rollbackWithError(http.StatusInternalServerError, "Failed to load categories", err.Error())
		return
	}

	categoryMap := make(map[int]int)
	for _, category := range categories {
		currentTime := time.Now()
		newCategory := models.FundCategory{
			CategoryName: category.CategoryName,
			Status:       category.Status,
			YearID:       newYear.YearID,
			CreateAt:     &currentTime,
			UpdateAt:     &currentTime,
		}
		if newCategory.Status == "" {
			newCategory.Status = "active"
		}

		if err := tx.Create(&newCategory).Error; err != nil {
			rollbackWithError(http.StatusInternalServerError, "Failed to copy categories", err.Error())
			return
		}
		categoryMap[category.CategoryID] = newCategory.CategoryID
	}

	// Copy subcategories
	subcategoryMap := make(map[int]int)
	if len(categoryMap) > 0 {
		originalCategoryIDs := make([]int, 0, len(categoryMap))
		for originalID := range categoryMap {
			originalCategoryIDs = append(originalCategoryIDs, originalID)
		}

		var subcategories []models.FundSubcategory
		if err := tx.Where("category_id IN (?) AND delete_at IS NULL", originalCategoryIDs).
			Find(&subcategories).Error; err != nil {
			rollbackWithError(http.StatusInternalServerError, "Failed to load subcategories", err.Error())
			return
		}

		for _, subcategory := range subcategories {
			mappedCategoryID, ok := categoryMap[subcategory.CategoryID]
			if !ok {
				continue
			}

			currentTime := time.Now()
			newSubcategory := models.FundSubcategory{
				CategoryID:      mappedCategoryID,
				SubcategoryName: subcategory.SubcategoryName,
				FundCondition:   subcategory.FundCondition,
				TargetRoles:     subcategory.TargetRoles,
				FormType:        subcategory.FormType,
				FormURL:         subcategory.FormURL,
				Status:          subcategory.Status,
				Comment:         subcategory.Comment,
				CreateAt:        &currentTime,
				UpdateAt:        &currentTime,
			}
			if newSubcategory.Status == "" {
				newSubcategory.Status = "active"
			}

			if err := tx.Create(&newSubcategory).Error; err != nil {
				rollbackWithError(http.StatusInternalServerError, "Failed to copy subcategories", err.Error())
				return
			}
			subcategoryMap[subcategory.SubcategoryID] = newSubcategory.SubcategoryID
		}
	}

	// Copy budgets
	budgetsCopied := 0
	if len(subcategoryMap) > 0 {
		originalSubcategoryIDs := make([]int, 0, len(subcategoryMap))
		for originalID := range subcategoryMap {
			originalSubcategoryIDs = append(originalSubcategoryIDs, originalID)
		}

		rows, err := tx.Table("subcategory_budgets").
			Select("subcategory_id, record_scope, allocated_amount, max_amount_per_year, max_grants, max_amount_per_grant, level, status, fund_description, comment").
			Where("delete_at IS NULL AND subcategory_id IN (?)", originalSubcategoryIDs).
			Rows()
		if err != nil {
			rollbackWithError(http.StatusInternalServerError, "Failed to load budgets", err.Error())
			return
		}
		defer rows.Close()

		for rows.Next() {
			var (
				subcategoryID     int
				recordScope       string
				allocatedAmount   float64
				maxAmountPerYear  sql.NullFloat64
				maxGrants         sql.NullInt64
				maxAmountPerGrant sql.NullFloat64
				level             sql.NullString
				statusValue       sql.NullString
				fundDescription   sql.NullString
				commentValue      sql.NullString
			)

			if err := rows.Scan(
				&subcategoryID,
				&recordScope,
				&allocatedAmount,
				&maxAmountPerYear,
				&maxGrants,
				&maxAmountPerGrant,
				&level,
				&statusValue,
				&fundDescription,
				&commentValue,
			); err != nil {
				rollbackWithError(http.StatusInternalServerError, "Failed to read budget row", err.Error())
				return
			}

			mappedSubcategoryID, ok := subcategoryMap[subcategoryID]
			if !ok {
				continue
			}

			scope := strings.ToLower(recordScope)

			insertAllocated := 0.0
			remainingBudget := 0.0
			var maxAmountPerYearVal interface{}
			var maxGrantsVal interface{}
			var remainingGrant interface{}

			if scope == "overall" {
				insertAllocated = allocatedAmount
				remainingBudget = allocatedAmount

				if maxAmountPerYear.Valid && maxAmountPerYear.Float64 > 0 {
					maxAmountPerYearVal = maxAmountPerYear.Float64
				}

				if maxGrants.Valid && maxGrants.Int64 > 0 {
					grants := int(maxGrants.Int64)
					maxGrantsVal = grants
					remainingGrant = grants
				}
			}

			var maxAmountPerGrantVal interface{}
			if maxAmountPerGrant.Valid && maxAmountPerGrant.Float64 > 0 {
				maxAmountPerGrantVal = maxAmountPerGrant.Float64
			}

			if scope != "overall" {
				maxAmountPerYearVal = nil
				maxGrantsVal = nil
				remainingGrant = nil
			}

			var levelVal interface{}
			if scope == "rule" && level.Valid && strings.TrimSpace(level.String) != "" {
				levelVal = level.String
			}

			statusText := "active"
			if statusValue.Valid && strings.TrimSpace(statusValue.String) != "" {
				statusText = statusValue.String
			}

			var fundDescriptionVal interface{}
			if fundDescription.Valid && strings.TrimSpace(fundDescription.String) != "" {
				fundDescriptionVal = fundDescription.String
			}

			var commentVal interface{}
			if commentValue.Valid && strings.TrimSpace(commentValue.String) != "" {
				commentVal = commentValue.String
			}

			currentTime := time.Now()
			if err := tx.Exec(
				`INSERT INTO subcategory_budgets (
                                        subcategory_id, record_scope, allocated_amount, used_amount, remaining_budget,
                                        max_amount_per_year, max_grants, max_amount_per_grant, remaining_grant, level,
                                        status, fund_description, comment, create_at, update_at
                                ) VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				mappedSubcategoryID,
				scope,
				insertAllocated,
				remainingBudget,
				maxAmountPerYearVal,
				maxGrantsVal,
				maxAmountPerGrantVal,
				remainingGrant,
				levelVal,
				statusText,
				fundDescriptionVal,
				commentVal,
				currentTime,
				currentTime,
			).Error; err != nil {
				rollbackWithError(http.StatusInternalServerError, "Failed to copy budgets", err.Error())
				return
			}

			budgetsCopied++
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize copy operation", "debug": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Copied fund configuration from year %s to %s", sourceYear.Year, targetYearValue),
		"year":    newYear,
		"copied": gin.H{
			"categories":    len(categoryMap),
			"subcategories": len(subcategoryMap),
			"budgets":       budgetsCopied,
		},
	})
}

// GetCategoryStats - Admin gets category statistics
func GetCategoryStats(c *gin.Context) {
	// Check if user is admin
	roleID, _ := c.Get("roleID")
	if roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Updated SQL to use the correct tables and views from v10 database
	query := `
		SELECT 
			fc.category_id,
			fc.category_name,
			fc.status,
			COALESCE(sub_count.subcategory_count, 0) as subcategory_count,
			COALESCE(app_count.application_count, 0) as application_count,
			COALESCE(budget_summary.total_allocated, 0) as total_allocated,
			COALESCE(budget_summary.total_used, 0) as total_used,
			COALESCE(budget_summary.total_remaining, 0) as total_remaining
		FROM fund_categories fc
		LEFT JOIN (
			SELECT 
				category_id, 
				COUNT(*) as subcategory_count 
			FROM fund_subcategories 
			WHERE delete_at IS NULL 
			GROUP BY category_id
		) sub_count ON fc.category_id = sub_count.category_id
		LEFT JOIN (
			SELECT 
				fs.category_id, 
				COUNT(fad.detail_id) as application_count
			FROM fund_subcategories fs
			LEFT JOIN fund_application_details fad ON fs.subcategory_id = fad.subcategory_id
			LEFT JOIN submissions s ON fad.submission_id = s.submission_id 
				AND s.deleted_at IS NULL AND s.submission_type = 'fund_application'
			WHERE fs.delete_at IS NULL
			GROUP BY fs.category_id
		) app_count ON fc.category_id = app_count.category_id
		LEFT JOIN (
			SELECT 
				fs.category_id,
				SUM(COALESCE(sb.allocated_amount, 0)) as total_allocated,
				SUM(COALESCE(sb.used_amount, 0)) as total_used,
				SUM(COALESCE(sb.remaining_budget, 0)) as total_remaining
			FROM fund_subcategories fs
			LEFT JOIN subcategory_budgets sb ON fs.subcategory_id = sb.subcategory_id 
				AND sb.delete_at IS NULL AND sb.status = 'active'
			WHERE fs.delete_at IS NULL
			GROUP BY fs.category_id
		) budget_summary ON fc.category_id = budget_summary.category_id
		WHERE fc.delete_at IS NULL
		ORDER BY fc.category_id`

	rows, err := config.DB.Raw(query).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch category statistics",
			"debug": err.Error(),
		})
		return
	}
	defer rows.Close()

	var stats []map[string]interface{}

	for rows.Next() {
		var (
			categoryID       int
			categoryName     string
			status           string
			subcategoryCount int
			applicationCount int
			totalAllocated   float64
			totalUsed        float64
			totalRemaining   float64
		)

		err := rows.Scan(
			&categoryID,
			&categoryName,
			&status,
			&subcategoryCount,
			&applicationCount,
			&totalAllocated,
			&totalUsed,
			&totalRemaining,
		)
		if err != nil {
			continue
		}

		stat := map[string]interface{}{
			"category_id":       categoryID,
			"category_name":     categoryName,
			"status":            status,
			"subcategory_count": subcategoryCount,
			"application_count": applicationCount,
			"total_allocated":   totalAllocated,
			"total_used":        totalUsed,
			"total_remaining":   totalRemaining,
		}

		stats = append(stats, stat)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
		"total":   len(stats),
	})
}
