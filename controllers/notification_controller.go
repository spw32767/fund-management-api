package controllers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"fund-management-api/config"
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
	SubmissionID     uint   `gorm:"column:submission_id"`
	SubmissionType   string `gorm:"column:submission_type"`
	UserID           uint   `gorm:"column:user_id"`
	SubmissionNumber string `gorm:"column:submission_number"`
}

func (submissionLite) TableName() string { return "submissions" }

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

func appBaseURL() string {
	base := os.Getenv("APP_BASE_URL")
	if base == "" {
		base = "http://localhost:3000"
	}
	return base
}

type emailMetaItem struct {
	Label string
	Value string
}

func buildEmailTemplate(subject string, paragraphs []string, meta []emailMetaItem, buttonText, buttonURL, footerHTML string) string {
	var contentBuilder strings.Builder
	for _, paragraph := range paragraphs {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed == "" {
			continue
		}
		contentBuilder.WriteString("<p style=\"margin:0 0 16px 0;\">")
		contentBuilder.WriteString(trimmed)
		contentBuilder.WriteString("</p>")
	}

	metaSection := ""
	if len(meta) > 0 {
		var rows []emailMetaItem
		rows = make([]emailMetaItem, 0, len(meta))
		for _, item := range meta {
			label := strings.TrimSpace(item.Label)
			value := strings.TrimSpace(item.Value)
			if label == "" || value == "" {
				continue
			}
			rows = append(rows, emailMetaItem{Label: label, Value: value})
		}
		if len(rows) > 0 {
			var metaBuilder strings.Builder
			metaBuilder.WriteString(`<tr><td style="padding:0 32px 24px 32px;">
<table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="border:1px solid #e5e7eb;border-radius:12px;background-color:#f9fafb;">
`)
			for i, row := range rows {
				border := "border-bottom:1px solid #e5e7eb;"
				if i == len(rows)-1 {
					border = ""
				}
				metaBuilder.WriteString(fmt.Sprintf(`<tr>
<td style="padding:12px 16px;font-size:13px;color:#6b7280;width:42%%;%s">%s</td>
<td style="padding:12px 16px;font-size:15px;color:#111827;font-weight:600;%s">%s</td>
</tr>
`, border, template.HTMLEscapeString(row.Label), border, template.HTMLEscapeString(row.Value)))
			}
			metaBuilder.WriteString(`</table>
</td></tr>
`)
			metaSection = metaBuilder.String()
		}
	}

	buttonSection := ""
	if strings.TrimSpace(buttonText) != "" && strings.TrimSpace(buttonURL) != "" {
		buttonSection = fmt.Sprintf(`<tr>
<td align="center" style="padding: 6px 32px 36px 32px;">
<a href="%s" style="display:inline-block;padding:12px 28px;background-color:#1d4ed8;color:#ffffff;text-decoration:none;border-radius:999px;font-weight:600;">%s</a>
</td>
</tr>`, template.HTMLEscapeString(buttonURL), template.HTMLEscapeString(buttonText))
	}

	footerSection := ""
	if strings.TrimSpace(footerHTML) != "" {
		footerSection = fmt.Sprintf(`<tr>
<td style="padding: 0 32px 32px 32px; color:#6b7280; font-size:13px; line-height:1.6;">%s</td>
</tr>`, footerHTML)
	}

	logoHTML := getEmailLogoHTML()
	if strings.TrimSpace(logoHTML) == "" {
		logoHTML = `<div style="width:64px;height:64px;border-radius:18px;background:linear-gradient(135deg,#0f172a,#1d4ed8);margin:0 auto 18px auto;display:flex;align-items:center;justify-content:center;">
<span style="font-size:20px;font-weight:700;color:#ffffff;">CP</span>
</div>`
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
</head>
<body style="margin:0;padding:0;background-color:#eef2f7;">
<table role="presentation" cellpadding="0" cellspacing="0" width="100%%" style="background-color:#eef2f7;">
<tr>
<td align="center" style="padding: 28px 16px;">
<table cellpadding="0" cellspacing="0" width="100%%" style="max-width:640px;background-color:#ffffff;border-radius:18px;overflow:hidden;box-shadow:0 18px 38px rgba(15,23,42,0.14);">
<tr>
<td style="padding:36px 32px 0 32px;text-align:center;background-color:#ffffff;">
%s
<h1 style="margin:0;font-size:24px;font-weight:700;color:#0f172a;">%s</h1>
<p style="margin:12px 0 0 0;color:#475569;font-size:14px;letter-spacing:0.03em;text-transform:uppercase;">Research Funding CP-KKU</p>
</td>
</tr>
<tr>
<td style="padding:24px 32px 8px 32px;color:#1f2937;font-size:16px;line-height:1.75;">
%s
</td>
</tr>
%s
%s
%s
</table>
</td>
</tr>
</table>
</body>
</html>`, template.HTMLEscapeString(subject), logoHTML, template.HTMLEscapeString(subject), contentBuilder.String(), metaSection, buttonSection, footerSection)
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
	if err := db.Select("submission_id, submission_type, user_id, submission_number").
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

	// ผู้ยื่น
	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		sub.UserID, "ส่งคำร้องสำเร็จ",
		fmt.Sprintf("ระบบได้รับคำร้อง %s ของคุณ %s แล้ว", sub.SubmissionNumber, submitterName),
		"success", sub.SubmissionID).Error

	// หัวหน้าสาขาปัจจุบัน
	headIDs := getCurrentDeptHeadIDs(db)
	var heads []userLite
	if len(headIDs) > 0 {
		_ = db.Where("user_id IN ?", headIDs).Find(&heads).Error
		for _, h := range heads {
			_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
				h.UserID, "คำร้องใหม่รอพิจารณา (หัวหน้าสาขา)",
				fmt.Sprintf("มีคำร้องใหม่ %s จากอาจารย์ %s รอพิจารณา", sub.SubmissionNumber, submitterName),
				"info", sub.SubmissionID).Error
		}
	}

	// email (best-effort)
	base := appBaseURL()
	go func() {
		if ownerEmail != "" {
			subj := "ส่งคำร้องสำเร็จ จากระบบบริหารจัดการทุนวิจัย"
			message := fmt.Sprintf("ระบบได้รับคำร้องหมายเลข <strong>%s</strong> ของ <strong>%s</strong> แล้ว",
				template.HTMLEscapeString(sub.SubmissionNumber), template.HTMLEscapeString(submitterName))
			meta := []emailMetaItem{
				{Label: "หมายเลขคำร้อง", Value: sub.SubmissionNumber},
			}
			if strings.TrimSpace(submitterName) != "" {
				meta = append(meta, emailMetaItem{Label: "ผู้ส่งคำร้อง", Value: submitterName})
			}
			body := buildEmailTemplate(subj, []string{message}, meta, "เปิดดู", base, "")
			sendMailSafe([]string{ownerEmail}, subj, body)
		}
		var emails []string
		for _, h := range heads {
			if h.Email != nil && *h.Email != "" {
				emails = append(emails, *h.Email)
			}
		}
		if len(emails) > 0 {
			subj := "มีคำร้องใหม่รอพิจารณา (หัวหน้าสาขา)"
			message := fmt.Sprintf("คำร้องหมายเลข <strong>%s</strong> จาก <strong>%s</strong> รอพิจารณา",
				template.HTMLEscapeString(sub.SubmissionNumber), template.HTMLEscapeString(submitterName))
			meta := []emailMetaItem{
				{Label: "หมายเลขคำร้อง", Value: sub.SubmissionNumber},
			}
			if strings.TrimSpace(submitterName) != "" {
				meta = append(meta, emailMetaItem{Label: "ผู้ส่งคำร้อง", Value: submitterName})
			}
			body := buildEmailTemplate(subj, []string{message}, meta, "ดูรายละเอียด", base, "")
			sendMailSafe(emails, subj, body)
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

	// ผู้ยื่น
	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		sub.UserID, "ผลพิจารณาจากหัวหน้าสาขา",
		fmt.Sprintf("คำร้องหมายเลข %s ของคุณได้รับการ \"เห็นควรพิจารณา\" จากหัวหน้าสาขาแล้ว", sub.SubmissionNumber),
		"success", sub.SubmissionID).Error

	// แอดมินทั้งหมด (role_id=3)
	var admins []userLite
	_ = db.Where("role_id = ?", 3).Find(&admins).Error
	for _, a := range admins {
		_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
			a.UserID, "คำร้องใหม่รอการตัดสินใจ (แอดมิน)",
			fmt.Sprintf("คำร้อง %s ผ่านการเห็นควรพิจารณาจากหัวหน้าสาขาแล้ว", sub.SubmissionNumber),
			"info", sub.SubmissionID).Error
	}

	// email
	base := appBaseURL()
	go func() {
		if ownerEmail != "" {
			subj := "ผลพิจารณาจากหัวหน้าสาขา: เห็นควรพิจารณา"
			message := fmt.Sprintf("คำร้องหมายเลข <strong>%s</strong> ของ <strong>%s</strong> ได้รับการ <strong>เห็นควรพิจารณา</strong>",
				template.HTMLEscapeString(sub.SubmissionNumber), template.HTMLEscapeString(submitterName))
			meta := []emailMetaItem{
				{Label: "หมายเลขคำร้อง", Value: sub.SubmissionNumber},
			}
			if strings.TrimSpace(submitterName) != "" {
				meta = append(meta, emailMetaItem{Label: "ผู้ส่งคำร้อง", Value: submitterName})
			}
			body := buildEmailTemplate(subj, []string{message}, meta, "เปิดดู", base, "")
			sendMailSafe([]string{ownerEmail}, subj, body)
		}
		var adminEmails []string
		for _, a := range admins {
			if a.Email != nil && *a.Email != "" {
				adminEmails = append(adminEmails, *a.Email)
			}
		}
		if len(adminEmails) > 0 {
			subj := "คำร้องใหม่รอการตัดสินใจ (ผ่านหัวหน้าสาขาแล้ว)"
			message := fmt.Sprintf("คำร้องหมายเลข <strong>%s</strong> ผ่านการเห็นควรพิจารณาจากหัวหน้าสาขาแล้ว",
				template.HTMLEscapeString(sub.SubmissionNumber))
			meta := []emailMetaItem{
				{Label: "หมายเลขคำร้อง", Value: sub.SubmissionNumber},
			}
			if strings.TrimSpace(submitterName) != "" {
				meta = append(meta, emailMetaItem{Label: "ผู้ส่งคำร้อง", Value: submitterName})
			}
			body := buildEmailTemplate(subj, []string{message}, meta, "เปิดดู", base, "")
			sendMailSafe(adminEmails, subj, body)
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

	reasonMessage := ""
	if strings.TrimSpace(req.Reason) != "" {
		reasonMessage = fmt.Sprintf(" เหตุผล: %s", template.HTMLEscapeString(req.Reason))
	}

	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		sub.UserID, "ผลพิจารณาจากหัวหน้าสาขา",
		fmt.Sprintf("คำร้องหมายเลข %s ของคุณได้รับการ \"ไม่เห็นควรพิจารณา\"%s", sub.SubmissionNumber, reasonMessage),
		"warning", sub.SubmissionID).Error

	// email
	base := appBaseURL()
	go func() {
		if ownerEmail != "" {
			subj := "ผลพิจารณาจากหัวหน้าสาขา: ไม่เห็นควรพิจารณา"
			message := fmt.Sprintf("คำร้องหมายเลข <strong>%s</strong> ของ <strong>%s</strong> ได้รับการ <strong>ไม่เห็นควรพิจารณา</strong>%s",
				template.HTMLEscapeString(sub.SubmissionNumber), template.HTMLEscapeString(submitterName), reasonMessage)
			meta := []emailMetaItem{
				{Label: "หมายเลขคำร้อง", Value: sub.SubmissionNumber},
			}
			if strings.TrimSpace(submitterName) != "" {
				meta = append(meta, emailMetaItem{Label: "ผู้ส่งคำร้อง", Value: submitterName})
			}
			body := buildEmailTemplate(subj, []string{message}, meta, "เปิดดู", base, "")
			sendMailSafe([]string{ownerEmail}, subj, body)
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

	msg := fmt.Sprintf("คำร้องหมายเลข %s ของคุณได้รับการอนุมัติ เป็นจำนวน %s บาท", sub.SubmissionNumber, amount)
	if announce != "" {
		msg += fmt.Sprintf(" (เลขอ้างอิงประกาศ: %s)", template.HTMLEscapeString(announce))
	}

	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		sub.UserID, "คำร้องได้รับการอนุมัติ", msg, "success", sub.SubmissionID).Error

	// email
	base := appBaseURL()
	go func() {
		if ownerEmail != "" {
			subj := "ผลการตัดสินใจ: อนุมัติ"
			announceNote := ""
			if announce != "" {
				announceNote = fmt.Sprintf(" (เลขอ้างอิงประกาศ: <strong>%s</strong>)", template.HTMLEscapeString(announce))
			}
			message := fmt.Sprintf("คำร้องหมายเลข <strong>%s</strong> ของ <strong>%s</strong> ได้รับการอนุมัติเป็นจำนวน <strong>%s บาท</strong>%s",
				template.HTMLEscapeString(sub.SubmissionNumber), template.HTMLEscapeString(submitterName), template.HTMLEscapeString(amount), announceNote)
			meta := []emailMetaItem{
				{Label: "หมายเลขคำร้อง", Value: sub.SubmissionNumber},
				{Label: "ผู้ส่งคำร้อง", Value: submitterName},
				{Label: "จำนวนเงินที่อนุมัติ", Value: fmt.Sprintf("%s บาท", amount)},
			}
			if announce != "" {
				meta = append(meta, emailMetaItem{Label: "เลขอ้างอิงประกาศ", Value: announce})
			}
			body := buildEmailTemplate(subj, []string{message}, meta, "ดูรายละเอียด", base, "")
			sendMailSafe([]string{ownerEmail}, subj, body)
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

	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		var rr struct{ Reason *string }
		_ = db.Raw(`SELECT admin_rejection_reason AS reason FROM submissions WHERE submission_id = ?`, sub.SubmissionID).Scan(&rr).Error
		if rr.Reason != nil {
			reason = *rr.Reason
		}
	}
	msg := fmt.Sprintf("คำร้องหมายเลข %s ของคุณไม่ได้รับการอนุมัติ", sub.SubmissionNumber)
	if reason != "" {
		msg += fmt.Sprintf(" เหตุผล: %s", template.HTMLEscapeString(reason))
	}

	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		sub.UserID, "ผลการตัดสินใจ: ไม่อนุมัติ", msg, "error", sub.SubmissionID).Error

	// email
	base := appBaseURL()
	go func() {
		if ownerEmail != "" {
			subj := "ผลการตัดสินใจ: ไม่อนุมัติ"
			intro := fmt.Sprintf("คำร้องหมายเลข <strong>%s</strong> ของ <strong>%s</strong> ไม่ได้รับการอนุมัติ",
				template.HTMLEscapeString(sub.SubmissionNumber), template.HTMLEscapeString(submitterName))
			paragraphs := []string{intro}
			if reason != "" {
				paragraphs = append(paragraphs, fmt.Sprintf("<strong>เหตุผล:</strong> %s", template.HTMLEscapeString(reason)))
			}
			meta := []emailMetaItem{
				{Label: "หมายเลขคำร้อง", Value: sub.SubmissionNumber},
			}
			if strings.TrimSpace(submitterName) != "" {
				meta = append(meta, emailMetaItem{Label: "ผู้ส่งคำร้อง", Value: submitterName})
			}
			body := buildEmailTemplate(subj, paragraphs, meta, "ดูรายละเอียด", base, "")
			sendMailSafe([]string{ownerEmail}, subj, body)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
