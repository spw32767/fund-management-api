package controllers

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GET /admin/ranking-weights
func GetRankingWeights(c *gin.Context) {
	weightService := services.NewRankingWeightService(config.DB)

	weights, err := weightService.GetWeights(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถดึงข้อมูลค่าน้ำหนักได้"})
		return
	}
	c.JSON(http.StatusOK, weights)
}

// PUT/POST /admin/ranking-weights
func UpdateRankingWeights(c *gin.Context) {
	editorID, ok := mustGetEditorID(c)
    if !ok {
        return
    }
	var body struct {
		Weights []models.RankingTierWeight `json:"weights"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลไม่ถูกต้อง"})
		return
	}

	weightService := services.NewRankingWeightService(config.DB)
	result, err := weightService.UpdateWeights(c.Request.Context(), body.Weights, editorID)
	if err != nil {
		InternalError(c, "ranking_weight", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "บันทึกสำเร็จ", "data": result})
}

// DELETE /admin/ranking-weights/:id
func DeleteRankingWeight(c *gin.Context) {
	idStr := c.Param("id")

	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID ไม่ถูกต้อง"})
		return
	}

	editorID, ok := mustGetEditorID(c)
	if !ok {
		return
	}
	weightService := services.NewRankingWeightService(config.DB)
	if err := weightService.DeleteWeight(c.Request.Context(), id, editorID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบข้อมูลค่าน้ำหนักได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ลบเกณฑ์ค่าน้ำหนักสำเร็จ"})
}