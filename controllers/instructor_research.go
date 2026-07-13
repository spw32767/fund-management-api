package controllers

import (
	"fund-management-api/services"
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
)

type ResearchController struct {
	service services.ResearchService
}


func NewResearchController(service services.ResearchService) *ResearchController {
	return &ResearchController{service: service}
}

// ตัวนี้ต้องทำหน้าที่รับ ID ของอาจารย์ แล้วส่งให้ Service ไปค้นตาราง Scopus/ThaiJO เท่านั้น
func (c *ResearchController) GetResearchDocuments(ctx *gin.Context) {
	// ดึง id ของอาจารย์จาก URL parameter
	userID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Invalid instructor ID"})
		return
	}
	
	//ไปเรียกใช้ service ตัวใหม่ที่เราแก้ปัญหา mismatch types ไปเมื่อกี้โดยตรง
	docs, err := c.service.GetResearchByUserID(ctx.Request.Context(), uint(userID))
	if err != nil {
		InternalError(ctx, "instructor_research", err)
		return
	}
	
	// ส่งข้อมูลประวัติงานวิจัยกลับไปหา React ทันที
	ctx.JSON(http.StatusOK, docs)
}