package controllers

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"fund-management-api/config"
	"fund-management-api/models"
)

/* ==========================
   Lightweight models (query-only)
   ========================== */

type Notification struct {
	NotificationID      uint       `gorm:"primaryKey;column:notification_id" json:"notification_id"`
	UserID              uint       `gorm:"column:user_id" json:"user_id"`
	Title               string     `gorm:"column:title" json:"title"`
	Message             string     `gorm:"column:message" json:"message"`
	Type                string     `gorm:"column:type" json:"type"` // info|success|warning|error
	RelatedSubmissionID *uint      `gorm:"column:related_submission_id" json:"related_submission_id,omitempty"`
	IsRead              bool       `gorm:"column:is_read" json:"is_read"`
	CreateAt            time.Time  `gorm:"column:create_at" json:"created_at"`
	UpdateAt            *time.Time `gorm:"column:update_at" json:"-"`
}

func (Notification) TableName() string { return "notifications" }

type userLite struct {
	UserID     uint    `gorm:"column:user_id"`
	RoleID     uint    `gorm:"column:role_id"`
	Email      *string `gorm:"column:email"`
	Prefix     *string `gorm:"column:prefix"`
	FName      *string `gorm:"column:user_fname"`
	LName      *string `gorm:"column:user_lname"`
	PositionID *uint   `gorm:"column:position_id"`
}

func (userLite) TableName() string { return "users" }

type positionLite struct {
	PositionID   uint    `gorm:"column:position_id"`
	PositionName *string `gorm:"column:position_name"`
}

func (positionLite) TableName() string { return "positions" }

type submissionLite struct {
	SubmissionID     uint       `gorm:"column:submission_id"`
	SubmissionType   string     `gorm:"column:submission_type"`
	UserID           uint       `gorm:"column:user_id"`
	SubmissionNumber string     `gorm:"column:submission_number"`
	SubmittedAt      *time.Time `gorm:"column:submitted_at"`
}

func (submissionLite) TableName() string { return "submissions" }

type templatedMessage struct {
	Title string
	Body  string
}

/* ==========================
   Helpers
   ========================== */

func getDB() *gorm.DB { return config.DB }

func getCurrentUserID(c *gin.Context) (uint, bool) {
	if v, ok := c.Get("userID"); ok {
		switch t := v.(type) {
		case int:
			return uint(t), true
		case int64:
			return uint(t), true
		case float64:
			return uint(t), true
		case uint:
			return t, true
		}
	}
	return 0, false
}

func getCurrentRoleID(c *gin.Context) (uint, bool) {
	if v, ok := c.Get("roleID"); ok {
		switch t := v.(type) {
		case int:
			return uint(t), true
		case int64:
			return uint(t), true
		case float64:
			return uint(t), true
		case uint:
			return t, true
		}
	}
	return 0, false
}

func fetchNotificationTemplate(db *gorm.DB, eventKey, sendTo string) (*models.NotificationMessage, error) {
	var tmpl models.NotificationMessage
	if err := db.Where("event_key = ? AND send_to = ? AND is_active = 1", eventKey, sendTo).
		First(&tmpl).Error; err != nil {
		return nil, err
	}
	return &tmpl, nil
}

