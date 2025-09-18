// controllers/approval_records.go
package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	// ใช้ DB กลางของโปรเจกต์
	"fund-management-api/config"
)

type ApprovalTotalsRow struct {
	UserID                 uint    `gorm:"column:user_id" json:"user_id"`
	ApplicantName          string  `gorm:"column:applicant_name" json:"applicant_name"`
	YearTH                 string  `gorm:"column:year_th" json:"year_th"`
	CategoryID             *uint   `gorm:"column:category_id" json:"category_id"`
	CategoryName           *string `gorm:"column:category_name" json:"category_name"`
	SubcategoryID          *uint   `gorm:"column:subcategory_id" json:"subcategory_id"`
	SubcategoryName        *string `gorm:"column:subcategory_name" json:"subcategory_name"`
	SubcategoryBudgetID    *uint   `gorm:"column:subcategory_budget_id" json:"subcategory_budget_id"`
	SubcategoryBudgetLabel *string `gorm:"column:subcategory_budget_label" json:"subcategory_budget_label"`
	TotalApprovedAmount    float64 `gorm:"column:total_approved_amount" json:"total_approved_amount"`
}

type ApprovalRecordRow struct {
	SubmissionID           uint    `gorm:"column:submission_id" json:"submission_id"`
	SubmissionNumber       string  `gorm:"column:submission_number" json:"submission_number"`
	SubmissionType         string  `gorm:"column:submission_type" json:"submission_type"`
	UserID                 uint    `gorm:"column:user_id" json:"user_id"`
	ApplicantName          string  `gorm:"column:applicant_name" json:"applicant_name"`
	YearTH                 string  `gorm:"column:year_th" json:"year_th"`
	CategoryID             *uint   `gorm:"column:category_id" json:"category_id"`
	CategoryName           *string `gorm:"column:category_name" json:"category_name"`
	SubcategoryID          *uint   `gorm:"column:subcategory_id" json:"subcategory_id"`
	SubcategoryName        *string `gorm:"column:subcategory_name" json:"subcategory_name"`
	SubcategoryBudgetID    *uint   `gorm:"column:subcategory_budget_id" json:"subcategory_budget_id"`
	SubcategoryBudgetLabel *string `gorm:"column:subcategory_budget_label" json:"subcategory_budget_label"`
	StatusID               uint    `gorm:"column:status_id" json:"status_id"`
	ApprovedBy             *uint   `gorm:"column:approved_by" json:"approved_by"`
	ApprovedAt             *string `gorm:"column:approved_at" json:"approved_at"`
	ApprovedAmount         float64 `gorm:"column:approved_amount" json:"approved_amount"`
}

func parseUintPtr(q string) *uint {
	if q == "" {
		return nil
	}
	n, err := strconv.ParseUint(q, 10, 64)
	if err != nil {
		return nil
	}
	u := uint(n)
	return &u
}
func parsePOS(q string, def int) int {
	if q == "" {
		return def
	}
	n, err := strconv.Atoi(q)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
func safeSortTotals(s string) string {
	whitelist := map[string]bool{
		"category_name": true, "subcategory_name": true, "subcategory_budget_label": true,
	}
	col := strings.ToLower(strings.TrimSpace(s))
	if whitelist[col] {
		return col
	}
	return "category_name"
}
func safeSortRecords(s string) string {
	whitelist := map[string]bool{
		"approved_at": true, "submission_number": true, "applicant_name": true,
	}
	col := strings.ToLower(strings.TrimSpace(s))
	if whitelist[col] {
		return col
	}
	return "approved_at"
}

// GET /api/v1/admin/approval-records/totals?teacher_id=&year=&category_id=&subcategory_id=&subcategory_budget_id=
func GetApprovalTotals(c *gin.Context) {
	db := config.DB

	teacherID := parseUintPtr(c.Query("teacher_id"))
	year := c.Query("year")
	catID := parseUintPtr(c.Query("category_id"))
	subID := parseUintPtr(c.Query("subcategory_id"))
	budgetID := parseUintPtr(c.Query("subcategory_budget_id"))

	var rows []ApprovalTotalsRow
	q := db.Table("v_approval_totals_by_teacher AS t")

	if teacherID != nil {
		q = q.Where("t.user_id = ?", *teacherID)
	}
	if year != "" {
		q = q.Where("t.year_th = ?", year)
	}
	if catID != nil {
		q = q.Where("t.category_id = ?", *catID)
	}
	if subID != nil {
		q = q.Where("t.subcategory_id = ?", *subID)
	}
	if budgetID != nil {
		q = q.Where("t.subcategory_budget_id = ?", *budgetID)
	}

	sort := safeSortTotals(c.Query("sort"))
	dir := strings.ToUpper(c.Query("dir"))
	if dir != "DESC" {
		dir = "ASC"
	}

	q = q.Order("t." + sort + " " + dir).
		Order("t.subcategory_name ASC").
		Order("t.subcategory_budget_label ASC")

	if err := q.Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	grand := 0.0
	for _, r := range rows {
		grand += r.TotalApprovedAmount
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"rows":        rows,
			"grand_total": grand,
		},
	})
}

// GET /api/v1/admin/approval-records?teacher_id=&year=&category_id=&subcategory_id=&subcategory_budget_id=&page=&page_size=&sort=&dir=
func GetApprovalRecords(c *gin.Context) {
	db := config.DB

	teacherID := parseUintPtr(c.Query("teacher_id"))
	year := c.Query("year")
	catID := parseUintPtr(c.Query("category_id"))
	subID := parseUintPtr(c.Query("subcategory_id"))
	budgetID := parseUintPtr(c.Query("subcategory_budget_id"))

	page := parsePOS(c.Query("page"), 1)
	size := parsePOS(c.Query("page_size"), 20)
	offset := (page - 1) * size

	var rows []ApprovalRecordRow
	q := db.Table("v_approval_records AS v")

	if teacherID != nil {
		q = q.Where("v.user_id = ?", *teacherID)
	}
	if year != "" {
		q = q.Where("v.year_th = ?", year)
	}
	if catID != nil {
		q = q.Where("v.category_id = ?", *catID)
	}
	if subID != nil {
		q = q.Where("v.subcategory_id = ?", *subID)
	}
	if budgetID != nil {
		q = q.Where("v.subcategory_budget_id = ?", *budgetID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sort := safeSortRecords(c.Query("sort"))
	dir := strings.ToUpper(c.Query("dir"))
	if dir != "ASC" {
		dir = "DESC"
	}

	if err := q.Order("v." + sort + " " + dir).
		Limit(size).
		Offset(offset).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": rows,
		"meta": gin.H{
			"page":      page,
			"page_size": size,
			"total":     total,
		},
	})
}
