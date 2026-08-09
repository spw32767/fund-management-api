package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// parseClientIDParam reads the :id path param as a client id.
func parseClientIDParam(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid client id"})
		return 0, false
	}
	return id, true
}

// GET /api/v1/admin/api-clients
func AdminListAPIClients(c *gin.Context) {
	svc := services.NewAPIClientService(nil)
	clients, err := svc.ListClients()
	if err != nil {
		InternalError(c, "api_clients", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": clients})
}

type createAPIClientRequest struct {
	Name         string   `json:"name"`
	Description  *string  `json:"description"`
	ContactEmail *string  `json:"contact_email"`
	Scopes       []string `json:"scopes"`
}

// POST /api/v1/admin/api-clients
func AdminCreateAPIClient(c *gin.Context) {
	var req createAPIClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name is required"})
		return
	}

	svc := services.NewAPIClientService(nil)
	client, granted, err := svc.CreateClient(req.Name, req.Description, req.ContactEmail, req.Scopes)
	if err != nil {
		if errors.Is(err, services.ErrAPIScopeNotFound) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": "one or more scopes are unknown"})
			return
		}
		InternalError(c, "api_clients", err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    gin.H{"client": client, "scopes": granted},
	})
}

// GET /api/v1/admin/api-clients/:id
func AdminGetAPIClient(c *gin.Context) {
	id, ok := parseClientIDParam(c)
	if !ok {
		return
	}
	svc := services.NewAPIClientService(nil)
	client, err := svc.GetClient(id)
	if err != nil {
		if errors.Is(err, services.ErrAPIClientNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "client not found"})
			return
		}
		InternalError(c, "api_clients", err)
		return
	}
	keys, err := svc.ListKeys(id)
	if err != nil {
		InternalError(c, "api_clients", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"client": client, "keys": keys},
	})
}

type updateAPIClientRequest struct {
	Name         *string   `json:"name"`
	Description  *string   `json:"description"`
	ContactEmail *string   `json:"contact_email"`
	Status       *string   `json:"status"`
	Scopes       *[]string `json:"scopes"`
}

// PATCH /api/v1/admin/api-clients/:id
func AdminUpdateAPIClient(c *gin.Context) {
	id, ok := parseClientIDParam(c)
	if !ok {
		return
	}
	var req updateAPIClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if req.Status != nil && *req.Status != "active" && *req.Status != "disabled" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "status must be 'active' or 'disabled'"})
		return
	}

	var scopeCodes []string
	if req.Scopes != nil {
		scopeCodes = *req.Scopes
	}

	svc := services.NewAPIClientService(nil)
	client, granted, err := svc.UpdateClient(id, req.Name, req.Description, req.ContactEmail, req.Status, scopeCodes)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAPIClientNotFound):
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "client not found"})
		case errors.Is(err, services.ErrAPIScopeNotFound):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": "one or more scopes are unknown"})
		default:
			InternalError(c, "api_clients", err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"client": client, "scopes": granted},
	})
}

type issueAPIKeyRequest struct {
	Label         *string `json:"label"`
	ExpiresInDays *int    `json:"expires_in_days"`
}

// POST /api/v1/admin/api-clients/:id/keys
// Returns the plaintext key exactly once.
func AdminIssueAPIKey(c *gin.Context) {
	id, ok := parseClientIDParam(c)
	if !ok {
		return
	}
	var req issueAPIKeyRequest
	// Body is optional; ignore bind errors when empty.
	_ = c.ShouldBindJSON(&req)

	var expiresAt *time.Time
	if req.ExpiresInDays != nil && *req.ExpiresInDays > 0 {
		t := time.Now().AddDate(0, 0, *req.ExpiresInDays)
		expiresAt = &t
	}

	svc := services.NewAPIClientService(nil)
	issued, err := svc.IssueKey(id, req.Label, expiresAt)
	if err != nil {
		if errors.Is(err, services.ErrAPIClientNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "client not found"})
			return
		}
		InternalError(c, "api_clients", err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Store this key now — it will not be shown again.",
		"data": gin.H{
			"raw_key": issued.RawKey,
			"key":     issued.Key,
		},
	})
}

// DELETE /api/v1/admin/api-clients/:id/keys/:keyId
func AdminRevokeAPIKey(c *gin.Context) {
	id, ok := parseClientIDParam(c)
	if !ok {
		return
	}
	keyID, err := strconv.ParseUint(strings.TrimSpace(c.Param("keyId")), 10, 64)
	if err != nil || keyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid key id"})
		return
	}

	svc := services.NewAPIClientService(nil)
	if err := svc.RevokeKey(id, keyID); err != nil {
		if errors.Is(err, services.ErrAPIClientNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "key not found for this client"})
			return
		}
		InternalError(c, "api_clients", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GET /api/v1/admin/api-clients/:id/logs?limit=100
func AdminListAPIClientLogs(c *gin.Context) {
	id, ok := parseClientIDParam(c)
	if !ok {
		return
	}
	limit := parseIntOrDefault(c.Query("limit"), 100)

	svc := services.NewAPIClientService(nil)
	logs, err := svc.ListRequestLogs(id, limit)
	if err != nil {
		InternalError(c, "api_clients", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": logs})
}
