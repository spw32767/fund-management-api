package controllers

import (
	"net/http"
	"strings"
	"time"

	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// ExtListUsers is the external (partner) faculty-directory endpoint:
//
//	GET /api/ext/v1/users?updated_since=2026-01-01T00:00:00Z&limit=100&offset=0
//
// It returns a flat, paginated list of all faculty (excluding soft-deleted). `updated_since`
// (RFC 3339) enables incremental sync; when omitted the full directory is returned. The
// consumer is expected to pull periodically and store the result in its own database.
func ExtListUsers(c *gin.Context) {
	var updatedSince *time.Time
	if raw := strings.TrimSpace(c.Query("updated_since")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			extValidationError(c, "updated_since must be an RFC 3339 datetime, e.g. 2026-01-01T00:00:00Z")
			return
		}
		updatedSince = &t
	}

	limit := parseIntOrDefault(c.Query("limit"), 100)
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := parseIntOrDefault(c.Query("offset"), 0)
	if offset < 0 {
		offset = 0
	}

	svc := services.NewUserDirectoryService(nil)
	items, total, err := svc.ListForPartner(updatedSince, limit, offset)
	if err != nil {
		InternalError(c, "ext_users", err)
		return
	}

	var updatedSinceOut interface{}
	if updatedSince != nil {
		updatedSinceOut = updatedSince.Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"paging": gin.H{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
		"filters": gin.H{
			"updated_since": updatedSinceOut,
		},
	})
}
