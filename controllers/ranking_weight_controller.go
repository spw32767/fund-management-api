package controllers

import (
    "net/http"
    "fund-management-api/config"
    "fund-management-api/services"
	"fund-management-api/models"
    "github.com/gin-gonic/gin"
	"fmt"
)

func GetRankingWeights(c *gin.Context) {
    // เรียกใช้ Service Pattern เดียวกับอาจารย์
    weightService := services.NewRankingWeightService(config.DB)
    
    weights, err := weightService.GetWeights(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถดึงข้อมูลค่าน้ำหนักได้"})
        return
    }
    c.JSON(http.StatusOK, weights)
}

func UpdateRankingWeights(c *gin.Context) {
    var body struct {
        Weights []models.RankingTierWeight `json:"weights"`
    }
    if err := c.ShouldBindJSON(&body); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลไม่ถูกต้อง"})
        return
    }

    weightService := services.NewRankingWeightService(config.DB)
    result, err := weightService.UpdateWeights(c.Request.Context(), body.Weights)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "บันทึกสำเร็จ", "data": result})
}

// DELETE /admin/ranking-weights/:id
func DeleteRankingWeight(c *gin.Context) {
    idStr := c.Param("id")
    
    // แปลง string ID จาก URL เป็น uint (หรือชนิดข้อมูลที่โมเดลใช้)
    var id uint
    if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID ไม่ถูกต้อง"})
        return
    }

    weightService := services.NewRankingWeightService(config.DB)
    if err := weightService.DeleteWeight(c.Request.Context(), id); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบข้อมูลค่าน้ำหนักได้"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "ลบเกณฑ์ค่าน้ำหนักสำเร็จ"})
}