package controllers

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var scopusAuthorIDPattern = regexp.MustCompile(`^[0-9]{5,}$`)

const (
	scopusAPIKeyConfigKey   = "X-ELS-APIKey"
	scopusLegacyAPIKeyField = "api_key"
)

// POST /api/v1/admin/user-publications/import/scopus?user_id=123&scopus_id=54683571200
func AdminImportScopusPublications(c *gin.Context) {
	uid := strings.TrimSpace(c.Query("user_id"))
	scopusID := strings.TrimSpace(c.Query("scopus_id"))
	if uid == "" || scopusID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing user_id or scopus_id"})
		return
	}

	id64, err := strconv.ParseUint(uid, 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user_id"})
		return
	}

	job := services.NewScopusIngestJobService(nil)
	res, err := job.RunForUser(c.Request.Context(), &services.ScopusIngestUserInput{
		UserID:         uint(id64),
		ScopusAuthorID: scopusID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "summary": res})
}

// POST /api/v1/admin/user-publications/import/scopus/all
func AdminImportScopusForAll(c *gin.Context) {
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

	job := services.NewScopusIngestJobService(nil)
	summary, err := job.RunForAll(c.Request.Context(), &services.ScopusIngestAllInput{
		UserIDs: userIDs,
		Limit:   limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "summary": summary})
}

// POST /api/v1/admin/scopus/metrics/backfill
func AdminBackfillCiteScoreMetrics(c *gin.Context) {
	metrics := services.NewCiteScoreMetricsService(nil, nil)
	summary, err := metrics.BackfillMissingMetrics(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "summary": summary})
}

// POST /api/v1/admin/scopus/metrics/refresh
func AdminRefreshCiteScoreMetrics(c *gin.Context) {
	metrics := services.NewCiteScoreMetricsService(nil, nil)
	summary, err := metrics.RefreshExistingMetrics(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "summary": summary})
}

type setScopusAuthorIDRequest struct {
	ScopusID string `json:"scopus_id"`
}

type scopusAPIKeyRequest struct {
	Value string `json:"value"`
}

// POST /api/v1/admin/users/:id/scopus-author
func AdminSetUserScopusAuthorID(c *gin.Context) {
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

	var payload setScopusAuthorIDRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	scopusID := strings.TrimSpace(payload.ScopusID)
	if scopusID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing scopus_id"})
		return
	}

	if !scopusAuthorIDPattern.MatchString(scopusID) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid scopus_id"})
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

	if err := config.DB.Model(&user).Update("Scopus_id", scopusID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user_id":   user.UserID,
			"scopus_id": scopusID,
		},
	})
}

// GET /api/v1/admin/scopus/config
func AdminGetScopusAPIKey(c *gin.Context) {
	var row models.ScopusConfig
	if err := config.DB.Where("`key` = ?", scopusAPIKeyConfigKey).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var legacy models.ScopusConfig
			if err := config.DB.Where("`key` = ?", scopusLegacyAPIKeyField).First(&legacy).Error; err == nil {
				legacy.Key = scopusAPIKeyConfigKey
				c.JSON(http.StatusOK, gin.H{"success": true, "data": legacy})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"key":   scopusAPIKeyConfigKey,
					"value": nil,
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	row.Key = scopusAPIKeyConfigKey
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// PUT /api/v1/admin/scopus/config
func AdminUpdateScopusAPIKey(c *gin.Context) {
	var payload scopusAPIKeyRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	value := strings.TrimSpace(payload.Value)
	if value == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing api key value"})
		return
	}

	row := models.ScopusConfig{Key: scopusAPIKeyConfigKey, Value: &value}
	if err := config.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// GET /api/v1/admin/scopus/import/jobs
func AdminListScopusAPIImportJobs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	var total int64
	if err := config.DB.Model(&models.ScopusAPIImportJob{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var jobs []models.ScopusAPIImportJob
	offset := (page - 1) * perPage
	if err := config.DB.Order("started_at DESC").Offset(offset).Limit(perPage).Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	jobIDs := make([]uint64, 0, len(jobs))
	for _, job := range jobs {
		jobIDs = append(jobIDs, job.ID)
	}

	requestStats := map[uint64]struct {
		Count         int
		ItemsReturned int
		LastRequest   *time.Time
	}{}

	if len(jobIDs) > 0 {
		type aggRow struct {
			JobID         uint64
			RequestCount  int
			ItemsReturned int
			LastRequest   *time.Time
		}

		var rows []aggRow
		if err := config.DB.
			Table("scopus_api_requests").
			Select("job_id, COUNT(*) AS request_count, COALESCE(SUM(items_returned),0) AS items_returned, MAX(created_at) AS last_request").
			Where("job_id IN ?", jobIDs).
			Group("job_id").
			Find(&rows).Error; err == nil {
			for _, row := range rows {
				requestStats[row.JobID] = struct {
					Count         int
					ItemsReturned int
					LastRequest   *time.Time
				}{
					Count:         row.RequestCount,
					ItemsReturned: row.ItemsReturned,
					LastRequest:   row.LastRequest,
				}
			}
		}
	}

	out := make([]gin.H, 0, len(jobs))
	for _, job := range jobs {
		stats := requestStats[job.ID]
		out = append(out, gin.H{
			"id":               job.ID,
			"service":          job.Service,
			"job_type":         job.JobType,
			"scopus_author_id": job.ScopusAuthorID,
			"query_string":     job.QueryString,
			"total_results":    job.TotalResults,
			"status":           job.Status,
			"error_message":    job.ErrorMessage,
			"started_at":       job.StartedAt,
			"finished_at":      job.FinishedAt,
			"created_at":       job.CreatedAt,
			"updated_at":       job.UpdatedAt,
			"request_count":    stats.Count,
			"items_returned":   stats.ItemsReturned,
			"last_request_at":  stats.LastRequest,
		})
	}

	totalPages := 0
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}

	hasNext := int64(page*perPage) < total
	hasPrev := page > 1

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    out,
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

// GET /api/v1/admin/scopus/import/jobs/:id/requests
func AdminListScopusAPIRequests(c *gin.Context) {
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
	if err := config.DB.Model(&models.ScopusAPIRequest{}).Where("job_id = ?", jobID).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var requests []models.ScopusAPIRequest
	offset := (page - 1) * perPage
	if err := config.DB.Order("created_at DESC").Where("job_id = ?", jobID).Offset(offset).Limit(perPage).Find(&requests).Error; err != nil {
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
		"data":    requests,
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
