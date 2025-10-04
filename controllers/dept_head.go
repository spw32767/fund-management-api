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
	type rowT struct {
		HeadUserID    sql.NullInt64
		EffectiveFrom sql.NullTime
		EffectiveTo   sql.NullTime
	}
	var row rowT

	// เทียบเวลาแบบ UTC ให้ตรงกับค่าที่บันทึก (จาก datetime-local -> ISO -> UTC)
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

	c.JSON(http.StatusOK, gin.H{
		"head_user_id":   headID,
		"effective_from": ef,
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

func AssignDeptHead(c *gin.Context) {
	var p AssignDeptHeadPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
		return
	}

	// parse ช่วงเวลา
	st, err1 := parseTimePtr(p.StartDate)
	en, err2 := parseTimePtr(p.EndDate)
	if st == nil || err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid start/end date"})
		return
	}

	// ---- NEW: backfill buffer 1 นาที เพื่อกันช่องว่างระหว่างสลับหัวหน้า ----
	nowUTC := time.Now().UTC()
	bufferStart := nowUTC.Add(-1 * time.Minute)

	// ใช้เวลาเริ่มที่ "ย้อน 1 นาที" ถ้าคนกรอกมาช้ากว่านั้น (คืออยู่อนาคตกว่า bufferStart)
	if st.After(bufferStart) {
		*st = bufferStart
	}

	// validate หลังปรับ buffer แล้ว
	if en != nil && st.After(*en) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "start_date must be before or equal to end_date"})
		return
	}

	changedBy := getUserIDAny(c)

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "begin tx failed"})
		return
	}

	// 1) หาแถวที่ "active ณ เวลาเริ่มใหม่ (st)" หรือทับซ้อนกับ st แล้ว demote role กลับ
	type oldAssign struct {
		HeadUserID    int
		RestoreRoleID sql.NullInt64
	}

	var olds []oldAssign
	if err := tx.Raw(`
		SELECT head_user_id,
		       COALESCE(restore_role_id, (SELECT role_id FROM users WHERE user_id = head_user_id)) AS restore_role_id
		FROM dept_head_assignments
		WHERE effective_from <= ?
		  AND (effective_to IS NULL OR effective_to > ?)
	`, st, st).Scan(&olds).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "query overlap failed"})
		return
	}

	// ปิดทุกแถวที่ทับซ้อนกับ st
	if err := tx.Exec(`
		UPDATE dept_head_assignments
		SET effective_to = ?, changed_by = ?, changed_at = UTC_TIMESTAMP()
		WHERE effective_from <= ?
		  AND (effective_to IS NULL OR effective_to > ?)
	`, st, changedBy, st, st).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "close previous assignments failed"})
		return
	}

	// คืน role ให้ทุกคนที่ถูกปิด
	for _, o := range olds {
		restore := 1 // fallback: teacher
		if o.RestoreRoleID.Valid {
			restore = int(o.RestoreRoleID.Int64)
		}
		if err := tx.Exec(`UPDATE users SET role_id = ? WHERE user_id = ? AND role_id = 4`, restore, o.HeadUserID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "demote previous head failed"})
			return
		}
	}

	// 2) โปรโมตคนใหม่ + บันทึกประวัติ
	var prevRoleID int
	if err := tx.Raw(`SELECT role_id FROM users WHERE user_id = ?`, p.HeadUserID).Row().Scan(&prevRoleID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "new head not found"})
		return
	}

	// โปรโมตเป็น dept_head
	if err := tx.Exec(`UPDATE users SET role_id = 4 WHERE user_id = ?`, p.HeadUserID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "promote new head failed"})
		return
	}

	// แทรก assignment ใหม่
	if err := tx.Exec(`
		INSERT INTO dept_head_assignments
			(head_user_id, restore_role_id, effective_from, effective_to, changed_by, changed_at, note)
		VALUES (?, ?, ?, ?, ?, UTC_TIMESTAMP(), ?)
	`, p.HeadUserID, prevRoleID, st, en, changedBy, p.Note).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "insert assignment failed"})
		return
	}

	// 3) (ออปชัน) cleanup: ใครที่มี assignment สิ้นสุดแล้ว แต่ยังติด role 4 อยู่ ให้คืน role ให้ถูกต้อง
	if err := tx.Exec(`
		UPDATE users u
		JOIN dept_head_assignments a ON a.head_user_id = u.user_id
		SET u.role_id = a.restore_role_id
		WHERE u.role_id = 4
		  AND a.effective_to IS NOT NULL
		  AND a.effective_to <= UTC_TIMESTAMP()
	`).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "cleanup orphan heads failed"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "commit failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
