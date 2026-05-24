package controllers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/models"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetMous retrieves all MOU records with filtering
func GetMous(c *gin.Context) {

	mouCode := c.DefaultQuery("mou_code", "")
	title := c.DefaultQuery("title", "")
	partnerName := c.DefaultQuery("partner_name", "")
	country := c.DefaultQuery("country", "")
	status := c.DefaultQuery("status", "")
	mouType := c.DefaultQuery("mou_type", "")
	level := c.DefaultQuery("level", "")
	isInternational := c.DefaultQuery("is_international", "")
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	page, _ := strconv.Atoi(pageStr)
	limit, _ := strconv.Atoi(limitStr)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	var mous []models.MouRecord
	query := config.DB.Where("mou_records.deleted_at IS NULL")

	// Apply filters
	if status != "" {
		query = query.Where("Status_id = ?", status)
	}
	if mouType != "" {
		query = query.Where("mou_type_id = ?", mouType)
	}
	if level != "" {
		query = query.Where("level = ?", level)
	}
	if isInternational != "" {
		if isInternational == "true" || isInternational == "1" {
			query = query.Where("is_international = ?", 1)
		} else {
			query = query.Where("is_international = ?", 0)
		}
	}
	if mouCode != "" {
		query = query.Where("mou_code LIKE ?", "%"+mouCode+"%")
	}
	if title != "" {
		query = query.Where("title LIKE ?", "%"+title+"%")
	}
	if partnerName != "" {
		subQuery := config.DB.Table("mou_partner").Select("mou_id").Where("partner_org LIKE ?", "%"+partnerName+"%")
		query = query.Where("mou_records.id IN (?)", subQuery)
	}
	if country != "" {
		query = query.Joins("LEFT JOIN countries ON countries.id = mou_records.country_id").
			Where("countries.name_th LIKE ? OR countries.name_en LIKE ?", "%"+country+"%", "%"+country+"%")
	}

	var total int64
	query.Model(&models.MouRecord{}).Count(&total)

	offset := (page - 1) * limit
	err := query.
		Preload("Status").
		Preload("MouType").
		Preload("Country").
		Preload("Coordinator").
		Preload("SignedByUser").
		Preload("Partners").
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Find(&mous).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch MOU records"})
		return
	}

	// Stats queries using raw SQL to avoid GORM chain conflicts
	statsWhere := "mou_records.deleted_at IS NULL"
	var statsArgs []interface{}
	if status != "" {
		statsWhere += " AND mou_records.Status_id = ?"
		statsArgs = append(statsArgs, status)
	}
	if mouType != "" {
		statsWhere += " AND mou_records.mou_type_id = ?"
		statsArgs = append(statsArgs, mouType)
	}
	if level != "" {
		statsWhere += " AND mou_records.level = ?"
		statsArgs = append(statsArgs, level)
	}
	if isInternational != "" {
		if isInternational == "true" || isInternational == "1" {
			statsWhere += " AND mou_records.is_international = 1"
		} else {
			statsWhere += " AND mou_records.is_international = 0"
		}
	}
	if mouCode != "" {
		statsWhere += " AND mou_records.mou_code LIKE ?"
		statsArgs = append(statsArgs, "%"+mouCode+"%")
	}
	if title != "" {
		statsWhere += " AND mou_records.title LIKE ?"
		statsArgs = append(statsArgs, "%"+title+"%")
	}
	if partnerName != "" {
		statsWhere += " AND mou_records.id IN (SELECT mou_id FROM mou_partner WHERE partner_org LIKE ?)"
		statsArgs = append(statsArgs, "%"+partnerName+"%")
	}
	if country != "" {
		statsWhere += " AND (countries.name_th LIKE ? OR countries.name_en LIKE ?)"
		statsArgs = append(statsArgs, "%"+country+"%", "%"+country+"%")
	}

	var activeCount, nearExpiryCount, expiredCount int64
	joinClause := "LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id"
	countryJoin := ""
	if country != "" {
		countryJoin = "LEFT JOIN countries ON countries.id = mou_records.country_id"
	}

	config.DB.Raw("SELECT COUNT(*) FROM mou_records "+countryJoin+" "+joinClause+" WHERE "+statsWhere+" AND (mou_status.name LIKE ? OR mou_status.name LIKE ?)",
		append(statsArgs, "%มีผล%", "%ใกล้หมดอายุ%")...).Scan(&activeCount)
	config.DB.Raw("SELECT COUNT(*) FROM mou_records "+countryJoin+" "+joinClause+" WHERE "+statsWhere+" AND (mou_status.name LIKE ? OR mou_status.name LIKE ?) AND mou_records.end_date IS NOT NULL AND mou_records.end_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY) AND mou_records.end_date >= CURDATE()",
		append(statsArgs, "%มีผล%", "%ใกล้หมดอายุ%")...).Scan(&nearExpiryCount)
	config.DB.Raw("SELECT COUNT(*) FROM mou_records "+countryJoin+" "+joinClause+" WHERE "+statsWhere+" AND (mou_status.name LIKE ? OR mou_status.name LIKE ?)",
		append(statsArgs, "%หมดอายุ%", "%Expired%")...).Scan(&expiredCount)

	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"data":              mous,
		"total":             total,
		"page":              page,
		"limit":             limit,
		"active_count":      activeCount,
		"near_expiry_count": nearExpiryCount,
		"expired_count":     expiredCount,
	})
}

