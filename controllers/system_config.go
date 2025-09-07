package controllers

import (
	"database/sql"
	"net/http"
	"time"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

//
// ===== Helpers & Payloads =====
//

// ใช้ตอนอัปเดตค่าจากฝั่งแอดมิน
type UpdateSystemConfigPayload struct {
	// หมายเหตุ: เดิมฝั่ง FE ใช้ปีเป็นข้อความได้ ให้ยอมรับเป็น string เพื่อความยืดหยุ่น
	CurrentYear string  `json:"current_year" binding:"required"`
	StartDate   *string `json:"start_date"` // อนุญาต null/"" = ไม่มีหน้าต่างเวลา
	EndDate     *string `json:"end_date"`   // อนุญาต null/""
}

// parseTimePtr รับ string หลายรูปแบบและแปลงเป็น *time.Time (หรือ nil หากว่าง)
func parseTimePtr(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	layouts := []string{
		"2006-01-02 15:04:05", // ฟอร์แมตที่ DB มักใช้
		time.RFC3339,          // ISO เช่น 2025-09-06T23:01:58Z
		"2006-01-02T15:04:05", // datetime-local (ไม่มีเขตเวลา)
		"2006-01-02",          // วันล้วน
	}
	var lastErr error
	for _, layout := range layouts {
		if tt, err := time.Parse(layout, *s); err == nil {
			return &tt, nil
		} else {
			lastErr = err
		}
	}
	return nil, lastErr
}

// formatPtrTime แปลง *time.Time -> *string ด้วย layout มาตรฐาน (คืน nil หากอินพุต nil)
func formatPtrTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02 15:04:05")
	return &s
}

//
// ===== Public (ไม่ต้องเป็นแอดมิน) =====
//

// GetCurrentYear returns the current year from system configuration
// GET /api/v1/system-config/current-year
func GetCurrentYear(c *gin.Context) {
	var row struct {
		CurrentYear sql.NullString
	}
	if err := config.DB.Raw(`
		SELECT current_year
		FROM system_config
		ORDER BY config_id DESC
		LIMIT 1
	`).Scan(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch current_year"})
		return
	}

	var cur interface{}
	if row.CurrentYear.Valid {
		// ส่งคืนเป็น string (เพื่อความเข้ากันได้กับ FE ที่อาจแสดงเป็นข้อความ)
		cur = row.CurrentYear.String
	} else {
		cur = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"current_year": cur,
	})
}

// GetSystemConfigWindow คืนข้อมูลหน้าต่างเวลา + สถานะเปิด/ปิด (effective)
// GET /api/v1/system-config/window
func GetSystemConfigWindow(c *gin.Context) {
	var row struct {
		CurrentYear sql.NullString
		StartDate   sql.NullTime
		EndDate     sql.NullTime
		LastUpdated sql.NullTime
	}

	if err := config.DB.Raw(`
		SELECT current_year, start_date, end_date, last_updated
		FROM system_config
		ORDER BY config_id DESC
		LIMIT 1
	`).Scan(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch system_config window"})
		return
	}

	// คืนค่าเวลาในรูปแบบ string (หรือ null)
	var startStr, endStr, lastStr *string
	if row.StartDate.Valid {
		startStr = formatPtrTime(&row.StartDate.Time)
	}
	if row.EndDate.Valid {
		endStr = formatPtrTime(&row.EndDate.Time)
	}
	if row.LastUpdated.Valid {
		lastStr = formatPtrTime(&row.LastUpdated.Time)
	}

	// คำนวณสถานะเปิด/ปิดแบบ "effective"
	// นิยาม:
	// - หากทั้ง start และ end เป็น null => ถือว่า "เปิด" ตามค่าเริ่มต้น (ไม่ล็อกเวลา)
	// - หากมีแค่ start   => เปิดหาก now >= start
	// - หากมีแค่ end     => เปิดหาก now <= end
	// - หากมีทั้งคู่      => เปิดหาก start <= now <= end
	now := time.Now().UTC()
	isOpenEffective := true
	if row.StartDate.Valid || row.EndDate.Valid {
		isOpenEffective = false
		if row.StartDate.Valid && row.EndDate.Valid {
			isOpenEffective = (now.Equal(row.StartDate.Time) || now.After(row.StartDate.Time)) &&
				(now.Equal(row.EndDate.Time) || now.Before(row.EndDate.Time))
		} else if row.StartDate.Valid {
			isOpenEffective = now.Equal(row.StartDate.Time) || now.After(row.StartDate.Time)
		} else if row.EndDate.Valid {
			isOpenEffective = now.Equal(row.EndDate.Time) || now.Before(row.EndDate.Time)
		}
	}

	// is_open_raw: ให้ตีความแบบกว้างที่สุด (หากไม่มีหน้าต่าง ให้เปิด)
	isOpenRaw := true
	if row.StartDate.Valid && row.EndDate.Valid {
		isOpenRaw = (now.Equal(row.StartDate.Time) || now.After(row.StartDate.Time)) &&
			(now.Equal(row.EndDate.Time) || now.Before(row.EndDate.Time))
	} else if row.StartDate.Valid {
		isOpenRaw = now.Equal(row.StartDate.Time) || now.After(row.StartDate.Time)
	} else if row.EndDate.Valid {
		isOpenRaw = now.Equal(row.EndDate.Time) || now.Before(row.EndDate.Time)
	} // ถ้าทั้งคู่ว่าง คงค่า true

	c.JSON(http.StatusOK, gin.H{
		"current_year": func() interface{} {
			if row.CurrentYear.Valid {
				return row.CurrentYear.String
			}
			return nil
		}(),
		"start_date":        startStr,
		"end_date":          endStr,
		"last_updated":      lastStr,
		"is_open_raw":       isOpenRaw,
		"is_open_effective": isOpenEffective,
		"now":               now.Format(time.RFC3339Nano),
	})
}

