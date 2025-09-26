package controllers

import (
	"net/http"
	"strconv"
	"time"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

// AssignDeptHeadPayload is the payload for assigning (or re-assigning) the department head.
// department_code removed per requirement; we track a single current head across the system.
type AssignDeptHeadPayload struct {
	HeadUserID    int     `json:"head_user_id" binding:"required"`
	EffectiveFrom *string `json:"effective_from"` // RFC3339; optional (defaults to now)
	Note          *string `json:"note"`           // optional
}

// GetCurrentDeptHead returns the currently active head (effective_to IS NULL).
func GetCurrentDeptHead(c *gin.Context) {
	var row struct {
		HeadUserID    *int       `json:"head_user_id"`
		EffectiveFrom *time.Time `json:"effective_from"`
	}

	if err := config.DB.Raw(`
		SELECT head_user_id, effective_from
		FROM dept_head_assignments
		WHERE effective_to IS NULL
		ORDER BY assignment_id DESC
		LIMIT 1
	`).Row().Scan(&row.HeadUserID, &row.EffectiveFrom); err != nil {
		// If no rows, scan may return nils; we'll just return null values below
	}

	var ef *string
	if row.EffectiveFrom != nil {
		v := row.EffectiveFrom.UTC().Format(time.RFC3339)
		ef = &v
	}

	c.JSON(http.StatusOK, gin.H{
		"head_user_id":   row.HeadUserID,
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

// AssignDeptHead closes the current active assignment (if any) and inserts a new one.
// This always creates a new row, even if assigning back to a previous person.
func AssignDeptHead(c *gin.Context) {
	var p AssignDeptHeadPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
		return
	}

	// Resolve effective_from
	effectiveFrom := time.Now().UTC()
	if p.EffectiveFrom != nil && *p.EffectiveFrom != "" {
		parsed, err := time.Parse(time.RFC3339, *p.EffectiveFrom)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid effective_from (RFC3339)"})
			return
		}
		effectiveFrom = parsed.UTC()
	}

	// Actor (changed_by)
	var changedBy *int
	if v, ok := c.Get("user_id"); ok {
		if id, ok2 := v.(int); ok2 {
			changedBy = &id
		}
	}

	// Use a transaction to ensure atomicity
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to begin tx"})
		return
	}

	// 1) Close current active (if any)
	if err := tx.Exec(`
		UPDATE dept_head_assignments
		SET effective_to = ?
		WHERE effective_to IS NULL
	`, effectiveFrom).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to close current"})
		return
	}

	// 2) Insert new assignment (always a new row)
	if err := tx.Exec(`
		INSERT INTO dept_head_assignments (head_user_id, effective_from, effective_to, changed_by, changed_at, note)
		VALUES (?, ?, NULL, ?, NOW(), ?)
	`, p.HeadUserID, effectiveFrom, changedBy, p.Note).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to insert new assignment"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to commit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
