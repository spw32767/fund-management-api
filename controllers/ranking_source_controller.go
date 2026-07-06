package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GET /admin/ranking-sources
func GetRankingSources(c *gin.Context) {
	sourceService := services.NewRankingSourceService(config.DB)

	sources, err := sourceService.GetSources(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถดึงข้อมูลแหล่งที่มาได้"})
		return
	}
	c.JSON(http.StatusOK, sources)
}

// PUT/POST /admin/ranking-sources
func UpdateRankingSources(c *gin.Context) {
	editorID, ok := mustGetEditorID(c)
if !ok {
    return
}

	var body struct {
		Sources []models.RankingSource `json:"sources"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "ข้อมูลไม่ถูกต้อง",
		})
		return
	}

	sourceService := services.NewRankingSourceService(config.DB)

	result, err := sourceService.UpdateSources(
		c.Request.Context(),
		body.Sources,
		editorID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "ปรับปรุงฐานข้อมูลแหล่งที่มาสำเร็จ",
		"data":    result,
	})
}

// DELETE /admin/ranking-sources/:id
func DeleteRankingSource(c *gin.Context) {
	idStr := c.Param("id")

	id64, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID แหล่งข้อมูลไม่ถูกต้อง"})
		return
	}
	id := uint(id64)

	sourceService := services.NewRankingSourceService(config.DB)
	editorID, ok := mustGetEditorID(c)
if !ok {
    return
}
	if err := sourceService.DeleteSource(c.Request.Context(), id, editorID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบข้อมูลแหล่งอ้างอิงได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ลบแหล่งข้อมูลสำเร็จ"})
}