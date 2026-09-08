package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/models"
	"fund-management-api/utils"

	"github.com/gin-gonic/gin"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const (
	defaultAdminUserPageSize = 20
	maxAdminUserPageSize     = 100
)

type adminManagedUserRow struct {
	UserID            int        `gorm:"column:user_id" json:"user_id"`
	Prefix            *string    `gorm:"column:prefix" json:"prefix,omitempty"`
	UserFname         string     `gorm:"column:user_fname" json:"user_fname"`
	UserLname         string     `gorm:"column:user_lname" json:"user_lname"`
	NameEn            *string    `gorm:"column:name_en" json:"name_en,omitempty"`
	Gender            *string    `gorm:"column:gender" json:"gender,omitempty"`
	Email             string     `gorm:"column:email" json:"email"`
	EmailNotification *string    `gorm:"column:email_notification" json:"email_notification,omitempty"`
	Tel               *string    `gorm:"column:tel" json:"tel,omitempty"`
	DateOfEmployment  *time.Time `gorm:"column:date_of_employment" json:"date_of_employment,omitempty"`
	ManagePosition    *string    `gorm:"column:manage_position" json:"manage_position,omitempty"`
	LabName           *string    `gorm:"column:lab_name" json:"lab_name,omitempty"`
	Room              *string    `gorm:"column:room" json:"room,omitempty"`
	RoleID            int        `gorm:"column:role_id" json:"role_id"`
	Role              string     `gorm:"column:role" json:"role"`
	PositionID        int        `gorm:"column:position_id" json:"position_id"`
	PositionName      string     `gorm:"column:position_name" json:"position_name"`
	AccountStatus     string     `gorm:"column:account_status" json:"account_status"`
	LocalAuth         bool       `gorm:"column:local_auth" json:"local_auth"`
	SSOAuth           bool       `gorm:"column:sso_auth" json:"sso_auth"`
	LastLoginAt       *time.Time `gorm:"column:last_login_at" json:"last_login_at,omitempty"`
	CreateAt          *time.Time `gorm:"column:create_at" json:"create_at,omitempty"`
	UpdateAt          *time.Time `gorm:"column:update_at" json:"update_at,omitempty"`
}

type adminUserWriteRequest struct {
	Prefix            string `json:"prefix"`
	UserFname         string `json:"user_fname"`
	UserLname         string `json:"user_lname"`
	NameEn            string `json:"name_en"`
	Gender            string `json:"gender"`
	Email             string `json:"email"`
	EmailNotification string `json:"email_notification"`
	Tel               string `json:"tel"`
	DateOfEmployment  string `json:"date_of_employment"`
	ManagePosition    string `json:"manage_position"`
	LabName           string `json:"lab_name"`
	Room              string `json:"room"`
	RoleID            int    `json:"role_id"`
	TemporaryPassword string `json:"temporary_password"`
}

type adminUserRoleOption struct {
	RoleID int    `gorm:"column:role_id" json:"role_id"`
	Role   string `gorm:"column:role" json:"role"`
}