// GetMouDetail retrieves a single MOU record with all related data
func GetMouDetail(c *gin.Context) {

	id := c.Param("id")

	var mou models.MouRecord
	err := config.DB.
		Preload("Status").
		Preload("MouType").
		Preload("Country").
		Preload("Coordinator").
		Preload("SignedByUser").
		Preload("Partners").
		Preload("Faculties.Faculty").
		Preload("Notifications").
		Preload("Activities.ActivityType").
		Preload("Activities.ActivityTypes").
		Preload("Activities.Coordinator").
		Preload("Activities.Creator").
		Preload("Activities.Okrs").
		Preload("Activities.Attachments").
		Preload("Attachments").
		Preload("Creator").
		Preload("Updater").
		Where("id = ? AND deleted_at IS NULL", id).
		First(&mou).Error

	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "MOU not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch MOU"})
		return
	}

	// Load country via raw JOIN (bypasses GORM *int foreign key mapping issues)
	var countryNameTh, countryNameEn string
	row := config.DB.Raw(`
		SELECT c.name_th, c.name_en
		FROM mou_records mr
		LEFT JOIN countries c ON c.id = mr.country_id
		WHERE mr.id = ?
	`, id).Row()
	row.Scan(&countryNameTh, &countryNameEn)
	if countryNameTh != "" {
		mou.Country = models.Country{
			ID:     0,
			NameTh: countryNameTh,
			NameEn: countryNameEn,
		}
	}

	// Manually load coordinator (avoids GORM Preload issue with *int foreign keys)
	if mou.CoordinatorID != nil {
		var coordinator models.User
		if err := config.DB.First(&coordinator, *mou.CoordinatorID).Error; err == nil {
			mou.Coordinator = coordinator
		}
	}

	// Manually load user data for each faculty (avoids GORM Preload issue with *int foreign keys)
	if len(mou.Faculties) > 0 {
		var userIDs []int
		for i := range mou.Faculties {
			if mou.Faculties[i].UserID != nil {
				userIDs = append(userIDs, *mou.Faculties[i].UserID)
			}
		}
		if len(userIDs) > 0 {
			var users []models.User
			config.DB.Where("user_id IN ?", userIDs).Find(&users)
			userMap := make(map[int]*models.User)
			for i := range users {
				userMap[users[i].UserID] = &users[i]
			}
			for i := range mou.Faculties {
				if mou.Faculties[i].UserID != nil {
					if u, ok := userMap[*mou.Faculties[i].UserID]; ok {
						mou.Faculties[i].User = u
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    mou,
	})
}

// DownloadMouAttachments streams all MOU attachments as a ZIP file
func DownloadMouAttachments(c *gin.Context) {
	id := c.Param("id")

	var mou models.MouRecord
	if err := config.DB.Where("id = ? AND deleted_at IS NULL", id).First(&mou).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "MOU not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch MOU"})
		}
		return
	}

	var attachments []models.MouAttachment
	if err := config.DB.Where("mou_id = ? AND deleted_at IS NULL", id).Find(&attachments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch attachments"})
		return
	}

	if len(attachments) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No attachments found"})
		return
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	for _, att := range attachments {
		func() {
			f, err := os.Open(att.FilePath)
			if err != nil {
				return
			}
			defer f.Close()

			w, err := zw.Create(att.FileName)
			if err != nil {
				return
			}

			io.Copy(w, f)
		}()
	}

	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create ZIP"})
		return
	}

	zipName := fmt.Sprintf("mou_%s_attachments.zip", mou.MouCode)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipName))
	c.Header("Content-Type", "application/zip")
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

// GetMouAttachment serves a single MOU attachment file (view or download)
func GetMouAttachment(c *gin.Context) {
	mouID := c.Param("id")
	attachID := c.Param("attachId")
	download := c.Query("dl") == "1"

	var att models.MouAttachment
	if err := config.DB.Where("id = ? AND mou_id = ? AND deleted_at IS NULL", attachID, mouID).First(&att).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch attachment"})
		}
		return
	}

	if _, err := os.Stat(att.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found on server"})
		return
	}

	if download {
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", att.FileName))
	} else {
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", att.FileName))
	}
	c.Header("Content-Type", att.MimeType)
	c.File(att.FilePath)
}

