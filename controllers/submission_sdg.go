package controllers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetActiveSDGs(c *gin.Context) {
	var sdgs []models.SDG
	if err := config.DB.Where("delete_at IS NULL").Order("sdg_number ASC").Find(&sdgs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch SDGs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "sdgs": sdgs})
}

func loadSubmissionSDGs(submissionID int) ([]models.SubmissionSDG, error) {
	var items []models.SubmissionSDG
	err := config.DB.Where("submission_id = ?", submissionID).Order("sdg_number_snapshot ASC").Find(&items).Error
	return items, err
}

func GetSubmissionSDGs(c *gin.Context) {
	submissionID, err := strconv.Atoi(c.Param("id"))
	if err != nil || submissionID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission id"})
		return
	}
	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)
	roleValue, isAdmin := c.Get("roleID")
	if roleID, ok := roleValue.(int); !isAdmin || !ok || roleID != 3 {
		query = query.Where("user_id = ?", c.MustGet("userID"))
	}
	if err := query.Preload("Status").First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}
	items, err := loadSubmissionSDGs(submissionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submission SDGs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "sdgs": items})
}

func UpdateSubmissionSDGs(c *gin.Context) {
	submissionID, err := strconv.Atoi(c.Param("id"))
	if err != nil || submissionID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid submission id"})
		return
	}
	userID, _ := c.Get("userID")
	roleID, _ := c.Get("roleID")

	var req struct {
		SDGIDs []int `json:"sdg_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SDGIDs == nil {
		req.SDGIDs = []int{}
	}
	seen := make(map[int]struct{}, len(req.SDGIDs))
	for _, id := range req.SDGIDs {
		if id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid SDG id"})
			return
		}
		if _, ok := seen[id]; ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Duplicate SDG id"})
			return
		}
		seen[id] = struct{}{}
	}

	var submission models.Submission
	query := config.DB.Where("submission_id = ? AND deleted_at IS NULL", submissionID)
	roleValue, isAdmin := roleID.(int)
	if !isAdmin || roleValue != 3 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Preload("Status").First(&submission).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Submission not found"})
		return
	}
	canEdit := submission.IsEditable() || strings.EqualFold(submission.Status.StatusCode, "needs_more_info")
	if (!isAdmin || roleValue != 3) && !canEdit {
		c.JSON(http.StatusConflict, gin.H{"error": "Submission is not editable"})
		return
	}

	var masters []models.SDG
	if len(req.SDGIDs) > 0 {
		if err := config.DB.Where("sdg_id IN ? AND delete_at IS NULL", req.SDGIDs).Find(&masters).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate SDGs"})
			return
		}
		if len(masters) != len(req.SDGIDs) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "One or more SDGs are not available"})
			return
		}
	}
	masterByID := make(map[int]models.SDG, len(masters))
	for _, item := range masters {
		masterByID[item.SDGID] = item
	}

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("submission_id = ?", submissionID).Delete(&models.SubmissionSDG{}).Error; err != nil {
			return err
		}
		now := time.Now()
		for _, id := range req.SDGIDs {
			master := masterByID[id]
			item := models.SubmissionSDG{
				SubmissionID: submissionID, SDGID: master.SDGID, SDGNumberSnapshot: master.SDGNumber,
				NameTHSnapshot: master.NameTH, NameENSnapshot: master.NameEN,
				DescriptionTHSnapshot: master.DescriptionTH, DescriptionENSnapshot: master.DescriptionEN,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save submission SDGs"})
		return
	}
	items, err := loadSubmissionSDGs(submissionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reload submission SDGs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "sdgs": items})
}
