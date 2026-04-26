package controllers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

const thaiJOBatchRunTimeout = 2 * time.Hour

type setThaiJOAuthorIDRequest struct {
	ThaiJOAuthorID string `json:"thaijo_author_id"`
}

type setThaiJOSyncEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// POST /api/v1/admin/user-publications/import/thaijo?user_id=123
func AdminImportThaiJOPublications(c *gin.Context) {
	uid := strings.TrimSpace(c.Query("user_id"))
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing user_id"})
		return
	}
	id64, err := strconv.ParseUint(uid, 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
		return
	}

	job := services.NewThaiJOIngestJobService(nil)
	res, err := job.RunForUser(c.Request.Context(), uint(id64))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "summary": res})
}

// POST /api/v1/admin/user-publications/import/thaijo/all
func AdminImportThaiJOForAll(c *gin.Context) {
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

	job := services.NewThaiJOIngestJobService(nil)
	activeRun, err := job.GetActiveBatchRun(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if activeRun != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "thaijo batch import already running",
			"data":    activeRun,
		})
		return
	}

	input := &services.ThaiJOIngestAllInput{UserIDs: userIDs, Limit: limit}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), thaiJOBatchRunTimeout)
		defer cancel()

		if _, err := job.RunForAll(ctx, input); err != nil {
			if errors.Is(err, services.ErrThaiJOBatchImportAlreadyRunning) {
				log.Printf("thaijo batch import skipped: job already running")
				return
			}
			log.Printf("thaijo batch import job failed: %v", err)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"summary": gin.H{"status": "running", "message": "batch import started"},
	})
}

// POST /api/v1/admin/users/:id/thaijo-author
func AdminSetUserThaiJOAuthorID(c *gin.Context) {
	uid := strings.TrimSpace(c.Param("id"))
	id64, err := strconv.ParseUint(uid, 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
		return
	}

	var payload setThaiJOAuthorIDRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	authorID := strings.TrimSpace(payload.ThaiJOAuthorID)
	if authorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing thaijo_author_id"})
		return
	}

	if err := config.DB.Table("users").Where("user_id = ?", uint(id64)).Update("thaijo_author_id", authorID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user_id": uint(id64), "thaijo_author_id": authorID}})
}

// POST /api/v1/admin/users/:id/thaijo-sync
func AdminSetUserThaiJOSyncEnabled(c *gin.Context) {
	uid := strings.TrimSpace(c.Param("id"))
	id64, err := strconv.ParseUint(uid, 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
		return
	}

	var payload setThaiJOSyncEnabledRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	enabledVal := 0
	if payload.Enabled {
		enabledVal = 1
	}
	if err := config.DB.Table("users").Where("user_id = ?", uint(id64)).Update("thaijo_sync_enabled", enabledVal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user_id": uint(id64), "thaijo_sync_enabled": payload.Enabled}})
}

// GET /api/v1/admin/users/thaijo
func AdminListUsersWithThaiJO(c *gin.Context) {
	limit := 20
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 200 {
		limit = 200
	}
	offset := 0
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	query := config.DB.Table("users").Select("user_id, user_fname, user_lname, email, thaijo_author_id, thaijo_sync_enabled")
	if onlyEnabled := strings.TrimSpace(c.Query("enabled_only")); onlyEnabled == "1" || strings.EqualFold(onlyEnabled, "true") {
		query = query.Where("thaijo_sync_enabled = 1")
	}

	type row struct {
		UserID           uint
		UserFname        *string
		UserLname        *string
		Email            *string
		ThaiJOAuthorID   *string `gorm:"column:thaijo_author_id"`
		ThaiJOSyncEnabled bool   `gorm:"column:thaijo_sync_enabled"`
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var rows []row
	if err := query.Order("user_id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{
			"user_id":             r.UserID,
			"name":                strings.TrimSpace(derefString(r.UserFname) + " " + derefString(r.UserLname)),
			"email":               derefString(r.Email),
			"thaijo_author_id":    r.ThaiJOAuthorID,
			"thaijo_sync_enabled": r.ThaiJOSyncEnabled,
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": out, "paging": gin.H{"total": total, "limit": limit, "offset": offset}})
}

// GET /api/v1/admin/thaijo/import/jobs
func AdminListThaiJOAPIImportJobs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	var total int64
	if err := config.DB.Model(&models.ThaiJOAPIImportJob{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var jobs []models.ThaiJOAPIImportJob
	offset := (page - 1) * perPage
	if err := config.DB.Order("started_at DESC").Offset(offset).Limit(perPage).Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": jobs, "pagination": gin.H{"current_page": page, "per_page": perPage, "total_count": total, "total_pages": totalPages, "has_next": int64(page*perPage) < total, "has_prev": page > 1}})
}

// GET /api/v1/admin/thaijo/import/jobs/:id/requests
func AdminListThaiJOAPIRequests(c *gin.Context) {
	jobIDParam := strings.TrimSpace(c.Param("id"))
	jobID, err := strconv.ParseUint(jobIDParam, 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid job id"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 20
	}

	var total int64
	if err := config.DB.Model(&models.ThaiJOAPIRequest{}).Where("job_id = ?", jobID).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var requests []models.ThaiJOAPIRequest
	offset := (page - 1) * perPage
	if err := config.DB.Order("created_at DESC").Where("job_id = ?", jobID).Offset(offset).Limit(perPage).Find(&requests).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": requests, "pagination": gin.H{"current_page": page, "per_page": perPage, "total_count": total, "total_pages": totalPages, "has_next": int64(page*perPage) < total, "has_prev": page > 1}})
}

// GET /api/v1/admin/thaijo/import/batch/runs
func AdminListThaiJOBatchImportRuns(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	var total int64
	if err := config.DB.Model(&models.ThaiJOBatchImportRun{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var runs []models.ThaiJOBatchImportRun
	offset := (page - 1) * perPage
	if err := config.DB.Order("started_at DESC").Offset(offset).Limit(perPage).Find(&runs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": runs, "pagination": gin.H{"current_page": page, "per_page": perPage, "total_count": total, "total_pages": totalPages, "has_next": int64(offset+perPage) < total, "has_prev": page > 1}})
}

// GET /api/v1/teacher/user-publications/thaijo
func GetUserThaiJOPublications(c *gin.Context) {
	userID, err := resolveTargetUserID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	limit := parseIntOrDefault(c.Query("limit"), 10)
	offset := parseIntOrDefault(c.Query("offset"), 0)
	search := c.Query("q")

	svc := services.NewThaiJOPublicationService(nil)
	items, total, err := svc.ListByUser(userID, limit, offset, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "paging": gin.H{"total": total, "limit": limit, "offset": offset}})
}

// GET /api/v1/admin/publications/thaijo?user_id=123
func AdminListThaiJOPublications(c *gin.Context) {
	uid := strings.TrimSpace(c.Query("user_id"))
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing user_id"})
		return
	}
	id64, err := strconv.ParseUint(uid, 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
		return
	}

	limit := parseIntOrDefault(c.Query("limit"), 10)
	offset := parseIntOrDefault(c.Query("offset"), 0)
	search := c.Query("q")

	svc := services.NewThaiJOPublicationService(nil)
	items, total, err := svc.ListByUser(uint(id64), limit, offset, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "paging": gin.H{"total": total, "limit": limit, "offset": offset}})
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
