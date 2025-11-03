package controllers

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

type runKkuPeopleRequest struct {
	DryRun bool `json:"dry_run"`
	Debug  bool `json:"debug"`
}

// POST /api/v1/admin/kku-people/scrape
func AdminRunKkuPeopleScrape(c *gin.Context) {
	var req runKkuPeopleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
		req = runKkuPeopleRequest{}
	}

	job := services.NewKkuPeopleImportJobService(nil)
	summary, run, err := job.Run(c.Request.Context(), &services.KkuPeopleImportInput{
		DryRun:        req.DryRun,
		Debug:         req.Debug,
		TriggerSource: "admin_api",
		LockName:      "kku_people_import_job",
		RecordRun:     true,
	})
	if err != nil {
		if errors.Is(err, services.ErrKkuPeopleImportAlreadyRunning) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": "kku people scraper already running"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"summary": summary,
		"run":     run,
	})
}

// GET /api/v1/admin/kku-people/status
func AdminGetKkuPeopleStatus(c *gin.Context) {
	runSvc := services.NewKkuPeopleImportRunService(nil)

	running, err := runSvc.GetRunning()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	last, err := runSvc.GetLatestCompleted()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"status": gin.H{
			"in_progress": running != nil,
			"current_run": running,
			"last_run":    last,
			"next_run_at": nil,
		},
	})
}

// GET /api/v1/admin/kku-people/logs
func AdminListKkuPeopleLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	runSvc := services.NewKkuPeopleImportRunService(nil)
	runs, total, err := runSvc.List(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	hasNext := int64(offset+limit) < total
	hasPrev := offset > 0

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    runs,
		"pagination": gin.H{
			"limit":    limit,
			"offset":   offset,
			"total":    total,
			"has_next": hasNext,
			"has_prev": hasPrev,
		},
	})
}
