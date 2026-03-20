package services

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"fund-management-api/config"

	"gorm.io/gorm"
)

type AuthorizationService struct {
	db *gorm.DB
}

var (
	authorizationServiceOnce sync.Once
	authorizationServiceInst *AuthorizationService
)

var defaultPermissionsByRole = map[int][]string{
	1: {
		"dashboard.view.self",
		"fund.request.create",
		"submission.read.own",
	},
	2: {
		"dashboard.view.self",
		"submission.read.own",
	},
	3: {
		"dashboard.view.admin",
		"report.export",
		"fund.request.create",
		"fund.request.approve",
		"submission.read.all",
		"scopus.publications.read",
		"scopus.publications.read_by_user",
		"scopus.publications.export",
		"scopus.publications.export_by_user",
	},
	4: {
		"dashboard.view.admin",
		"submission.read.department",
	},
	5: {
		"dashboard.view.admin",
		"submission.read.all",
	},
}

func GetAuthorizationService() *AuthorizationService {
	authorizationServiceOnce.Do(func() {
		authorizationServiceInst = &AuthorizationService{db: config.DB}
	})
	return authorizationServiceInst
}

func (s *AuthorizationService) ResolvePermissionCodes(userID int, roleID int) ([]string, error) {
	set := map[string]struct{}{}

	for _, code := range defaultPermissionsByRole[roleID] {
		trimmed := strings.TrimSpace(strings.ToLower(code))
		if trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}

	if !s.hasAuthorizationTables() {
		return sortedPermissionKeys(set), nil
	}

	if err := s.mergeRolePermissions(userID, roleID, set); err != nil {
		return sortedPermissionKeys(set), nil
	}

	if err := s.applyUserOverrides(userID, set); err != nil {
		return sortedPermissionKeys(set), nil
	}

	return sortedPermissionKeys(set), nil
}

func (s *AuthorizationService) HasPermission(userID int, roleID int, permissionCode string) bool {
	code := strings.TrimSpace(strings.ToLower(permissionCode))
	if code == "" {
		return false
	}

	permissions, err := s.ResolvePermissionCodes(userID, roleID)
	if err != nil {
		return false
	}

	for _, p := range permissions {
		if p == code {
			return true
		}
	}

	return false
}

func (s *AuthorizationService) hasAuthorizationTables() bool {
	required := []string{"permissions", "role_permissions"}
	for _, table := range required {
		if !s.tableExists(table) {
			return false
		}
	}
	return true
}

func (s *AuthorizationService) tableExists(tableName string) bool {
	if s.db == nil {
		return false
	}

	var count int64
	err := s.db.Raw(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
		tableName,
	).Scan(&count).Error
	if err != nil {
		return false
	}

	return count > 0
}

func (s *AuthorizationService) mergeRolePermissions(userID int, roleID int, set map[string]struct{}) error {
	if s.db == nil {
		return nil
	}

	query := `
		SELECT DISTINCT p.code
		FROM permissions p
		INNER JOIN role_permissions rp ON rp.permission_id = p.permission_id
		LEFT JOIN user_roles ur
			ON ur.role_id = rp.role_id
			AND ur.user_id = ?
			AND (ur.is_active = 1 OR ur.is_active IS NULL)
			AND ur.delete_at IS NULL
		WHERE p.delete_at IS NULL
		  AND rp.delete_at IS NULL
		  AND (rp.role_id = ? OR ur.user_id IS NOT NULL)
	`

	rows, err := s.db.Raw(query, userID, roleID).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			continue
		}
		trimmed := strings.TrimSpace(strings.ToLower(code))
		if trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}

	return rows.Err()
}

func (s *AuthorizationService) applyUserOverrides(userID int, set map[string]struct{}) error {
	if s.db == nil || !s.tableExists("user_permissions") {
		return nil
	}

	type overrideRow struct {
		Code   string
		Effect string
	}

	var rows []overrideRow
	err := s.db.Raw(`
		SELECT p.code, up.effect
		FROM user_permissions up
		INNER JOIN permissions p ON p.permission_id = up.permission_id
		WHERE up.user_id = ?
		  AND up.delete_at IS NULL
		  AND p.delete_at IS NULL
	`, userID).Scan(&rows).Error
	if err != nil {
		return err
	}

	for _, row := range rows {
		code := strings.TrimSpace(strings.ToLower(row.Code))
		effect := strings.TrimSpace(strings.ToLower(row.Effect))
		if code == "" {
			continue
		}
		switch effect {
		case "deny":
			delete(set, code)
		default:
			set[code] = struct{}{}
		}
	}

	return nil
}

func sortedPermissionKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return []string{}
	}

	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func FormatMissingPermissions(required []string) string {
	if len(required) == 0 {
		return ""
	}
	clean := make([]string, 0, len(required))
	for _, item := range required {
		v := strings.TrimSpace(strings.ToLower(item))
		if v != "" {
			clean = append(clean, v)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return fmt.Sprintf("missing required permission: %s", strings.Join(clean, ", "))
}
