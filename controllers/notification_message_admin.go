package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"fund-management-api/config"
	"fund-management-api/models"
)

type notificationMessageRequest struct {
	EventKey      string   `json:"event_key" binding:"required"`
	SendTo        string   `json:"send_to" binding:"required"`
	TitleTemplate string   `json:"title_template" binding:"required"`
	BodyTemplate  string   `json:"body_template" binding:"required"`
	Description   *string  `json:"description"`
	Variables     []string `json:"variables"`
	IsActive      *bool    `json:"is_active"`
}

func normalizeAudience(value string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "user", "dept_head", "admin":
		return v, nil
	default:
		return "", errors.New("invalid send_to; must be user, dept_head, or admin")
	}
}

func buildVariablesJSON(values []string) json.RawMessage {
	if len(values) == 0 {
		return json.RawMessage("[]")
	}

	cleaned := make([]string, 0, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	data, err := json.Marshal(cleaned)
	if err != nil {
		return json.RawMessage("[]")
	}
	return data
}

// ListNotificationMessages - GET /api/v1/admin/notification-messages
func ListNotificationMessages(c *gin.Context) {
	db := config.DB

	var messages []models.NotificationMessage

	q := db.Model(&models.NotificationMessage{})
	if eventKey := strings.TrimSpace(c.Query("event_key")); eventKey != "" {
		q = q.Where("event_key = ?", eventKey)
	}
	if sendTo := strings.TrimSpace(c.Query("send_to")); sendTo != "" {
		q = q.Where("send_to = ?", sendTo)
	}
	if isActive := strings.TrimSpace(c.Query("is_active")); isActive != "" {
		if isActive == "true" || isActive == "1" {
			q = q.Where("is_active = 1")
		} else if isActive == "false" || isActive == "0" {
			q = q.Where("is_active = 0")
		}
	}

	if err := q.Order("event_key, send_to").Find(&messages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to list notification messages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"items":   messages,
		"total":   len(messages),
	})
}

// CreateNotificationMessage - POST /api/v1/admin/notification-messages
func CreateNotificationMessage(c *gin.Context) {
	var req notificationMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	audience, err := normalizeAudience(req.SendTo)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	now := time.Now()
	msg := models.NotificationMessage{
		EventKey:      strings.TrimSpace(req.EventKey),
		SendTo:        audience,
		TitleTemplate: strings.TrimSpace(req.TitleTemplate),
		BodyTemplate:  strings.TrimSpace(req.BodyTemplate),
		DefaultTitle:  strings.TrimSpace(req.TitleTemplate),
		DefaultBody:   strings.TrimSpace(req.BodyTemplate),
		Description:   req.Description,
		Variables:     buildVariablesJSON(req.Variables),
		DefaultVars:   buildVariablesJSON(req.Variables),
		IsActive:      true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if req.IsActive != nil {
		msg.IsActive = *req.IsActive
	}

	if userID, ok := c.Get("userID"); ok {
		switch v := userID.(type) {
		case int:
			uid := uint(v)
			msg.UpdatedBy = &uid
		case int64:
			uid := uint(v)
			msg.UpdatedBy = &uid
		case uint:
			uid := v
			msg.UpdatedBy = &uid
		}
	}

	if err := config.DB.Create(&msg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to create notification message"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "notification_message": msg})
}

// UpdateNotificationMessage - PUT /api/v1/admin/notification-messages/:id
func UpdateNotificationMessage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	var req notificationMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	audience, err := normalizeAudience(req.SendTo)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	var msg models.NotificationMessage
	if err := config.DB.First(&msg, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "notification message not found"})
		return
	}

	updates := map[string]interface{}{
		"event_key":      strings.TrimSpace(req.EventKey),
		"send_to":        audience,
		"title_template": strings.TrimSpace(req.TitleTemplate),
		"body_template":  strings.TrimSpace(req.BodyTemplate),
		"description":    req.Description,
		"variables":      buildVariablesJSON(req.Variables),
		"updated_at":     time.Now(),
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if userID, ok := c.Get("userID"); ok {
		switch v := userID.(type) {
		case int:
			uid := uint(v)
			updates["updated_by"] = uid
		case int64:
			uid := uint(v)
			updates["updated_by"] = uid
		case uint:
			updates["updated_by"] = v
		}
	}

	if err := config.DB.Model(&msg).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to update notification message"})
		return
	}

	if err := config.DB.First(&msg, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to reload notification message"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "notification_message": msg})
}

// ResetNotificationMessage - POST /api/v1/admin/notification-messages/:id/reset
func ResetNotificationMessage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	var msg models.NotificationMessage
	if err := config.DB.First(&msg, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "notification message not found"})
		return
	}

	defaultTitle := strings.TrimSpace(msg.DefaultTitle)
	if defaultTitle == "" {
		defaultTitle = msg.TitleTemplate
	}

	defaultBody := strings.TrimSpace(msg.DefaultBody)
	if defaultBody == "" {
		defaultBody = msg.BodyTemplate
	}

	defaultVars := msg.DefaultVars
	if len(strings.TrimSpace(string(defaultVars))) == 0 {
		defaultVars = msg.Variables
	}

	updates := map[string]interface{}{
		"title_template": defaultTitle,
		"body_template":  defaultBody,
		"variables":      defaultVars,
		"updated_at":     time.Now(),
		"is_active":      true,
	}

	if userID, ok := c.Get("userID"); ok {
		switch v := userID.(type) {
		case int:
			uid := uint(v)
			updates["updated_by"] = uid
		case int64:
			uid := uint(v)
			updates["updated_by"] = uid
		case uint:
			updates["updated_by"] = v
		}
	}

	if err := config.DB.Model(&msg).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to reset notification message"})
		return
	}

	if err := config.DB.First(&msg, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to reload notification message"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "notification_message": msg})
}