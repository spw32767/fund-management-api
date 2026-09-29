package controllers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/admin/scopus/author-roles/status
func AdminGetScopusAuthorRoleStatus(c *gin.Context) {
	svc := services.NewScopusAuthorRoleService(nil, nil)
	coverage, err := svc.Coverage(c.Request.Context())
	if err != nil {
		InternalError(c, "scopus", err)
		return
	}
	active, err := svc.ActiveRun(c.Request.Context())
	if err != nil {
		InternalError(c, "scopus", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"coverage": coverage, "active_run": active}})
}

type startScopusAuthorRoleRunRequest struct {
	RunType        string `json:"run_type"`
	ConfirmRefresh bool   `json:"confirm_refresh"`
}

// POST /api/v1/admin/scopus/author-roles/runs
func AdminStartScopusAuthorRoleRun(c *gin.Context) {
	var req startScopusAuthorRoleRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if req.RunType != "backfill" && req.RunType != "retry_review" && req.RunType != "refresh" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "run_type must be backfill, retry_review, or refresh"})
		return
	}
	if req.RunType == "refresh" && !req.ConfirmRefresh {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "confirm_refresh is required"})
		return
	}
	svc := services.NewScopusAuthorRoleService(nil, nil)
	run, err := svc.StartRun(c.Request.Context(), req.RunType)
	if errors.Is(err, services.ErrScopusAuthorRoleRunActive) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "scopus author role run already active"})
		return
	}
	if err != nil {
		InternalError(c, "scopus", err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		svc.ExecuteRun(ctx, run)
		log.Printf("scopus author roles: run %d finished", run.ID)
	}()
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": run})
}

// GET /api/v1/admin/scopus/author-roles/runs
func AdminListScopusAuthorRoleRuns(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 10
	}
	svc := services.NewScopusAuthorRoleService(nil, nil)
	runs, total, err := svc.ListRuns(c.Request.Context(), page, perPage)
	if err != nil {
		InternalError(c, "scopus", err)
		return
	}
	offset := (page - 1) * perPage
	c.JSON(http.StatusOK, gin.H{
		"success": true, "data": runs,
		"pagination": gin.H{
			"current_page": page, "per_page": perPage, "total_count": total,
			"total_pages": int((total + int64(perPage) - 1) / int64(perPage)),
			"has_next":    int64(offset+perPage) < total, "has_prev": page > 1,
		},
	})
}
