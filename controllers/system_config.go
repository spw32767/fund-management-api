package controllers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

// ====== slot -> column map (ใช้ซ้ำหลาย handler) ======
var announcementSlotColumns = map[string]string{
	"main":             "main_annoucement", // สะกดตาม schema เดิม
	"reward":           "reward_announcement",
	"activity_support": "activity_support_announcement",
	"conference":       "conference_announcement",
	"service":          "service_announcement",
}

func isValidSlot(slot string) bool {
	_, ok := announcementSlotColumns[slot]
	return ok
}

// ===== Helpers =====

// parseTimePtr ใช้แปลง string -> *time.Time (หรือ nil)
func parseTimePtr(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	layouts := []string{
		"2006-01-02 15:04:05", // MySQL DATETIME
		time.RFC3339,          // ISO8601
		"2006-01-02T15:04:05", // datetime-local (no TZ)
		"2006-01-02",          // date only
	}
	var lastErr error
	for _, layout := range layouts {
		if tt, err := time.Parse(layout, *s); err == nil {
			tu := tt.UTC()
			return &tu, nil
		} else {
			lastErr = err
		}
	}
	return nil, lastErr
}

// formatPtrTime แปลง *time.Time -> *string ("YYYY-MM-DD HH:mm:ss") หรือ nil
func formatPtrTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02 15:04:05")
	return &s
}

// computeOpen: is_open_raw / is_open_effective จาก window
func computeOpen(start, end *time.Time, now time.Time) (bool, bool) {
	isOpen := true
	if start != nil && end != nil {
		isOpen = (now.Equal(*start) || now.After(*start)) && (now.Equal(*end) || now.Before(*end))
	} else if start != nil && end == nil {
		isOpen = now.Equal(*start) || now.After(*start)
	} else if start == nil && end != nil {
		isOpen = now.Equal(*end) || now.Before(*end)
	} else {
		// ทั้งคู่ว่าง = เปิดตลอด (ตามพฤติกรรมเดิม)
		isOpen = true
	}
	return isOpen, isOpen
}

// getUserIDAny: ดึง user id จาก context/header ให้ครอบคลุมหลายชนิดข้อมูล (ตั้งชื่อไม่ชนไฟล์อื่น)
func getUserIDAny(c *gin.Context) *int {
	keys := []string{"user_id", "admin_id", "uid", "id"}
	for _, k := range keys {
		if v, ok := c.Get(k); ok {
			switch t := v.(type) {
			case int:
				id := t
				return &id
			case int64:
				id := int(t)
				return &id
			case float64:
				id := int(t)
				return &id
			case string:
				if id64, err := strconv.ParseInt(t, 10, 64); err == nil {
					id := int(id64)
					return &id
				}
			}
		}
	}
	// สำรองอ่านจาก Header
	if hv := c.GetHeader("X-User-Id"); hv != "" {
		if id64, err := strconv.ParseInt(hv, 10, 64); err == nil {
			id := int(id64)
			return &id
		}
	}
	return nil
}

// fetchCurrentAnnAssignment: ดึง assignment ของ slot ที่ "กำลังมีผล" หรือ "กำลังจะมาถึง" (แก้ไขเพิ่มเติม)
func fetchCurrentAnnAssignment(slot string) (annID *int, start *time.Time, end *time.Time, err error) {
	var row struct {
		AnnouncementID sql.NullInt64
		StartDate      sql.NullTime
		EndDate        sql.NullTime
	}

	// 1. Try to fetch the currently active assignment (start <= NOW() and (end IS NULL or end >= NOW()))
	qActive := `
		SELECT announcement_id, start_date, end_date
		FROM announcement_assignments
		WHERE slot_code = ?
		  AND start_date <= NOW()
		  AND (end_date IS NULL OR end_date >= NOW())
		ORDER BY changed_at DESC, start_date DESC
		LIMIT 1
	`

	err2 := config.DB.Raw(qActive, slot).Scan(&row).Error
	if err2 != nil && err2 != sql.ErrNoRows {
		return nil, nil, nil, err2
	}

	// 2. If no active assignment is found, try to fetch the NEXT UPCOMING assignment (start > NOW())
	if !row.AnnouncementID.Valid { // หากไม่พบรายการที่ Active
		qUpcoming := `
            SELECT announcement_id, start_date, end_date
            FROM announcement_assignments
            WHERE slot_code = ?
              AND start_date > NOW()
            ORDER BY start_date ASC, changed_at DESC
            LIMIT 1
        `
		// Clear row struct to ensure accurate check after scan
		row = struct {
			AnnouncementID sql.NullInt64
			StartDate      sql.NullTime
			EndDate        sql.NullTime
		}{}

		err3 := config.DB.Raw(qUpcoming, slot).Scan(&row).Error
		if err3 != nil && err3 != sql.ErrNoRows {
			return nil, nil, nil, err3
		}
	}

	// Process the found row (either active or upcoming)
	if row.AnnouncementID.Valid {
		x := int(row.AnnouncementID.Int64)
		annID = &x
	}

	if row.StartDate.Valid {
		t := row.StartDate.Time.UTC()
		start = &t
	}

	if row.EndDate.Valid {
		t := row.EndDate.Time.UTC()
		end = &t
	}

	// Ensure we return if no assignment was found in either query
	if annID == nil && start == nil && end == nil {
		return nil, nil, nil, nil
	}

	return annID, start, end, nil
}

