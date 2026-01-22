package controllers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/utils"

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

func getCurrentBEYearStrFromConfig() string {
	type row struct {
		CurrentYear *string
	}
	var r row
	_ = config.DB.Raw(`
		SELECT current_year
		FROM system_config
		ORDER BY config_id DESC
		LIMIT 1
	`).Scan(&r).Error

	if r.CurrentYear != nil {
		only := onlyDigits(*r.CurrentYear)
		if len(only) >= 4 {
			return only[:4]
		}
	}

	return fmt.Sprintf("%04d", time.Now().Year()+543)
}

// getUserIDAny: ดึง user id จากหลายแหล่ง (context -> header -> JWT -> cookies)
// โดยไม่ต้องแก้โค้ดฝั่ง FE
func getUserIDAny(c *gin.Context) *int {
	// 1) ค่าใน Gin context (middleware อาจตั้งให้คีย์ต่างกัน)
	for _, k := range []string{"user_id", "admin_id", "uid", "id"} {
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
				if n, err := strconv.Atoi(t); err == nil {
					return &n
				}
			case json.Number:
				if n, err := t.Int64(); err == nil {
					id := int(n)
					return &id
				}
			case map[string]interface{}:
				if id := extractIDFromMap(t); id != nil {
					return id
				}
			}
		}
	}

	// 2) Header สำรองแบบเดิม
	if hv := c.GetHeader("X-User-Id"); hv != "" {
		if n, err := strconv.Atoi(hv); err == nil {
			return &n
		}
	}

	// 3) บางระบบอาจยัด claims ทั้งก้อนลง context
	if v, ok := c.Get("claims"); ok {
		if m, ok := v.(map[string]interface{}); ok {
			if id := extractIDFromMap(m); id != nil {
				return id
			}
		}
	}

	// 4) Authorization: Bearer <JWT>  -> ถอด payload (base64url) แล้วอ่าน claims
	if ah := c.GetHeader("Authorization"); strings.HasPrefix(strings.ToLower(ah), "bearer ") {
		if id := extractIDFromJWT(ah[7:]); id != nil {
			return id
		}
	}

	// 5) ลองดูใน cookies ทั่วไปที่มักเก็บ JWT
	for _, ck := range []string{"access_token", "jwt", "token"} {
		if token, err := c.Cookie(ck); err == nil && token != "" {
			if id := extractIDFromJWT(token); id != nil {
				return id
			}
		}
	}

	return nil
}

// ===== helpers สำหรับ getUserIDAny =====

func extractIDFromMap(m map[string]interface{}) *int {
	for _, k := range []string{"user_id", "id", "uid", "sub"} {
		if v, ok := m[k]; ok {
			if n, ok := toInt(v); ok {
				return &n
			}
		}
	}
	// บางระบบห่อไว้ใต้ "user"
	if u, ok := m["user"]; ok {
		if um, ok := u.(map[string]interface{}); ok {
			return extractIDFromMap(um)
		}
	}
	return nil
}

func toInt(v interface{}) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return int(n), true
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return n, true
		}
	}
	return 0, false
}

func extractIDFromJWT(token string) *int {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload := parts[1]
	// pad base64url
	if m := len(payload) % 4; m != 0 {
		payload += strings.Repeat("=", 4-m)
	}
	b, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return nil
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(b, &claims); err != nil {
		return nil
	}
	return extractIDFromMap(claims)
}

