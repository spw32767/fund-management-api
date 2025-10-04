package controllers

import (
	"database/sql"
	"net/http"
	"time"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

// ===== Helpers =====

// parseTimePtr takes a *string (possibly nil/empty) and parses it into *time.Time (or nil)
func parseTimePtr(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	layouts := []string{
		"2006-01-02 15:04:05", // common MySQL DATETIME
		time.RFC3339,          // ISO8601
		"2006-01-02T15:04:05", // datetime-local without TZ
		"2006-01-02",          // date only
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

// formatPtrTime converts *time.Time to *string with a standard layout ("YYYY-MM-DD HH:mm:ss")
// returns nil if t is nil
func formatPtrTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02 15:04:05")
	return &s
}

// computeOpen takes start/end pointers and now, and returns (is_open_raw, is_open_effective)
// is_open_raw: according to window only
// is_open_effective: same as raw for now (reserved in case of future overrides)
func computeOpen(start, end *time.Time, now time.Time) (bool, bool) {
	isOpen := true
	if start != nil && end != nil {
		isOpen = (now.Equal(*start) || now.After(*start)) && (now.Equal(*end) || now.Before(*end))
	} else if start != nil && end == nil {
		isOpen = now.Equal(*start) || now.After(*start)
	} else if start == nil && end != nil {
		isOpen = now.Equal(*end) || now.Before(*end)
	} else {
		// both nil => treat as open (as used in your prior logs)
		isOpen = true
	}
	return isOpen, isOpen
}

// ===== Handlers =====

// GetSystemConfigCurrentYear returns only the latest current_year (as string or null)
func GetSystemConfigCurrentYear(c *gin.Context) {
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
		cur = row.CurrentYear.String
	} else {
		cur = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"current_year": cur,
	})
}

// GetSystemConfigWindow returns the window + all new columns (flat JSON)
func GetSystemConfigWindow(c *gin.Context) {
	now := time.Now().UTC()

	var row struct {
		ConfigID      int
		SystemVersion sql.NullString
		CurrentYear   sql.NullString
		StartDate     sql.NullTime
		EndDate       sql.NullTime
		LastUpdated   sql.NullTime
		UpdatedBy     sql.NullInt64

		MainAnnoucement             sql.NullInt64
		RewardAnnouncement          sql.NullInt64
		ActivitySupportAnnouncement sql.NullInt64
		ConferenceAnnouncement      sql.NullInt64
		ServiceAnnouncement         sql.NullInt64
		KkuReportYear               sql.NullString
		Installment                 sql.NullInt64
	}

	if err := config.DB.Raw(`
                SELECT
                  config_id, system_version, current_year, start_date, end_date, last_updated, updated_by,
                  main_annoucement, reward_announcement, activity_support_announcement, conference_announcement, service_announcement,
                  kku_report_year, installment
                FROM system_config
                ORDER BY config_id DESC
                LIMIT 1
        `).Scan(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch system_config window"})
		return
	}

	// format pointers
	var startPtr, endPtr, lastPtr *time.Time
	if row.StartDate.Valid {
		start := row.StartDate.Time.UTC()
		startPtr = &start
	}
	if row.EndDate.Valid {
		end := row.EndDate.Time.UTC()
		endPtr = &end
	}
	if row.LastUpdated.Valid {
		lu := row.LastUpdated.Time.UTC()
		lastPtr = &lu
	}

	isOpenRaw, isOpenEff := computeOpen(startPtr, endPtr, time.Now().UTC())

	toIntPtr := func(n sql.NullInt64) *int {
		if n.Valid {
			v := int(n.Int64)
			return &v
		}
		return nil
	}
	var updBy *int
	if row.UpdatedBy.Valid {
		v := int(row.UpdatedBy.Int64)
		updBy = &v
	}

	c.JSON(http.StatusOK, gin.H{
		"config_id": row.ConfigID,
		"system_version": func() *string {
			if row.SystemVersion.Valid {
				s := row.SystemVersion.String
				return &s
			}
			return nil
		}(),
		"current_year": func() interface{} {
			if row.CurrentYear.Valid {
				return row.CurrentYear.String
			}
			return nil
		}(),
		"start_date":   formatPtrTime(startPtr),
		"end_date":     formatPtrTime(endPtr),
		"last_updated": formatPtrTime(lastPtr),
		"updated_by":   updBy,

		// announcements (IDs to system_config.config_id per your schema)
		"main_annoucement":              toIntPtr(row.MainAnnoucement),
		"reward_announcement":           toIntPtr(row.RewardAnnouncement),
		"activity_support_announcement": toIntPtr(row.ActivitySupportAnnouncement),
		"conference_announcement":       toIntPtr(row.ConferenceAnnouncement),
		"service_announcement":          toIntPtr(row.ServiceAnnouncement),

		"kku_report_year": func() interface{} {
			if row.KkuReportYear.Valid {
				return row.KkuReportYear.String
			}
			return nil
		}(),
		"installment": func() interface{} {
			if row.Installment.Valid {
				return int(row.Installment.Int64)
			}
			return nil
		}(),

		"is_open_raw":       isOpenRaw,
		"is_open_effective": isOpenEff,
		"now":               now.Format(time.RFC3339Nano),
	})
}

