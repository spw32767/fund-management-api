// controllers/fund_document_bulk_operations.go
package controllers

import (
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BulkCreateFundDocumentRequirements สร้าง requirements หลายรายการพร้อมกัน
// POST /api/admin/fund-document-requirements/bulk-create
func BulkCreateFundDocumentRequirements(c *gin.Context) {
	var request struct {
		Requirements []models.CreateFundDocumentRequirementRequest `json:"requirements" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Start transaction
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var createdRequirements []models.FundDocumentRequirement
	var errors []string

	for i, req := range request.Requirements {
		// Validate fund type
		if !models.IsFundTypeValid(req.FundType) {
			errors = append(errors, fmt.Sprintf("Invalid fund type at index %d", i))
			continue
		}

		// Create requirement
		requirement := models.FundDocumentRequirement{
			FundType:       req.FundType,
			SubcategoryID:  req.SubcategoryID,
			DocumentTypeID: req.DocumentTypeID,
			IsRequired:     req.IsRequired,
			DisplayOrder:   req.DisplayOrder,
			IsActive:       req.IsActive,
		}

		// Set condition rules if provided
		if req.ConditionRules != nil {
			if err := requirement.SetConditionRules(req.ConditionRules); err != nil {
				errors = append(errors, fmt.Sprintf("Invalid condition rules at index %d: %v", i, err))
				continue
			}
		}

		if err := tx.Create(&requirement).Error; err != nil {
			errors = append(errors, fmt.Sprintf("Failed to create requirement at index %d: %v", i, err))
			continue
		}

		createdRequirements = append(createdRequirements, requirement)
	}

	if len(errors) > 0 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"errors":  errors,
		})
		return
	}

	tx.Commit()

	c.JSON(http.StatusCreated, gin.H{
		"success":      true,
		"message":      fmt.Sprintf("Created %d document requirements successfully", len(createdRequirements)),
		"requirements": createdRequirements,
		"total":        len(createdRequirements),
	})
}

// BulkUpdateFundDocumentRequirements อัพเดท requirements หลายรายการพร้อมกัน
// PUT /api/admin/fund-document-requirements/bulk-update
func BulkUpdateFundDocumentRequirements(c *gin.Context) {
	var request struct {
		Updates []struct {
			RequirementID int                                         `json:"requirement_id" binding:"required"`
			Data          models.UpdateFundDocumentRequirementRequest `json:"data" binding:"required"`
		} `json:"updates" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Start transaction
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var updatedRequirements []models.FundDocumentRequirement
	var errors []string

	for i, update := range request.Updates {
		// Find existing requirement
		var requirement models.FundDocumentRequirement
		if err := tx.First(&requirement, update.RequirementID).Error; err != nil {
			errors = append(errors, fmt.Sprintf("Requirement not found at index %d: ID %d", i, update.RequirementID))
			continue
		}

		// Update fields
		if update.Data.FundType != nil {
			if !models.IsFundTypeValid(*update.Data.FundType) {
				errors = append(errors, fmt.Sprintf("Invalid fund type at index %d", i))
				continue
			}
			requirement.FundType = *update.Data.FundType
		}

		if update.Data.SubcategoryID != nil {
			requirement.SubcategoryID = update.Data.SubcategoryID
		}

		if update.Data.DocumentTypeID != nil {
			requirement.DocumentTypeID = *update.Data.DocumentTypeID
		}

		if update.Data.IsRequired != nil {
			requirement.IsRequired = *update.Data.IsRequired
		}

		if update.Data.DisplayOrder != nil {
			requirement.DisplayOrder = *update.Data.DisplayOrder
		}

		if update.Data.IsActive != nil {
			requirement.IsActive = *update.Data.IsActive
		}

		if update.Data.ConditionRules != nil {
			if err := requirement.SetConditionRules(update.Data.ConditionRules); err != nil {
				errors = append(errors, fmt.Sprintf("Invalid condition rules at index %d: %v", i, err))
				continue
			}
		}

		if err := tx.Save(&requirement).Error; err != nil {
			errors = append(errors, fmt.Sprintf("Failed to update requirement at index %d: %v", i, err))
			continue
		}

		updatedRequirements = append(updatedRequirements, requirement)
	}

	if len(errors) > 0 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"errors":  errors,
		})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      fmt.Sprintf("Updated %d document requirements successfully", len(updatedRequirements)),
		"requirements": updatedRequirements,
		"total":        len(updatedRequirements),
	})
}