// CreateMou creates a new MOU record
func CreateMou(c *gin.Context) {

	var req models.CreateMouRequest
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		dataStr := c.PostForm("data")
		if dataStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'data' field in multipart form"})
			return
		}
		if err := json.Unmarshal([]byte(dataStr), &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON in 'data' field: " + err.Error()})
			return
		}
	} else {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Parse dates from "DD/MM/YYYY" format
	startDate, err := parseDateString(req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start_date format. Use DD/MM/YYYY"})
		return
	}

	var endDate *time.Time
	if req.EndDate != "" {
		parsedEndDate, err := parseDateString(req.EndDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end_date format. Use DD/MM/YYYY"})
			return
		}
		endDate = &parsedEndDate
	}

	if req.YearOfSigning > 0 && (req.YearOfSigning < 1900 || req.YearOfSigning > 2155) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year_of_signing. Must be a 4-digit year between 1900-2155"})
		return
	}

	// Check for duplicate MOU code
	var existingMou models.MouRecord
	if err := config.DB.Where("mou_code = ?", req.MouCode).First(&existingMou).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "MOU code already exists"})
		return
	}

	// Set default status
	var statusID int = 1
	if req.StatusID != nil && *req.StatusID > 0 {
		statusID = *req.StatusID
	} else {
		var status models.MouStatus
		if err := config.DB.Where("name LIKE ?", "%ร่าง%").First(&status).Error; err == nil {
			statusID = status.ID
		}
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID"})
		return
	}

	mou := models.MouRecord{
		MouCode:          req.MouCode,
		Title:            req.Title,
		Description:      req.Description,
		StatusID:         statusID,
		MouTypeID:        req.MouTypeID,
		Level:            req.Level,
		StartDate:        startDate,
		EndDate:          endDate,
		YearOfSigning:    &req.YearOfSigning,
		SignedBy:         req.SignedBy,
		CountryID:        req.CountryID,
		IsInternational:  req.IsInternational,
		CoordinatorID:    req.CoordinatorID,
		Notes:            req.Notes,
		NotifyDaysBefore: req.NotifyDaysBefore,
		CreatedBy:        uid,
	}

	if err := config.DB.Create(&mou).Error; err != nil {
		fmt.Println("Error creating MOU:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create MOU"})
		return
	}

	// Create partner record
	if req.PartnerName != "" {
		partner := models.MouPartner{
			MouID:         mou.ID,
			PartnerOrg:    req.PartnerName,
			PartnerTypeID: req.PartnerTypeID,
		}
		if err := config.DB.Create(&partner).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create MOU partner: " + err.Error()})
			return
		}
	}

	// Create faculty records
	if len(req.Faculties) > 0 {
		for _, f := range req.Faculties {
			hasFaculty := f.FacultyID > 0
			hasExtName := f.ExternalName != nil && *f.ExternalName != ""
			hasExtOrg := f.ExternalOrg != nil && *f.ExternalOrg != ""
			if !hasFaculty && !hasExtName && !hasExtOrg {
				continue
			}
			mouFaculty := models.MouFaculty{
				MouID: mou.ID,
			}
			if hasFaculty {
				fid := f.FacultyID
				mouFaculty.FacultyID = &fid
			}
			if f.UserID > 0 {
				uid := f.UserID
				mouFaculty.UserID = &uid
			}
			if hasExtName {
				mouFaculty.ExternalName = f.ExternalName
			}
			if hasExtOrg {
				mouFaculty.ExternalOrg = f.ExternalOrg
			}
			if err := config.DB.Create(&mouFaculty).Error; err != nil {
				fmt.Println("Error creating MOU faculty:", err)
			}
		}
	} else {
		for _, fid := range req.FacultyIDs {
			if fid <= 0 {
				continue
			}
			mouFaculty := models.MouFaculty{
				MouID:     mou.ID,
				FacultyID: &fid,
			}
			if err := config.DB.Create(&mouFaculty).Error; err != nil {
				fmt.Println("Error creating MOU faculty:", err)
			}
		}
	}

	// Create notification record
	notifyDays := 0
	if req.NotifyDaysBefore != nil {
		notifyDays = *req.NotifyDaysBefore
	}
	if req.CoordinatorID != nil && *req.CoordinatorID > 0 {
		notification := models.MouNotification{
			MouID:      mou.ID,
			StaffID:    *req.CoordinatorID,
			DaysBefore: notifyDays,
		}
		if err := config.DB.Create(&notification).Error; err != nil {
			fmt.Println("Error creating MOU notification:", err)
		}
	}

	// Handle file uploads from multipart form
	if strings.HasPrefix(contentType, "multipart/form-data") {
		form, err := c.MultipartForm()
		if err == nil {
			uploadFiles := form.File["files"]
			uploadDir := filepath.Join("uploads", "mou")
			if err := os.MkdirAll(uploadDir, 0755); err == nil {
				for _, f := range uploadFiles {
					savePath := filepath.Join(uploadDir, fmt.Sprintf("%d_%s", mou.ID, f.Filename))
					if err := c.SaveUploadedFile(f, savePath); err == nil {
						attachment := models.MouAttachment{
							MouID:      mou.ID,
							FileName:   f.Filename,
							FilePath:   savePath,
							MimeType:   f.Header.Get("Content-Type"),
							UploadedBy: uid,
						}
						config.DB.Create(&attachment)
					}
				}
			}
		}
	}

	// Reload with associations
	config.DB.Preload("Status").
		Preload("MouType").
		Preload("Country").
		Preload("Coordinator").
		Preload("SignedByUser").
		Preload("Partners").
		First(&mou, mou.ID)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "MOU created successfully",
		"data":    mou,
	})
}

