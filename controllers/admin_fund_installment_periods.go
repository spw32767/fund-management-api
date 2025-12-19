package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var activeInstallmentStatuses = []string{"active", "enabled", "open", "current"}

type fundSelection struct {
	Level         string
	Keyword       string
	ParentKeyword *string
}

var allowedFundLevels = map[string]struct{}{
	"category":    {},
	"subcategory": {},
}

var legacyFundSelections = map[string]fundSelection{
	"main_support": {
		Level:   "category",
		Keyword: "ทุนอุดหนุนกิจกรรม",
	},
	"main_promotion": {
		Level:   "category",
		Keyword: "ทุนส่งเสริมการวิจัย",
	},
	"international_presentation": {
		Level:         "subcategory",
		Keyword:       "ทุนนำเสนอต่างประเทศ",
		ParentKeyword: ptrString("ทุนส่งเสริมการวิจัย"),
	},
}

type adminFundInstallmentPeriodResponse struct {
	InstallmentPeriodID int     `json:"installment_period_id"`
	FundLevel           *string `json:"fund_level,omitempty"`
	FundKeyword         *string `json:"fund_keyword,omitempty"`
	FundParentKeyword   *string `json:"fund_parent_keyword,omitempty"`
	FundType            *string `json:"fund_type,omitempty"`
	YearID              int     `json:"year_id"`
	InstallmentNumber   int     `json:"installment_number"`
	CutoffDate          string  `json:"cutoff_date"`
	Name                *string `json:"name,omitempty"`
	Status              string  `json:"status"`
	Remark              *string `json:"remark,omitempty"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
	DeletedAt           *string `json:"deleted_at,omitempty"`
}

type adminFundInstallmentPeriodPaging struct {
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

type adminFundInstallmentPeriodListResponse struct {
	Success bool                                 `json:"success"`
	Data    []adminFundInstallmentPeriodResponse `json:"data"`
	Periods []adminFundInstallmentPeriodResponse `json:"periods"`
	Paging  adminFundInstallmentPeriodPaging     `json:"paging"`
}

type adminFundInstallmentPeriodRequest struct {
	FundType          *string `json:"fund_type"`
	FundLevel         *string `json:"fund_level"`
	FundKeyword       *string `json:"fund_keyword"`
	FundParentKeyword *string `json:"fund_parent_keyword"`
	YearID            *int    `json:"year_id"`
	InstallmentNumber *int    `json:"installment_number"`
	CutoffDate        *string `json:"cutoff_date"`
	Name              *string `json:"name"`
	Status            *string `json:"status"`
	Remark            *string `json:"remark"`
}

func AdminListFundInstallmentPeriods(c *gin.Context) {
	yearIDParam := strings.TrimSpace(c.Query("year_id"))
	if yearIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "year_id is required",
		})
		return
	}

	fundTypeQuery := strings.TrimSpace(c.Query("fund_type"))
	fundLevelQuery := strings.TrimSpace(c.Query("fund_level"))
	fundKeywordQuery := strings.TrimSpace(c.Query("fund_keyword"))
	fundParentKeywordQuery := strings.TrimSpace(c.Query("fund_parent_keyword"))
	selection, fundErr := normalizeFundSelection(
		stringPtrIfNotEmpty(fundTypeQuery),
		stringPtrIfNotEmpty(fundLevelQuery),
		stringPtrIfNotEmpty(fundKeywordQuery),
		stringPtrIfNotEmpty(fundParentKeywordQuery),
	)
	if fundErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fundErr.Error(),
		})
		return
	}
	if selection == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "fund_level and fund_keyword are required",
		})
		return
	}

	yearID, err := strconv.Atoi(yearIDParam)
	if err != nil || yearID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid year_id",
		})
		return
	}

	statusParam := strings.ToLower(strings.TrimSpace(c.DefaultQuery("status", "active")))
	includeDeleted := parseBoolQuery(c.DefaultQuery("include_deleted", "false"))

	limit := parseLimit(c.DefaultQuery("limit", "50"))
	offset := parseOffset(c.DefaultQuery("offset", "0"))

	baseQuery := config.DB.Model(&models.FundInstallmentPeriod{}).
		Where("year_id = ?", yearID)

	if selection != nil {
		baseQuery = baseQuery.Where("fund_level = ? AND fund_keyword = ?", selection.Level, selection.Keyword)
	}

	if !includeDeleted {
		baseQuery = baseQuery.Where("deleted_at IS NULL")
	}

	switch statusParam {
	case "active":
		baseQuery = baseQuery.Where("(status IS NULL OR LOWER(TRIM(status)) IN ?)", activeInstallmentStatuses)
	case "inactive":
		baseQuery = baseQuery.Where("status IS NOT NULL AND LOWER(TRIM(status)) NOT IN ?", activeInstallmentStatuses)
	case "all":
	default:
		if statusParam != "active" && statusParam != "inactive" && statusParam != "all" {
			baseQuery = baseQuery.Where("(status IS NULL OR LOWER(TRIM(status)) IN ?)", activeInstallmentStatuses)
		}
	}

	countQuery := baseQuery.Session(&gorm.Session{})

	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to fetch fund installment periods",
		})
		return
	}

	listQuery := baseQuery.Order("installment_number ASC, cutoff_date ASC")
	if limit > 0 {
		listQuery = listQuery.Limit(limit).Offset(offset)
	}

	var periods []models.FundInstallmentPeriod
	if err := listQuery.Find(&periods).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to fetch fund installment periods",
		})
		return
	}

	responses := make([]adminFundInstallmentPeriodResponse, 0, len(periods))
	for _, period := range periods {
		responses = append(responses, newAdminFundInstallmentPeriodResponse(period))
	}

	paging := adminFundInstallmentPeriodPaging{
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}

	c.JSON(http.StatusOK, adminFundInstallmentPeriodListResponse{
		Success: true,
		Data:    responses,
		Periods: responses,
		Paging:  paging,
	})
}

func AdminCreateFundInstallmentPeriod(c *gin.Context) {
	var req adminFundInstallmentPeriodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	selection, fundErr := normalizeFundSelection(req.FundType, req.FundLevel, req.FundKeyword, req.FundParentKeyword)
	if fundErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": fundErr.Error()})
		return
	}
	if selection == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "fund_level and fund_keyword are required"})
		return
	}

	if req.YearID == nil || *req.YearID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "year_id is required"})
		return
	}

	if req.InstallmentNumber == nil || *req.InstallmentNumber <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "installment_number must be greater than 0"})
		return
	}

	if req.CutoffDate == nil || strings.TrimSpace(*req.CutoffDate) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "cutoff_date is required"})
		return
	}

	cutoffDate, err := time.Parse("2006-01-02", strings.TrimSpace(*req.CutoffDate))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "cutoff_date must be in YYYY-MM-DD format"})
		return
	}

	normalizedStatus, statusErr := normalizeInstallmentStatus(req.Status)
	if statusErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": statusErr.Error()})
		return
	}

	if err := ensureYearExists(*req.YearID); err != nil {
		respondYearLookupError(c, err)
		return
	}

	if err := purgeSoftDeletedInstallmentPeriods(config.DB, *selection, *req.YearID, *req.InstallmentNumber); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to clear deleted installment periods"})
		return
	}

	if conflictErr := checkInstallmentConflicts(0, *selection, *req.YearID, *req.InstallmentNumber, cutoffDate); conflictErr != nil {
		respondConflictError(c, conflictErr)
		return
	}

	period := models.FundInstallmentPeriod{
		FundLevel:         &selection.Level,
		FundKeyword:       &selection.Keyword,
		FundParentKeyword: selection.ParentKeyword,
		YearID:            *req.YearID,
		InstallmentNumber: *req.InstallmentNumber,
		CutoffDate:        cutoffDate,
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			period.Name = nil
		} else {
			period.Name = &name
		}
	}

	if normalizedStatus != nil {
		period.Status = normalizedStatus
	}

	if req.Remark != nil {
		remark := strings.TrimSpace(*req.Remark)
		if remark == "" {
			period.Remark = nil
		} else {
			period.Remark = &remark
		}
	}

	if err := config.DB.Create(&period).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to create fund installment period"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "fund installment period created",
		"period":  newAdminFundInstallmentPeriodResponse(period),
	})
}

func AdminUpdateFundInstallmentPeriod(c *gin.Context) {
	periodIDParam := c.Param("id")
	periodID, err := strconv.Atoi(periodIDParam)
	if err != nil || periodID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid installment period id"})
		return
	}

	var req adminFundInstallmentPeriodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	var period models.FundInstallmentPeriod
	if err := config.DB.Where("installment_period_id = ?", periodID).First(&period).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "fund installment period not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load fund installment period"})
		return
	}

	updates := map[string]interface{}{}
	currentSelection := selectionFromPeriod(period)

	if req.FundType != nil || req.FundLevel != nil || req.FundKeyword != nil || req.FundParentKeyword != nil {
		selection, fundErr := normalizeFundSelection(req.FundType, req.FundLevel, req.FundKeyword, req.FundParentKeyword)
		if fundErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": fundErr.Error()})
			return
		}
		if selection == nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "fund_level and fund_keyword are required"})
			return
		}

		currentSelection = *selection
		updates["fund_level"] = selection.Level
		updates["fund_keyword"] = selection.Keyword
		updates["fund_parent_keyword"] = selection.ParentKeyword
		period.FundLevel = &selection.Level
		period.FundKeyword = &selection.Keyword
		period.FundParentKeyword = selection.ParentKeyword
	}

	if req.YearID != nil && *req.YearID != period.YearID {
		if *req.YearID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "year_id must be greater than 0"})
			return
		}
		if err := ensureYearExists(*req.YearID); err != nil {
			respondYearLookupError(c, err)
			return
		}
		updates["year_id"] = *req.YearID
		period.YearID = *req.YearID
	}

	if req.InstallmentNumber != nil {
		if *req.InstallmentNumber <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "installment_number must be greater than 0"})
			return
		}
		updates["installment_number"] = *req.InstallmentNumber
		period.InstallmentNumber = *req.InstallmentNumber
	}

	if req.CutoffDate != nil {
		trimmed := strings.TrimSpace(*req.CutoffDate)
		if trimmed == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "cutoff_date is required"})
			return
		}
		parsed, parseErr := time.Parse("2006-01-02", trimmed)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "cutoff_date must be in YYYY-MM-DD format"})
			return
		}
		updates["cutoff_date"] = parsed
		period.CutoffDate = parsed
	}

	normalizedStatus, statusErr := normalizeInstallmentStatus(req.Status)
	if statusErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": statusErr.Error()})
		return
	}
	if req.Status != nil {
		if normalizedStatus == nil {
			updates["status"] = nil
			period.Status = nil
		} else {
			updates["status"] = *normalizedStatus
			period.Status = normalizedStatus
		}
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			updates["name"] = nil
			period.Name = nil
		} else {
			updates["name"] = name
			period.Name = &name
		}
	}

	if req.Remark != nil {
		remark := strings.TrimSpace(*req.Remark)
		if remark == "" {
			updates["remark"] = nil
			period.Remark = nil
		} else {
			updates["remark"] = remark
			period.Remark = &remark
		}
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "no changes applied",
			"period":  newAdminFundInstallmentPeriodResponse(period),
		})
		return
	}

	if err := purgeSoftDeletedInstallmentPeriods(config.DB, currentSelection, period.YearID, period.InstallmentNumber); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to clear deleted installment periods"})
		return
	}

	if conflictErr := checkInstallmentConflicts(period.InstallmentPeriodID, currentSelection, period.YearID, period.InstallmentNumber, period.CutoffDate); conflictErr != nil {
		respondConflictError(c, conflictErr)
		return
	}

	if err := config.DB.Model(&models.FundInstallmentPeriod{}).
		Where("installment_period_id = ?", period.InstallmentPeriodID).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to update fund installment period"})
		return
	}

	if err := config.DB.Where("installment_period_id = ?", period.InstallmentPeriodID).First(&period).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to reload fund installment period"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "fund installment period updated",
		"period":  newAdminFundInstallmentPeriodResponse(period),
	})
}

func AdminDeleteFundInstallmentPeriod(c *gin.Context) {
	periodIDParam := c.Param("id")
	periodID, err := strconv.Atoi(periodIDParam)
	if err != nil || periodID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid installment period id"})
		return
	}

	var period models.FundInstallmentPeriod
	if err := config.DB.Where("installment_period_id = ?", periodID).First(&period).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "fund installment period not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load fund installment period"})
		return
	}

	if period.DeletedAt != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "fund installment period already deleted"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"deleted_at": &now,
	}

	if isInstallmentPeriodActive(period.Status) {
		updates["status"] = "inactive"
	}

	if err := config.DB.Model(&models.FundInstallmentPeriod{}).
		Where("installment_period_id = ?", period.InstallmentPeriodID).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to delete fund installment period"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "fund installment period deleted",
	})
}

func AdminRestoreFundInstallmentPeriod(c *gin.Context) {
	periodIDParam := c.Param("id")
	periodID, err := strconv.Atoi(periodIDParam)
	if err != nil || periodID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid installment period id"})
		return
	}

	var period models.FundInstallmentPeriod
	if err := config.DB.Unscoped().Where("installment_period_id = ?", periodID).First(&period).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "fund installment period not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load fund installment period"})
		return
	}

	updates := map[string]interface{}{
		"deleted_at": gorm.Expr("NULL"),
	}

	if !isInstallmentPeriodActive(period.Status) {
		updates["status"] = "active"
	}

	if err := config.DB.Model(&models.FundInstallmentPeriod{}).
		Where("installment_period_id = ?", period.InstallmentPeriodID).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to restore fund installment period"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "fund installment period restored",
	})
}

func AdminCopyFundInstallmentPeriods(c *gin.Context) {
	var req struct {
		SourceYearID int    `json:"source_year_id" binding:"required"`
		TargetYear   string `json:"target_year"`
		TargetYearID *int   `json:"target_year_id"`
		FundType     string `json:"fund_type"`
		FundLevel    string `json:"fund_level"`
		FundKeyword  string `json:"fund_keyword"`
		FundParent   string `json:"fund_parent_keyword"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if req.SourceYearID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "source_year_id must be greater than 0"})
		return
	}

	selection, fundErr := normalizeFundSelection(
		stringPtrIfNotEmpty(req.FundType),
		stringPtrIfNotEmpty(req.FundLevel),
		stringPtrIfNotEmpty(req.FundKeyword),
		stringPtrIfNotEmpty(req.FundParent),
	)
	if fundErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": fundErr.Error()})
		return
	}
	if selection == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "fund_level and fund_keyword are required"})
		return
	}

	targetYearValue := strings.TrimSpace(req.TargetYear)
	if req.TargetYearID == nil && targetYearValue == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "target_year or target_year_id is required"})
		return
	}

	var sourceYear models.Year
	if err := config.DB.Where("year_id = ? AND delete_at IS NULL", req.SourceYearID).
		First(&sourceYear).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "source year not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load source year"})
		return
	}

	usingExistingTarget := false
	var targetYear models.Year

	if req.TargetYearID != nil {
		if err := config.DB.Where("year_id = ? AND delete_at IS NULL", *req.TargetYearID).
			First(&targetYear).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "target year not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load target year"})
			return
		}
		usingExistingTarget = true
		if targetYearValue == "" {
			targetYearValue = targetYear.Year
		}
	} else {
		// Ensure the requested year does not already exist
		if targetYearValue == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "target_year is required"})
			return
		}

		var existing models.Year
		if err := config.DB.Where("year = ? AND delete_at IS NULL", targetYearValue).
			First(&existing).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": "target year already exists"})
			return
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to verify target year"})
			return
		}
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to start transaction"})
		return
	}

	rollbackWithError := func(status int, message string, debug string) {
		tx.Rollback()
		payload := gin.H{"success": false, "error": message}
		if debug != "" {
			payload["debug"] = debug
		}
		c.JSON(status, payload)
	}

	if usingExistingTarget {
		// Reload the latest target year details within the transaction
		if err := tx.Where("year_id = ?", targetYear.YearID).First(&targetYear).Error; err != nil {
			rollbackWithError(http.StatusInternalServerError, "failed to lock target year", err.Error())
			return
		}
	} else {
		now := time.Now()
		targetYear = models.Year{
			Year:   targetYearValue,
			Status: sourceYear.Status,
		}
		if strings.TrimSpace(targetYear.Status) == "" {
			targetYear.Status = "active"
		}
		targetYear.CreateAt = &now
		targetYear.UpdateAt = &now

		if err := tx.Create(&targetYear).Error; err != nil {
			rollbackWithError(http.StatusInternalServerError, "failed to create target year", err.Error())
			return
		}
	}

	var sourcePeriods []models.FundInstallmentPeriod
	if err := tx.Where("year_id = ? AND fund_level = ? AND fund_keyword = ? AND deleted_at IS NULL", req.SourceYearID, selection.Level, selection.Keyword).
		Order("installment_number ASC, cutoff_date ASC").
		Find(&sourcePeriods).Error; err != nil {
		rollbackWithError(http.StatusInternalServerError, "failed to load source installments", err.Error())
		return
	}

	existingNumbers := make(map[string]struct{})
	if usingExistingTarget {
		var targetPeriods []models.FundInstallmentPeriod
		if err := tx.Where("year_id = ? AND fund_level = ? AND fund_keyword = ? AND deleted_at IS NULL", targetYear.YearID, selection.Level, selection.Keyword).
			Find(&targetPeriods).Error; err != nil {
			rollbackWithError(http.StatusInternalServerError, "failed to load target installments", err.Error())
			return
		}
		for _, period := range targetPeriods {
			periodSelection := selectionFromPeriod(period)
			key := fmt.Sprintf("%s-%s-%d", periodSelection.Level, periodSelection.Keyword, period.InstallmentNumber)
			existingNumbers[key] = struct{}{}
		}
	}

	sourceYearCE, hasSourceYearCE := parseCalendarYearValue(sourceYear.Year)
	targetYearCE, hasTargetYearCE := parseCalendarYearValue(targetYear.Year)
	yearDiff := 0
	if hasSourceYearCE && hasTargetYearCE {
		yearDiff = targetYearCE - sourceYearCE
	}

	createdCount := 0
	skippedCount := 0

	for _, period := range sourcePeriods {
		periodSelection := selectionFromPeriod(period)
		key := fmt.Sprintf("%s-%s-%d", periodSelection.Level, periodSelection.Keyword, period.InstallmentNumber)
		if _, exists := existingNumbers[key]; exists {
			skippedCount++
			continue
		}

		if err := purgeSoftDeletedInstallmentPeriods(tx, periodSelection, targetYear.YearID, period.InstallmentNumber); err != nil {
			rollbackWithError(http.StatusInternalServerError, "failed to clear deleted installment periods", err.Error())
			return
		}

		currentTime := time.Now()
		cutoff := period.CutoffDate
		if yearDiff != 0 && !period.CutoffDate.IsZero() {
			cutoff = period.CutoffDate.AddDate(yearDiff, 0, 0)
		}

		newPeriod := models.FundInstallmentPeriod{
			FundLevel:         &periodSelection.Level,
			FundKeyword:       &periodSelection.Keyword,
			FundParentKeyword: periodSelection.ParentKeyword,
			YearID:            targetYear.YearID,
			InstallmentNumber: period.InstallmentNumber,
			CutoffDate:        cutoff,
			CreatedAt:         currentTime,
			UpdatedAt:         currentTime,
		}

		if period.Name != nil {
			name := strings.TrimSpace(*period.Name)
			if name != "" {
				copyName := name
				newPeriod.Name = &copyName
			}
		}

		normalizedStatus, _ := normalizeInstallmentStatus(period.Status)
		if normalizedStatus != nil {
			status := *normalizedStatus
			newPeriod.Status = &status
		} else if period.Status != nil {
			status := strings.TrimSpace(*period.Status)
			if status != "" {
				statusCopy := status
				newPeriod.Status = &statusCopy
			}
		}

		if period.Remark != nil {
			remark := strings.TrimSpace(*period.Remark)
			if remark != "" {
				remarkCopy := remark
				newPeriod.Remark = &remarkCopy
			}
		}

		if err := tx.Create(&newPeriod).Error; err != nil {
			rollbackWithError(http.StatusInternalServerError, "failed to copy installment period", err.Error())
			return
		}

		existingNumbers[key] = struct{}{}
		createdCount++
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to finalize copy operation"})
		return
	}

	targetYearLabel := strings.TrimSpace(targetYear.Year)
	if targetYearLabel == "" {
		targetYearLabel = fmt.Sprintf("%d", targetYear.YearID)
	}

	message := fmt.Sprintf("copied %d installment periods to year %s", createdCount, targetYearLabel)
	if usingExistingTarget {
		message = fmt.Sprintf("copied %d installment periods to existing year %s", createdCount, targetYearLabel)
	}
	if createdCount == 0 {
		message = fmt.Sprintf("no installment periods were copied to year %s", targetYearLabel)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         message,
		"target_year_id":  targetYear.YearID,
		"target_year":     targetYear.Year,
		"existing_target": usingExistingTarget,
		"copied": gin.H{
			"created":      createdCount,
			"skipped":      skippedCount,
			"source_total": len(sourcePeriods),
		},
	})
}