func AdminListManagedUsers(c *gin.Context) {
	page := parsePositiveQueryInt(c.Query("page"), 1)
	pageSize := parsePositiveQueryInt(c.Query("page_size"), defaultAdminUserPageSize)
	if pageSize > maxAdminUserPageSize {
		pageSize = maxAdminUserPageSize
	}

	query := config.DB.Table("users AS u").
		Where("u.delete_at IS NULL").
		Where("u.is_test = ?", 0)

	if search := strings.ToLower(strings.TrimSpace(c.Query("q"))); search != "" {
		like := "%" + search + "%"
		query = query.Where(`LOWER(CONCAT(COALESCE(u.prefix, ''), ' ', COALESCE(u.user_fname, ''), ' ', COALESCE(u.user_lname, ''), ' ', COALESCE(u.email, ''))) LIKE ?`, like)
	}
	if roleID := parsePositiveQueryInt(c.Query("role_id"), 0); roleID > 0 {
		query = query.Where("u.role_id = ?", roleID)
	}
	if positionID := parsePositiveQueryInt(c.Query("position_id"), 0); positionID > 0 {
		query = query.Where("u.position_id = ?", positionID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		InternalError(c, "admin_user_management: count", err)
		return
	}

	rows := make([]adminManagedUserRow, 0)
	if total > 0 {
		err := query.
			Select(adminManagedUserSelect).
			Joins("LEFT JOIN roles AS r ON r.role_id = u.role_id").
			Joins("LEFT JOIN positions AS p ON p.position_id = u.position_id").
			Order("u.user_fname ASC, u.user_lname ASC, u.user_id ASC").
			Limit(pageSize).
			Offset((page - 1) * pageSize).
			Scan(&rows).Error
		if err != nil {
			InternalError(c, "admin_user_management: list", err)
			return
		}
	}

	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rows,
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

func AdminGetManagedUserOptions(c *gin.Context) {
	roles := make([]adminUserRoleOption, 0)
	if err := config.DB.Table("roles").
		Select("role_id, role").
		Where("delete_at IS NULL").
		Order("role_id ASC").
		Scan(&roles).Error; err != nil {
		InternalError(c, "admin_user_management: roles", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"roles": roles,
		},
	})
}

func AdminCreateManagedUser(c *gin.Context) {
	adminID, ok := mustGetEditorID(c)
	if !ok {
		return
	}

	var req adminUserWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		return
	}
	req.normalize()

	employmentDate, validationError := validateAdminUserRequest(req, true)
	if validationError != nil {
		c.JSON(http.StatusBadRequest, validationError)
		return
	}
	if err := validateAdminUserReferences(config.DB, req.RoleID); err != nil {
		writeAdminUserReferenceError(c, err)
		return
	}
	if exists, err := adminUserEmailExists(config.DB, req.Email, 0); err != nil {
		InternalError(c, "admin_user_management: email check", err)
		return
	} else if exists {
		writeAdminUserEmailConflict(c)
		return
	}

	hashedPassword, err := utils.HashPassword(req.TemporaryPassword)
	if err != nil {
		InternalError(c, "admin_user_management: password", err)
		return
	}

	now := time.Now()
	active := "A"
	user := models.User{
		UserFname:         req.UserFname,
		UserLname:         req.UserLname,
		Email:             req.Email,
		Password:          &hashedPassword,
		RoleID:            req.RoleID,
		DateOfEmployment:  employmentDate,
		CreateAt:          &now,
		UpdateAt:          &now,
		Prefix:            nullableAdminUserString(req.Prefix),
		NameEn:            nullableAdminUserString(req.NameEn),
		Gender:            req.Gender,
		EmailNotification: nullableAdminUserString(req.EmailNotification),
		Tel:               nullableAdminUserString(req.Tel),
		ManagePosition:    nullableAdminUserString(req.ManagePosition),
		LabName:           nullableAdminUserString(req.LabName),
		Room:              nullableAdminUserString(req.Room),
		AccountStatus:     &active,
	}

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("PositionID").Create(&user).Error; err != nil {
			return err
		}
		if err := upsertAdminUserPrimaryRole(tx, user.UserID, req.RoleID, now, false); err != nil {
			return err
		}
		return createAdminUserAudit(tx, c, adminID, "create", user.UserID, nil, adminUserAuditValues(user))
	})
	if err != nil {
		if isDuplicateEntryError(err) {
			writeAdminUserEmailConflict(c)
			return
		}
		InternalError(c, "admin_user_management: create", err)
		return
	}

	created, err := getAdminManagedUserByID(config.DB, user.UserID)
	if err != nil {
		InternalError(c, "admin_user_management: read created user", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": created})
}

func AdminUpdateManagedUser(c *gin.Context) {
	adminID, ok := mustGetEditorID(c)
	if !ok {
		return
	}
	userID, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "รหัสผู้ใช้ไม่ถูกต้อง"})
		return
	}

	var req adminUserWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		return
	}
	req.normalize()

	employmentDate, validationError := validateAdminUserRequest(req, false)
	if validationError != nil {
		c.JSON(http.StatusBadRequest, validationError)
		return
	}
	if err := validateAdminUserReferences(config.DB, req.RoleID); err != nil {
		writeAdminUserReferenceError(c, err)
		return
	}

	var existing models.User
	if err := config.DB.Where("user_id = ? AND delete_at IS NULL", userID).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "ไม่พบผู้ใช้"})
			return
		}
		InternalError(c, "admin_user_management: find user", err)
		return
	}
	if exists, err := adminUserEmailExists(config.DB, req.Email, userID); err != nil {
		InternalError(c, "admin_user_management: email check", err)
		return
	} else if exists {
		writeAdminUserEmailConflict(c)
		return
	}

	oldValues := adminUserAuditValues(existing)
	now := time.Now()
	updates := map[string]interface{}{
		"prefix":             nullableAdminUserString(req.Prefix),
		"user_fname":         req.UserFname,
		"user_lname":         req.UserLname,
		"Name_en":            nullableAdminUserString(req.NameEn),
		"gender":             nullableAdminUserString(req.Gender),
		"email":              req.Email,
		"email_notification": nullableAdminUserString(req.EmailNotification),
		"TEL":                nullableAdminUserString(req.Tel),
		"date_of_employment": employmentDate,
		"manage_position":    nullableAdminUserString(req.ManagePosition),
		"LAB_Name":           nullableAdminUserString(req.LabName),
		"Room":               nullableAdminUserString(req.Room),
		"role_id":            req.RoleID,
		"update_at":          now,
	}

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).
			Where("user_id = ? AND delete_at IS NULL", userID).
			Updates(updates).Error; err != nil {
			return err
		}
		if existing.RoleID != req.RoleID {
			if err := upsertAdminUserPrimaryRole(tx, userID, req.RoleID, now, true); err != nil {
				return err
			}
		}

		updatedAuditValues := map[string]interface{}{
			"prefix": req.Prefix, "user_fname": req.UserFname, "user_lname": req.UserLname,
			"name_en": req.NameEn, "gender": req.Gender, "email": req.Email,
			"email_notification": req.EmailNotification, "tel": req.Tel,
			"date_of_employment": req.DateOfEmployment, "manage_position": req.ManagePosition,
			"lab_name": req.LabName, "room": req.Room, "role_id": req.RoleID,
		}
		return createAdminUserAudit(tx, c, adminID, "update", userID, oldValues, updatedAuditValues)
	})
	if err != nil {
		if isDuplicateEntryError(err) {
			writeAdminUserEmailConflict(c)
			return
		}
		InternalError(c, "admin_user_management: update", err)
		return
	}

	updated, err := getAdminManagedUserByID(config.DB, userID)
	if err != nil {
		InternalError(c, "admin_user_management: read updated user", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": updated})
}

const adminManagedUserSelect = `
	u.user_id,
	u.prefix,
	COALESCE(u.user_fname, '') AS user_fname,
	COALESCE(u.user_lname, '') AS user_lname,
	u.Name_en AS name_en,
	u.gender,
	COALESCE(u.email, '') AS email,
	u.email_notification,
	u.TEL AS tel,
	u.date_of_employment,
	u.manage_position,
	u.LAB_Name AS lab_name,
	u.Room AS room,
	COALESCE(u.role_id, 0) AS role_id,
	COALESCE(r.role, '') AS role,
	COALESCE(u.position_id, 0) AS position_id,
	COALESCE(p.position_name, '') AS position_name,
	COALESCE(u.Is_active, '') AS account_status,
	CASE WHEN u.password IS NOT NULL AND u.password <> '' THEN 1 ELSE 0 END AS local_auth,
	CASE WHEN EXISTS (
		SELECT 1 FROM auth_identities ai
		WHERE ai.user_id = u.user_id AND ai.provider = 'kku_sso' AND ai.delete_at IS NULL
	) THEN 1 ELSE 0 END AS sso_auth,
	u.last_login_at,
	u.create_at,
	u.update_at`

var (
	errAdminUserRoleNotFound = errors.New("role not found")
)

func (req *adminUserWriteRequest) normalize() {
	req.Prefix = utils.SanitizeInput(req.Prefix)
	req.UserFname = utils.SanitizeInput(req.UserFname)
	req.UserLname = utils.SanitizeInput(req.UserLname)
	req.NameEn = utils.SanitizeInput(req.NameEn)
	req.Gender = utils.SanitizeInput(req.Gender)
	req.Email = strings.ToLower(utils.SanitizeInput(req.Email))
	req.EmailNotification = strings.ToLower(utils.SanitizeInput(req.EmailNotification))
	req.Tel = utils.SanitizeInput(req.Tel)
	req.DateOfEmployment = utils.SanitizeInput(req.DateOfEmployment)
	req.ManagePosition = utils.SanitizeInput(req.ManagePosition)
	req.LabName = utils.SanitizeInput(req.LabName)
	req.Room = utils.SanitizeInput(req.Room)
	req.TemporaryPassword = utils.SanitizeInput(req.TemporaryPassword)
}

func validateAdminUserRequest(req adminUserWriteRequest, creating bool) (*time.Time, gin.H) {
	required := []struct {
		field string
		value string
	}{
		{"prefix", req.Prefix},
		{"user_fname", req.UserFname},
		{"user_lname", req.UserLname},
		{"email", req.Email},
	}
	for _, item := range required {
		if item.value == "" {
			return nil, gin.H{"success": false, "error": "กรุณากรอกข้อมูลที่จำเป็นให้ครบ", "field": item.field}
		}
	}
	if !utils.ValidateEmail(req.Email) {
		return nil, gin.H{"success": false, "error": "รูปแบบอีเมลไม่ถูกต้อง", "field": "email"}
	}
	if req.EmailNotification != "" && !utils.ValidateEmail(req.EmailNotification) {
		return nil, gin.H{"success": false, "error": "รูปแบบอีเมลแจ้งเตือนไม่ถูกต้อง", "field": "email_notification"}
	}
	maxLengths := []struct {
		field string
		value string
		max   int
	}{
		{"prefix", req.Prefix, 50}, {"user_fname", req.UserFname, 255},
		{"user_lname", req.UserLname, 255}, {"name_en", req.NameEn, 255},
		{"gender", req.Gender, 255}, {"email", req.Email, 255},
		{"email_notification", req.EmailNotification, 255}, {"tel", req.Tel, 50},
		{"manage_position", req.ManagePosition, 255}, {"lab_name", req.LabName, 255},
		{"room", req.Room, 255},
	}
	for _, item := range maxLengths {
		if len([]rune(item.value)) > item.max {
			return nil, gin.H{"success": false, "error": "ข้อมูลยาวเกินกว่าที่ระบบรองรับ", "field": item.field}
		}
	}
	if req.RoleID <= 0 {
		return nil, gin.H{"success": false, "error": "กรุณาเลือกตำแหน่ง", "field": "role_id"}
	}
	if creating {
		if valid, message := utils.ValidatePassword(req.TemporaryPassword); !valid {
			return nil, gin.H{"success": false, "error": message, "field": "temporary_password"}
		}
	}
	if req.DateOfEmployment == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", req.DateOfEmployment)
	if err != nil {
		return nil, gin.H{"success": false, "error": "รูปแบบวันเริ่มงานไม่ถูกต้อง", "field": "date_of_employment"}
	}
	return &parsed, nil
}

func validateAdminUserReferences(db *gorm.DB, roleID int) error {
	var count int64
	if err := db.Model(&models.Role{}).
		Where("role_id = ? AND delete_at IS NULL", roleID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errAdminUserRoleNotFound
	}
	return nil
}

func writeAdminUserReferenceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errAdminUserRoleNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "ไม่พบตำแหน่งที่เลือก", "field": "role_id"})
	default:
		InternalError(c, "admin_user_management: references", err)
	}
}