// UpdateMou updates an existing MOU record
func UpdateMou(c *gin.Context) {

	id := c.Param("id")
	var req models.UpdateMouRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var mou models.MouRecord
	if err := config.DB.Where("id = ? AND deleted_at IS NULL", id).First(&mou).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "MOU not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch MOU"})
		}
		return
	}

	// Update fields if provided
	if req.Title != nil {
		mou.Title = *req.Title
	}
	if req.Description != nil {
		mou.Description = *req.Description
	}
	if req.MouTypeID != nil {
		mou.MouTypeID = *req.MouTypeID
	}
	if req.StatusID != nil {
		mou.StatusID = *req.StatusID
	}
	if req.IsInternational != nil {
		mou.IsInternational = *req.IsInternational
	}
	if req.EndDate != nil {
		endDate, err := parseDateString(*req.EndDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end_date format. Use DD/MM/YYYY"})
			return
		}
		mou.EndDate = &endDate
	}
	if req.MouCode != nil {
		mou.MouCode = *req.MouCode
	}
	if req.Level != nil {
		mou.Level = *req.Level
	}
	if req.CountryID != nil {
		mou.CountryID = req.CountryID
	}
	if req.CoordinatorID != nil {
		mou.CoordinatorID = req.CoordinatorID
	}
	if req.StartDate != nil {
		startDate, err := parseDateString(*req.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start_date format. Use DD/MM/YYYY"})
			return
		}
		mou.StartDate = startDate
	}
	if req.YearOfSigning != nil {
		if *req.YearOfSigning < 1900 || *req.YearOfSigning > 2155 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year_of_signing. Must be a 4-digit year between 1900-2155"})
			return
		}
		mou.YearOfSigning = req.YearOfSigning
	}
	if req.NotifyDaysBefore != nil {
		mou.NotifyDaysBefore = req.NotifyDaysBefore
		config.DB.Model(&models.MouNotification{}).Where("mou_id = ?", mou.ID).Update("days_before", *req.NotifyDaysBefore)
	}
	if req.SignedBy != nil {
		mou.SignedBy = req.SignedBy
	}
	if req.Notes != nil {
		mou.Notes = *req.Notes
	}

	// Set UpdatedBy from authenticated user
	if userID, exists := c.Get("userID"); exists {
		if uid, ok := userID.(int); ok {
			mou.UpdatedBy = &uid
		}
	}

	if err := config.DB.Save(&mou).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update MOU"})
		return
	}

	// Update partner: delete old, insert new
	if req.PartnerName != nil {
		config.DB.Where("mou_id = ?", mou.ID).Delete(&models.MouPartner{})
		if *req.PartnerName != "" {
			partner := models.MouPartner{
				MouID:      mou.ID,
				PartnerOrg: *req.PartnerName,
			}
			if req.PartnerTypeID != nil {
				partner.PartnerTypeID = *req.PartnerTypeID
			}
			config.DB.Create(&partner)
		}
	}

	// Update faculty: delete old, insert new
	if req.Faculties != nil || req.FacultyIDs != nil {
		config.DB.Where("mou_id = ?", mou.ID).Delete(&models.MouFaculty{})
		if len(req.Faculties) > 0 {
			for _, f := range req.Faculties {
				hasFaculty := f.FacultyID > 0
				hasExtName := f.ExternalName != nil && *f.ExternalName != ""
				hasExtOrg := f.ExternalOrg != nil && *f.ExternalOrg != ""
				if !hasFaculty && !hasExtName && !hasExtOrg {
					continue
				}
				mouFaculty := models.MouFaculty{
					MouID: mou.ID,
				}
				if hasFaculty {
					fidCopy := f.FacultyID
					mouFaculty.FacultyID = &fidCopy
				}
				if f.UserID > 0 {
					uidCopy := f.UserID
					mouFaculty.UserID = &uidCopy
				}
				if hasExtName {
					mouFaculty.ExternalName = f.ExternalName
				}
				if hasExtOrg {
					mouFaculty.ExternalOrg = f.ExternalOrg
				}
				config.DB.Create(&mouFaculty)
			}
		} else if req.FacultyIDs != nil {
			for _, fid := range req.FacultyIDs {
				if fid <= 0 {
					continue
				}
				fidCopy := fid
				mouFaculty := models.MouFaculty{
					MouID:     mou.ID,
					FacultyID: &fidCopy,
				}
				config.DB.Create(&mouFaculty)
			}
		}
	}

	// Delete removed attachments
	if len(req.RemovedAttachmentIDs) > 0 {
		config.DB.Where("id IN ? AND mou_id = ?", req.RemovedAttachmentIDs, mou.ID).Delete(&models.MouAttachment{})
	}

	// Update notification: delete old, insert new
	if req.NotifyDaysBefore != nil || req.CoordinatorID != nil {
		config.DB.Where("mou_id = ?", mou.ID).Delete(&models.MouNotification{})
		notifyDays := 30
		if req.NotifyDaysBefore != nil && *req.NotifyDaysBefore > 0 {
			notifyDays = *req.NotifyDaysBefore
		}
		staffID := mou.CoordinatorID
		if req.CoordinatorID != nil {
			staffID = req.CoordinatorID
		}
		if staffID != nil && *staffID > 0 {
			notification := models.MouNotification{
				MouID:      mou.ID,
				StaffID:    *staffID,
				DaysBefore: notifyDays,
			}
			config.DB.Create(&notification)
		}
	}

	// Reload with associations
	config.DB.Preload("Status").
		Preload("MouType").
		Preload("Country").
		Preload("Coordinator").
		Preload("SignedByUser").
		Preload("Partners").
		First(&mou, mou.ID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "MOU updated successfully",
		"data":    mou,
	})
}

// DeleteMou soft deletes an MOU record
func DeleteMou(c *gin.Context) {

	id := c.Param("id")

	var mou models.MouRecord
	if err := config.DB.Where("id = ? AND deleted_at IS NULL", id).First(&mou).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "MOU not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch MOU"})
		}
		return
	}

	if err := config.DB.Delete(&mou).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete MOU"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "MOU deleted successfully",
	})
}

// GetMouStatuses retrieves all available MOU statuses
func GetMouStatuses(c *gin.Context) {

	var statuses []models.MouStatus

	// Note: deleted_at has DEFAULT CURRENT_TIMESTAMP in this schema, so we omit the soft-delete filter
	if err := config.DB.Find(&statuses).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch statuses"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    statuses,
	})
}

