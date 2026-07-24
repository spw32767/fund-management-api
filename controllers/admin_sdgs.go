package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func requireAdminForSDG(c *gin.Context) bool {
	roleID, exists := c.Get("roleID")
	if !exists || roleID.(int) != 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return false
	}
	return true
}

func GetAdminSDGs(c *gin.Context) {
	if !requireAdminForSDG(c) {
		return
	}

	var sdgs []models.SDG
	if err := config.DB.Where("delete_at IS NULL").Order("sdg_number ASC").Find(&sdgs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch SDGs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "sdgs": sdgs, "total": len(sdgs)})
}

type sdgRequest struct {
	SDGNumber     int    `json:"sdg_number" binding:"required,min=1,max=17"`
	NameTH        string `json:"name_th" binding:"required"`
	NameEN        string `json:"name_en" binding:"required"`
	DescriptionTH string `json:"description_th"`
	DescriptionEN string `json:"description_en"`
}

func validateSDGRequest(req *sdgRequest) error {
	req.NameTH = strings.TrimSpace(req.NameTH)
	req.NameEN = strings.TrimSpace(req.NameEN)
	if req.NameTH == "" || req.NameEN == "" {
		return errors.New("name_th and name_en are required")
	}
	return nil
}

func CreateAdminSDG(c *gin.Context) {
	if !requireAdminForSDG(c) {
		return
	}
	var req sdgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateSDGRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var existing models.SDG
	if err := config.DB.Where("sdg_number = ?", req.SDGNumber).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "SDG number already exists"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check SDG number"})
		return
	}

	now := time.Now()
	sdg := models.SDG{SDGNumber: req.SDGNumber, NameTH: req.NameTH, NameEN: req.NameEN, DescriptionTH: &req.DescriptionTH, DescriptionEN: &req.DescriptionEN, CreateAt: now, UpdateAt: now}
	if err := config.DB.Create(&sdg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create SDG"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "sdg": sdg})
}

func UpdateAdminSDG(c *gin.Context) {
	if !requireAdminForSDG(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid SDG id"})
		return
	}
	var req sdgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateSDGRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var sdg models.SDG
	if err := config.DB.Where("sdg_id = ? AND delete_at IS NULL", id).First(&sdg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SDG not found"})
		return
	}
	var duplicate models.SDG
	if err := config.DB.Where("sdg_number = ? AND sdg_id != ? AND delete_at IS NULL", req.SDGNumber, id).First(&duplicate).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "SDG number already exists"})
		return
	}
	now := time.Now()
	updates := map[string]interface{}{"sdg_number": req.SDGNumber, "name_th": req.NameTH, "name_en": req.NameEN, "description_th": req.DescriptionTH, "description_en": req.DescriptionEN, "update_at": now}
	if err := config.DB.Model(&sdg).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update SDG"})
		return
	}
	config.DB.First(&sdg, id)
	c.JSON(http.StatusOK, gin.H{"success": true, "sdg": sdg})
}
