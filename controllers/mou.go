package controllers

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
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

	title := c.DefaultQuery("title", "")
	mouCode := c.DefaultQuery("mou_code", "")
	partnerName := c.DefaultQuery("partner_name", "")
	country := c.DefaultQuery("country", "")
	status := c.DefaultQuery("status", "")
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
	if title != "" {
		query = query.Where("title LIKE ?", "%"+title+"%")
	}
	if mouCode != "" {
		query = query.Where("mou_code LIKE ?", "%"+mouCode+"%")
	}
	if partnerName != "" {
		subQuery := config.DB.Table("mou_partner").Select("mou_id").Where("partner_org LIKE ?", "%"+partnerName+"%")
		query = query.Where("mou_records.id IN (?)", subQuery)
	}
	if country != "" {
		query = query.Joins("LEFT JOIN countries ON countries.id = mou_records.Country_id").
			Where("countries.name_th LIKE ? OR countries.name_en LIKE ?", "%"+country+"%", "%"+country+"%")
	}

	var total int64
	query.Model(&models.MouRecord{}).Count(&total)

	offset := (page - 1) * limit
	err := query.
		Preload("Status").
		Preload("Coordinator").
		Preload("Partners.PartnerType").
		Preload("Faculties.Faculty").
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Find(&mous).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch MOU records"})
		return
	}

	// Manually load countries
	var countryIDs []int
	for _, m := range mous {
		if m.CountryID != nil {
			countryIDs = append(countryIDs, *m.CountryID)
		}
	}
	if len(countryIDs) > 0 {
		var countries []models.Country
		config.DB.Find(&countries, countryIDs)
		countryMap := make(map[int]*models.Country)
		for i := range countries {
			countryMap[countries[i].ID] = &countries[i]
		}
		for i := range mous {
			if mous[i].CountryID != nil {
				if c, ok := countryMap[*mous[i].CountryID]; ok {
					mous[i].Country = c
				}
			}
		}
	}

	// Stats queries using raw SQL to avoid GORM chain conflicts
	statsWhere := "mou_records.deleted_at IS NULL"
	var statsArgs []interface{}
	if status != "" {
		statsWhere += " AND mou_records.Status_id = ?"
		statsArgs = append(statsArgs, status)
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
		countryJoin = "LEFT JOIN countries ON countries.id = mou_records.Country_id"
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
		Preload("Coordinator").
		Preload("Partners.PartnerType").
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

	// Load MOU events from mou_notification_log
	var mouEvents []models.MouNotificationLog
	config.DB.Where("mou_id = ?", mou.ID).Order("sent_at DESC").Preload("Actor").Find(&mouEvents)

	// Load country via raw JOIN (bypasses GORM *int foreign key mapping issues)
	var countryNameTh, countryNameEn string
	config.DB.Raw(`
		SELECT c.name_th, c.name_en
		FROM mou_records mr
		LEFT JOIN countries c ON c.id = mr.Country_id
		WHERE mr.id = ?
	`, id).Row().Scan(&countryNameTh, &countryNameEn)
	if countryNameTh != "" {
		mou.Country = &models.Country{
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

	zipName := fmt.Sprintf("mou_%d_attachments.zip", mou.ID)
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

	if req.YearOfSigning != "" {
		if _, err := time.Parse("2006-01-02", req.YearOfSigning); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year_of_signing format. Use YYYY-MM-DD"})
			return
		}
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
		Title:            req.Title,
		Description:      req.Description,
		StatusID:         statusID,
		Level:            req.Level,
		StartDate:        startDate,
		EndDate:          endDate,
		CountryID:        req.CountryID,
		IsInternational:  req.IsInternational,
		CoordinatorID:    req.CoordinatorID,
		Notes:            req.Notes,
		NotifyDaysBefore: req.NotifyDaysBefore,
		CreatedBy:        uid,
	}

	if req.SignedBy != nil {
		mou.SignedBy = *req.SignedBy
	}

	if req.YearOfSigning != "" {
		parsed, _ := time.Parse("2006-01-02", req.YearOfSigning)
		mou.YearOfSigning = &parsed
	}

	if err := config.DB.Create(&mou).Error; err != nil {
		fmt.Println("Error creating MOU:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create MOU"})
		return
	}

	// Auto-generate mou_code if not provided
	if req.MouCode != "" {
		config.DB.Model(&mou).Update("mou_code", req.MouCode)
	} else {
		mouCode := fmt.Sprintf("MOU-%06d", mou.ID)
		config.DB.Model(&mou).Update("mou_code", mouCode)
		mou.MouCode = mouCode
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
			if f.Email != nil {
				mouFaculty.Email = f.Email
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

	// Log creation event
	msg := fmt.Sprintf("สร้าง %s", mou.MouCode)
	config.DB.Create(&models.MouNotificationLog{
		MouID:   mou.ID,
		Action:  "สร้าง MOU",
		ActorID: uid,
		SentTo:  uid,
		Channel: "system",
		Success: true,
		Message: &msg,
	})

	// Reload with associations
	config.DB.Preload("Status").
		Preload("Country").
		Preload("Coordinator").
		Preload("Partners.PartnerType").
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

	// Validate when changing to Active: require year_of_signing + signed_by
	if req.StatusID != nil && *req.StatusID == 2 {
		if mou.YearOfSigning == nil &&
			(req.YearOfSigning == nil || *req.YearOfSigning == "") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณากรอกวันเดือนปีที่ลงนามก่อนเปลี่ยนสถานะเป็น 'มีผลบังคับใช้'"})
			return
		}
		if mou.SignedBy == "" &&
			(req.SignedBy == nil || *req.SignedBy == "") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณากรอกผู้ลงนามก่อนเปลี่ยนสถานะเป็น 'มีผลบังคับใช้'"})
			return
		}
	}

	// Log status change
	logStatusChange := false
	var oldStatusID int
	if req.StatusID != nil {
		oldStatusID = mou.StatusID
		if oldStatusID != *req.StatusID {
			logStatusChange = true
		}
	}

	// Update fields if provided
	if req.Title != nil {
		mou.Title = *req.Title
	}
	if req.Description != nil {
		mou.Description = *req.Description
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
		if *req.YearOfSigning != "" {
			parsed, err := time.Parse("2006-01-02", *req.YearOfSigning)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year_of_signing format. Use YYYY-MM-DD"})
				return
			}
			mou.YearOfSigning = &parsed
		} else {
			mou.YearOfSigning = nil
		}
	}
	if req.NotifyDaysBefore != nil {
		mou.NotifyDaysBefore = req.NotifyDaysBefore
		config.DB.Model(&models.MouNotification{}).Where("mou_id = ?", mou.ID).Update("days_before", *req.NotifyDaysBefore)
	}
	if req.SignedBy != nil {
		mou.SignedBy = *req.SignedBy
	}
	if req.Notes != nil {
		mou.Notes = *req.Notes
	}
	if req.MouCode != nil {
		mou.MouCode = *req.MouCode
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

	// Log status change
	if logStatusChange {
		description := fmt.Sprintf("เปลี่ยนสถานะจาก %d เป็น %d", oldStatusID, mou.StatusID)
		var oldStatus, newStatus models.MouStatus
		config.DB.First(&oldStatus, oldStatusID)
		config.DB.First(&newStatus, mou.StatusID)
		if oldStatus.ID != 0 && newStatus.ID != 0 {
			description = fmt.Sprintf("เปลี่ยนสถานะจาก %s เป็น %s", oldStatus.Name, newStatus.Name)
		}
		var userID int
		if uid, exists := c.Get("userID"); exists {
			userID, _ = uid.(int)
		}
		desc := description
		config.DB.Create(&models.MouNotificationLog{
			MouID:   mou.ID,
			Action:  "เปลี่ยนสถานะ MOU",
			ActorID: userID,
			SentTo:  userID,
			Channel: "system",
			Success: true,
			Message: &desc,
		})
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
			if f.Email != nil {
				mouFaculty.Email = f.Email
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
		Preload("Country").
		Preload("Coordinator").
		Preload("Partners.PartnerType").
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
	activeQuery := config.DB.Preload("Status").Preload("Partners").
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
	expiredQuery := config.DB.Preload("Status").Preload("Partners").
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

	// Active MOUs per year (MOUs in effect during each year)
	type YearlyCount struct {
		Year  int `json:"year"`
		Count int `json:"count"`
	}
	var yearlyData []YearlyCount

	var minYear, maxYear int
	config.DB.Raw("SELECT COALESCE(MIN(YEAR(start_date)), YEAR(CURDATE())) FROM mou_records WHERE deleted_at IS NULL").Scan(&minYear)
	config.DB.Raw("SELECT COALESCE(MAX(YEAR(COALESCE(end_date, CURDATE()))), YEAR(CURDATE())) FROM mou_records WHERE deleted_at IS NULL").Scan(&maxYear)
	if maxYear < time.Now().Year() {
		maxYear = time.Now().Year()
	}
	maxYear++

	var mouDateRanges []struct {
		StartDate time.Time
		EndDate   *time.Time
	}
	config.DB.Table("mou_records").
		Select("mou_records.start_date, mou_records.end_date").
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND (mou_status.name LIKE ? OR mou_status.name LIKE ?)", "%มีผล%", "%ใกล้หมดอายุ%").
		Find(&mouDateRanges)
	for y := minYear; y <= maxYear; y++ {
		count := 0
		for _, m := range mouDateRanges {
			if m.StartDate.Year() <= y && (m.EndDate == nil || m.EndDate.Year() >= y) {
				count++
			}
		}
		if count > 0 {
			yearlyData = append(yearlyData, YearlyCount{Year: y, Count: count})
		}
	}

	// MOUs that are expired (exclude "near expiry")
	var renewMous []models.MouRecord
	config.DB.Preload("Status").Preload("Partners").
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND mou_status.name NOT LIKE ? AND mou_status.name LIKE ?", "%ใกล้%", "%หมดอายุ%").
		Order("mou_records.end_date ASC").
		Limit(20).
		Find(&renewMous)

	// MOUs near expiry
	var nearExpiryMous []models.MouRecord
	config.DB.Preload("Status").Preload("Partners").
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL AND mou_status.name LIKE ?", "%ใกล้หมดอายุ%").
		Order("mou_records.end_date ASC").
		Limit(20).
		Find(&nearExpiryMous)

	// Recent activities
	var recentActivities []models.MouNotificationLog
	config.DB.Order("sent_at DESC").Limit(15).Preload("Actor").Preload("Mou").Find(&recentActivities)

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
		"activeMous":       activeMous,
		"expiredMous":      expiredMous,
		"yearlyData":       yearlyData,
		"recentActivities": recentActivities,
		"renewMous":        renewMous,
		"nearExpiryMous":   nearExpiryMous,
	})
}

