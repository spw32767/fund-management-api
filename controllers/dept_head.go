package controllers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

// AssignDeptHeadPayload: รับค่าตามที่หน้า FE ส่งมา (start_date/end_date)
type AssignDeptHeadPayload struct {
	HeadUserID int     `json:"head_user_id" binding:"required"`
	StartDate  *string `json:"start_date"   binding:"required"` // ISO/RFC3339 หรือ "YYYY-MM-DDTHH:mm:ss"
	EndDate    *string `json:"end_date"`                        // optional
	Note       *string `json:"note"`                            // optional
}

// GetCurrentDeptHead returns the head whose window covers "now" (UTC).
func GetCurrentDeptHead(c *gin.Context) {
	var row struct {
		HeadUserID    sql.NullInt64
		EffectiveFrom sql.NullTime
		EffectiveTo   sql.NullTime
	}

	// ใช้ UTC_TIMESTAMP() เพื่อให้เทียบกับค่า DATETIME ที่เราเก็บเป็น UTC
	if err := config.DB.Raw(`
		SELECT head_user_id, effective_from, effective_to
		FROM dept_head_assignments
		WHERE effective_from <= UTC_TIMESTAMP()
		  AND (effective_to IS NULL OR effective_to >= UTC_TIMESTAMP())
		ORDER BY effective_from DESC, assignment_id DESC
		LIMIT 1
	`).Row().Scan(&row.HeadUserID, &row.EffectiveFrom, &row.EffectiveTo); err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "query current head failed"})
		return
	}

	var headID *int
	if row.HeadUserID.Valid {
		v := int(row.HeadUserID.Int64)
		headID = &v
	}

	var ef *string
	if row.EffectiveFrom.Valid {
		v := row.EffectiveFrom.Time.UTC().Format(time.RFC3339)
		ef = &v
	}

	// (ถ้าจะส่ง effective_to ไปด้วย ก็ได้ แต่ FE ตอนนี้ไม่ได้ใช้)
	// var et *string
	// if row.EffectiveTo.Valid {
	// 	v := row.EffectiveTo.Time.UTC().Format(time.RFC3339)
	// 	et = &v
	// }

	c.JSON(http.StatusOK, gin.H{
		"head_user_id":   headID,
		"effective_from": ef,
		// "effective_to":   et,
	})
}

// ListDeptHeadHistory lists prior/current assignments ordered by latest first.
func ListDeptHeadHistory(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	type item struct {
		AssignmentID  int        `json:"assignment_id"`
		HeadUserID    int        `json:"head_user_id"`
		EffectiveFrom time.Time  `json:"effective_from"`
		EffectiveTo   *time.Time `json:"effective_to"`
		ChangedBy     *int       `json:"changed_by"`
		ChangedAt     time.Time  `json:"changed_at"`
		Note          *string    `json:"note"`
	}
	var items []item

	if err := config.DB.Raw(`
		SELECT assignment_id, head_user_id, effective_from, effective_to, changed_by, changed_at, note
		FROM dept_head_assignments
		ORDER BY assignment_id DESC
		LIMIT ? OFFSET ?
	`, limit, offset).Scan(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to list history"})
		return
	}

	// Format times to RFC3339 for consistency
	resp := make([]gin.H, 0, len(items))
	for _, it := range items {
		var efTo *string
		if it.EffectiveTo != nil {
			v := it.EffectiveTo.UTC().Format(time.RFC3339)
			efTo = &v
		}
		resp = append(resp, gin.H{
			"assignment_id":  it.AssignmentID,
			"head_user_id":   it.HeadUserID,
			"effective_from": it.EffectiveFrom.UTC().Format(time.RFC3339),
			"effective_to":   efTo,
			"changed_by":     it.ChangedBy,
			"changed_at":     it.ChangedAt.UTC().Format(time.RFC3339),
			"note":           it.Note,
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "items": resp})
}

// AssignDeptHead closes the current active assignment (if any) and inserts a new one.
// - รับ start_date/end_date
// - บันทึก changed_by ทั้งตอน UPDATE (ปิดของเดิม) และตอน INSERT (ของใหม่)
func AssignDeptHead(c *gin.Context) {
	var p AssignDeptHeadPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
		return
	}

	// แปลงเวลาแบบเดียวกับ helper ใน system_config.go
	st, err1 := parseTimePtr(p.StartDate)
	en, err2 := parseTimePtr(p.EndDate)
	if st == nil || err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid start/end date"})
		return
	}
	if en != nil && st.After(*en) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "start_date must be before or equal to end_date"})
		return
	}

	// ผู้ปฏิบัติ: ใช้ helper เดียวกับไฟล์ system_config.go (อ่านจาก context/header/JWT/cookie)
	changedBy := getUserIDAny(c)

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "begin tx failed"})
		return
	}

	// 1) ปิด assignment ที่ยังเปิดอยู่ (ถ้ามี) และบันทึก changed_by
	var cur struct {
		HeadUserID    *int
		RestoreRoleID *int
	}
	if err := tx.Raw(`
        SELECT head_user_id, restore_role_id
        FROM dept_head_assignments
        WHERE effective_to IS NULL
        ORDER BY assignment_id DESC
        LIMIT 1
    `).Row().Scan(&cur.HeadUserID, &cur.RestoreRoleID); err != nil && err != sql.ErrNoRows {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "query current assignment failed"})
		return
	}

	if cur.HeadUserID != nil && cur.RestoreRoleID != nil {
		// ปิด assignment เดิม: effective_to = start_date + อัปเดต changed_by/changed_at
		if err := tx.Exec(`
            UPDATE dept_head_assignments
            SET effective_to = ?, changed_by = ?, changed_at = NOW()
            WHERE effective_to IS NULL
        `, st, changedBy).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "close current assignment failed"})
			return
		}

		// ลดบทบาทหัวหน้าคนเดิมกลับไปตาม restore_role_id
		if err := tx.Exec(`UPDATE users SET role_id = ? WHERE user_id = ?`, *cur.RestoreRoleID, *cur.HeadUserID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "demote current head failed"})
			return
		}
	}

	// 2) โปรโมตหัวหน้าคนใหม่ และบันทึกประวัติ
	var prevRoleID int
	if err := tx.Raw(`SELECT role_id FROM users WHERE user_id = ?`, p.HeadUserID).Row().Scan(&prevRoleID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "new head not found"})
		return
	}

	// อัปเดตบทบาทเป็น dept_head (role_id = 4)
	if err := tx.Exec(`UPDATE users SET role_id = 4 WHERE user_id = ?`, p.HeadUserID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "promote new head failed"})
		return
	}

	// แทรกแถวใหม่: effective_from = st, effective_to = en (อาจเป็น NULL), changed_by = user ปัจจุบัน
	if err := tx.Exec(`
        INSERT INTO dept_head_assignments
            (head_user_id, restore_role_id, effective_from, effective_to, changed_by, changed_at, note)
        VALUES (?, ?, ?, ?, ?, NOW(), ?)
    `, p.HeadUserID, prevRoleID, st, en, changedBy, p.Note).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "insert assignment failed"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "commit failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