// fetchCurrentAnnAssignment: ดึง assignment ของ slot ที่ "กำลังมีผล" หรือ "กำลังจะมาถึง" (แก้ไขเพิ่มเติม)
func fetchCurrentAnnAssignment(slot string) (annID *int, start *time.Time, end *time.Time, err error) {
	var row struct {
		AnnouncementID sql.NullInt64
		StartDate      sql.NullTime
		EndDate        sql.NullTime
	}

	// 1) แถวที่ Active ตอนนี้
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

	// 2) ถ้าไม่เจอ ให้หาแถวที่ "กำลังจะเริ่ม" อันถัดไป
	if !row.AnnouncementID.Valid {
		qUpcoming := `
			SELECT announcement_id, start_date, end_date
			FROM announcement_assignments
			WHERE slot_code = ?
			  AND start_date > NOW()
			ORDER BY start_date ASC, changed_at DESC
			LIMIT 1
		`
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

	// Process
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

	// no assignment found
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

// GET /api/v1/system-config/submission-usage
func GetSubmissionUsageLimit(c *gin.Context) {
	userIDValue, _ := c.Get("userID")
	userID, ok := userIDValue.(int)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user context"})
		return
	}

	type row struct {
		MaxSubmissionsPerYear sql.NullInt64
		CurrentYear           sql.NullString
	}
	var cfg row
	if err := config.DB.Raw(`
		SELECT max_submissions_per_year, current_year
		FROM system_config
		ORDER BY config_id DESC
		LIMIT 1
	`).Scan(&cfg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch system_config"})
		return
	}

	maxAllowed := 5
	if cfg.MaxSubmissionsPerYear.Valid && cfg.MaxSubmissionsPerYear.Int64 > 0 {
		maxAllowed = int(cfg.MaxSubmissionsPerYear.Int64)
	}

	yearStr := getCurrentBEYearStrFromConfig()
	if cfg.CurrentYear.Valid {
		if only := onlyDigits(cfg.CurrentYear.String); len(only) >= 4 {
			yearStr = only[:4]
		}
	}

	var yearID sql.NullInt64
	if err := config.DB.Raw(`
		SELECT year_id
		FROM years
		WHERE year = ? AND delete_at IS NULL
		ORDER BY year_id DESC
		LIMIT 1
	`, yearStr).Scan(&yearID).Error; err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve year"})
		return
	}

	used := 0
	if yearID.Valid {
		var usedCount sql.NullInt64
		if err := config.DB.Raw(`
			SELECT COUNT(*)
			FROM submissions s
			JOIN application_status st ON st.application_status_id = s.status_id
			WHERE s.deleted_at IS NULL
			  AND s.user_id = ?
			  AND s.year_id = ?
			  AND s.submission_type IN ('fund_application', 'publication_reward')
			  AND st.status_code NOT IN (?, ?)
		`, userID, yearID.Int64, utils.StatusCodeRejected, utils.StatusCodeDraft).Scan(&usedCount).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count submissions"})
			return
		}
		if usedCount.Valid {
			used = int(usedCount.Int64)
		}
	}

	overLimit := used >= maxAllowed
	remaining := maxAllowed - used
	if remaining < 0 {
		remaining = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"year": yearStr,
		"year_id": func() *int {
			if yearID.Valid {
				v := int(yearID.Int64)
				return &v
			}
			return nil
		}(),
		"max_submissions_per_year": maxAllowed,
		"used_submissions":         used,
		"remaining_submissions":    remaining,
		"over_limit":               overLimit,
	})
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
		ContactInfo   sql.NullString

		MainAnnoucement             sql.NullInt64
		RewardAnnouncement          sql.NullInt64
		ActivitySupportAnnouncement sql.NullInt64
		ConferenceAnnouncement      sql.NullInt64
		ServiceAnnouncement         sql.NullInt64
		KkuReportYear               sql.NullString
		Installment                 sql.NullInt64
		MaxSubmissionsPerYear       sql.NullInt64
	}

	if err := config.DB.Raw(`
		SELECT
                  config_id, system_version, current_year, start_date, end_date, last_updated, updated_by, contact_info,
                  main_annoucement, reward_announcement, activity_support_announcement, conference_announcement, service_announcement,
                  kku_report_year, installment, max_submissions_per_year
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
		"contact_info": func() interface{} {
			if row.ContactInfo.Valid {
				return strings.TrimSpace(row.ContactInfo.String)
			}
			return nil
		}(),

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
		"max_submissions_per_year": func() interface{} {
			if row.MaxSubmissionsPerYear.Valid {
				return int(row.MaxSubmissionsPerYear.Int64)
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
		ContactInfo   sql.NullString `json:"contact_info"`

		MainAnnoucement             sql.NullInt64  `json:"main_annoucement"`
		RewardAnnouncement          sql.NullInt64  `json:"reward_announcement"`
		ActivitySupportAnnouncement sql.NullInt64  `json:"activity_support_announcement"`
		ConferenceAnnouncement      sql.NullInt64  `json:"conference_announcement"`
		ServiceAnnouncement         sql.NullInt64  `json:"service_announcement"`
		KkuReportYear               sql.NullString `json:"kku_report_year"`
		Installment                 sql.NullInt64  `json:"installment"`
		MaxSubmissionsPerYear       sql.NullInt64  `json:"max_submissions_per_year"`
	}

	if err := config.DB.Raw(`
	SELECT
                  config_id, system_version, current_year, start_date, end_date, last_updated, updated_by, contact_info,
                  main_annoucement, reward_announcement, activity_support_announcement, conference_announcement, service_announcement,
                  kku_report_year, installment, max_submissions_per_year
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
		"contact_info": func() interface{} {
			if row.ContactInfo.Valid {
				return strings.TrimSpace(row.ContactInfo.String)
			}
			return nil
		}(),

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
		"max_submissions_per_year": func() interface{} {
			if row.MaxSubmissionsPerYear.Valid {
				return int(row.MaxSubmissionsPerYear.Int64)
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
	CurrentYear           *string `json:"current_year"`
	StartDate             *string `json:"start_date"`
	EndDate               *string `json:"end_date"`
	ContactInfo           *string `json:"contact_info"`
	MaxSubmissionsPerYear *int    `json:"max_submissions_per_year"`
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
		maxSubmissions := p.MaxSubmissionsPerYear
		if maxSubmissions == nil {
			zero := 0
			maxSubmissions = &zero
		}
		// insert
		if err := config.DB.Exec(`
                        INSERT INTO system_config (current_year, start_date, end_date, contact_info, max_submissions_per_year, last_updated, updated_by)
                        VALUES (?, ?, ?, ?, ?, NOW(), ?)
                `, p.CurrentYear, stPtr, enPtr, p.ContactInfo, maxSubmissions, updatedBy).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to insert system_config"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	maxSubmissions := p.MaxSubmissionsPerYear
	if maxSubmissions == nil {
		var currentMax sql.NullInt64
		if err := config.DB.Raw(`
			SELECT max_submissions_per_year
			FROM system_config
			WHERE config_id = ?
		`, int(cfgID.Int64)).Row().Scan(&currentMax); err == nil && currentMax.Valid {
			val := int(currentMax.Int64)
			maxSubmissions = &val
		} else {
			zero := 0
			maxSubmissions = &zero
		}
	}

	// update
	if err := config.DB.Exec(`
                UPDATE system_config
                SET current_year = ?, start_date = ?, end_date = ?, contact_info = ?, max_submissions_per_year = ?, last_updated = NOW(), updated_by = ?
                WHERE config_id = ?
        `, p.CurrentYear, stPtr, enPtr, p.ContactInfo, maxSubmissions, updatedBy, int(cfgID.Int64)).Error; err != nil {
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

// ===== New: GET /api/v1/system-config/dept-head/eligible-roles =====

// คืนรายการ role key สำหรับการคัดกรองรายชื่อผู้ใช้ที่เลือกเป็น "หัวหน้าสาขา" ได้ (แบบไดนามิก)
// แหล่งข้อมูล:
//  1. ENV: DEPT_HEAD_ELIGIBLE_ROLES (เช่น "teacher,dept_head")
//  2. Fallback: ["teacher", "dept_head"]
func GetDeptHeadEligibleRoles(c *gin.Context) {
	// อ่านจาก ENV
	env := strings.TrimSpace(os.Getenv("DEPT_HEAD_ELIGIBLE_ROLES"))
	var roles []string
	if env != "" {
		// รองรับตัวคั่นได้หลายแบบ: , ; |
		parts := strings.FieldsFunc(env, func(r rune) bool {
			return r == ',' || r == ';' || r == '|'
		})
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				roles = append(roles, strings.ToLower(p))
			}
		}
	}

	// fallback
	if len(roles) == 0 {
		roles = []string{"teacher", "dept_head"}
	}

	// dedupe
	seen := map[string]struct{}{}
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		k := strings.ToLower(strings.TrimSpace(r))
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}

	c.JSON(http.StatusOK, gin.H{"data": out})
}
