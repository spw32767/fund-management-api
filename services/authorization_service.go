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
		"portal.member.access",
		"dashboard.view.self",
		"fund.request.create",
		"fund.request.update",
		"fund.request.delete",
		"publication.reward.manage_own",
		"submission.read.own",
		"ui.page.member.dashboard.view",
		"ui.page.member.profile.view",
		"ui.page.member.research_fund.view",
		"ui.page.member.promotion_fund.view",
		"ui.page.member.applications.view",
		"ui.page.member.received_funds.view",
		"ui.page.member.announcements.view",
		"ui.page.member.projects.view",
		"ui.page.member.notifications.view",
	},
	2: {
		"portal.member.access",
		"dashboard.view.self",
		"fund.request.create",
		"fund.request.update",
		"fund.request.delete",
		"publication.reward.manage_own",
		"submission.read.own",
		"ui.page.member.dashboard.view",
		"ui.page.member.profile.view",
		"ui.page.member.research_fund.view",
		"ui.page.member.promotion_fund.view",
		"ui.page.member.applications.view",
		"ui.page.member.received_funds.view",
		"ui.page.member.announcements.view",
		"ui.page.member.projects.view",
		"ui.page.member.notifications.view",
	},
	3: {
		"portal.admin.access",
		"access.view",
		"access.manage",
		"dashboard.view.admin",
		"report.export",
		"fund.request.create",
		"fund.request.update",
		"fund.request.delete",
		"fund.request.approve",
		"publication.reward.manage_own",
		"publication.reward.approve",
		"publication.reward.rate.manage",
		"announcement.manage",
		"fund.form.manage",
		"submission.read.all",
		"scopus.publications.read",
		"scopus.publications.read_by_user",
		"scopus.publications.export",
		"scopus.publications.export_by_user",
		"ui.page.admin.dashboard.view",
		"ui.page.admin.research_dashboard.view",
		"ui.page.admin.research_fund.view",
		"ui.page.admin.promotion_fund.view",
		"ui.page.admin.applications.view",
		"ui.page.admin.scopus.view",
		"ui.page.admin.fund_settings.view",
		"ui.page.admin.projects.view",
		"ui.page.admin.approval_records.view",
		"ui.page.admin.import_export.view",
		"ui.page.admin.academic_imports.view",
		"ui.page.admin.access_control.view",
	},
	4: {
		"portal.member.access",
		"dashboard.view.self",
		"fund.request.create",
		"fund.request.update",
		"fund.request.delete",
		"publication.reward.manage_own",
		"submission.read.own",
		"submission.read.department",
		"dept_head.review.recommend",
		"dept_head.review.reject",
		"dept_head.review.request_revision",
		"ui.page.member.dashboard.view",
		"ui.page.member.profile.view",
		"ui.page.member.research_fund.view",
		"ui.page.member.promotion_fund.view",
		"ui.page.member.applications.view",
		"ui.page.member.received_funds.view",
		"ui.page.member.announcements.view",
		"ui.page.member.projects.view",
		"ui.page.member.notifications.view",
		"ui.page.member.dept_review.view",
	},
	5: {
		"portal.executive.access",
		"dashboard.view.admin",
		"ui.page.admin.dashboard.view",
	},
}

var impliedPermissions = map[string][]string{
	"ui.page.admin.dashboard.view":          {"dashboard.view.admin"},
	"ui.page.admin.research_dashboard.view": {"scopus.publications.read"},
	"ui.page.admin.applications.view":       {"submission.read.all"},
	"ui.page.admin.scopus.view":             {"scopus.publications.read"},
	"ui.page.admin.import_export.view":      {"report.export"},
	"ui.page.admin.access_control.view":     {"access.view"},
	"ui.page.member.dept_review.view":       {"submission.read.department"},
}

func GetAuthorizationService() *AuthorizationService {
	authorizationServiceOnce.Do(func() {
		authorizationServiceInst = &AuthorizationService{db: config.DB}
	})
	return authorizationServiceInst
}

func (s *AuthorizationService) ResolvePermissionCodes(userID int, roleID int) ([]string, error) {
	set := map[string]struct{}{}

	if !s.hasAuthorizationTables() {
		applyDefaultRolePermissions(set, roleID)
		applyPermissionImplications(set, nil)
		return sortedPermissionKeys(set), nil
	}

	if err := s.mergeRolePermissions(userID, roleID, set); err != nil {
		applyDefaultRolePermissions(set, roleID)
		applyPermissionImplications(set, nil)
		return sortedPermissionKeys(set), nil
	}

	if err := s.applyUserOverrides(userID, set); err != nil {
		applyDefaultRolePermissions(set, roleID)
		applyPermissionImplications(set, nil)
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
		applyPermissionImplications(set, nil)
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

	explicitDeny := map[string]struct{}{}

	for _, row := range rows {
		code := strings.TrimSpace(strings.ToLower(row.Code))
		effect := strings.TrimSpace(strings.ToLower(row.Effect))
		if code == "" {
			continue
		}
		switch effect {
		case "deny":
			explicitDeny[code] = struct{}{}
			delete(set, code)
		default:
			set[code] = struct{}{}
		}
	}

	applyPermissionImplications(set, explicitDeny)

	for code := range explicitDeny {
		delete(set, code)
	}

	return nil
}

func applyDefaultRolePermissions(set map[string]struct{}, roleID int) {
	for _, code := range defaultPermissionsByRole[roleID] {
		trimmed := strings.TrimSpace(strings.ToLower(code))
		if trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
}

func applyPermissionImplications(set map[string]struct{}, explicitDeny map[string]struct{}) {
	if len(set) == 0 {
		return
	}

	changed := true
	for changed {
		changed = false
		for source, targets := range impliedPermissions {
			if _, exists := set[source]; !exists {
				continue
			}
			for _, target := range targets {
				normalizedTarget := strings.TrimSpace(strings.ToLower(target))
				if normalizedTarget == "" {
					continue
				}
				if explicitDeny != nil {
					if _, denied := explicitDeny[normalizedTarget]; denied {
						continue
					}
				}
				if _, exists := set[normalizedTarget]; !exists {
					set[normalizedTarget] = struct{}{}
					changed = true
				}
			}
		}

		for code := range set {
			if strings.HasPrefix(code, "ui.page.admin.") {
				if explicitDeny != nil {
					if _, denied := explicitDeny["portal.admin.access"]; denied {
						continue
					}
				}
				if _, exists := set["portal.admin.access"]; !exists {
					set["portal.admin.access"] = struct{}{}
					changed = true
				}
			}

			if strings.HasPrefix(code, "ui.page.member.") {
				if explicitDeny != nil {
					if _, denied := explicitDeny["portal.member.access"]; denied {
						continue
					}
				}
				if _, exists := set["portal.member.access"]; !exists {
					set["portal.member.access"] = struct{}{}
					changed = true
				}
			}
		}
	}
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