// GetMouTypes retrieves all available MOU types
func GetMouTypes(c *gin.Context) {

	var types []models.MouType

	// Note: deleted_at has DEFAULT CURRENT_TIMESTAMP in this schema, so we omit the soft-delete filter
	if err := config.DB.Where("is_active = ?", true).Find(&types).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch MOU types"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types,
	})
}

// GetMouLevels retrieves distinct MOU levels
func GetMouLevels(c *gin.Context) {

	var levels []string
	if err := config.DB.Model(&models.MouRecord{}).
		Where("deleted_at IS NULL AND level IS NOT NULL AND level != ?", "").
		Distinct("level").
		Pluck("level", &levels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch levels"})
		return
	}

	// If no records exist yet, read ENUM values from schema
	if len(levels) == 0 {
		var raw string
		config.DB.Raw(
			"SELECT COLUMN_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?",
			"mou_records", "level",
		).Scan(&raw)
		// raw is like "enum('university','faculty')"
		if len(raw) > 6 {
			trim := raw[5 : len(raw)-1] // strip "enum(" and ")"
			levels = strings.Split(trim, ",")
			for i := range levels {
				levels[i] = strings.Trim(levels[i], "' ")
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    levels,
	})
}

// GetCountries retrieves all available countries
func GetCountries(c *gin.Context) {

	var countries []models.Country

	// Note: deleted_at has DEFAULT CURRENT_TIMESTAMP in this schema, so we omit the soft-delete filter
	if err := config.DB.Where("is_active = ?", true).Order("name_th").Find(&countries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch countries"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    countries,
	})
}

// GetFaculties retrieves all available faculties
func GetFaculties(c *gin.Context) {

	var faculties []models.Faculty

	// Note: deleted_at has DEFAULT CURRENT_TIMESTAMP in this schema, so we omit the soft-delete filter
	if err := config.DB.Where("is_active = ?", true).Order("name_th").Find(&faculties).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch faculties"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    faculties,
	})
}

// GetActivityTypes retrieves all available activity types
func GetActivityTypes(c *gin.Context) {
	var types []models.MouActivityType
	if err := config.DB.Where("is_active = ?", true).Find(&types).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch activity types"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types,
	})
}

// GetOkrList retrieves all available OKRs
func GetOkrList(c *gin.Context) {
	var okrs []models.MouOKR
	if err := config.DB.Find(&okrs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch OKRs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    okrs,
	})
}

// CreateActivityType creates a new activity type
func CreateActivityType(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณากรอกชื่อประเภทกิจกรรม"})
		return
	}
	actType := models.MouActivityType{
		Name:        req.Name,
		Description: req.Description,
		IsActive:    true,
	}
	if err := config.DB.Create(&actType).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create activity type"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": actType})
}

// UpdateActivityType updates an existing activity type
func UpdateActivityType(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	var actType models.MouActivityType
	if err := config.DB.First(&actType, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Activity type not found"})
		return
	}
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณากรอกชื่อประเภทกิจกรรม"})
		return
	}
	actType.Name = req.Name
	actType.Description = req.Description
	if err := config.DB.Save(&actType).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update activity type"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": actType})
}

// DeleteActivityType soft-deletes an activity type
func DeleteActivityType(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	var actType models.MouActivityType
	if err := config.DB.First(&actType, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Activity type not found"})
		return
	}
	now := time.Now()
	actType.DeletedAt = &now
	actType.IsActive = false
	if err := config.DB.Save(&actType).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete activity type"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "ลบประเภทกิจกรรมเรียบร้อย"})
}

// CreateOkr creates a new OKR
func CreateOkr(c *gin.Context) {
	var req struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณากรอกรหัส OKR"})
		return
	}
	okr := models.MouOKR{
		Title:       req.Title,
		Description: req.Description,
		Category:    req.Category,
	}
	if err := config.DB.Create(&okr).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create OKR"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": okr})
}

// UpdateOkr updates an existing OKR
func UpdateOkr(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	var okr models.MouOKR
	if err := config.DB.First(&okr, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "OKR not found"})
		return
	}
	var req struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณากรอกรหัส OKR"})
		return
	}
	okr.Title = req.Title
	okr.Description = req.Description
	okr.Category = req.Category
	if err := config.DB.Save(&okr).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update OKR"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": okr})
}

// DeleteOkr soft-deletes an OKR
func DeleteOkr(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	var okr models.MouOKR
	if err := config.DB.First(&okr, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "OKR not found"})
		return
	}
	now := time.Now()
	okr.DeletedAt = &now
	if err := config.DB.Save(&okr).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete OKR"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "ลบ OKR เรียบร้อย"})
}

