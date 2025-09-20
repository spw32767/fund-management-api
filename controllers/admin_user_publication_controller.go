package controllers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"fund-management-api/config"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// POST /api/v1/admin/user-publications/import/scholar?user_id=123&author_id=W2k2JXwAAAAJ
func AdminImportScholarPublications(c *gin.Context) {
	uid := c.Query("user_id")
	authorID := c.Query("author_id")
	if uid == "" || authorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing user_id or author_id"})
		return
	}

	id64, err := strconv.ParseUint(uid, 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
		return
	}
	userID := uint(id64)

	job := services.NewScholarImportJobService(nil)
	ctx := context.WithoutCancel(c.Request.Context())

	summary, err := job.RunForUser(ctx, &services.ScholarImportUserInput{
		UserID:   userID,
		AuthorID: authorID,
	})
	if err != nil {
		var scriptErr *services.ScholarScriptError
		if errors.As(err, &scriptErr) {
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": scriptErr.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"summary": gin.H{
			"fetched": summary.PublicationsFetched,
			"created": summary.PublicationsCreated,
			"updated": summary.PublicationsUpdated,
			"failed":  summary.PublicationsFailed,
		},
	})
}

// POST /api/v1/admin/user-publications/import/scholar/all
// Optional: ?user_ids=1,2,3  (CSV subset)
// Optional: ?limit=50  (max users to process in one call)
func AdminImportScholarForAll(c *gin.Context) {
	var userIDs []uint
	if csv := strings.TrimSpace(c.Query("user_ids")); csv != "" {
		parts := strings.Split(csv, ",")
		for _, p := range parts {
			if id64, err := strconv.ParseUint(strings.TrimSpace(p), 10, 64); err == nil && id64 > 0 {
				userIDs = append(userIDs, uint(id64))
			}
		}
	}

	limit := 0
	if limStr := strings.TrimSpace(c.Query("limit")); limStr != "" {
		if lim, err := strconv.Atoi(limStr); err == nil && lim > 0 {
			limit = lim
		}
	}

	job := services.NewScholarImportJobService(nil)
	ctx := context.WithoutCancel(c.Request.Context())

	summary, err := job.RunForAll(ctx, &services.ScholarImportAllInput{
		UserIDs:       userIDs,
		Limit:         limit,
		TriggerSource: "admin_api",
		LockName:      "scholar_import_job",
		RecordRun:     true,
	})
	if err != nil {
		if errors.Is(err, services.ErrScholarImportAlreadyRunning) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": "scholar import already running"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "summary": summary})
}

type AdminUserLite struct {
	UserID uint   `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}

// GET /api/v1/admin/users/search?q=smith&limit=10
func AdminSearchUsers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing q"})
		return
	}
	limit := 10
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		// ignore parse errors silently
	}

	type row struct {
		UserID    uint
		UserFname *string
		UserLname *string
		Email     *string
	}

	var rows []row
	like := "%" + q + "%"
	if err := config.DB.
		Table("users").
		Select("user_id, user_fname, user_lname, email").
		Where("CONCAT(COALESCE(user_fname,''),' ',COALESCE(user_lname,'')) LIKE ? OR email LIKE ?", like, like).
		Limit(limit).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	out := make([]AdminUserLite, 0, len(rows))
	for _, r := range rows {
		fn, ln := "", ""
		if r.UserFname != nil {
			fn = *r.UserFname
		}
		if r.UserLname != nil {
			ln = *r.UserLname
		}
		name := strings.TrimSpace(fn + " " + ln)
		email := ""
		if r.Email != nil {
			email = *r.Email
		}
		out = append(out, AdminUserLite{UserID: r.UserID, Name: name, Email: email})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
