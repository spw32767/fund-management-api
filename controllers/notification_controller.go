package controllers

import (
	"fmt"
	"html/template"
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
	if posName != "" {
		return strings.TrimSpace(posName + f + " " + l)
	}
	return strings.TrimSpace(f + " " + l)
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
	_ = db.Select("user_id, role_id, email, user_fname, user_lname, position_id").
		First(&owner, "user_id = ?", userID).Error

	posName := ""
	if owner.PositionID != nil {
		var p positionLite
		if err := db.Select("position_id, position_name").
			First(&p, "position_id = ?", *owner.PositionID).Error; err == nil && p.PositionName != nil {
			posName = *p.PositionName
		}
	}
	displayName = template.HTMLEscapeString(buildThaiDisplayName(owner, posName))
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

	// ผู้ยื่น
	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		sub.UserID, "ส่งคำร้องสำเร็จ",
		fmt.Sprintf("ระบบได้รับคำร้อง %s ของคุณ %s แล้ว", sub.SubmissionNumber, ownerName),
		"success", sub.SubmissionID).Error

	// หัวหน้าสาขาปัจจุบัน
	headIDs := getCurrentDeptHeadIDs(db)
	var heads []userLite
	if len(headIDs) > 0 {
		_ = db.Where("user_id IN ?", headIDs).Find(&heads).Error
		for _, h := range heads {
			_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
				h.UserID, "คำร้องใหม่รอพิจารณา (หัวหน้าสาขา)",
				fmt.Sprintf("มีคำร้องใหม่ %s จากอาจารย์ %s รอพิจารณา", sub.SubmissionNumber, ownerName),
				"info", sub.SubmissionID).Error
		}
	}

	// email (best-effort)
	base := appBaseURL()
	go func() {
		if ownerEmail != "" {
			subj := "ส่งคำร้องสำเร็จ จากระบบบริหารจัดการทุนวิจัย"
			body := fmt.Sprintf(`<p>ระบบได้รับคำร้องหมายเลข <strong>%s</strong> แล้ว</p><p><a href="%[2]s">เปิดดู</a></p>`,
				template.HTMLEscapeString(sub.SubmissionNumber), base)
			_ = config.SendMail([]string{ownerEmail}, subj, body)
		}
		var emails []string
		for _, h := range heads {
			if h.Email != nil && *h.Email != "" {
				emails = append(emails, *h.Email)
			}
		}
		if len(emails) > 0 {
			subj := "มีคำร้องใหม่รอพิจารณา (หัวหน้าสาขา)"
			body := fmt.Sprintf(`<p>คำร้องหมายเลข <strong>%s</strong> จาก <strong>%s</strong> รอพิจารณา</p><p><a href="%[2]s">ดูรายละเอียด</a></p>`,
				template.HTMLEscapeString(sub.SubmissionNumber), ownerName, base)
			_ = config.SendMail(emails, subj, body)
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
	_, ownerEmail := loadOwnerDisplay(db, sub.UserID)

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
			body := fmt.Sprintf(`<p>คำร้องหมายเลข <strong>%s</strong> ได้รับการ <strong>เห็นควรพิจารณา</strong></p><p><a href="%[2]s">เปิดดู</a></p>`,
				template.HTMLEscapeString(sub.SubmissionNumber), base)
			_ = config.SendMail([]string{ownerEmail}, subj, body)
		}
		var adminEmails []string
		for _, a := range admins {
			if a.Email != nil && *a.Email != "" {
				adminEmails = append(adminEmails, *a.Email)
			}
		}
		if len(adminEmails) > 0 {
			subj := "คำร้องใหม่รอการตัดสินใจ (ผ่านหัวหน้าสาขาแล้ว)"
			body := fmt.Sprintf(`<p>คำร้องหมายเลข <strong>%s</strong> ผ่านการเห็นควรพิจารณาจากหัวหน้าสาขาแล้ว</p><p><a href="%[2]s">เปิดดู</a></p>`,
				template.HTMLEscapeString(sub.SubmissionNumber), base)
			_ = config.SendMail(adminEmails, subj, body)
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
	_, ownerEmail := loadOwnerDisplay(db, sub.UserID)

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
			body := fmt.Sprintf(`<p>คำร้องหมายเลข <strong>%s</strong> ของคุณได้รับการ <strong>ไม่เห็นควรพิจารณา</strong>%s</p><p><a href="%[3]s">เปิดดู</a></p>`,
				template.HTMLEscapeString(sub.SubmissionNumber), template.HTMLEscapeString(reasonMessage), base)
			_ = config.SendMail([]string{ownerEmail}, subj, body)
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

	_, ownerEmail := loadOwnerDisplay(db, sub.UserID)
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
			body := fmt.Sprintf(`<p>%s</p><p><a href="%s">ดูรายละเอียด</a></p>`, template.HTMLEscapeString(msg), base)
			_ = config.SendMail([]string{ownerEmail}, subj, body)
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

	_, ownerEmail := loadOwnerDisplay(db, sub.UserID)

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
			body := fmt.Sprintf(`<p>%s</p><p><a href="%s">ดูรายละเอียด</a></p>`, template.HTMLEscapeString(msg), base)
			_ = config.SendMail([]string{ownerEmail}, subj, body)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
