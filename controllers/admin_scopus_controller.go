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