// CopyRequirementsToSubcategory คัดลอก requirements จาก subcategory หนึ่งไปอีกอัน
// POST /api/admin/fund-document-requirements/copy
func CopyRequirementsToSubcategory(c *gin.Context) {
	var request struct {
		SourceSubcategoryID *int   `json:"source_subcategory_id"` // nil = copy จาก general requirements
		TargetSubcategoryID int    `json:"target_subcategory_id" binding:"required"`
		FundType            string `json:"fund_type" binding:"required"`
		OverwriteExisting   bool   `json:"overwrite_existing"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate fund type
	if !models.IsFundTypeValid(request.FundType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid fund type"})
		return
	}

	// Check if target subcategory exists
	var targetSubcategory models.FundSubcategory
	if err := config.DB.First(&targetSubcategory, request.TargetSubcategoryID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Target subcategory not found"})
		return
	}

	// Get source requirements
	query := config.DB.Where("fund_type = ?", request.FundType)
	if request.SourceSubcategoryID != nil {
		query = query.Where("subcategory_id = ?", *request.SourceSubcategoryID)
	} else {
		query = query.Where("subcategory_id IS NULL")
	}

	var sourceRequirements []models.FundDocumentRequirement
	if err := query.Find(&sourceRequirements).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch source requirements"})
		return
	}

	if len(sourceRequirements) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No source requirements found"})
		return
	}

	// Start transaction
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Delete existing requirements if overwrite is enabled
	if request.OverwriteExisting {
		if err := tx.Where("fund_type = ? AND subcategory_id = ?", request.FundType, request.TargetSubcategoryID).
			Delete(&models.FundDocumentRequirement{}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete existing requirements"})
			return
		}
	}

	var copiedRequirements []models.FundDocumentRequirement
	var skipped []int

	for _, source := range sourceRequirements {
		// Check if requirement already exists
		var existingCount int64
		tx.Model(&models.FundDocumentRequirement{}).
			Where("fund_type = ? AND subcategory_id = ? AND document_type_id = ?",
				request.FundType, request.TargetSubcategoryID, source.DocumentTypeID).
			Count(&existingCount)

		if existingCount > 0 && !request.OverwriteExisting {
			skipped = append(skipped, source.DocumentTypeID)
			continue
		}

		// Create new requirement
		newRequirement := models.FundDocumentRequirement{
			FundType:       source.FundType,
			SubcategoryID:  &request.TargetSubcategoryID,
			DocumentTypeID: source.DocumentTypeID,
			IsRequired:     source.IsRequired,
			DisplayOrder:   source.DisplayOrder,
			ConditionRules: source.ConditionRules,
			IsActive:       source.IsActive,
		}

		if err := tx.Create(&newRequirement).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to create requirement for document type %d", source.DocumentTypeID),
			})
			return
		}

		copiedRequirements = append(copiedRequirements, newRequirement)
	}

	tx.Commit()

	c.JSON(http.StatusCreated, gin.H{
		"success":      true,
		"message":      fmt.Sprintf("Copied %d requirements to subcategory %d", len(copiedRequirements), request.TargetSubcategoryID),
		"copied":       len(copiedRequirements),
		"skipped":      len(skipped),
		"skipped_ids":  skipped,
		"requirements": copiedRequirements,
	})
}

// GetRequirementsSummary ดึงสรุปข้อมูล requirements แยกตาม fund type และ subcategory
// GET /api/admin/fund-document-requirements/summary
func GetRequirementsSummary(c *gin.Context) {
	type SummaryData struct {
		FundType        string  `json:"fund_type"`
		SubcategoryID   *int    `json:"subcategory_id"`
		SubcategoryName *string `json:"subcategory_name"`
		TotalDocuments  int     `json:"total_documents"`
		RequiredDocs    int     `json:"required_docs"`
		OptionalDocs    int     `json:"optional_docs"`
	}

	var summaries []SummaryData

	query := `
	SELECT 
		fdr.fund_type,
		fdr.subcategory_id,
		fs.subcategory_name,
		COUNT(*) as total_documents,
		SUM(CASE WHEN fdr.is_required = 1 THEN 1 ELSE 0 END) as required_docs,
		SUM(CASE WHEN fdr.is_required = 0 THEN 1 ELSE 0 END) as optional_docs
	FROM fund_document_requirements fdr
	LEFT JOIN fund_subcategories fs ON fdr.subcategory_id = fs.subcategory_id
	WHERE fdr.is_active = 1
	GROUP BY fdr.fund_type, fdr.subcategory_id, fs.subcategory_name
	ORDER BY fdr.fund_type, fdr.subcategory_id`

	if err := config.DB.Raw(query).Scan(&summaries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get summary"})
		return
	}

	// Group by fund type
	grouped := make(map[string][]SummaryData)
	totalsByFundType := make(map[string]struct {
		Total    int `json:"total"`
		Required int `json:"required"`
		Optional int `json:"optional"`
	})

	for _, summary := range summaries {
		grouped[summary.FundType] = append(grouped[summary.FundType], summary)

		totals := totalsByFundType[summary.FundType]
		totals.Total += summary.TotalDocuments
		totals.Required += summary.RequiredDocs
		totals.Optional += summary.OptionalDocs
		totalsByFundType[summary.FundType] = totals
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"summaries": summaries,
		"grouped":   grouped,
		"totals":    totalsByFundType,
	})
}
