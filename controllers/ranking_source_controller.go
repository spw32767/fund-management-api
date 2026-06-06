package controllers

import (
	"net/http"
	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/services"
	"github.com/gin-gonic/gin"
	"strconv"

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

// PUT /admin/ranking-sources
func UpdateRankingSources(c *gin.Context) {
	var body struct {
		Sources []models.RankingSource `json:"sources"` // ตรงกับก้อน { sources: payload } ของหน้าบ้าน
	}
	
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลไม่ถูกต้อง"})
		return
	}

	sourceService := services.NewRankingSourceService(config.DB)
	result, err := sourceService.UpdateSources(c.Request.Context(), body.Sources)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ปรับปรุงฐานข้อมูลแหล่งที่มาสำเร็จ", "data": result})
}

func DeleteRankingSource(c *gin.Context) {
    //รับค่า idStr มาจาก URL (อันนี้คือบรรทัดที่ 48 ของคุณ)
    idStr := c.Param("id") 
    
    // นำ idStr มาใช้คู่กับ strconv เพื่อแปลงเป็นตัวเลข (ตรงนี้แหละที่จะเคลียร์เออร์เรอร์ทั้งคู่!)
    id64, err := strconv.ParseUint(idStr, 10, 32)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID แหล่งข้อมูลไม่ถูกต้อง"})
        return
    }
    id := uint(id64)

    //เรียกใช้ Service เพื่อสั่งลบข้อมูลใน DB ทันที
    sourceService := services.NewRankingSourceService(config.DB)
    if err := sourceService.DeleteSource(c.Request.Context(), id); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบข้อมูลแหล่งอ้างอิงได้"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "ลบแหล่งข้อมูลสำเร็จ"})
}