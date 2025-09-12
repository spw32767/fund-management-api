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
	UserID uint    `gorm:"column:user_id"`
	RoleID uint    `gorm:"column:role_id"`
	Email  *string `gorm:"column:email"`
	FName  *string `gorm:"column:user_fname"`
	LName  *string `gorm:"column:user_lname"`
}

func (userLite) TableName() string { return "users" }

type submissionLite struct {
	SubmissionID uint `gorm:"column:submission_id"`
	UserID       uint `gorm:"column:user_id"`
}

func (submissionLite) TableName() string { return "submissions" }

/* ==========================
   Helpers
   ========================== */

func getDB() *gorm.DB { return config.DB }

func getCurrentUserID(c *gin.Context) (uint, bool) {
	// middleware ตั้งเป็น "userID" ไม่ใช่ "user_id"
	// ดูได้จาก auth middleware ที่ c.Set("userID", claims.UserID) :contentReference[oaicite:4]{index=4}
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
	// ตั้งเป็น "roleID" ใน middleware :contentReference[oaicite:5]{index=5}
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

	// ลองใช้ Stored Procedure ก่อน (คุณสร้างไว้แล้วใน DB)
	if err := db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		req.UserID, req.Title, req.Message, req.Type, req.RelatedSubmissionID,
	).Error; err != nil {
		// ถ้า CALL ไม่ได้ ให้ insert ตรง
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

	// ตรวจสอบ submission
	var sub submissionLite
	if err := db.First(&sub, "submission_id = ?", sid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}

	// อนุญาตเฉพาะเจ้าของคำร้องหรือแอดมิน
	if uid != sub.UserID && roleID != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	// แจ้งผู้ยื่น
	_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
		sub.UserID,
		"ส่งคำร้องสำเร็จ",
		"ระบบได้รับคำร้องของคุณแล้ว",
		"success",
		sid,
	).Error

	// แจ้งแอดมินทั้งหมด (role_id = 3)
	var admins []userLite
	if err := db.Where("role_id = 3").Find(&admins).Error; err == nil {
		for _, ad := range admins {
			_ = db.Exec(`CALL CreateNotification(?,?,?,?,?)`,
				ad.UserID,
				"มีคำร้องใหม่",
				"มีคำร้องใหม่เข้าระบบ กรุณาตรวจสอบ",
				"info",
				sid,
			).Error
		}
	}

	// base URL ของเว็บ (ไม่ต้องลิงก์เข้าหน้าเจาะจง)
	base := os.Getenv("APP_BASE_URL")
	if base == "" {
		base = "http://localhost:3000"
	}

	// ดึงข้อมูลเจ้าของคำร้อง (มีอยู่แล้วด้านบน)
	var owner userLite
	_ = db.Select("user_id, email, user_fname, user_lname").
		First(&owner, "user_id = ?", sub.UserID).Error

	// เตรียมชื่อผู้ส่งแสดงผล + escape HTML
	fullName := strings.TrimSpace(fmt.Sprintf("%s %s",
		func() string {
			if owner.FName != nil {
				return *owner.FName
			}
			return ""
		}(),
		func() string {
			if owner.LName != nil {
				return *owner.LName
			}
			return ""
		}(),
	))
	safeName := template.HTMLEscapeString(fullName)

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
		// ---- ผู้ยื่น (อาจารย์ผู้ส่ง) ----
		if owner.Email != nil && *owner.Email != "" {
			subj := "ส่งคำร้องสำเร็จ (ระบบทุนตีพิมพ์)"
			body := fmt.Sprintf(
				`<p>ระบบได้รับคำร้อง <strong>%d</strong> ของคุณ <strong>%s</strong> แล้ว สามารถตรวจสอบคำร้องได้ที่ <a href="%[3]s">%[3]s</a></p>`,
				sid, safeName, base,
			)
			if err := config.SendMail([]string{*owner.Email}, subj, body); err != nil {
				log.Printf("[MAIL][owner=%s] send failed: %v", *owner.Email, err)
			} else {
				log.Printf("[MAIL][owner=%s] sent", *owner.Email)
			}
		} else {
			log.Printf("[MAIL] owner email empty (user_id=%d) -> skip", sub.UserID)
		}

		// ---- แอดมิน ----
		if len(adminEmails) > 0 {
			subj := "มีคำร้องใหม่เข้าระบบ (ทุนตีพิมพ์)"
			body := fmt.Sprintf(
				`<p>มีคำร้องใหม่ <strong>%d</strong> จากอาจารย์ <strong>%s</strong> แล้ว สามารถตรวจสอบคำร้องได้ที่ <a href="%[3]s">%[3]s</a></p>`,
				sid, safeName, base,
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
}