// ===== Handlers =====

// GET /api/v1/system-config/current-year
func GetSystemConfigCurrentYear(c *gin.Context) {
	var row struct {
		CurrentYear sql.NullString
	}
	if err := config.DB.Raw(`
		SELECT current_year FROM system_config
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

	c.JSON(http.StatusOK, gin.H{"current_year": cur})
}

// GET /api/v1/system-config/window
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

	// window ของประกาศที่ "กำลังมีผล" ราย slot
	mainID, mainS, mainE, _ := fetchCurrentAnnAssignment("main")
	rewardID, rewardS, rewardE, _ := fetchCurrentAnnAssignment("reward")
	actID, actS, actE, _ := fetchCurrentAnnAssignment("activity_support")
	confID, confS, confE, _ := fetchCurrentAnnAssignment("conference")
	svcID, svcS, svcE, _ := fetchCurrentAnnAssignment("service")

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

		"main_annoucement":              toIntPtr(row.MainAnnoucement),
		"reward_announcement":           toIntPtr(row.RewardAnnouncement),
		"activity_support_announcement": toIntPtr(row.ActivitySupportAnnouncement),
		"conference_announcement":       toIntPtr(row.ConferenceAnnouncement),
		"service_announcement":          toIntPtr(row.ServiceAnnouncement),

		// window ปัจจุบันรายช่อง (จาก announcement_assignments)
		"main_start_date":                            formatPtrTime(mainS),
		"main_end_date":                              formatPtrTime(mainE),
		"reward_start_date":                          formatPtrTime(rewardS),
		"reward_end_date":                            formatPtrTime(rewardE),
		"activity_support_start_date":                formatPtrTime(actS),
		"activity_support_end_date":                  formatPtrTime(actE),
		"conference_start_date":                      formatPtrTime(confS),
		"conference_end_date":                        formatPtrTime(confE),
		"service_start_date":                         formatPtrTime(svcS),
		"service_end_date":                           formatPtrTime(svcE),
		"main_effective_announcement_id":             mainID,
		"reward_effective_announcement_id":           rewardID,
		"activity_support_effective_announcement_id": actID,
		"conference_effective_announcement_id":       confID,
		"service_effective_announcement_id":          svcID,

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

// GET /api/v1/admin/system-config
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

	// window ปัจจุบันรายช่อง (จาก announcement_assignments)
	mainID, mainS, mainE, _ := fetchCurrentAnnAssignment("main")
	rewardID, rewardS, rewardE, _ := fetchCurrentAnnAssignment("reward")
	actID, actS, actE, _ := fetchCurrentAnnAssignment("activity_support")
	confID, confS, confE, _ := fetchCurrentAnnAssignment("conference")
	svcID, svcS, svcE, _ := fetchCurrentAnnAssignment("service")

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

		"main_start_date":                            formatPtrTime(mainS),
		"main_end_date":                              formatPtrTime(mainE),
		"reward_start_date":                          formatPtrTime(rewardS),
		"reward_end_date":                            formatPtrTime(rewardE),
		"activity_support_start_date":                formatPtrTime(actS),
		"activity_support_end_date":                  formatPtrTime(actE),
		"conference_start_date":                      formatPtrTime(confS),
		"conference_end_date":                        formatPtrTime(confE),
		"service_start_date":                         formatPtrTime(svcS),
		"service_end_date":                           formatPtrTime(svcE),
		"main_effective_announcement_id":             mainID,
		"reward_effective_announcement_id":           rewardID,
		"activity_support_effective_announcement_id": actID,
		"conference_effective_announcement_id":       confID,
		"service_effective_announcement_id":          svcID,

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

// PUT /api/v1/admin/system-config
type updateWindowPayload struct {
	CurrentYear *string `json:"current_year"`
	StartDate   *string `json:"start_date"`
	EndDate     *string `json:"end_date"`
}

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

	updatedBy := getUserIDAny(c)

	// หาแถวล่าสุด
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
		// insert
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

	// update
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

// PATCH /api/v1/admin/system-config/announcements/:slot
type setAnnouncementPayload struct {
	AnnouncementID *int    `json:"announcement_id"` // null = เคลียร์
	StartDate      *string `json:"start_date"`      // จำเป็นเมื่อ AnnouncementID != nil
	EndDate        *string `json:"end_date"`        // จำเป็นเมื่อ AnnouncementID != nil
}

func UpdateSystemConfigAnnouncement(c *gin.Context) {
	slot := c.Param("slot")
	col, ok := announcementSlotColumns[slot]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid slot"})
		return
	}

	var p setAnnouncementPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
		return
	}

	updatedBy := getUserIDAny(c)

	// ดู window ปัจจุบัน (global) ต้องมี start/end ก่อน
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
	if !row.ConfigID.Valid || !row.StartDate.Valid || !row.EndDate.Valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "กรุณาตั้งค่า start_date และ end_date ก่อนบันทึกประกาศ",
		})
		return
	}

	// อัปเดตคอลัมน์ใน system_config (compat เดิม)
	q := `
		UPDATE system_config
		SET ` + col + ` = ?, last_updated = NOW(), updated_by = ?
		WHERE config_id = ?
	`
	if err := config.DB.Exec(q, p.AnnouncementID, updatedBy, int(row.ConfigID.Int64)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to update system_config"})
		return
	}

	// ถ้าเคลียร์ประกาศ => ไม่บันทึกประวัติช่วงว่าง
	if p.AnnouncementID == nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	// ต้องมีช่วงเวลา
	stPtr, err1 := parseTimePtr(p.StartDate)
	enPtr, err2 := parseTimePtr(p.EndDate)
	if err1 != nil || err2 != nil || stPtr == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid or missing start/end date"})
		return
	}
	if enPtr != nil && stPtr.After(*enPtr) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "start_date must be before or equal to end_date"})
		return
	}

	// บันทึกประวัติลง announcement_assignments (changed_by อิงจาก updatedBy)
	if err := config.DB.Exec(`
		INSERT INTO announcement_assignments
			(slot_code, announcement_id, start_date, end_date, changed_by, changed_at)
		VALUES
			(?, ?, ?, ?, ?, NOW())
	`, slot, p.AnnouncementID, stPtr, enPtr, updatedBy).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to insert announcement assignment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GET /api/v1/admin/system-config/announcements/:slot/history
func ListAnnouncementHistory(c *gin.Context) {
	slot := c.Param("slot")
	if !isValidSlot(slot) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slot"})
		return
	}

	limit := 50
	offset := 0
	if qs := c.Query("limit"); qs != "" {
		if v, err := strconv.Atoi(qs); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}
	if qs := c.Query("offset"); qs != "" {
		if v, err := strconv.Atoi(qs); err == nil && v >= 0 {
			offset = v
		}
	}

	type rowT struct {
		AssignmentID   int           `json:"assignment_id"`
		SlotCode       string        `json:"slot_code"`
		AnnouncementID sql.NullInt64 `json:"announcement_id"`
		StartDate      time.Time     `json:"start_date"`
		EndDate        sql.NullTime  `json:"end_date"`
		ChangedBy      sql.NullInt64 `json:"changed_by"`
		ChangedAt      time.Time     `json:"changed_at"`
	}
	var rows []rowT

	q := `
		SELECT assignment_id, slot_code, announcement_id, start_date, end_date, changed_by, changed_at
		FROM announcement_assignments
		WHERE slot_code = ?
		ORDER BY changed_at DESC, start_date DESC
		LIMIT ? OFFSET ?
	`
	if err := config.DB.Raw(q, slot, limit, offset).Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch announcement history"})
		return
	}

	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		var annID *int
		if r.AnnouncementID.Valid {
			v := int(r.AnnouncementID.Int64)
			annID = &v
		}
		var endStr *string
		if r.EndDate.Valid {
			s := r.EndDate.Time.UTC().Format("2006-01-02 15:04:05")
			endStr = &s
		}
		var chBy *int
		if r.ChangedBy.Valid {
			v := int(r.ChangedBy.Int64)
			chBy = &v
		}
		items = append(items, gin.H{
			"assignment_id":   r.AssignmentID,
			"slot_code":       r.SlotCode,
			"announcement_id": annID,
			"start_date":      r.StartDate.UTC().Format("2006-01-02 15:04:05"),
			"end_date":        endStr,
			"changed_by":      chBy,
			"changed_at":      r.ChangedAt.UTC().Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}
