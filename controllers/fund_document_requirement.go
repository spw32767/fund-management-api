// controllers/fund_document_requirement.go
package controllers

import (
	"fund-management-api/config"
	"fund-management-api/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetFundDocumentRequirements ดึงรายการเอกสารที่ต้องใช้สำหรับทุนประเภทต่างๆ
// GET /api/fund-document-requirements
func GetFundDocumentRequirements(c *gin.Context) {
	var query models.DocumentRequirementQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build query
	db := config.DB.Table("v_fund_document_requirements")

	// Apply filters
	if query.FundType != "" {
		db = db.Where("fund_type = ?", query.FundType)
	}

	if query.SubcategoryID != nil {
		db = db.Where("subcategory_id = ? OR subcategory_id IS NULL", *query.SubcategoryID)
	}

	if query.IsRequired != nil {
		db = db.Where("is_required = ?", *query.IsRequired)
	}

	if query.IsActive != nil {
		db = db.Where("is_active = ?", *query.IsActive)
	}

	// Execute query
	var requirements []models.FundDocumentRequirementView
	if err := db.Order("display_order ASC").Find(&requirements).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch document requirements"})
		return
	}

	// Calculate summary
	summary := calculateSummary(requirements)

	// Transform for frontend compatibility
	result := transformForFrontend(requirements)

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"requirements": result,
		"summary":      summary,
		"total":        len(requirements),
	})
}

// GetDocumentTypesLegacy รองรับ API เดิมเพื่อ backward compatibility
// GET /api/document-types
func GetDocumentTypesLegacy(c *gin.Context) {
	category := c.Query("category")
	fundType := c.Query("fund_type")
	subcategoryID := c.Query("subcategory_id")

	// Convert legacy category to fund_type
	if category != "" && fundType == "" {
		fundType = models.ConvertCategoryToFundType(category)
	}

	// Default to fund_application if not specified
	if fundType == "" {
		fundType = models.FundTypeFundApplication
	}

	// Build query for new system
	query := models.DocumentRequirementQuery{
		FundType: fundType,
	}

	if subcategoryID != "" {
		if id, err := strconv.Atoi(subcategoryID); err == nil {
			query.SubcategoryID = &id
		}
	}

	// Get requirements using new system
	db := config.DB.Table("v_fund_document_requirements").
		Where("fund_type = ?", query.FundType)

	if query.SubcategoryID != nil {
		db = db.Where("subcategory_id = ? OR subcategory_id IS NULL", *query.SubcategoryID)
	}

	var requirements []models.FundDocumentRequirementView
	if err := db.Order("display_order ASC").Find(&requirements).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch document types"})
		return
	}

	// Transform to legacy format
	var result []map[string]interface{}
	for _, req := range requirements {
		result = append(result, map[string]interface{}{
			"id":                 req.DocumentTypeID,
			"document_type_id":   req.DocumentTypeID,
			"code":               req.DocumentCode,
			"name":               req.DocumentTypeName,
			"document_type_name": req.DocumentTypeName,
			"required":           req.IsRequired,
			"is_required":        req.IsRequired,
			"multiple":           false, // TODO: get from document_types if needed
			"category":           req.DocumentCategory,
			"display_order":      req.DisplayOrder,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"document_types": result,
		"success":        true,
	})
}

// CreateFundDocumentRequirement สร้าง requirement ใหม่ (Admin only)
// POST /api/admin/fund-document-requirements
func CreateFundDocumentRequirement(c *gin.Context) {
	var req models.CreateFundDocumentRequirementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate fund type
	if !models.IsFundTypeValid(req.FundType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid fund type"})
		return
	}

	// Check if document type exists
	var documentType models.DocumentType
	if err := config.DB.First(&documentType, req.DocumentTypeID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type not found"})
		return
	}

	// Check if subcategory exists (if specified)
	if req.SubcategoryID != nil {
		var subcategory models.FundSubcategory
		if err := config.DB.First(&subcategory, *req.SubcategoryID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Subcategory not found"})
			return
		}
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid condition rules"})
			return
		}
	}

	if err := config.DB.Create(&requirement).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create requirement"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":     true,
		"message":     "Document requirement created successfully",
		"requirement": requirement,
	})
}

// UpdateFundDocumentRequirement อัพเดท requirement (Admin only)
// PUT /api/admin/fund-document-requirements/:id
func UpdateFundDocumentRequirement(c *gin.Context) {
	id := c.Param("id")

	var req models.UpdateFundDocumentRequirementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find existing requirement
	var requirement models.FundDocumentRequirement
	if err := config.DB.First(&requirement, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Requirement not found"})
		return
	}

	// Update fields
	if req.FundType != nil {
		if !models.IsFundTypeValid(*req.FundType) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid fund type"})
			return
		}
		requirement.FundType = *req.FundType
	}

	if req.SubcategoryID != nil {
		requirement.SubcategoryID = req.SubcategoryID
	}

	if req.DocumentTypeID != nil {
		requirement.DocumentTypeID = *req.DocumentTypeID
	}

	if req.IsRequired != nil {
		requirement.IsRequired = *req.IsRequired
	}

	if req.DisplayOrder != nil {
		requirement.DisplayOrder = *req.DisplayOrder
	}

	if req.IsActive != nil {
		requirement.IsActive = *req.IsActive
	}

	if req.ConditionRules != nil {
		if err := requirement.SetConditionRules(req.ConditionRules); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid condition rules"})
			return
		}
	}

	if err := config.DB.Save(&requirement).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update requirement"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "Document requirement updated successfully",
		"requirement": requirement,
	})
}