func newAdminFundInstallmentPeriodResponse(period models.FundInstallmentPeriod) adminFundInstallmentPeriodResponse {
	cutoff := ""
	if !period.CutoffDate.IsZero() {
		cutoff = period.CutoffDate.Format("2006-01-02")
	}

	selection := selectionFromPeriod(period)
	legacyType := legacyFundType(selection)

	status := ""
	if period.Status != nil {
		status = strings.TrimSpace(*period.Status)
	}
	if status == "" {
		status = "active"
	}

	createdAt := period.CreatedAt.UTC().Format(time.RFC3339)
	updatedAt := period.UpdatedAt.UTC().Format(time.RFC3339)

	var deletedAt *string
	if period.DeletedAt != nil && !period.DeletedAt.IsZero() {
		formatted := period.DeletedAt.UTC().Format(time.RFC3339)
		deletedAt = &formatted
	}

	return adminFundInstallmentPeriodResponse{
		InstallmentPeriodID: period.InstallmentPeriodID,
		FundLevel:           ptrString(selection.Level),
		FundKeyword:         ptrString(selection.Keyword),
		FundParentKeyword:   selection.ParentKeyword,
		FundType:            legacyType,
		YearID:              period.YearID,
		InstallmentNumber:   period.InstallmentNumber,
		CutoffDate:          cutoff,
		Name:                period.Name,
		Status:              status,
		Remark:              period.Remark,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
		DeletedAt:           deletedAt,
	}
}

