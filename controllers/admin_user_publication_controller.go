package controllers

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var scholarAuthorIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{10,}$`)

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
	summary, err := job.RunForUser(c.Request.Context(), &services.ScholarImportUserInput{
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
	summary, err := job.RunForAll(c.Request.Context(), &services.ScholarImportAllInput{
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
	UserID          uint    `json:"user_id"`
	Name            string  `json:"name"`
	Email           string  `json:"email"`
	ScholarAuthorID *string `json:"scholar_author_id,omitempty"`
	ScopusAuthorID  *string `json:"scopus_id,omitempty"`
}

type setScholarAuthorIDRequest struct {
	AuthorID string `json:"author_id"`
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
		UserID          uint
		UserFname       *string
		UserLname       *string
		Email           *string
		ScholarAuthorID *string
		ScopusID        *string
	}

	var rows []row
	like := "%" + q + "%"
	if err := config.DB.
		Table("users").
		Select("user_id, user_fname, user_lname, email, scholar_author_id, Scopus_id AS scopus_id").
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
		out = append(out, AdminUserLite{
			UserID:          r.UserID,
			Name:            name,
			Email:           email,
			ScholarAuthorID: r.ScholarAuthorID,
			ScopusAuthorID:  r.ScopusID,
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// POST /api/v1/admin/users/:id/scholar-author
func AdminSetUserScholarAuthorID(c *gin.Context) {
	uid := strings.TrimSpace(c.Param("id"))
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing user_id"})
		return
	}

	id64, err := strconv.ParseUint(uid, 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
		return
	}

	var payload setScholarAuthorIDRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	authorID := strings.TrimSpace(payload.AuthorID)
	if authorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing author_id"})
		return
	}

	normalized := authorID
	if strings.HasPrefix(normalized, "_") {
		normalized = strings.TrimPrefix(normalized, "_")
	} else {
		authorID = "_" + normalized
	}

	if !scholarAuthorIDPattern.MatchString(normalized) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid author_id"})
		return
	}

	var user models.User
	if err := config.DB.Where("user_id = ?", uint(id64)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if err := config.DB.Model(&user).Update("scholar_author_id", authorID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user_id":           user.UserID,
			"scholar_author_id": authorID,
		},
	})
}

// GET /api/v1/admin/user-publications/import/scholar/runs
func AdminListScholarImportRuns(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	var total int64
	if err := config.DB.Model(&models.ScholarImportRun{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var runs []models.ScholarImportRun
	offset := (page - 1) * perPage
	if err := config.DB.Order("started_at DESC").Offset(offset).Limit(perPage).Find(&runs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	totalPages := 0
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}

	hasNext := int64(page*perPage) < total
	hasPrev := page > 1

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    runs,
		"pagination": gin.H{
			"current_page": page,
			"per_page":     perPage,
			"total_count":  total,
			"total_pages":  totalPages,
			"has_next":     hasNext,
			"has_prev":     hasPrev,
		},
	})
}
