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

// fetchCurrentAnnAssignment returns the current-effective assignment (id + window) for a given slot
func fetchCurrentAnnAssignment(slot string) (annID *int, start *time.Time, end *time.Time, err error) {
	var row struct {
		AnnouncementID sql.NullInt64
		StartDate      sql.NullTime
		EndDate        sql.NullTime
	}
	// เลือก assignment ที่กำลังมีผล ณ ตอนนี้ (start<=NOW() และ end IS NULL หรือ end>=NOW())
	// ใช้ลำดับ changed_at DESC, start_date DESC เพื่อเอาแถวล่าสุด (ในกรณีมีหลายแถวทับกัน)
	q := `
		SELECT announcement_id, start_date, end_date
		FROM announcement_assignments
		WHERE slot_code = ? 
		  AND start_date <= NOW() 
		  AND (end_date IS NULL OR end_date >= NOW())
		ORDER BY changed_at DESC, start_date DESC
		LIMIT 1
	`
	if err2 := config.DB.Raw(q, slot).Scan(&row).Error; err2 != nil && err2 != sql.ErrNoRows {
		return nil, nil, nil, err2
	}
	if row.AnnouncementID.Valid {
		id := int(row.AnnouncementID.Int64)
		annID = &id
	}
	if row.StartDate.Valid {
		t := row.StartDate.Time.UTC()
		start = &t
	}
	if row.EndDate.Valid {
		t := row.EndDate.Time.UTC()
		end = &t
	}
	return annID, start, end, nil
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

	// ดึง window ของประกาศที่ "กำลังมีผล" ราย slot
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

		// announcements (IDs in system_config)
		"main_annoucement":              toIntPtr(row.MainAnnoucement),
		"reward_announcement":           toIntPtr(row.RewardAnnouncement),
		"activity_support_announcement": toIntPtr(row.ActivitySupportAnnouncement),
		"conference_announcement":       toIntPtr(row.ConferenceAnnouncement),
		"service_announcement":          toIntPtr(row.ServiceAnnouncement),

		// + window ที่กำลังมีผลรายช่อง (ดึงจาก announcement_assignments)
		"main_start_date":             formatPtrTime(mainS),
		"main_end_date":               formatPtrTime(mainE),
		"reward_start_date":           formatPtrTime(rewardS),
		"reward_end_date":             formatPtrTime(rewardE),
		"activity_support_start_date": formatPtrTime(actS),
		"activity_support_end_date":   formatPtrTime(actE),
		"conference_start_date":       formatPtrTime(confS),
		"conference_end_date":         formatPtrTime(confE),
		"service_start_date":          formatPtrTime(svcS),
		"service_end_date":            formatPtrTime(svcE),

		// (ทางเลือก) ส่ง id ของ assignment ปัจจุบันกลับด้วย (ถ้ามี)
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

	// ดึง window ของประกาศที่ "กำลังมีผล" ราย slot
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

		// + window ที่กำลังมีผลรายช่อง (ดึงจาก announcement_assignments)
		"main_start_date":             formatPtrTime(mainS),
		"main_end_date":               formatPtrTime(mainE),
		"reward_start_date":           formatPtrTime(rewardS),
		"reward_end_date":             formatPtrTime(rewardE),
		"activity_support_start_date": formatPtrTime(actS),
		"activity_support_end_date":   formatPtrTime(actE),
		"conference_start_date":       formatPtrTime(confS),
		"conference_end_date":         formatPtrTime(confE),
		"service_start_date":          formatPtrTime(svcS),
		"service_end_date":            formatPtrTime(svcE),

		// (ทางเลือก) ส่ง id ของ assignment ปัจจุบันกลับด้วย (ถ้ามี)
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

// UpdateSystemConfigAnnouncement อัปเดตประกาศทีละช่องด้วย slot param + (บันทึกประวัติลง announcement_assignments)
type setAnnouncementPayload struct {
	AnnouncementID *int    `json:"announcement_id"` // null = เคลียร์ค่า
	StartDate      *string `json:"start_date"`      // optional (จำเป็นเมื่อ AnnouncementID != nil)
	EndDate        *string `json:"end_date"`        // optional (จำเป็นเมื่อ AnnouncementID != nil)
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

	// หาแถวล่าสุด + ตรวจว่ามี window ครบแล้วหรือยัง (global)
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

	// บังคับต้องมี window (manual start/end) ก่อน (ตาม requirement เดิมของคุณ)
	if !row.ConfigID.Valid || !row.StartDate.Valid || !row.EndDate.Valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "กรุณาตั้งค่า start_date และ end_date ก่อนบันทึกประกาศ",
		})
		return
	}

	// อัปเดตคอลัมน์ที่เลือกในแถวล่าสุด (ความเข้ากันได้ย้อนหลัง)
	q := `
		UPDATE system_config
		SET ` + col + ` = ?, last_updated = NOW(), updated_by = ?
		WHERE config_id = ?
	`
	if err := config.DB.Exec(q, p.AnnouncementID, updatedBy, int(row.ConfigID.Int64)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to update system_config"})
		return
	}

	// ถ้าไม่ได้ตั้งประกาศ (announcement_id == null) → เคลียร์ค่าอย่างเดียว ไม่บันทึกประวัติช่วงว่าง
	if p.AnnouncementID == nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	// มีประกาศ → ต้องมีช่วงเวลา
	stPtr, err1 := parseTimePtr(p.StartDate)
	enPtr, err2 := parseTimePtr(p.EndDate)
	if err1 != nil || err2 != nil || stPtr == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid or missing start/end date"})
		return
	}
	// ตรวจตรรกะ start<=end (ถ้า end ไม่ว่าง)
	if enPtr != nil && stPtr.After(*enPtr) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "start_date must be before or equal to end_date"})
		return
	}

	// บันทึกประวัติลง announcement_assignments
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