// GetMouActiveByYear returns MOUs active in a given year
func GetMouActiveByYear(c *gin.Context) {
	yearStr := c.Query("year")
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid year"})
		return
	}

	greg := year
	if year > 2155 {
		greg = year - 543
	}

	var mous []models.MouRecord
	config.DB.Preload("Status").Preload("Partners").
		Joins("LEFT JOIN mou_status ON mou_status.id = mou_records.Status_id").
		Where("mou_records.deleted_at IS NULL").
		Where("mou_status.name NOT LIKE ? AND mou_status.name NOT LIKE ? AND mou_status.name NOT LIKE ? AND mou_status.name NOT LIKE ?", "%ร่าง%", "%รอดำเนินการ%", "%ต่ออายุ%", "%ยกเลิก%").
		Where("YEAR(mou_records.start_date) <= ?", greg).
		Where("mou_records.end_date IS NULL OR YEAR(mou_records.end_date) >= ?", greg).
		Order("mou_records.start_date ASC").
		Find(&mous)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": mous})
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

// ExportMouCsv exports filtered MOU list as CSV file
func ExportMouCsv(c *gin.Context) {
	title := c.DefaultQuery("title", "")
	partnerName := c.DefaultQuery("partner_name", "")
	country := c.DefaultQuery("country", "")
	status := c.DefaultQuery("status", "")
	level := c.DefaultQuery("level", "")
	isInternational := c.DefaultQuery("is_international", "")

	var mous []models.MouRecord
	query := config.DB.Where("mou_records.deleted_at IS NULL")

	if status != "" {
		query = query.Where("Status_id = ?", status)
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
	if title != "" {
		query = query.Where("title LIKE ?", "%"+title+"%")
	}
	if partnerName != "" {
		subQuery := config.DB.Table("mou_partner").Select("mou_id").Where("partner_org LIKE ?", "%"+partnerName+"%")
		query = query.Where("mou_records.id IN (?)", subQuery)
	}
	if country != "" {
		query = query.Joins("LEFT JOIN countries ON countries.id = mou_records.Country_id").
			Where("countries.name_th LIKE ? OR countries.name_en LIKE ?", "%"+country+"%", "%"+country+"%")
	}

	err := query.
		Preload("Status").
		Preload("Coordinator").
		Preload("Partners").
		Order("created_at DESC").
		Find(&mous).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to export MOU records"})
		return
	}

	// Manually load countries for each MOU
	var countryIDs []int
	for i := range mous {
		if mous[i].CountryID != nil {
			countryIDs = append(countryIDs, *mous[i].CountryID)
		}
	}
	if len(countryIDs) > 0 {
		var countries []models.Country
		config.DB.Find(&countries, countryIDs)
		countryMap := make(map[int]*models.Country)
		for i := range countries {
			countryMap[countries[i].ID] = &countries[i]
		}
		for i := range mous {
			if mous[i].CountryID != nil {
				if c, ok := countryMap[*mous[i].CountryID]; ok {
					mous[i].Country = c
				}
			}
		}
	}

	var buf bytes.Buffer
	// Write BOM for Excel compatibility
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(&buf)

	// Header
	writer.Write([]string{
		"รหัส MOU",
		"ชื่อโครงการ",
		"ประเภท MOU",
		"ระดับ",
		"สถานะ",
		"ประเทศ",
		"ผู้ประสานงาน",
		"วันที่เริ่มต้น",
		"วันที่สิ้นสุด",
		"ปีที่ลงนาม",
		"คู่ความร่วมมือ",
		"ระหว่างประเทศ",
	})

	for _, mou := range mous {
		partnerOrgs := ""
		for i, p := range mou.Partners {
			if i > 0 {
				partnerOrgs += "; "
			}
			partnerOrgs += p.PartnerOrg
		}
		coordinatorName := ""
		if mou.CoordinatorID != nil && *mou.CoordinatorID > 0 && mou.Coordinator.UserID > 0 {
			coordinatorName = mou.Coordinator.UserFname + " " + mou.Coordinator.UserLname
		}
		countryName := ""
		if mou.Country != nil {
			countryName = mou.Country.NameTh
		}
		intlFlag := "ไม่"
		if mou.IsInternational {
			intlFlag = "ใช่"
		}
		endDate := ""
		if mou.EndDate != nil {
			endDate = mou.EndDate.Format("02/01/2006")
		}
		startDate := ""
		if !mou.StartDate.IsZero() {
			startDate = mou.StartDate.Format("02/01/2006")
		}
		yearSign := ""
		if mou.YearOfSigning != nil {
			yearSign = mou.YearOfSigning.Format("02/01/2006")
		}

		writer.Write([]string{
			mou.Title,
			mou.Title,
			mou.Level,
			mou.Status.Name,
			countryName,
			coordinatorName,
			startDate,
			endDate,
			yearSign,
			partnerOrgs,
			intlFlag,
		})
	}
	writer.Flush()

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=mou_export_"+time.Now().Format("20060102_150405")+".csv")
	c.String(http.StatusOK, buf.String())
}

