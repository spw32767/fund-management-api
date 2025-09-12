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
   Models (lightweight)
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
		// รูปแบบ: รองศาสตราจารย์สมชาย ใจดี  (ตำแหน่งชิดหน้าชื่อ, เว้นก่อนนามสกุล)
		return strings.TrimSpace(posName + f + " " + l)
	}
	// ไม่มีตำแหน่ง → สมชาย ใจดี
	return strings.TrimSpace(f + " " + l)
}

/* ==========================
   Requests
   ========================== */

type createNotifReq struct {
	UserID              uint   `json:"user_id"` // ถ้าไม่ส่งจะใช้ user ปัจจุบัน
	Title               string `json:"title" binding:"required"`
	Message             string `json:"message" binding:"required"`
	Type                string `json:"type" binding:"required"` // info|success|warning|error
	RelatedSubmissionID *uint  `json:"related_submission_id"`
}

/* ==========================
   Handlers
   ========================== */

// POST /api/v1/notifications
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

	// พยายามใช้ Stored Procedure ก่อน
	if err := db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		req.UserID, req.Title, req.Message, req.Type, req.RelatedSubmissionID,
	).Error; err != nil {
		// ถ้า SP ใช้ไม่ได้ → insert ตรง
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

// GET /api/v1/notifications?unreadOnly=1&limit=20&offset=0
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

// GET /api/v1/notifications/counter
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

// PATCH /api/v1/notifications/:id/read
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

// POST /api/v1/notifications/mark-all-read
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

// POST /api/v1/notifications/events/submissions/:submissionId/submitted
// - บันทึก notification ลง DB (ผู้ยื่น + แอดมิน)
// - ส่งอีเมล โดยใช้ "เลขคำร้อง = submission_number" และชื่อที่มี "ยศตำแหน่ง"
func NotifySubmissionSubmitted(c *gin.Context) {
	db := getDB()

	uid, ok := getCurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	roleID, _ := getCurrentRoleID(c) // 3 = admin

	idParam := c.Param("submissionId")
	sid, err := strconv.Atoi(idParam)
	if err != nil || sid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission id"})
		return
	}

	// โหลด submission: ต้องมีทั้ง submission_id, user_id, submission_number
	var sub submissionLite
	if err := db.Select("submission_id, user_id, submission_number").
		First(&sub, "submission_id = ?", sid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}

	// อนุญาตเฉพาะเจ้าของคำร้องหรือแอดมิน
	if uid != sub.UserID && roleID != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	// ดึงข้อมูลผู้ส่ง (owner) รวม position_id
	var owner userLite
	_ = db.Select("user_id, role_id, email, user_fname, user_lname, position_id").
		First(&owner, "user_id = ?", sub.UserID).Error

	// หา position_name (ถ้ามี)
	posName := ""
	if owner.PositionID != nil {
		var p positionLite
		if err := db.Select("position_id, position_name").
			First(&p, "position_id = ?", *owner.PositionID).Error; err == nil && p.PositionName != nil {
			posName = *p.PositionName
		}
	}
	// display name พร้อมยศ และ escape HTML
	displayName := template.HTMLEscapeString(buildThaiDisplayName(owner, posName))

	// ===== บันทึก Notification ลง DB =====
	// ผู้ยื่น
	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		sub.UserID,
		"ส่งคำร้องสำเร็จ",
		fmt.Sprintf("ระบบได้รับคำร้อง %s ของคุณ %s แล้ว", sub.SubmissionNumber, displayName),
		"success",
		sid,
	).Error

	// แอดมินทั้งหมด (role_id = 3)
	var admins []userLite
	if err := db.Where("role_id = 3").Find(&admins).Error; err == nil {
		for _, ad := range admins {
			_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
				ad.UserID,
				"มีคำร้องใหม่",
				fmt.Sprintf("มีคำร้องใหม่ %s จากอาจารย์ %s แล้ว", sub.SubmissionNumber, displayName),
				"info",
				sid,
			).Error
		}
	}

	// ===== ส่งอีเมล =====
	base := os.Getenv("APP_BASE_URL")
	if base == "" {
		base = "http://localhost:3000"
	}

	// รวมอีเมลแอดมิน
	var adminEmails []string
	for _, ad := range admins {
		if ad.Email != nil && *ad.Email != "" {
			adminEmails = append(adminEmails, *ad.Email)
		}
	}

	// log config คร่าว ๆ
	log.Printf("[MAIL] host=%s port=%s from=%s toOwner=%t adminCount=%d",
		os.Getenv("SMTP_HOST"), os.Getenv("SMTP_PORT"), os.Getenv("SMTP_FROM"),
		owner.Email != nil && *owner.Email != "", len(adminEmails),
	)

	// ส่งเมลแบบ async (log error เสมอ)
	go func() {
		// ผู้ยื่น
		if owner.Email != nil && *owner.Email != "" {
			subj := "ส่งคำร้องสำเร็จ จากระบบบริหารจัดการทุนวิจัย"
			body := fmt.Sprintf(
				`<p>ระบบได้รับคำร้อง <strong>%s</strong> ของ <strong>%s</strong> แล้ว สามารถตรวจสอบคำร้องได้ที่ <a href="%[3]s">%[3]s</a></p>`,
				template.HTMLEscapeString(sub.SubmissionNumber), displayName, base,
			)
			if err := config.SendMail([]string{*owner.Email}, subj, body); err != nil {
				log.Printf("[MAIL][owner=%s] send failed: %v", *owner.Email, err)
			} else {
				log.Printf("[MAIL][owner=%s] sent", *owner.Email)
			}
		} else {
			log.Printf("[MAIL] owner email empty (user_id=%d) -> skip", sub.UserID)
		}

		// แอดมิน
		if len(adminEmails) > 0 {
			subj := "มีคำร้องใหม่ จากระบบบริหารจัดการทุนวิจัย"
			body := fmt.Sprintf(
				`<p>มีคำร้องใหม่ <strong>%s</strong> จาก <strong>%s</strong> แล้ว สามารถตรวจสอบคำร้องได้ที่ <a href="%[3]s">%[3]s</a></p>`,
				template.HTMLEscapeString(sub.SubmissionNumber), displayName, base,
			)
			if err := config.SendMail(adminEmails, subj, body); err != nil {
				log.Printf("[MAIL][admin %d recipients] send failed: %v", len(adminEmails), err)
			} else {
				log.Printf("[MAIL][admin %d recipients] sent", len(adminEmails))
			}
		} else {
			log.Printf("[MAIL] no admin emails -> skip")
		}
	}()

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