func applyTemplatePlaceholders(text string, data map[string]string) string {
	result := text
	for key, value := range data {
		placeholder := "{{" + key + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

func buildTemplatedMessage(db *gorm.DB, eventKey, sendTo string, data map[string]string) (templatedMessage, error) {
	tmpl, err := fetchNotificationTemplate(db, eventKey, sendTo)
	if err != nil {
		return templatedMessage{}, fmt.Errorf("notification template missing for event %s -> %s", eventKey, sendTo)
	}

	msg := templatedMessage{
		Title: applyTemplatePlaceholders(tmpl.TitleTemplate, data),
		Body:  applyTemplatePlaceholders(tmpl.BodyTemplate, data),
	}
	return msg, nil
}

func buildThaiDisplayName(owner userLite, posName string) string {
	prefix := strings.TrimSpace(func() string {
		if owner.Prefix != nil {
			return *owner.Prefix
		}
		return ""
	}())
	f := strings.TrimSpace(func() string {
		if owner.FName != nil {
			return *owner.FName
		}
		return ""
	}())
	l := strings.TrimSpace(func() string {
		if owner.LName != nil {
			return *owner.LName
		}
		return ""
	}())

	parts := make([]string, 0, 4)
	if posName != "" {
		parts = append(parts, posName)
	}
	if prefix != "" {
		parts = append(parts, prefix)
	}
	if f != "" {
		parts = append(parts, f)
	}
	if l != "" {
		parts = append(parts, l)
	}

	return strings.TrimSpace(strings.Join(parts, " "))
}

// หา "หัวหน้าสาขาปัจจุบัน": system_config → dept_head_history → fallback role_id=4
func getCurrentDeptHeadIDs(db *gorm.DB) []uint {
	ids := make([]uint, 0, 2)

	// A) system_config.current_dept_head_user_id
	var one struct{ UserID uint }
	if err := db.Raw(`
		SELECT current_dept_head_user_id AS user_id
		FROM system_config
		WHERE current_dept_head_user_id IS NOT NULL
		ORDER BY updated_at DESC LIMIT 1
	`).Scan(&one).Error; err == nil && one.UserID != 0 {
		ids = append(ids, one.UserID)
	}

	// B) ตารางประวัติ (ช่วงเวลาปัจจุบัน)
	var rows []struct{ UserID uint }
	if err := db.Raw(`
		SELECT user_id
		FROM dept_head_history
		WHERE start_at <= NOW() AND (end_at IS NULL OR end_at > NOW())
		ORDER BY start_at DESC
	`).Scan(&rows).Error; err == nil {
		for _, r := range rows {
			if r.UserID != 0 {
				ids = append(ids, r.UserID)
			}
		}
	}

	// C) Fallback: ผู้ใช้ role_id = 4
	if len(ids) == 0 {
		var heads []userLite
		if err := db.Where("role_id = ?", 4).Find(&heads).Error; err == nil {
			for _, h := range heads {
				if h.UserID != 0 {
					ids = append(ids, h.UserID)
				}
			}
		}
	}

	// unique
	seen := map[uint]bool{}
	out := make([]uint, 0, len(ids))
	for _, v := range ids {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func loadOwnerDisplay(db *gorm.DB, userID uint) (displayName string, email string) {
	var owner userLite
	_ = db.Select("user_id, role_id, email, prefix, user_fname, user_lname, position_id").
		First(&owner, "user_id = ?", userID).Error

	posName := ""
	if owner.PositionID != nil {
		var p positionLite
		if err := db.Select("position_id, position_name").
			First(&p, "position_id = ?", *owner.PositionID).Error; err == nil && p.PositionName != nil {
			posName = *p.PositionName
		}
	}
	displayName = strings.TrimSpace(buildThaiDisplayName(owner, posName))
	if owner.Email != nil {
		email = *owner.Email
	}
	return
}

func getApprovedAmountDisplay(db *gorm.DB, sub submissionLite) (string, bool) {
	switch sub.SubmissionType {
	case "fund_application":
		var d struct{ ApprovedAmount *string }
		if err := db.Raw(`SELECT approved_amount FROM fund_application_details WHERE submission_id = ?`, sub.SubmissionID).
			Scan(&d).Error; err == nil && d.ApprovedAmount != nil {
			return *d.ApprovedAmount, true
		}
	case "publication_reward":
		var d struct{ TotalApproveAmount *string }
		if err := db.Raw(`SELECT total_approve_amount FROM publication_reward_details WHERE submission_id = ?`, sub.SubmissionID).
			Scan(&d).Error; err == nil && d.TotalApproveAmount != nil {
			return *d.TotalApproveAmount, true
		}
	}
	return "", false
}

func getSubmissionTitle(db *gorm.DB, sub submissionLite) string {
	var title string

	switch sub.SubmissionType {
	case "fund_application":
		var d struct{ ProjectTitle *string }
		if err := db.Raw(`SELECT project_title FROM fund_application_details WHERE submission_id = ? LIMIT 1`, sub.SubmissionID).
			Scan(&d).Error; err == nil && d.ProjectTitle != nil {
			title = *d.ProjectTitle
		}
	case "publication_reward":
		var d struct{ PaperTitle *string }
		if err := db.Raw(`SELECT paper_title FROM publication_reward_details WHERE submission_id = ? LIMIT 1`, sub.SubmissionID).
			Scan(&d).Error; err == nil && d.PaperTitle != nil {
			title = *d.PaperTitle
		}
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return "-"
	}
	return title
}

func formatSubmittedAt(sub submissionLite) string {
	if sub.SubmittedAt == nil {
		return "-"
	}

	loc, _ := time.LoadLocation("Asia/Bangkok")
	t := sub.SubmittedAt.In(loc)
	return t.Format("02/01/2006 15:04")
}

func appBaseURL() string {
	return chooseBaseURL(os.Getenv("APP_BASE_URL"), true)
}

func appContactInfo() string {
	raw := strings.TrimSpace(os.Getenv("APP_CONTACT_INFO"))
	if raw == "" {
		return "-"
	}
	return raw
}

func normalizeBaseURL(candidate string) string {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return ""
	}
	if !strings.HasSuffix(trimmed, "/") {
		trimmed += "/"
	}
	return trimmed
}

func chooseBaseURL(raw string, preferPublic bool) string {
	raw = strings.TrimSpace(raw)
	candidates := parseLogoList(raw)
	if len(candidates) == 0 {
		if raw != "" {
			candidates = append(candidates, raw)
		}
	}

	var fallback string
	for _, candidate := range candidates {
		normalized := normalizeBaseURL(candidate)
		if normalized == "" {
			continue
		}
		if fallback == "" {
			fallback = normalized
		}
		if !preferPublic || isPublicBaseURL(normalized) {
			return normalized
		}
	}

	return fallback
}

func isPublicBaseURL(candidate string) bool {
	u, err := url.Parse(candidate)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback()
	}
	return true
}

func buildFormalEmailHTML(subject, recipientName, message string) string {
	name := strings.TrimSpace(recipientName)
	if name == "" {
		name = "ผู้รับ"
	}

	escapedSubject := template.HTMLEscapeString(subject)
	escapedGreeting := template.HTMLEscapeString(fmt.Sprintf("เรียน %s", name))
	escapedMessage := template.HTMLEscapeString(strings.TrimSpace(message))
	escapedMessage = strings.ReplaceAll(strings.ReplaceAll(escapedMessage, "\r\n", "\n"), "\r", "\n")
	escapedMessage = strings.ReplaceAll(escapedMessage, "\n", "<br />")

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
</head>
<body style="margin:0;padding:0;background-color:#f9fafb;font-family:'Segoe UI',Tahoma,Arial,sans-serif;">
<div style="max-width:640px;margin:0 auto;padding:24px 20px;">
  <div style="background-color:#ffffff;border:1px solid #e5e7eb;border-radius:12px;padding:24px 24px 28px 24px;">
    <p style="margin:0 0 16px 0;font-size:16px;line-height:1.7;color:#111827;">%s</p>
    <p style="margin:0 0 0 0;font-size:16px;line-height:1.7;color:#111827;word-break:break-word;">%s</p>
  </div>
</div>
</body>
</html>`, escapedSubject, escapedGreeting, escapedMessage)
}

func sendMailSafe(to []string, subject, html string) {
	if err := config.SendMail(to, subject, html); err != nil {
		log.Printf("notification email send failed (subject=%q to=%v): %v", subject, to, err)
	}
}

/* ==========================
   Request payloads
   ========================== */

type createNotifReq struct {
	UserID              uint   `json:"user_id"` // ถ้าไม่ส่งจะใช้ user ปัจจุบัน
	Title               string `json:"title" binding:"required"`
	Message             string `json:"message" binding:"required"`
	Type                string `json:"type" binding:"required"` // info|success|warning|error
	RelatedSubmissionID *uint  `json:"related_submission_id"`
}

type notifyApprovedReq struct {
	AnnounceRef string `json:"announce_reference_number"`
}
type notifyRejectedReq struct {
	Reason string `json:"reason"`
}
type notifyDeptHeadReq struct {
	Comment string `json:"comment"`
	Reason  string `json:"reason"`
}

/* ==========================
   CRUD
   ========================== */

func CreateNotification(c *gin.Context) {
	db := getDB()

	var req createNotifReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.UserID == 0 {
		if uid, ok := getCurrentUserID(c); ok {
			req.UserID = uid
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}

	if err := db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		req.UserID, req.Title, req.Message, req.Type, req.RelatedSubmissionID,
	).Error; err != nil {
		// fallback insert ตรง
		n := Notification{
			UserID:              req.UserID,
			Title:               req.Title,
			Message:             req.Message,
			Type:                req.Type,
			RelatedSubmissionID: req.RelatedSubmissionID,
			IsRead:              false,
			CreateAt:            time.Now(),
		}
		if e2 := db.Create(&n).Error; e2 != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": e2.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "notification_id": n.NotificationID})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func GetNotifications(c *gin.Context) {
	db := getDB()

	uid, ok := getCurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	unreadOnly := strings.TrimSpace(c.Query("unreadOnly"))
	limitStr := strings.TrimSpace(c.Query("limit"))
	offsetStr := strings.TrimSpace(c.Query("offset"))

	limit := 20
	offset := 0
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 && v <= 100 {
		limit = v
	}
	if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
		offset = v
	}

	q := db.Model(&Notification{}).Where("user_id = ?", uid)
	if unreadOnly == "1" || strings.EqualFold(unreadOnly, "true") {
		q = q.Where("is_read = 0")
	}

	var items []Notification
	if err := q.Order("create_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func GetNotificationCounter(c *gin.Context) {
	db := getDB()

	uid, ok := getCurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var n int64
	if err := db.Model(&Notification{}).
		Where("user_id = ? AND is_read = 0", uid).
		Count(&n).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"unread": n})
}

func MarkNotificationRead(c *gin.Context) {
	db := getDB()

	uid, ok := getCurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := db.Model(&Notification{}).
		Where("notification_id = ? AND user_id = ?", id, uid).
		Update("is_read", true).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func MarkAllNotificationsRead(c *gin.Context) {
	db := getDB()

	uid, ok := getCurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if err := db.Model(&Notification{}).
		Where("user_id = ? AND is_read = 0", uid).
		Update("is_read", true).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

/* ==========================
   Event endpoints (NEW flow)
   ========================== */

// POST /api/v1/notifications/events/submissions/:submissionId/submitted
// -> แจ้งผู้ยื่น + หัวหน้าสาขาปัจจุบัน (ไม่แจ้งแอดมิน)
func NotifySubmissionSubmitted(c *gin.Context) {
	db := getDB()

	uid, ok := getCurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	roleID, _ := getCurrentRoleID(c) // อนุญาต owner หรือ admin

	sid, err := strconv.Atoi(c.Param("submissionId"))
	if err != nil || sid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission id"})
		return
	}

	var payload struct {
		SubmitterName string `json:"submitter_name"`
	}
	_ = c.ShouldBindJSON(&payload)

	var sub submissionLite
	if err := db.Select("submission_id, submission_type, user_id, submission_number, submitted_at").
		First(&sub, "submission_id = ?", sid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}
	if uid != sub.UserID && roleID != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	ownerName, ownerEmail := loadOwnerDisplay(db, sub.UserID)
	ownerName = strings.TrimSpace(ownerName)

	submitterName := strings.TrimSpace(payload.SubmitterName)
	if submitterName == "" {
		submitterName = ownerName
	}

	submissionTitle := getSubmissionTitle(db, sub)
	submittedAt := formatSubmittedAt(sub)
	webURL := strings.TrimSpace(appBaseURL())
	if webURL == "" {
		webURL = "-"
	}

	data := map[string]string{
		"submission_number": sub.SubmissionNumber,
		"submitter_name":    submitterName,
		"submission_title":  submissionTitle,
		"submitted_at":      submittedAt,
		"web_url":           webURL,
	}

	userMsg, err := buildTemplatedMessage(db, "submission_submitted", "user", data)
	if err != nil {
		log.Printf("notify submission submitted: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification template missing"})
		return
	}

	headTemplate, err := fetchNotificationTemplate(db, "submission_submitted", "dept_head")
	if err != nil {
		log.Printf("notify submission submitted: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification template missing"})
		return
	}

	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`, sub.UserID, userMsg.Title, userMsg.Body, "success", sub.SubmissionID).Error

	headIDs := getCurrentDeptHeadIDs(db)
	var heads []userLite
	headMessages := make([]struct {
		User userLite
		Msg  templatedMessage
	}, 0, len(headIDs))
	if len(headIDs) > 0 {
		_ = db.Where("user_id IN ?", headIDs).Find(&heads).Error
		for _, h := range heads {
			headData := map[string]string{}
			for k, v := range data {
				headData[k] = v
			}
			headData["depthead_name"] = buildThaiDisplayName(h, "")

			msg := templatedMessage{
				Title: applyTemplatePlaceholders(headTemplate.TitleTemplate, headData),
				Body:  applyTemplatePlaceholders(headTemplate.BodyTemplate, headData),
			}

			headMessages = append(headMessages, struct {
				User userLite
				Msg  templatedMessage
			}{User: h, Msg: msg})

			_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`, h.UserID, msg.Title, msg.Body, "info", sub.SubmissionID).Error
		}
	}

	go func() {
		if ownerEmail != "" {
			subj := userMsg.Title
			emailBody := buildFormalEmailHTML(subj, submitterName, userMsg.Body)
			sendMailSafe([]string{ownerEmail}, subj, emailBody)
		}

		for _, hm := range headMessages {
			if hm.User.Email == nil || *hm.User.Email == "" {
				continue
			}
			subj := hm.Msg.Title
			name := buildThaiDisplayName(hm.User, "")
			emailBody := buildFormalEmailHTML(subj, name, hm.Msg.Body)
			sendMailSafe([]string{*hm.User.Email}, subj, emailBody)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/v1/notifications/events/submissions/:submissionId/dept-head/recommended
// -> แจ้ง "ผู้ยื่น" (เห็นควรพิจารณา) แล้ว "แจ้งแอดมิน"
func NotifyDeptHeadRecommended(c *gin.Context) {
	db := getDB()

	sid, err := strconv.Atoi(c.Param("submissionId"))
	if err != nil || sid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission id"})
		return
	}

	var sub submissionLite
	if err := db.Select("submission_id, submission_type, user_id, submission_number").
		First(&sub, "submission_id = ?", sid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}
	ownerName, ownerEmail := loadOwnerDisplay(db, sub.UserID)
	ownerName = strings.TrimSpace(ownerName)

	submitterName := ownerName

	submissionTitle := getSubmissionTitle(db, sub)
	webURL := strings.TrimSpace(appBaseURL())
	if webURL == "" {
		webURL = "-"
	}

	data := map[string]string{
		"submission_number": sub.SubmissionNumber,
		"submitter_name":    submitterName,
		"submission_title":  submissionTitle,
		"web_url":           webURL,
	}

	userMsg, err := buildTemplatedMessage(db, "dept_head_recommended", "user", data)
	if err != nil {
		log.Printf("notify dept head recommended: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification template missing"})
		return
	}

	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`, sub.UserID, userMsg.Title, userMsg.Body, "success", sub.SubmissionID).Error

	adminTemplate, err := fetchNotificationTemplate(db, "dept_head_recommended", "admin")
	if err != nil {
		log.Printf("notify dept head recommended: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification template missing"})
		return
	}

	var admins []userLite
	_ = db.Where("role_id = ?", 3).Find(&admins).Error
	for _, a := range admins {
		adminData := map[string]string{}
		for k, v := range data {
			adminData[k] = v
		}
		adminData["admin_name"] = buildThaiDisplayName(a, "")

		adminMsg := templatedMessage{
			Title: applyTemplatePlaceholders(adminTemplate.TitleTemplate, adminData),
			Body:  applyTemplatePlaceholders(adminTemplate.BodyTemplate, adminData),
		}

		_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`, a.UserID, adminMsg.Title, adminMsg.Body, "info", sub.SubmissionID).Error
	}

	go func() {
		if ownerEmail != "" {
			subj := userMsg.Title
			emailBody := buildFormalEmailHTML(subj, submitterName, userMsg.Body)
			sendMailSafe([]string{ownerEmail}, subj, emailBody)
		}
		for _, a := range admins {
			if a.Email == nil || *a.Email == "" {
				continue
			}

			adminData := map[string]string{}
			for k, v := range data {
				adminData[k] = v
			}
			adminData["admin_name"] = buildThaiDisplayName(a, "")

			adminMsg := templatedMessage{
				Title: applyTemplatePlaceholders(adminTemplate.TitleTemplate, adminData),
				Body:  applyTemplatePlaceholders(adminTemplate.BodyTemplate, adminData),
			}

			subj := adminMsg.Title
			name := adminData["admin_name"]
			emailBody := buildFormalEmailHTML(subj, name, adminMsg.Body)
			sendMailSafe([]string{*a.Email}, subj, emailBody)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/v1/notifications/events/submissions/:submissionId/dept-head/not-recommended
// -> แจ้ง "ผู้ยื่น" (ไม่เห็นควรพิจารณา) — ไม่แจ้งแอดมิน
func NotifyDeptHeadNotRecommended(c *gin.Context) {
	db := getDB()

	sid, err := strconv.Atoi(c.Param("submissionId"))
	if err != nil || sid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission id"})
		return
	}

	var req notifyDeptHeadReq
	_ = c.ShouldBindJSON(&req)

	var sub submissionLite
	if err := db.Select("submission_id, submission_type, user_id, submission_number").
		First(&sub, "submission_id = ?", sid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}
	ownerName, ownerEmail := loadOwnerDisplay(db, sub.UserID)
	ownerName = strings.TrimSpace(ownerName)

	submitterName := ownerName

	reasonText := strings.TrimSpace(req.Reason)
	if reasonText == "" {
		reasonText = "ไม่ระบุ"
	}

	webURL := strings.TrimSpace(appBaseURL())
	if webURL == "" {
		webURL = "-"
	}

	data := map[string]string{
		"submission_number":     sub.SubmissionNumber,
		"submitter_name":        submitterName,
		"head_rejection_reason": reasonText,
		"web_url":               webURL,
	}

	msg, err := buildTemplatedMessage(db, "dept_head_not_recommended", "user", data)
	if err != nil {
		log.Printf("notify dept head not recommended: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification template missing"})
		return
	}

	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`, sub.UserID, msg.Title, msg.Body, "warning", sub.SubmissionID).Error

	go func() {
		if ownerEmail != "" {
			subj := msg.Title
			emailBody := buildFormalEmailHTML(subj, submitterName, msg.Body)
			sendMailSafe([]string{ownerEmail}, subj, emailBody)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/v1/notifications/events/submissions/:submissionId/approved
// -> (แอดมิน) อนุมัติ: แจ้งผู้ยื่น + แสดง "จำนวนเงินอนุมัติ" (อ่านจากตาราง detail)
// หมายเหตุ: สถานะ/เหตุผล ให้ยึดจากตาราง submissions เท่านั้น (controller นี้ไม่ไปแก้/อ่าน detail ยกเว้นดึงยอดเงินเพื่อแจ้ง)
func NotifyAdminApproved(c *gin.Context) {
	db := getDB()

	// เฉพาะแอดมิน
	_, ok := getCurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	roleID, _ := getCurrentRoleID(c)
	if roleID != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	sid, err := strconv.Atoi(c.Param("submissionId"))
	if err != nil || sid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission id"})
		return
	}

	var body notifyApprovedReq
	_ = c.ShouldBindJSON(&body)
	announce := strings.TrimSpace(body.AnnounceRef)

	var sub submissionLite
	if err := db.Select("submission_id, submission_type, user_id, submission_number").
		First(&sub, "submission_id = ?", sid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}

	ownerName, ownerEmail := loadOwnerDisplay(db, sub.UserID)
	ownerName = strings.TrimSpace(ownerName)

	submitterName := ownerName
	amount, okAmt := getApprovedAmountDisplay(db, sub)
	if !okAmt || strings.TrimSpace(amount) == "" {
		amount = "0.00"
	}

	announceRef := announce
	if announceRef == "" {
		announceRef = "-"
	}

	submissionTitle := getSubmissionTitle(db, sub)
	webURL := strings.TrimSpace(appBaseURL())
	if webURL == "" {
		webURL = "-"
	}

	contactInfo := appContactInfo()

	data := map[string]string{
		"submission_number": sub.SubmissionNumber,
		"submitter_name":    submitterName,
		"submission_title":  submissionTitle,
		"amount":            amount,
		"announce_ref":      announceRef,
		"contact_info":      contactInfo,
		"web_url":           webURL,
	}

	msg, err := buildTemplatedMessage(db, "admin_approved", "user", data)
	if err != nil {
		log.Printf("notify admin approved: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification template missing"})
		return
	}

	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`, sub.UserID, msg.Title, msg.Body, "success", sub.SubmissionID).Error

	go func() {
		if ownerEmail != "" {
			subj := msg.Title
			emailBody := buildFormalEmailHTML(subj, submitterName, msg.Body)
			sendMailSafe([]string{ownerEmail}, subj, emailBody)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/v1/notifications/events/submissions/:submissionId/rejected
// -> (แอดมิน) ไม่อนุมัติ: แจ้งผู้ยื่น + เหตุผล (อ่านจาก submissions ถ้า payload ไม่ส่งมา)
func NotifyAdminRejected(c *gin.Context) {
	db := getDB()

	// เฉพาะแอดมิน
	_, ok := getCurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	roleID, _ := getCurrentRoleID(c)
	if roleID != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	sid, err := strconv.Atoi(c.Param("submissionId"))
	if err != nil || sid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission id"})
		return
	}

	var body notifyRejectedReq
	_ = c.ShouldBindJSON(&body)

	var sub submissionLite
	if err := db.Select("submission_id, submission_type, user_id, submission_number").
		First(&sub, "submission_id = ?", sid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}

	ownerName, ownerEmail := loadOwnerDisplay(db, sub.UserID)
	ownerName = strings.TrimSpace(ownerName)

	submitterName := ownerName
	submissionTitle := getSubmissionTitle(db, sub)
	webURL := strings.TrimSpace(appBaseURL())
	if webURL == "" {
		webURL = "-"
	}

	contactInfo := appContactInfo()

	adminRejectionReason := strings.TrimSpace(body.Reason)
	if adminRejectionReason == "" {
		var rr struct{ Reason *string }
		_ = db.Raw(`SELECT admin_rejection_reason AS reason FROM submissions WHERE submission_id = ?`, sub.SubmissionID).Scan(&rr).Error
		if rr.Reason != nil {
			adminRejectionReason = strings.TrimSpace(*rr.Reason)
		}
	}
	if adminRejectionReason == "" {
		adminRejectionReason = "ไม่ระบุ"
	}

	data := map[string]string{
		"submission_number":      sub.SubmissionNumber,
		"submitter_name":         submitterName,
		"submission_title":       submissionTitle,
		"admin_rejection_reason": adminRejectionReason,
		"web_url":                webURL,
		"contact_info":           contactInfo,
	}

	msg, err := buildTemplatedMessage(db, "admin_rejected", "user", data)
	if err != nil {
		log.Printf("notify admin rejected: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification template missing"})
		return
	}

	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		sub.UserID, msg.Title, msg.Body, "error", sub.SubmissionID).Error

	go func() {
		if ownerEmail != "" {
			subj := msg.Title
			emailBody := buildFormalEmailHTML(subj, submitterName, msg.Body)
			sendMailSafe([]string{ownerEmail}, subj, emailBody)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