// GetMouNotificationRecipients returns users assigned to a specific notification
func GetMouNotificationRecipients(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var notif models.MouNotification
	if err := config.DB.Preload("Staff").First(&notif, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    notif.Staff,
	})
}

// ListMouNotificationRecipients returns all potential recipients grouped by type
func ListMouNotificationRecipients(c *gin.Context) {
	var recipients []models.NotificationRecipient

	// Coordinators (users assigned as coordinator in any MOU)
	var coordinators []struct {
		ID    int
		Fname string
		Lname string
		Email string
	}
	config.DB.Table("mou_records").
		Select("DISTINCT u.user_id as id, u.user_fname as fname, u.user_lname as lname, u.email").
		Joins("JOIN users u ON u.user_id = mou_records.coordinator_id").
		Where("mou_records.deleted_at IS NULL").
		Where("u.Is_active = ?", "A").
		Find(&coordinators)
	for _, u := range coordinators {
		recipients = append(recipients, models.NotificationRecipient{
			ID:   u.ID,
			Name: u.Fname + " " + u.Lname,
			Email: u.Email,
			Type: "coordinator",
		})
	}

	// Faculty responsible persons (users linked via mou_faculties.user_id)
	var facultyStaff []struct {
		ID          int
		Fname       string
		Lname       string
		Email       string
		FacultyName string
	}
	config.DB.Table("mou_faculties").
		Select("u.user_id as id, u.user_fname as fname, u.user_lname as lname, u.email, f.name_th as faculty_name").
		Joins("JOIN faculties f ON f.id = mou_faculties.faculty_id").
		Joins("JOIN users u ON u.user_id = mou_faculties.user_id").
		Where("u.Is_active = ?", "A").
		Where("mou_faculties.user_id IS NOT NULL").
		Find(&facultyStaff)
	for _, u := range facultyStaff {
		recipients = append(recipients, models.NotificationRecipient{
			ID:          u.ID,
			Name:        u.Fname + " " + u.Lname,
			Email:       u.Email,
			Type:        "faculty",
			FacultyName: u.FacultyName,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    recipients,
	})
}

// GetMouNotificationPreview returns HTML preview of the notification email
func GetMouNotificationPreview(c *gin.Context) {
	mouIDStr := c.Query("mou_id")
	if mouIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "mou_id is required"})
		return
	}

	var mou models.MouRecord
	if err := config.DB.Preload("Partners").
		Preload("Faculties").
		Preload("Status").
		First(&mou, mouIDStr).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "MOU not found"})
		return
	}

	var setting models.MouNotificationSetting
	config.DB.First(&setting, 1)

	if v := c.Query("include_mou_code"); v != "" {
		setting.IncludeMouCode = v == "true"
	}
	if v := c.Query("include_title"); v != "" {
		setting.IncludeTitle = v == "true"
	}
	if v := c.Query("include_partner"); v != "" {
		setting.IncludePartner = v == "true"
	}
	if v := c.Query("include_dates"); v != "" {
		setting.IncludeDates = v == "true"
	}
	if v := c.Query("include_level"); v != "" {
		setting.IncludeLevel = v == "true"
	}
	if v := c.Query("include_status"); v != "" {
		setting.IncludeStatus = v == "true"
	}

	var meta []emailMetaItem
	if setting.IncludeMouCode && mou.MouCode != "" {
		meta = append(meta, emailMetaItem{Label: "รหัส MOU", Value: mou.MouCode})
	}
	if setting.IncludeTitle && mou.Title != "" {
		meta = append(meta, emailMetaItem{Label: "ชื่อ MOU", Value: mou.Title})
	}
	if setting.IncludePartner {
		var partners []string
		for _, p := range mou.Partners {
			if p.PartnerOrg != "" {
				partners = append(partners, p.PartnerOrg)
			}
		}
		if len(partners) > 0 {
			meta = append(meta, emailMetaItem{Label: "คู่ความร่วมมือ", Value: strings.Join(partners, ", ")})
		}
	}
	if setting.IncludeDates {
		if !mou.StartDate.IsZero() && mou.EndDate != nil {
			meta = append(meta, emailMetaItem{
				Label: "วันที่เริ่มต้น-สิ้นสุด",
				Value: mou.StartDate.Format("02/01/2006") + " - " + mou.EndDate.Format("02/01/2006"),
			})
		}
	}
	if setting.IncludeLevel && mou.Level != "" {
		levelLabel := map[string]string{"university": "มหาวิทยาลัย", "faculty": "คณะ"}[mou.Level]
		if levelLabel == "" {
			levelLabel = mou.Level
		}
		meta = append(meta, emailMetaItem{Label: "ระดับความร่วมมือ", Value: levelLabel})
	}
	if setting.IncludeStatus {
		meta = append(meta, emailMetaItem{Label: "สถานะ", Value: mou.Status.Name})
	}

	subject := "แจ้งเตือน MOU ใกล้หมดอายุ"
	paragraphs := []string{
		fmt.Sprintf("MOU เรื่อง \"%s\" จะหมดอายุในวันที่ %s", mou.Title, mou.EndDate.Format("02/01/2006")),
		"กรุณาดำเนินการต่ออายุหรือติดต่อผู้รับผิดชอบ",
	}
	html := buildEmailTemplate(subject, paragraphs, meta, "", "", "")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"subject": subject,
			"html":    html,
		},
	})
}