// GetMouDashboard returns aggregate stats for the dashboard
func GetMouDashboard(c *gin.Context) {
	var totalCount, activeCount, nearExpiryCount, expiredCount, pendingCount, cancelledCount, draftCount, renewedCount int64

	yearParam := c.Query("year")
	yearFilter := ""
	var yearArgs []interface{}
	if yearParam != "" {
		y, err := strconv.Atoi(yearParam)
		if err == nil {
			greg := y
			if y > 2155 {
				greg = y - 543
			}
			yearStart := fmt.Sprintf("%d-01-01", greg)
			yearEnd := fmt.Sprintf("%d-12-31", greg)
			yearFilter = " AND mou_records.start_date <= ? AND (mou_records.end_date IS NULL OR mou_records.end_date >= ?)"
			yearArgs = []interface{}{yearEnd, yearStart}
		}
	}

	config.DB.Model(&models.MouRecord{}).Where("deleted_at IS NULL"+yearFilter, yearArgs...).Count(&totalCount)
	config.DB.Model(&models.MouRecord{}).
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND (mou_status.name LIKE ? OR mou_status.name LIKE ?)"+yearFilter, append([]interface{}{"%มีผล%", "%ใกล้หมดอายุ%"}, yearArgs...)...).
		Count(&activeCount)
	config.DB.Model(&models.MouRecord{}).
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND (mou_status.name LIKE ? OR mou_status.name LIKE ?) AND mou_records.end_date IS NOT NULL AND mou_records.end_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY) AND mou_records.end_date >= CURDATE()"+yearFilter, append([]interface{}{"%มีผล%", "%ใกล้หมดอายุ%"}, yearArgs...)...).
		Count(&nearExpiryCount)
	config.DB.Model(&models.MouRecord{}).
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND (mou_status.name LIKE ?)"+yearFilter, append([]interface{}{"%หมดอายุ%"}, yearArgs...)...).
		Count(&expiredCount)
	config.DB.Model(&models.MouRecord{}).
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND (mou_status.name LIKE ?)"+yearFilter, append([]interface{}{"%รอดำเนินการ%"}, yearArgs...)...).
		Count(&pendingCount)
	config.DB.Model(&models.MouRecord{}).
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND (mou_status.name LIKE ?)"+yearFilter, append([]interface{}{"%ยกเลิก%"}, yearArgs...)...).
		Count(&cancelledCount)
	config.DB.Model(&models.MouRecord{}).
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND (mou_status.name LIKE ?)"+yearFilter, append([]interface{}{"%ร่าง%"}, yearArgs...)...).
		Count(&draftCount)
	config.DB.Model(&models.MouRecord{}).
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND (mou_status.name LIKE ?)"+yearFilter, append([]interface{}{"%ต่ออายุ%"}, yearArgs...)...).
		Count(&renewedCount)

	// Active MOUs for the selected year
	var activeMous []models.MouRecord
	activeQuery := config.DB.Preload("Status").Preload("Partners").Preload("MouType").
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND (mou_status.name LIKE ? OR mou_status.name LIKE ?)", "%มีผล%", "%ใกล้หมดอายุ%")
	if yearParam != "" {
		y, err := strconv.Atoi(yearParam)
		if err == nil {
			greg := y
			if y > 2155 {
				greg = y - 543
			}
			activeQuery = activeQuery.Where("mou_records.start_date <= ? AND (mou_records.end_date IS NULL OR mou_records.end_date >= ?)",
				fmt.Sprintf("%d-12-31", greg), fmt.Sprintf("%d-01-01", greg))
		}
	}
	activeQuery.Order("mou_records.end_date ASC").Limit(50).Find(&activeMous)

	// MOUs expiring within the selected year
	var expiredMous []models.MouRecord
	expiredQuery := config.DB.Preload("Status").Preload("Partners").Preload("MouType").
		Where("mou_records.deleted_at IS NULL")
	if yearParam != "" {
		y, err := strconv.Atoi(yearParam)
		if err == nil {
			greg := y
			if y > 2155 {
				greg = y - 543
			}
			expiredQuery = expiredQuery.Where("mou_records.end_date >= ? AND mou_records.end_date <= ?",
				fmt.Sprintf("%d-01-01", greg), fmt.Sprintf("%d-12-31", greg))
		}
	}
	expiredQuery.Order("mou_records.end_date DESC").Limit(20).Find(&expiredMous)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total":      totalCount,
			"active":     activeCount,
			"nearExpiry": nearExpiryCount,
			"expired":    expiredCount,
			"pending":    pendingCount,
			"cancelled":  cancelledCount,
			"draft":      draftCount,
			"renewed":    renewedCount,
		},
		"activeMous":  activeMous,
		"expiredMous": expiredMous,
	})
}