//
// ===== Admin Only =====
//

// GetSystemConfigAdmin: คืนค่าทั้งแถวล่าสุด (admin ใช้เติมฟอร์ม)
// GET /api/v1/admin/system-config
func GetSystemConfigAdmin(c *gin.Context) {
	var row struct {
		ConfigID      int            `json:"config_id"`
		SystemVersion sql.NullString `json:"system_version"`
		CurrentYear   sql.NullString `json:"current_year"`
		StartDate     sql.NullTime   `json:"start_date"`
		EndDate       sql.NullTime   `json:"end_date"`
		LastUpdated   sql.NullTime   `json:"last_updated"`
		UpdatedBy     sql.NullInt64  `json:"updated_by"`
	}

	if err := config.DB.Raw(`
		SELECT config_id, system_version, current_year, start_date, end_date, last_updated, updated_by
		FROM system_config
		ORDER BY config_id DESC
		LIMIT 1
	`).Scan(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to fetch system_config"})
		return
	}

	var startStr, endStr, lastStr, verStr, curStr *string
	if row.SystemVersion.Valid {
		s := row.SystemVersion.String
		verStr = &s
	}
	if row.CurrentYear.Valid {
		s := row.CurrentYear.String
		curStr = &s
	}
	if row.StartDate.Valid {
		startStr = formatPtrTime(&row.StartDate.Time)
	}
	if row.EndDate.Valid {
		endStr = formatPtrTime(&row.EndDate.Time)
	}
	if row.LastUpdated.Valid {
		lastStr = formatPtrTime(&row.LastUpdated.Time)
	}

	var updBy *int
	if row.UpdatedBy.Valid {
		v := int(row.UpdatedBy.Int64)
		updBy = &v
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"config_id":      row.ConfigID,
			"system_version": verStr,
			"current_year":   curStr,
			"start_date":     startStr,
			"end_date":       endStr,
			"last_updated":   lastStr,
			"updated_by":     updBy,
		},
	})
}

// UpdateSystemConfig: อัปเดตค่าล่าสุดของ system_config (current_year, start_date, end_date)
// PUT /api/v1/admin/system-config
func UpdateSystemConfig(c *gin.Context) {
	var p UpdateSystemConfigPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
		return
	}

	// ตรวจตรรกะเวลา (หากกำหนดทั้งคู่)
	if p.StartDate != nil && p.EndDate != nil && *p.StartDate != "" && *p.EndDate != "" {
		st, err1 := parseTimePtr(p.StartDate)
		en, err2 := parseTimePtr(p.EndDate)
		if err1 == nil && err2 == nil && st != nil && en != nil && st.After(*en) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "start_date must be before end_date"})
			return
		}
	}

	// หา config_id ล่าสุด
	var cfgID sql.NullInt64
	if err := config.DB.Raw(`
		SELECT config_id FROM system_config ORDER BY config_id DESC LIMIT 1
	`).Scan(&cfgID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to find latest config"})
		return
	}

	// แปลงวันเวลา
	stPtr, err1 := parseTimePtr(p.StartDate)
	enPtr, err2 := parseTimePtr(p.EndDate)
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format"})
		return
	}

	// ดึง user_id จาก context (หาก middleware auth ใส่ไว้)
	var updatedBy *int
	if v, ok := c.Get("user_id"); ok {
		if id, ok2 := v.(int); ok2 {
			updatedBy = &id
		}
	}

	// ถ้ายังไม่มีแถวในตารางนี้เลย => insert แถวแรก
	if !cfgID.Valid || cfgID.Int64 == 0 {
		ver := "1.0.0" // ตั้งค่าเริ่มต้น หากไม่มีค่าใน DB
		if err := config.DB.Exec(`
			INSERT INTO system_config (system_version, current_year, start_date, end_date, last_updated, updated_by)
			VALUES (?, ?, ?, ?, NOW(), ?)
		`, ver, p.CurrentYear, stPtr, enPtr, updatedBy).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to insert system_config"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	// มีแถวแล้ว => อัปเดตแถวล่าสุด
	if err := config.DB.Exec(`
		UPDATE system_config
		SET current_year = ?, start_date = ?, end_date = ?, last_updated = NOW(), updated_by = ?
		WHERE config_id = ?
	`, p.CurrentYear, stPtr, enPtr, updatedBy, int(cfgID.Int64)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to update system_config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