// SendMouNotifications sends pending email notifications for expiring MOUs
func SendMouNotifications(c *gin.Context) {
	today := time.Now().Truncate(24 * time.Hour)

	var setting models.MouNotificationSetting
	config.DB.First(&setting, 1)

	var notifications []models.MouNotification
	config.DB.Where("is_sent = ?", false).
		Preload("Staff").
		Preload("Mou").
		Preload("Mou.Status").
		Preload("Mou.Partners").
		Preload("Mou.Faculties").
		Find(&notifications)

	var sentCount, failedCount int
	for _, notif := range notifications {
		if notif.Mou.EndDate == nil {
			continue
		}
		notifyDate := notif.Mou.EndDate.AddDate(0, 0, -notif.DaysBefore)
		if notifyDate.After(today) {
			continue
		}

		staffEmail := notif.Staff.EmailNotification
		if staffEmail == nil || *staffEmail == "" {
			staffEmail = &notif.Staff.Email
		}
		if staffEmail == nil || *staffEmail == "" {
			continue
		}

		// Build email content based on settings
		var meta []emailMetaItem
		if setting.IncludeMouCode && notif.Mou.MouCode != "" {
			meta = append(meta, emailMetaItem{Label: "รหัส MOU", Value: notif.Mou.MouCode})
		}
		if setting.IncludeTitle && notif.Mou.Title != "" {
			meta = append(meta, emailMetaItem{Label: "ชื่อ MOU", Value: notif.Mou.Title})
		}
		if setting.IncludePartner {
			var partners []string
			for _, p := range notif.Mou.Partners {
				if p.PartnerOrg != "" {
					partners = append(partners, p.PartnerOrg)
				}
			}
			if len(partners) > 0 {
				meta = append(meta, emailMetaItem{Label: "คู่ความร่วมมือ", Value: strings.Join(partners, ", ")})
			}
		}
		if setting.IncludeDates {
			if !notif.Mou.StartDate.IsZero() && notif.Mou.EndDate != nil {
				meta = append(meta, emailMetaItem{
					Label: "วันที่เริ่มต้น-สิ้นสุด",
					Value: notif.Mou.StartDate.Format("02/01/2006") + " - " + notif.Mou.EndDate.Format("02/01/2006"),
				})
			}
		}
		if setting.IncludeLevel && notif.Mou.Level != "" {
			levelLabel := map[string]string{"university": "มหาวิทยาลัย", "faculty": "คณะ"}[notif.Mou.Level]
			if levelLabel == "" {
				levelLabel = notif.Mou.Level
			}
			meta = append(meta, emailMetaItem{Label: "ระดับความร่วมมือ", Value: levelLabel})
		}
		if setting.IncludeStatus {
			meta = append(meta, emailMetaItem{Label: "สถานะ", Value: notif.Mou.Status.Name})
		}

		subject := "แจ้งเตือน MOU ใกล้หมดอายุ"
		paragraphs := []string{
			fmt.Sprintf("เรียน %s %s", notif.Staff.UserFname, notif.Staff.UserLname),
			fmt.Sprintf("MOU เรื่อง \"%s\" จะหมดอายุในวันที่ %s", notif.Mou.Title, notif.Mou.EndDate.Format("02/01/2006")),
			"กรุณาดำเนินการต่ออายุหรือติดต่อผู้รับผิดชอบ",
		}
		body := buildEmailTemplate(subject, paragraphs, meta, "", "", "")

		err := config.SendMail([]string{*staffEmail}, subject, body)
		logMsg := ""
		success := err == nil
		if err != nil {
			logMsg = err.Error()
			failedCount++
		} else {
			sentCount++
			now := time.Now()
			notif.IsSent = true
			notif.SentAt = &now
			config.DB.Save(&notif)
		}

		notifID := notif.ID
		logEntry := models.MouNotificationLog{
			NotificationID: &notifID,
			SentTo:         notif.StaffID,
			Channel:        "email",
			Success:        success,
		}
		if logMsg != "" {
			logEntry.Message = &logMsg
		}
		config.DB.Create(&logEntry)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"sent_count":    sentCount,
		"failed_count":  failedCount,
		"message":       fmt.Sprintf("ส่งสำเร็จ %d รายการ, ล้มเหลว %d รายการ", sentCount, failedCount),
	})
}

