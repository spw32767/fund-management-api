package controllers

import (
	"fund-management-api/config"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GetCurrentYear returns the current year from system configuration
func GetCurrentYear(c *gin.Context) {
	var result struct {
		CurrentYear int `json:"current_year"`
	}

	if err := config.DB.Table("system_config").
		Select("current_year").
		Order("config_id DESC").
		Limit(1).
		Scan(&result).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch current year"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"current_year": result.CurrentYear})
}

// GetApplicationWindow returns live application window from system_config,
// computed in Asia/Bangkok timezone, and disables all caches so changes take effect immediately.
func GetApplicationWindow(c *gin.Context) {
	// Disable caches (immediate effect when admin updates config)
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	// Read as strings to avoid type issues (current_year is varchar in DB)
	// Schema: system_config(current_year, start_date, end_date, last_updated, ...)
	// see dump for types.
	var row struct {
		CurrentYear string `json:"current_year"`
		StartStr    string `json:"start_date"`
		EndStr      string `json:"end_date"`
		LastUpdated string `json:"last_updated"`
	}

	if err := config.DB.Table("system_config").
		Select(`
			current_year,
			DATE_FORMAT(start_date, '%Y-%m-%d %H:%i:%s') AS start_date,
			DATE_FORMAT(end_date,   '%Y-%m-%d %H:%i:%s') AS end_date,
			DATE_FORMAT(last_updated, '%Y-%m-%d %H:%i:%s') AS last_updated
		`).
		Limit(1). // มี 1 แถวเดียวในระบบ
		Scan(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch system_config"})
		return
	}

	loc, _ := time.LoadLocation("Asia/Bangkok")
	now := time.Now().In(loc)

	parseTS := func(s string) (*time.Time, bool) {
		if s == "" || s == "0000-00-00 00:00:00" {
			return nil, false
		}
		t, err := time.ParseInLocation("2006-01-02 15:04:05", s, loc)
		if err != nil {
			return nil, false
		}
		return &t, true
	}

	start, hasStart := parseTS(row.StartStr)
	end, hasEnd := parseTS(row.EndStr)

	// Window evaluation
	windowState := "not_configured" // no start/end → ถือว่าไม่ตั้งค่า
	isOpen := true                  // ค่าเริ่มต้น: เปิด ถ้าไม่กำหนดช่วงเวลา
	var secsUntilOpen *int64
	var secsUntilClose *int64

	if hasStart && hasEnd {
		if now.Before(*start) {
			windowState = "before_window"
			isOpen = false
			remain := int64(start.Sub(now).Seconds())
			secsUntilOpen = &remain
		} else if (now.Equal(*start) || now.After(*start)) && now.Before(*end) {
			windowState = "open"
			isOpen = true
			remain := int64(end.Sub(now).Seconds())
			secsUntilClose = &remain
		} else {
			windowState = "after_window"
			isOpen = false
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"current_year":        row.CurrentYear, // เช่น "2568" (varchar)
		"start_date":          row.StartStr,    // "YYYY-MM-DD HH:mm:ss"
		"end_date":            row.EndStr,      // "YYYY-MM-DD HH:mm:ss"
		"is_open":             isOpen,          // สำหรับ FE ใช้เปิด/ปิดปุ่ม
		"window_state":        windowState,     // before_window | open | after_window | not_configured
		"seconds_until_open":  secsUntilOpen,   // นับถอยหลังเปิด (ถ้ามี)
		"seconds_until_close": secsUntilClose,  // นับถอยหลังปิด (ถ้ามี)
		"server_now":          now.Format("2006-01-02 15:04:05"),
		"last_updated":        row.LastUpdated,
	})
}