// DeleteFundDocumentRequirement ลบ requirement (Admin only)
// DELETE /api/admin/fund-document-requirements/:id
func DeleteFundDocumentRequirement(c *gin.Context) {
	id := c.Param("id")

	var requirement models.FundDocumentRequirement
	if err := config.DB.First(&requirement, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Requirement not found"})
		return
	}

	if err := config.DB.Delete(&requirement).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete requirement"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Document requirement deleted successfully",
	})
}

// GetFundDocumentRequirementsAdmin ดึงข้อมูลทั้งหมดสำหรับ Admin
// GET /api/admin/fund-document-requirements
func GetFundDocumentRequirementsAdmin(c *gin.Context) {
	var requirements []models.FundDocumentRequirementView
	if err := config.DB.Table("v_fund_document_requirements").
		Order("fund_type, subcategory_id, display_order").
		Find(&requirements).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch requirements"})
		return
	}

	// Group by fund type for easier management
	grouped := make(map[string][]models.FundDocumentRequirementView)
	for _, req := range requirements {
		grouped[req.FundType] = append(grouped[req.FundType], req)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"requirements": requirements,
		"grouped":      grouped,
		"total":        len(requirements),
	})
}

// ValidateDocumentRequirements ตรวจสอบความถูกต้องของเอกสารสำหรับ submission
// POST /api/fund-document-requirements/validate
func ValidateDocumentRequirements(c *gin.Context) {
	var request struct {
		FundType      string                 `json:"fund_type" binding:"required"`
		SubcategoryID *int                   `json:"subcategory_id"`
		Documents     []int                  `json:"documents"` // document_type_ids ที่มี
		Params        map[string]interface{} `json:"params"`    // parameters สำหรับ condition validation
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get required documents
	db := config.DB.Table("v_fund_document_requirements").
		Where("fund_type = ? AND is_required = ?", request.FundType, true)

	if request.SubcategoryID != nil {
		db = db.Where("subcategory_id = ? OR subcategory_id IS NULL", *request.SubcategoryID)
	}

	var requiredDocs []models.FundDocumentRequirementView
	if err := db.Find(&requiredDocs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get requirements"})
		return
	}

	// Check validation
	var missing []models.FundDocumentRequirementView
	var valid = true

	for _, req := range requiredDocs {
		// Check if document is provided
		found := false
		for _, docID := range request.Documents {
			if docID == req.DocumentTypeID {
				found = true
				break
			}
		}

		if !found {
			// Check condition rules
			requirement := models.FundDocumentRequirement{
				ConditionRules: req.ConditionRules,
			}

			if requirement.ValidateCondition(request.Params) {
				missing = append(missing, req)
				valid = false
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"valid":   valid,
		"missing": missing,
		"summary": map[string]interface{}{
			"required_count": len(requiredDocs),
			"provided_count": len(request.Documents),
			"missing_count":  len(missing),
		},
	})
}

// Helper functions

func calculateSummary(requirements []models.FundDocumentRequirementView) models.RequirementsSummary {
	summary := models.RequirementsSummary{
		TotalDocuments: len(requirements),
	}

	for _, req := range requirements {
		if req.IsRequired {
			summary.RequiredDocuments++
		} else {
			summary.OptionalDocuments++
		}
	}

	return summary
}

func transformForFrontend(requirements []models.FundDocumentRequirementView) []map[string]interface{} {
	var result []map[string]interface{}

	for _, req := range requirements {
		result = append(result, map[string]interface{}{
			"requirement_id":     req.RequirementID,
			"fund_type":          req.FundType,
			"subcategory_id":     req.SubcategoryID,
			"subcategory_name":   req.SubcategoryName,
			"document_type_id":   req.DocumentTypeID,
			"document_type_name": req.DocumentTypeName,
			"document_code":      req.DocumentCode,
			"document_category":  req.DocumentCategory,
			"is_required":        req.IsRequired,
			"display_order":      req.DisplayOrder,
			"condition_rules":    req.ConditionRules,
			"is_active":          req.IsActive,

			// Legacy compatibility fields
			"id":       req.DocumentTypeID,
			"code":     req.DocumentCode,
			"name":     req.DocumentTypeName,
			"required": req.IsRequired,
			"multiple": false,
			"category": req.DocumentCategory,
		})
	}

	return result
}