func ensureYearExists(yearID int) error {
	var year models.Year
	if err := config.DB.Where("year_id = ? AND delete_at IS NULL", yearID).First(&year).Error; err != nil {
		return err
	}
	return nil
}

func checkInstallmentConflicts(currentID int, selection fundSelection, yearID, installmentNumber int, cutoffDate time.Time) error {
	var existing models.FundInstallmentPeriod
	numberQuery := config.DB.Where(
		"year_id = ? AND installment_number = ? AND fund_level = ? AND fund_keyword = ? AND deleted_at IS NULL",
		yearID,
		installmentNumber,
		selection.Level,
		selection.Keyword,
	)
	if currentID > 0 {
		numberQuery = numberQuery.Where("installment_period_id <> ?", currentID)
	}
	if err := numberQuery.First(&existing).Error; err == nil {
		return fmt.Errorf("installment number %d already exists for this year", installmentNumber)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	cutoffQuery := config.DB.Where(
		"year_id = ? AND cutoff_date = ? AND fund_level = ? AND fund_keyword = ? AND deleted_at IS NULL",
		yearID,
		cutoffDate,
		selection.Level,
		selection.Keyword,
	)
	if currentID > 0 {
		cutoffQuery = cutoffQuery.Where("installment_period_id <> ?", currentID)
	}
	if err := cutoffQuery.First(&existing).Error; err == nil {
		return fmt.Errorf("cutoff date already exists for this year")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	return nil
}

func normalizeFundSelection(fundType *string, fundLevel *string, fundKeyword *string, fundParentKeyword *string) (*fundSelection, error) {
	legacyValue, legacyErr := normalizeFundTypeKeyword(fundType)
	if legacyErr != nil {
		return nil, legacyErr
	}
	if legacyValue != nil {
		return legacyValue, nil
	}

	normalizedLevel, levelErr := normalizeFundLevel(fundLevel)
	if levelErr != nil {
		return nil, levelErr
	}

	normalizedKeyword, keywordErr := normalizeFundKeyword(fundKeyword)
	if keywordErr != nil {
		return nil, keywordErr
	}

	if normalizedLevel == nil || normalizedKeyword == nil {
		return nil, fmt.Errorf("fund_level and fund_keyword are required")
	}

	normalizedParent, parentErr := normalizeFundKeyword(fundParentKeyword)
	if parentErr != nil {
		return nil, parentErr
	}

	return &fundSelection{
		Level:         *normalizedLevel,
		Keyword:       *normalizedKeyword,
		ParentKeyword: normalizedParent,
	}, nil
}

func normalizeFundTypeKeyword(input *string) (*fundSelection, error) {
	if input == nil {
		return nil, nil
	}

	normalized := strings.ToLower(strings.TrimSpace(*input))
	if normalized == "" {
		return nil, nil
	}

	if selection, ok := legacyFundSelections[normalized]; ok {
		copy := selection
		return &copy, nil
	}

	keys := make([]string, 0, len(legacyFundSelections))
	for key := range legacyFundSelections {
		keys = append(keys, key)
	}
	return nil, fmt.Errorf("fund_type must be one of %s or specify fund_level and fund_keyword", strings.Join(keys, ", "))
}

func normalizeFundLevel(input *string) (*string, error) {
	if input == nil {
		return nil, nil
	}

	level := strings.ToLower(strings.TrimSpace(*input))
	if level == "" {
		return nil, nil
	}

	if _, ok := allowedFundLevels[level]; !ok {
		return nil, fmt.Errorf("fund_level must be either 'category' or 'subcategory'")
	}

	return &level, nil
}

func normalizeFundKeyword(input *string) (*string, error) {
	if input == nil {
		return nil, nil
	}

	keyword := strings.TrimSpace(*input)
	if keyword == "" {
		return nil, nil
	}

	return &keyword, nil
}

func selectionFromPeriod(period models.FundInstallmentPeriod) fundSelection {
	level := strings.TrimSpace(ptrValue(period.FundLevel))
	if level == "" {
		level = "category"
	}

	keyword := strings.TrimSpace(ptrValue(period.FundKeyword))

	parent := normalizeStoredKeyword(period.FundParentKeyword)

	return fundSelection{
		Level:         level,
		Keyword:       keyword,
		ParentKeyword: parent,
	}
}

func legacyFundType(selection fundSelection) *string {
	for key, value := range legacyFundSelections {
		if value.Level == selection.Level && value.Keyword == selection.Keyword {
			if (value.ParentKeyword == nil && selection.ParentKeyword == nil) || (value.ParentKeyword != nil && selection.ParentKeyword != nil && *value.ParentKeyword == *selection.ParentKeyword) {
				return ptrString(key)
			}
		}
	}
	return nil
}

func ptrString(value string) *string {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return ptrString(value)
}

func ptrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeStoredKeyword(value *string) *string {
	normalized, err := normalizeFundKeyword(value)
	if err != nil {
		return nil
	}
	return normalized
}

func normalizeInstallmentStatus(input *string) (*string, error) {
	if input == nil {
		return nil, nil
	}

	trimmed := strings.ToLower(strings.TrimSpace(*input))
	if trimmed == "" {
		return nil, nil
	}

	switch trimmed {
	case "active", "inactive":
		return &trimmed, nil
	default:
		return nil, fmt.Errorf("status must be 'active' or 'inactive'")
	}
}

func parseBoolQuery(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseLimit(value string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func parseOffset(value string) int {
	offset, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func respondYearLookupError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "year not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to verify year"})
}

func respondConflictError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}
	if strings.Contains(err.Error(), "already exists") {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
}

func parseCalendarYearValue(value string) (int, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}

	numeric, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}

	if numeric > 2400 {
		return numeric - 543, true
	}

	return numeric, true
}

func purgeSoftDeletedInstallmentPeriods(db *gorm.DB, selection fundSelection, yearID int, installmentNumber int) error {
	return db.Unscoped().Where(
		"year_id = ? AND installment_number = ? AND fund_level = ? AND fund_keyword = ? AND deleted_at IS NOT NULL",
		yearID,
		installmentNumber,
		selection.Level,
		selection.Keyword,
	).Delete(&models.FundInstallmentPeriod{}).Error
}
