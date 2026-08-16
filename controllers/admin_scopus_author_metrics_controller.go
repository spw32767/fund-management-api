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

const authorMetricsRunTimeout = 1 * time.Hour

// POST /api/v1/admin/scopus/author-metrics/refresh?user_ids=1,2&limit=10
// ดึง h-index ของอาจารย์ทุกคน (อิง users.scopus_id) แล้วเก็บ snapshot รายวัน แบบ async
func AdminRefreshAuthorMetrics(c *gin.Context) {
	var userIDs []uint
	if csv := strings.TrimSpace(c.Query("user_ids")); csv != "" {
		for _, p := range strings.Split(csv, ",") {
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

	svc := services.NewAuthorMetricsService(nil, nil)
	activeRun, err := svc.GetActiveRun(c.Request.Context())
	if err != nil {
		InternalError(c, "scopus", err)
		return
	}
	if activeRun != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "scopus author metrics job already running",
			"data":    activeRun,
		})
		return
	}

	input := &services.ScopusAuthorMetricsAllInput{UserIDs: userIDs, Limit: limit, TriggerSource: "admin_ui"}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), authorMetricsRunTimeout)
		defer cancel()

		if _, err := svc.RunForAll(ctx, input); err != nil {
			if errors.Is(err, services.ErrAuthorMetricsAlreadyRunning) {
				log.Printf("scopus author metrics skipped: job already running")
				return
			}
			log.Printf("scopus author metrics job failed: %v", err)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"summary": gin.H{"status": "running", "message": "author metrics refresh started"},
	})
}

// GET /api/v1/admin/scopus/author-metrics/runs?page=1&per_page=20
func AdminListAuthorMetricsRuns(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	query := config.DB.Model(&models.ScopusAuthorMetricsRun{})

	var total int64
	if err := query.Count(&total).Error; err != nil {
		InternalError(c, "scopus", err)
		return
	}

	var runs []models.ScopusAuthorMetricsRun
	offset := (page - 1) * perPage
	if err := query.Order("started_at DESC").Offset(offset).Limit(perPage).Find(&runs).Error; err != nil {
		InternalError(c, "scopus", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    runs,
		"pagination": gin.H{
			"current_page": page,
			"per_page":     perPage,
			"total_count":  total,
			"total_pages":  int((total + int64(perPage) - 1) / int64(perPage)),
			"has_next":     int64(offset+perPage) < total,
			"has_prev":     page > 1,
		},
	})
}

// GET /api/v1/admin/scopus/author-metrics/hgraph?scopus_id=54683571200&year_from=2011&year_to=2026
// สร้าง Hirsch h-graph (documents เรียงตาม citations) จาก scopus_documents ที่ ingest ไว้แล้ว
func AdminGetScopusAuthorHIndexGraph(c *gin.Context) {
	scopusID := strings.TrimSpace(c.Query("scopus_id"))
	if scopusID == "" {
		// fallback: resolve from user_id -> users.scopus_id
		if uidStr := strings.TrimSpace(c.Query("user_id")); uidStr != "" {
			if uid, err := strconv.Atoi(uidStr); err == nil && uid > 0 {
				var u models.User
				if err := config.DB.Select("Scopus_id").Where("user_id = ?", uid).First(&u).Error; err == nil && u.ScopusID != nil {
					scopusID = strings.TrimSpace(*u.ScopusID)
				}
			}
		}
	}
	if scopusID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing scopus_id or user_id"})
		return
	}

	var yearFrom, yearTo *int
	if v := strings.TrimSpace(c.Query("year_from")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			yearFrom = &y
		}
	}
	if v := strings.TrimSpace(c.Query("year_to")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			yearTo = &y
		}
	}

	svc := services.NewAuthorHGraphService(nil)
	graph, err := svc.GetGraph(c.Request.Context(), scopusID, yearFrom, yearTo)
	if err != nil {
		InternalError(c, "scopus", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": graph})
}