// CreateMouActivity creates a new activity under an MOU
func CreateMouActivity(c *gin.Context) {
	var req models.CreateMouActivityRequest
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		dataStr := c.PostForm("data")
		if dataStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'data' field in multipart form"})
			return
		}
		if err := json.Unmarshal([]byte(dataStr), &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON in 'data' field: " + err.Error()})
			return
		}
	} else {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Verify MOU exists
	var mou models.MouRecord
	if err := config.DB.Where("id = ? AND deleted_at IS NULL", req.MouID).First(&mou).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "MOU not found"})
		return
	}

	// Verify activity types exist
	var actTypes []models.MouActivityType
	if err := config.DB.Where("id IN ? AND is_active = ?", req.ActivityTypeIDs, true).Find(&actTypes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify activity types"})
		return
	}
	if len(actTypes) != len(req.ActivityTypeIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "One or more activity types not found"})
		return
	}

	// Parse dates
	activityStart, err := parseDateString(req.ActivityStart)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid activity_start format. Use DD/MM/YYYY"})
		return
	}
	activityEnd, err := parseDateString(req.ActivityEnd)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid activity_end format. Use DD/MM/YYYY"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID"})
		return
	}

	firstTypeID := actTypes[0].ID
	activity := models.MouActivity{
		MouID:           req.MouID,
		Title:           req.Title,
		ActivityTypeID:  &firstTypeID,
		ActivityStart:   activityStart,
		ActivityEnd:     activityEnd,
		Location:        req.Location,
		ParticipantCount: req.ParticipantCount,
		Objective:       req.Objective,
		Description:     req.Description,
		Plan:            req.Plan,
		Notes:           req.Notes,
		CoordinatorOther: req.CoordinatorOther,
		CoordinatorOrg:  req.CoordinatorOrg,
		CreatedBy:       uid,
	}

	if req.CoordinatorID != nil {
		activity.CoordinatorID = req.CoordinatorID
	}
	if req.CoordinatorOther != "" {
		activity.CoordinatorOther = req.CoordinatorOther
	}

	tx := config.DB.Begin()

	if err := tx.Create(&activity).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create activity"})
		return
	}

	// Link activity types (many-to-many)
	for _, at := range actTypes {
		if err := tx.Exec("INSERT INTO mou_activity_activity_type (activity_id, activity_type_id) VALUES (?, ?)", activity.ID, at.ID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link activity types"})
			return
		}
	}

	// Link OKRs (many-to-many)
	for _, okrID := range req.OKRIDs {
		if okrID > 0 {
			if err := tx.Exec("INSERT INTO mou_activity_okr (activity_id, okr_id) VALUES (?, ?)", activity.ID, okrID).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link OKRs"})
				return
			}
		}
	}

	tx.Commit()

	// Handle file uploads from multipart form
	if strings.HasPrefix(contentType, "multipart/form-data") {
		form, err := c.MultipartForm()
		if err == nil {
			uploadFiles := form.File["files"]
			uploadDir := filepath.Join("uploads", "activity")
			if err := os.MkdirAll(uploadDir, 0755); err == nil {
				for _, f := range uploadFiles {
					savePath := filepath.Join(uploadDir, fmt.Sprintf("%d_%s", activity.ID, f.Filename))
					if err := c.SaveUploadedFile(f, savePath); err == nil {
						attachment := models.MouActivityAttachment{
							ActivityID: activity.ID,
							FileName:   f.Filename,
							FilePath:   savePath,
							MimeType:   f.Header.Get("Content-Type"),
							UploadedBy: uid,
						}
						config.DB.Create(&attachment)
					}
				}
			}
		}
	}

	// Reload with relations
	config.DB.Preload("ActivityType").Preload("ActivityTypes").Preload("Okrs").Preload("Creator").First(&activity, activity.ID)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Activity created successfully",
		"data":    activity,
	})
}

// GetMouActivity retrieves a single activity by ID
func GetMouActivity(c *gin.Context) {
	id := c.Param("id")

	var activity models.MouActivity
	if err := config.DB.
		Preload("ActivityType").
		Preload("ActivityTypes").
		Preload("Coordinator").
		Preload("Creator").
		Preload("Updater").
		Preload("Okrs").
		Preload("Attachments").
		Preload("Mou").
		Where("deleted_at IS NULL").
		First(&activity, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Activity not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    activity,
	})
}

// UpdateMouActivity updates an existing activity
func UpdateMouActivity(c *gin.Context) {
	id := c.Param("id")

	var req models.UpdateMouActivityRequest
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		dataStr := c.PostForm("data")
		if dataStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'data' field in multipart form"})
			return
		}
		if err := json.Unmarshal([]byte(dataStr), &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON in 'data' field: " + err.Error()})
			return
		}
	} else {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	var activity models.MouActivity
	if err := config.DB.Where("deleted_at IS NULL").First(&activity, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Activity not found"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID"})
		return
	}

	// Parse dates (keep existing if empty)
	if req.ActivityStart != "" {
		activityStart, err := parseDateString(req.ActivityStart)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid activity_start format. Use DD/MM/YYYY"})
			return
		}
		activity.ActivityStart = activityStart
	}
	if req.ActivityEnd != "" {
		activityEnd, err := parseDateString(req.ActivityEnd)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid activity_end format. Use DD/MM/YYYY"})
			return
		}
		activity.ActivityEnd = activityEnd
	}

	activity.Title = req.Title
	activity.Location = req.Location
	activity.ParticipantCount = req.ParticipantCount
	activity.Objective = req.Objective
	activity.Description = req.Description
	activity.Plan = req.Plan
	activity.Notes = req.Notes
	activity.CoordinatorOther = req.CoordinatorOther
	activity.CoordinatorOrg = req.CoordinatorOrg
	activity.UpdatedBy = &uid

	if req.CoordinatorID != nil {
		activity.CoordinatorID = req.CoordinatorID
	} else {
		activity.CoordinatorID = nil
	}
	if req.CoordinatorOther != "" {
		activity.CoordinatorOther = req.CoordinatorOther
	} else if req.CoordinatorID != nil {
		activity.CoordinatorOther = ""
	}

	tx := config.DB.Begin()

	if err := tx.Save(&activity).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update activity"})
		return
	}

	// Update activity types (many-to-many)
	if len(req.ActivityTypeIDs) > 0 {
		if err := tx.Exec("DELETE FROM mou_activity_activity_type WHERE activity_id = ?", activity.ID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear activity types"})
			return
		}
		for _, atID := range req.ActivityTypeIDs {
			if err := tx.Exec("INSERT INTO mou_activity_activity_type (activity_id, activity_type_id) VALUES (?, ?)", activity.ID, atID).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update activity types"})
				return
			}
		}
	}

	// Update OKRs (many-to-many)
	if len(req.OKRIDs) > 0 {
		if err := tx.Exec("DELETE FROM mou_activity_okr WHERE activity_id = ?", activity.ID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear OKRs"})
			return
		}
		for _, okrID := range req.OKRIDs {
			if okrID > 0 {
				if err := tx.Exec("INSERT INTO mou_activity_okr (activity_id, okr_id) VALUES (?, ?)", activity.ID, okrID).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update OKRs"})
					return
				}
			}
		}
	}

	tx.Commit()

	// Reload with relations
	config.DB.Preload("ActivityType").Preload("ActivityTypes").Preload("Coordinator").Preload("Creator").Preload("Updater").Preload("Okrs").Preload("Attachments").Preload("Mou").First(&activity, activity.ID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Activity updated successfully",
		"data":    activity,
	})
}