// GetSystemConfigAdmin returns all columns under {success:true, data:{...}}
func GetSystemConfigAdmin(c *gin.Context) {
	now := time.Now().UTC()

	var row struct {
		ConfigID      int            `json:"config_id"`
		SystemVersion sql.NullString `json:"system_version"`
		CurrentYear   sql.NullString `json:"current_year"`
		StartDate     sql.NullTime   `json:"start_date"`
		EndDate       sql.NullTime   `json:"end_date"`
		LastUpdated   sql.NullTime   `json:"last_updated"`
		UpdatedBy     sql.NullInt64  `json:"updated_by"`

		MainAnnoucement             sql.NullInt64  `json:"main_annoucement"`
		RewardAnnouncement          sql.NullInt64  `json:"reward_announcement"`
		ActivitySupportAnnouncement sql.NullInt64  `json:"activity_support_announcement"`
		ConferenceAnnouncement      sql.NullInt64  `json:"conference_announcement"`
		ServiceAnnouncement         sql.NullInt64  `json:"service_announcement"`
		KkuReportYear               sql.NullString `json:"kku_report_year"`
		Installment                 sql.NullInt64  `json:"installment"`
	}

	if err := config.DB.Raw(`
                SELECT
                  config_id, system_version, current_year, start_date, end_date, last_updated, updated_by,
                  main_annoucement, reward_announcement, activity_support_announcement, conference_announcement, service_announcement,
                  kku_report_year, installment
                FROM system_config
                ORDER BY config_id DESC
                LIMIT 1
        `).Scan(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to fetch system_config"})
		return
	}

	// format pointers
	var startPtr, endPtr, lastPtr *time.Time
	if row.StartDate.Valid {
		start := row.StartDate.Time.UTC()
		startPtr = &start
	}
	if row.EndDate.Valid {
		end := row.EndDate.Time.UTC()
		endPtr = &end
	}
	if row.LastUpdated.Valid {
		lu := row.LastUpdated.Time.UTC()
		lastPtr = &lu
	}
	isOpenRaw, isOpenEff := computeOpen(startPtr, endPtr, now)

	toIntPtr := func(n sql.NullInt64) *int {
		if n.Valid {
			v := int(n.Int64)
			return &v
		}
		return nil
	}
	var updBy *int
	if row.UpdatedBy.Valid {
		v := int(row.UpdatedBy.Int64)
		updBy = &v
	}

	data := gin.H{
		"config_id": row.ConfigID,
		"system_version": func() *string {
			if row.SystemVersion.Valid {
				s := row.SystemVersion.String
				return &s
			}
			return nil
		}(),
		"current_year": func() interface{} {
			if row.CurrentYear.Valid {
				return row.CurrentYear.String
			}
			return nil
		}(),
		"start_date":   formatPtrTime(startPtr),
		"end_date":     formatPtrTime(endPtr),
		"last_updated": formatPtrTime(lastPtr),
		"updated_by":   updBy,

		"main_annoucement":              toIntPtr(row.MainAnnoucement),
		"reward_announcement":           toIntPtr(row.RewardAnnouncement),
		"activity_support_announcement": toIntPtr(row.ActivitySupportAnnouncement),
		"conference_announcement":       toIntPtr(row.ConferenceAnnouncement),
		"service_announcement":          toIntPtr(row.ServiceAnnouncement),

		"kku_report_year": func() interface{} {
			if row.KkuReportYear.Valid {
				return row.KkuReportYear.String
			}
			return nil
		}(),
		"installment": func() interface{} {
			if row.Installment.Valid {
				return int(row.Installment.Int64)
			}
			return nil
		}(),

		"is_open_raw":       isOpenRaw,
		"is_open_effective": isOpenEff,
		"now":               now.Format(time.RFC3339Nano),
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// UpdateSystemConfigWindow upserts current_year / start_date / end_date (admin use)
type updateWindowPayload struct {
	CurrentYear *string `json:"current_year"`
	StartDate   *string `json:"start_date"` // string; we will parse to *time.Time
	EndDate     *string `json:"end_date"`
}

// UpdateSystemConfigWindow updates the latest row or inserts a new one
func UpdateSystemConfigWindow(c *gin.Context) {
	var p updateWindowPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
		return
	}

	stPtr, err1 := parseTimePtr(p.StartDate)
	enPtr, err2 := parseTimePtr(p.EndDate)
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format"})
		return
	}

	// Get user_id from context (if auth middleware sets it)
	var updatedBy *int
	if v, ok := c.Get("user_id"); ok {
		if id, ok2 := v.(int); ok2 {
			updatedBy = &id
		}
	}

	// Check if a row exists
	var cfgID sql.NullInt64
	if err := config.DB.Raw(`
		SELECT config_id
		FROM system_config
		ORDER BY config_id DESC
		LIMIT 1
	`).Row().Scan(&cfgID); err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to query system_config"})
		return
	}

	if !cfgID.Valid {
		// Insert new row
		if err := config.DB.Exec(`
			INSERT INTO system_config (current_year, start_date, end_date, last_updated, updated_by)
			VALUES (?, ?, ?, NOW(), ?)
		`, p.CurrentYear, stPtr, enPtr, updatedBy).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to insert system_config"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	// Update latest row
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

// UpdateSystemConfigAnnouncement อัปเดตประกาศทีละช่องด้วย slot param
type setAnnouncementPayload struct {
	AnnouncementID *int `json:"announcement_id"` // null = เคลียร์ค่า
}

func UpdateSystemConfigAnnouncement(c *gin.Context) {
	// slot: main | reward | activity_support | conference | service
	slot := c.Param("slot")

	// map slot -> column ในตาราง system_config
	colMap := map[string]string{
		"main":             "main_annoucement", // สะกดตาม schema เดิม
		"reward":           "reward_announcement",
		"activity_support": "activity_support_announcement",
		"conference":       "conference_announcement",
		"service":          "service_announcement",
	}
	col, ok := colMap[slot]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid slot"})
		return
	}

	var p setAnnouncementPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
		return
	}

	// user_id (ถ้ามี auth middleware ใส่ไว้แล้ว)
	var updatedBy *int
	if v, ok := c.Get("user_id"); ok {
		if id, ok2 := v.(int); ok2 {
			updatedBy = &id
		}
	}

	// หาแถวล่าสุด + ตรวจว่ามี window ครบแล้วหรือยัง
	var row struct {
		ConfigID  sql.NullInt64
		StartDate sql.NullTime
		EndDate   sql.NullTime
	}
	if err := config.DB.Raw(`
		SELECT config_id, start_date, end_date
		FROM system_config
		ORDER BY config_id DESC
		LIMIT 1
	`).Scan(&row).Error; err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to query system_config"})
		return
	}

	// บังคับต้องมี window (manual start/end) ก่อน
	if !row.ConfigID.Valid || !row.StartDate.Valid || !row.EndDate.Valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "กรุณาตั้งค่า start_date และ end_date ก่อนบันทึกประกาศ",
		})
		return
	}

	// อัปเดตคอลัมน์ที่เลือกในแถวล่าสุด
	q := `
		UPDATE system_config
		SET ` + col + ` = ?, last_updated = NOW(), updated_by = ?
		WHERE config_id = ?
	`
	if err := config.DB.Exec(q, p.AnnouncementID, updatedBy, int(row.ConfigID.Int64)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to update system_config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