// GetMouNotificationSetting returns the single-row notification config (creates default if missing)
func GetMouNotificationSetting(c *gin.Context) {
	var setting models.MouNotificationSetting
	err := config.DB.Preload("Updater").First(&setting, 1).Error
	if err != nil {
		setting = models.MouNotificationSetting{ID: 1}
		config.DB.Create(&setting)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": setting})
}

// UpdateMouNotificationSetting updates the notification config (partial update)
func UpdateMouNotificationSetting(c *gin.Context) {
	userID := c.GetInt("user_id")

	var req models.UpdateMouNotificationSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body"})
		return
	}

	var setting models.MouNotificationSetting
	if err := config.DB.First(&setting, 1).Error; err != nil {
		setting = models.MouNotificationSetting{ID: 1}
		config.DB.Create(&setting)
	}

	if req.DefaultDaysBefore != nil {
		setting.DefaultDaysBefore = *req.DefaultDaysBefore
	}
	if req.NotifyCoordinator != nil {
		setting.NotifyCoordinator = *req.NotifyCoordinator
	}
	if req.NotifyFacultyResponsible != nil {
		setting.NotifyFacultyResponsible = *req.NotifyFacultyResponsible
	}
	if req.NotifyExternal != nil {
		setting.NotifyExternal = *req.NotifyExternal
	}
	if req.IncludeMouCode != nil {
		setting.IncludeMouCode = *req.IncludeMouCode
	}
	if req.IncludeTitle != nil {
		setting.IncludeTitle = *req.IncludeTitle
	}
	if req.IncludePartner != nil {
		setting.IncludePartner = *req.IncludePartner
	}
	if req.IncludeDates != nil {
		setting.IncludeDates = *req.IncludeDates
	}
	if req.IncludeLevel != nil {
		setting.IncludeLevel = *req.IncludeLevel
	}
	if req.IncludeStatus != nil {
		setting.IncludeStatus = *req.IncludeStatus
	}

	setting.UpdatedBy = &userID
	config.DB.Save(&setting)
	config.DB.Preload("Updater").First(&setting, 1)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": setting})
}
