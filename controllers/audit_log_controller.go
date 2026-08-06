package controllers

import (
	"net/http"
	"strconv"
	"fund-management-api/config"
	"fund-management-api/services"
	"github.com/gin-gonic/gin"
)

// GET /admin/audit-logs
// Query params (ทั้งหมด optional):
//   ?action=INSERT|UPDATE|DELETE
//   ?table=instructor_expertises
//   ?editor_id=15
//   ?target_user_id=1019
func GetAuditLogs(c *gin.Context) {
	filter := services.AuditLogFilter{
		Action:       c.Query("action"),
		TargetTable:  c.Query("table"),
	}

	if v := c.Query("editor_id"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			filter.UserEditID = id
		}
	}
	if v := c.Query("target_user_id"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			filter.TargetUserID = id
		}
	}

	svc := services.NewAuditLogService(config.DB)
	logs, err := svc.GetLogs(c.Request.Context(), filter)
	if err != nil {
		InternalError(c, "audit_log: list logs", err)
		return
	}
	c.JSON(http.StatusOK, logs)
}

// GET /admin/audit-logs/:id
func GetAuditLogByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID ไม่ถูกต้อง"})
		return
	}

	svc := services.NewAuditLogService(config.DB)
	log, err := svc.GetLogByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบ log ID " + c.Param("id")})
		return
	}
	c.JSON(http.StatusOK, log)
}

// GET /admin/audit-logs/tables
// คืน list ของ target_table ที่ไม่ซ้ำ ใช้สำหรับ dropdown filter
func GetAuditLogTables(c *gin.Context) {
	svc := services.NewAuditLogService(config.DB)
	tables, err := svc.GetDistinctTables(c.Request.Context())
	if err != nil {
		InternalError(c, "audit_log: list tables", err)
		return
	}
	c.JSON(http.StatusOK, tables)
}