// DeleteMouActivity soft-deletes an activity
func DeleteMouActivity(c *gin.Context) {
	id := c.Param("id")

	var activity models.MouActivity
	if err := config.DB.Where("deleted_at IS NULL").First(&activity, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Activity not found"})
		return
	}

	if err := config.DB.Delete(&activity).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete activity"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Activity deleted successfully",
	})
}

// DeleteMouActivityAttachment soft-deletes an activity attachment
func DeleteMouActivityAttachment(c *gin.Context) {
	attachID := c.Param("attachId")

	var attach models.MouActivityAttachment
	if err := config.DB.Where("deleted_at IS NULL").First(&attach, attachID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return
	}

	if err := config.DB.Delete(&attach).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete attachment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Attachment deleted successfully",
	})
}

// GetMouPartnerTypes returns all active partner types
func GetMouPartnerTypes(c *gin.Context) {
	var types []models.MouPartnerType
	config.DB.Where("deleted_at IS NULL AND is_active = ?", true).Find(&types)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types,
	})
}

// CreateMouPartnerType creates a new partner type
func CreateMouPartnerType(c *gin.Context) {
	var req struct {
		NameTh      string  `json:"name_th" binding:"required"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pt := models.MouPartnerType{
		NameTh:      req.NameTh,
		Description: req.Description,
	}
	if err := config.DB.Create(&pt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create partner type"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": pt})
}

// UpdateMouPartnerType updates a partner type
func UpdateMouPartnerType(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var pt models.MouPartnerType
	if err := config.DB.Where("deleted_at IS NULL").First(&pt, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Partner type not found"})
		return
	}

	var req struct {
		NameTh      *string `json:"name_th"`
		Description *string `json:"description"`
		IsActive    *bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.NameTh != nil {
		updates["name_th"] = *req.NameTh
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if err := config.DB.Model(&pt).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update partner type"})
		return
	}

	config.DB.First(&pt, id)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": pt})
}

// DeleteMouPartnerType soft-deletes a partner type
func DeleteMouPartnerType(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var pt models.MouPartnerType
	if err := config.DB.Where("deleted_at IS NULL").First(&pt, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Partner type not found"})
		return
	}

	now := time.Now()
	pt.DeletedAt = &now
	if err := config.DB.Save(&pt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete partner type"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Partner type deleted"})
}

// Helper function to parse date string in DD/MM/YYYY format
func parseDateString(dateStr string) (time.Time, error) {
	// Expected format: DD/MM/YYYY
	parts := strings.Split(dateStr, "/")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid date format")
	}

	day, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, err
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, err
	}

	year, err := strconv.Atoi(parts[2])
	if err != nil {
		return time.Time{}, err
	}

	// Convert Buddhist year to Gregorian if necessary (Thai year is typically 543 years ahead)
	// For now, assuming it could be either format, detect by checking if year > 2500
	if year > 2500 {
		year = year - 543
	}

	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), nil
}

// GetMouNotifications returns MOUs that are near expiry or expired for the bell icon
func GetMouNotifications(c *gin.Context) {
	var nearExpiry []models.MouRecord
	var expired []models.MouRecord
	var nearExpiryCount, expiredCount int64

	today := time.Now().Truncate(24 * time.Hour)
	ninetyDays := today.AddDate(0, 0, 90)

	config.DB.Where("deleted_at IS NULL AND end_date IS NOT NULL AND end_date >= ? AND end_date <= ? AND (Status_id = ? OR Status_id = ?)",
		today, ninetyDays, 2, 7).
		Preload("Status").Preload("Partners").
		Order("end_date ASC").
		Find(&nearExpiry)
	nearExpiryCount = int64(len(nearExpiry))

	config.DB.Where("deleted_at IS NULL AND end_date IS NOT NULL AND end_date < ? AND (Status_id = ? OR Status_id = ?)",
		today, 2, 7).
		Preload("Status").Preload("Partners").
		Order("end_date ASC").
		Find(&expired)
	expiredCount = int64(len(expired))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"nearExpiry": gin.H{
				"count": nearExpiryCount,
				"items": nearExpiry,
			},
			"expired": gin.H{
				"count": expiredCount,
				"items": expired,
			},
			"total": nearExpiryCount + expiredCount,
		},
	})
}

// RenewMou extends an expired MOU: sets new end_date and status to "ต่ออายุ"
func RenewMou(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var mou models.MouRecord
	if err := config.DB.Where("deleted_at IS NULL").First(&mou, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "MOU not found"})
		return
	}

	var req struct {
		NewEndDate string `json:"new_end_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	parsedDate, err := parseDateString(req.NewEndDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบวันที่ไม่ถูกต้อง (ต้องเป็น DD/MM/YYYY)"})
		return
	}

	// Find or create "ต่ออายุ" status
	var renewedStatus models.MouStatus
	config.DB.Where("name LIKE ?", "%ต่ออายุ%").First(&renewedStatus)

	mou.EndDate = &parsedDate
	mou.StatusID = renewedStatus.ID
	if err := config.DB.Save(&mou).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to renew MOU"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "ต่ออายุ MOU สำเร็จ"})
}
