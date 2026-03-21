package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type accessRoleRow struct {
	RoleID          int    `json:"role_id"`
	Role            string `json:"role"`
	PermissionCount int64  `json:"permission_count"`
}

type accessPermissionRow struct {
	PermissionID int    `json:"permission_id"`
	Code         string `json:"code"`
	Resource     string `json:"resource"`
	Action       string `json:"action"`
	Description  string `json:"description"`
}

type accessRoleDetail struct {
	RoleID int    `json:"role_id"`
	Role   string `json:"role"`
}

type accessUserDetail struct {
	UserID  int    `json:"user_id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	RoleID  int    `json:"role_id"`
	Role    string `json:"role"`
	RoleKey string `json:"role_key"`
}

type accessUserOverride struct {
	Code   string `json:"code"`
	Effect string `json:"effect"`
}

type updateRolePermissionsRequest struct {
	PermissionCodes []string `json:"permission_codes"`
}

type updateUserOverridesRequest struct {
	Overrides []accessUserOverride `json:"overrides"`
}

func AdminListAccessRoles(c *gin.Context) {
	var roles []accessRoleRow
	err := config.DB.Raw(`
		SELECT r.role_id,
		       COALESCE(r.role, CONCAT('role_', r.role_id)) AS role,
		       COUNT(DISTINCT p.permission_id) AS permission_count
		FROM roles r
		LEFT JOIN role_permissions rp
			ON rp.role_id = r.role_id
			AND rp.delete_at IS NULL
		LEFT JOIN permissions p
			ON p.permission_id = rp.permission_id
			AND p.delete_at IS NULL
		WHERE r.delete_at IS NULL
		GROUP BY r.role_id, r.role
		ORDER BY r.role_id ASC
	`).Scan(&roles).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": roles})
}

func AdminListAccessPermissions(c *gin.Context) {
	var permissions []accessPermissionRow
	err := config.DB.Raw(`
		SELECT permission_id,
		       code,
		       COALESCE(resource, '') AS resource,
		       COALESCE(action, '') AS action,
		       COALESCE(description, '') AS description
		FROM permissions
		WHERE delete_at IS NULL
		ORDER BY resource ASC, action ASC, code ASC
	`).Scan(&permissions).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": permissions})
}

func AdminGetRolePermissions(c *gin.Context) {
	roleID, err := parsePositiveIntParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	role, err := getAccessRoleByID(roleID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "role not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	permissions, codes, err := getRolePermissions(roleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"role":             role,
		"permissions":      permissions,
		"permission_codes": codes,
	})
}

func AdminUpdateRolePermissions(c *gin.Context) {
	roleID, err := parsePositiveIntParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	role, err := getAccessRoleByID(roleID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "role not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var req updateRolePermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	codes := normalizeCodeList(req.PermissionCodes)
	permissionIDByCode, missingCodes, err := getPermissionIDsByCode(codes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if len(missingCodes) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "some permission codes do not exist",
			"missing": missingCodes,
		})
		return
	}

	now := time.Now()
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"UPDATE role_permissions SET delete_at = ?, update_at = ? WHERE role_id = ? AND delete_at IS NULL",
			now,
			now,
			roleID,
		).Error; err != nil {
			return err
		}

		for _, code := range codes {
			permissionID := permissionIDByCode[code]
			if err := tx.Exec(`
				INSERT INTO role_permissions (role_id, permission_id, create_at, update_at, delete_at)
				VALUES (?, ?, ?, ?, NULL)
				ON DUPLICATE KEY UPDATE
					delete_at = NULL,
					update_at = VALUES(update_at)
			`, roleID, permissionID, now, now).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	permissions, assignedCodes, err := getRolePermissions(roleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"role":             role,
		"permissions":      permissions,
		"permission_codes": assignedCodes,
		"assigned_count":   len(assignedCodes),
	})
}

func AdminGetUserPermissionOverrides(c *gin.Context) {
	userID, err := parsePositiveIntParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	user, err := getAccessUserByID(userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	overrides, err := getUserPermissionOverrides(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	authz := services.GetAuthorizationService()
	effectivePermissions, _ := authz.ResolvePermissionCodes(user.UserID, user.RoleID)

	c.JSON(http.StatusOK, gin.H{
		"success":               true,
		"user":                  user,
		"overrides":             overrides,
		"effective_permissions": effectivePermissions,
	})
}

func AdminUpdateUserPermissionOverrides(c *gin.Context) {
	userID, err := parsePositiveIntParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	user, err := getAccessUserByID(userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var req updateUserOverridesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	overrideByCode := map[string]string{}
	for _, item := range req.Overrides {
		code := strings.TrimSpace(strings.ToLower(item.Code))
		effect := strings.TrimSpace(strings.ToLower(item.Effect))
		if code == "" {
			continue
		}
		if effect != "allow" && effect != "deny" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("invalid effect '%s' for code '%s'", item.Effect, item.Code),
			})
			return
		}
		overrideByCode[code] = effect
	}

	overrideCodes := make([]string, 0, len(overrideByCode))
	for code := range overrideByCode {
		overrideCodes = append(overrideCodes, code)
	}
	overrideCodes = normalizeCodeList(overrideCodes)

	permissionIDByCode, missingCodes, err := getPermissionIDsByCode(overrideCodes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if len(missingCodes) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "some permission codes do not exist",
			"missing": missingCodes,
		})
		return
	}

	now := time.Now()
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"UPDATE user_permissions SET delete_at = ?, update_at = ? WHERE user_id = ? AND delete_at IS NULL",
			now,
			now,
			userID,
		).Error; err != nil {
			return err
		}

		for _, code := range overrideCodes {
			permissionID := permissionIDByCode[code]
			effect := overrideByCode[code]
			if err := tx.Exec(`
				INSERT INTO user_permissions (user_id, permission_id, effect, create_at, update_at, delete_at)
				VALUES (?, ?, ?, ?, ?, NULL)
				ON DUPLICATE KEY UPDATE
					effect = VALUES(effect),
					delete_at = NULL,
					update_at = VALUES(update_at)
			`, userID, permissionID, effect, now, now).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	overrides, err := getUserPermissionOverrides(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	authz := services.GetAuthorizationService()
	effectivePermissions, _ := authz.ResolvePermissionCodes(user.UserID, user.RoleID)

	c.JSON(http.StatusOK, gin.H{
		"success":               true,
		"user":                  user,
		"overrides":             overrides,
		"override_count":        len(overrides),
		"effective_permissions": effectivePermissions,
	})
}

func AdminGetUserEffectivePermissions(c *gin.Context) {
	userID, err := parsePositiveIntParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	user, err := getAccessUserByID(userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	authz := services.GetAuthorizationService()
	effectivePermissions, _ := authz.ResolvePermissionCodes(user.UserID, user.RoleID)

	c.JSON(http.StatusOK, gin.H{
		"success":               true,
		"user":                  user,
		"effective_permissions": effectivePermissions,
	})
}

func parsePositiveIntParam(c *gin.Context, paramName string) (int, error) {
	raw := strings.TrimSpace(c.Param(paramName))
	if raw == "" {
		return 0, fmt.Errorf("missing %s", paramName)
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid %s", paramName)
	}
	return value, nil
}

func normalizeCodeList(codes []string) []string {
	if len(codes) == 0 {
		return []string{}
	}
	set := map[string]struct{}{}
	out := make([]string, 0, len(codes))
	for _, item := range codes {
		code := strings.TrimSpace(strings.ToLower(item))
		if code == "" {
			continue
		}
		if _, exists := set[code]; exists {
			continue
		}
		set[code] = struct{}{}
		out = append(out, code)
	}
	return out
}

func getPermissionIDsByCode(codes []string) (map[string]int, []string, error) {
	result := map[string]int{}
	if len(codes) == 0 {
		return result, []string{}, nil
	}

	type row struct {
		PermissionID int
		Code         string
	}

	var rows []row
	err := config.DB.Raw(`
		SELECT permission_id, code
		FROM permissions
		WHERE delete_at IS NULL
		  AND code IN ?
	`, codes).Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}

	for _, item := range rows {
		code := strings.TrimSpace(strings.ToLower(item.Code))
		if code != "" {
			result[code] = item.PermissionID
		}
	}

	missing := make([]string, 0)
	for _, code := range codes {
		if _, exists := result[code]; !exists {
			missing = append(missing, code)
		}
	}

	return result, missing, nil
}

func getAccessRoleByID(roleID int) (accessRoleDetail, error) {
	var role accessRoleDetail
	err := config.DB.Raw(`
		SELECT role_id, COALESCE(role, CONCAT('role_', role_id)) AS role
		FROM roles
		WHERE role_id = ?
		  AND delete_at IS NULL
		LIMIT 1
	`, roleID).Scan(&role).Error
	if err != nil {
		return accessRoleDetail{}, err
	}
	if role.RoleID == 0 {
		return accessRoleDetail{}, gorm.ErrRecordNotFound
	}
	return role, nil
}

func getRolePermissions(roleID int) ([]accessPermissionRow, []string, error) {
	var permissions []accessPermissionRow
	err := config.DB.Raw(`
		SELECT p.permission_id,
		       p.code,
		       COALESCE(p.resource, '') AS resource,
		       COALESCE(p.action, '') AS action,
		       COALESCE(p.description, '') AS description
		FROM role_permissions rp
		INNER JOIN permissions p ON p.permission_id = rp.permission_id
		WHERE rp.role_id = ?
		  AND rp.delete_at IS NULL
		  AND p.delete_at IS NULL
		ORDER BY p.resource ASC, p.action ASC, p.code ASC
	`, roleID).Scan(&permissions).Error
	if err != nil {
		return nil, nil, err
	}

	codes := make([]string, 0, len(permissions))
	for _, item := range permissions {
		code := strings.TrimSpace(strings.ToLower(item.Code))
		if code != "" {
			codes = append(codes, code)
		}
	}

	return permissions, codes, nil
}

func getAccessUserByID(userID int) (accessUserDetail, error) {
	type row struct {
		UserID    int
		Email     *string
		UserFname *string
		UserLname *string
		RoleID    int
		Role      *string
	}

	var data row
	err := config.DB.Raw(`
		SELECT u.user_id,
		       u.email,
		       u.user_fname,
		       u.user_lname,
		       COALESCE(u.role_id, 0) AS role_id,
		       r.role
		FROM users u
		LEFT JOIN roles r ON r.role_id = u.role_id
		WHERE u.user_id = ?
		  AND u.delete_at IS NULL
		LIMIT 1
	`, userID).Scan(&data).Error
	if err != nil {
		return accessUserDetail{}, err
	}
	if data.UserID == 0 {
		return accessUserDetail{}, gorm.ErrRecordNotFound
	}

	nameParts := make([]string, 0, 2)
	if data.UserFname != nil {
		if v := strings.TrimSpace(*data.UserFname); v != "" {
			nameParts = append(nameParts, v)
		}
	}
	if data.UserLname != nil {
		if v := strings.TrimSpace(*data.UserLname); v != "" {
			nameParts = append(nameParts, v)
		}
	}
	name := strings.TrimSpace(strings.Join(nameParts, " "))

	email := ""
	if data.Email != nil {
		email = strings.TrimSpace(*data.Email)
	}

	roleName := ""
	if data.Role != nil {
		roleName = strings.TrimSpace(*data.Role)
	}
	roleKey := strings.ToLower(roleName)

	return accessUserDetail{
		UserID:  data.UserID,
		Email:   email,
		Name:    name,
		RoleID:  data.RoleID,
		Role:    roleName,
		RoleKey: roleKey,
	}, nil
}

func getUserPermissionOverrides(userID int) ([]accessUserOverride, error) {
	type row struct {
		Code   string
		Effect string
	}

	var rows []row
	err := config.DB.Raw(`
		SELECT p.code, up.effect
		FROM user_permissions up
		INNER JOIN permissions p ON p.permission_id = up.permission_id
		WHERE up.user_id = ?
		  AND up.delete_at IS NULL
		  AND p.delete_at IS NULL
		ORDER BY p.code ASC
	`, userID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	overrides := make([]accessUserOverride, 0, len(rows))
	for _, item := range rows {
		code := strings.TrimSpace(strings.ToLower(item.Code))
		effect := strings.TrimSpace(strings.ToLower(item.Effect))
		if code == "" || (effect != "allow" && effect != "deny") {
			continue
		}
		overrides = append(overrides, accessUserOverride{
			Code:   code,
			Effect: effect,
		})
	}

	return overrides, nil
}