func adminUserEmailExists(db *gorm.DB, email string, excludeUserID int) (bool, error) {
	query := db.Unscoped().Model(&models.User{}).Where("LOWER(email) = ?", strings.ToLower(email))
	if excludeUserID > 0 {
		query = query.Where("user_id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func writeAdminUserEmailConflict(c *gin.Context) {
	c.JSON(http.StatusConflict, gin.H{"success": false, "error": "อีเมลนี้มีอยู่ในระบบแล้ว", "field": "email"})
}

func isDuplicateEntryError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func upsertAdminUserPrimaryRole(tx *gorm.DB, userID, roleID int, now time.Time, replace bool) error {
	if replace {
		if err := tx.Exec(`
			UPDATE user_roles
			SET is_primary = 0, is_active = 0, delete_at = ?, update_at = ?
			WHERE user_id = ? AND is_primary = 1 AND role_id <> ? AND delete_at IS NULL
		`, now, now, userID, roleID).Error; err != nil {
			return err
		}
	}
	return tx.Exec(`
		INSERT INTO user_roles (user_id, role_id, is_primary, is_active, create_at, update_at, delete_at)
		VALUES (?, ?, 1, 1, ?, ?, NULL)
		ON DUPLICATE KEY UPDATE
			is_primary = 1,
			is_active = 1,
			update_at = VALUES(update_at),
			delete_at = NULL
	`, userID, roleID, now, now).Error
}

func createAdminUserAudit(tx *gorm.DB, c *gin.Context, adminID int, action string, userID int, oldValues, newValues map[string]interface{}) error {
	var oldJSON, newJSON, changedFieldsJSON *string
	if oldValues != nil {
		if data, err := json.Marshal(oldValues); err == nil {
			value := string(data)
			oldJSON = &value
		}
	}
	if newValues != nil {
		if data, err := json.Marshal(newValues); err == nil {
			value := string(data)
			newJSON = &value
		}
	}
	changedFields := make([]string, 0)
	for key, newValue := range newValues {
		if oldValues == nil || !reflect.DeepEqual(oldValues[key], newValue) {
			changedFields = append(changedFields, key)
		}
	}
	sort.Strings(changedFields)
	if data, err := json.Marshal(changedFields); err == nil {
		value := string(data)
		changedFieldsJSON = &value
	}
	description := "Admin created a user account"
	if action == "update" {
		description = "Admin updated a user account"
	}
	userAgent := c.GetHeader("User-Agent")
	return tx.Create(&models.AuditLog{
		UserID: adminID, Action: action, EntityType: "user", EntityID: &userID,
		ChangedFields: changedFieldsJSON, OldValues: oldJSON, NewValues: newJSON, Description: &description,
		IPAddress: c.ClientIP(), UserAgent: &userAgent, CreatedAt: time.Now(),
	}).Error
}

func adminUserAuditValues(user models.User) map[string]interface{} {
	dateOfEmployment := ""
	if user.DateOfEmployment != nil {
		dateOfEmployment = user.DateOfEmployment.Format("2006-01-02")
	}
	return map[string]interface{}{
		"prefix": nullableAdminUserValue(user.Prefix), "user_fname": user.UserFname,
		"user_lname": user.UserLname, "name_en": nullableAdminUserValue(user.NameEn),
		"gender": user.Gender, "email": user.Email,
		"email_notification": nullableAdminUserValue(user.EmailNotification),
		"tel":                nullableAdminUserValue(user.Tel), "date_of_employment": dateOfEmployment,
		"manage_position": nullableAdminUserValue(user.ManagePosition),
		"lab_name":        nullableAdminUserValue(user.LabName), "room": nullableAdminUserValue(user.Room),
		"role_id": user.RoleID,
	}
}

func getAdminManagedUserByID(db *gorm.DB, userID int) (adminManagedUserRow, error) {
	var row adminManagedUserRow
	err := db.Table("users AS u").
		Select(adminManagedUserSelect).
		Joins("LEFT JOIN roles AS r ON r.role_id = u.role_id").
		Joins("LEFT JOIN positions AS p ON p.position_id = u.position_id").
		Where("u.user_id = ? AND u.delete_at IS NULL", userID).
		Scan(&row).Error
	if err != nil {
		return row, err
	}
	if row.UserID == 0 {
		return row, gorm.ErrRecordNotFound
	}
	return row, nil
}

func parsePositiveQueryInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func nullableAdminUserString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func nullableAdminUserValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
