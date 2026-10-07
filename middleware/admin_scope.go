package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequireAdminAreaPermission prevents one delegated admin-page grant from
// authorizing unrelated /admin endpoints. Primary admins retain the legacy
// admin API; delegated users need the permission for the requested area and
// a manage permission for mutations.
func RequireAdminAreaPermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		roleID, ok := c.Get("roleID")
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "AUTH_CONTEXT_MISSING", "error": "Role information not found"})
			return
		}
		if roleID == 3 {
			c.Next()
			return
		}

		required := delegatedAdminPermissions(c.Request.Method, c.FullPath())
		if len(required) == 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "INSUFFICIENT_PERMISSIONS", "error": "Insufficient permissions for this resource"})
			return
		}
		RequirePermission(required...)(c)
	}
}

func delegatedAdminPermissions(method, fullPath string) []string {
	path := strings.TrimPrefix(fullPath, "/api/v1/admin")
	if path == fullPath || path == "" {
		return nil
	}
	read := method == http.MethodGet || method == http.MethodHead
	under := func(prefix string) bool { return path == prefix || strings.HasPrefix(path, prefix+"/") }
	if !read {
		switch {
		case under("/access"):
			return []string{"access.manage"}
		case under("/api-clients"), under("/scopus/config"):
			return []string{"api.clients.manage"}
		case under("/users"), under("/import/users"), under("/files"):
			return []string{"users.manage"}
		case under("/submissions"), under("/applications"):
			return []string{"fund.request.approve", "publication.reward.approve"}
		case under("/notification-messages"), under("/announcements"):
			return []string{"announcement.manage"}
		case under("/reward-config"):
			return []string{"publication.reward.rate.manage"}
		case under("/instructors"), under("/ranking-weights"), under("/ranking-sources"), under("/courses"):
			return []string{"portal.card.researcher_management.access"}
		default:
			return nil
		}
	}

	switch {
	case under("/access"):
		return []string{"access.view", "access.manage", "ui.page.admin.access_control.view"}
	case under("/api-clients"):
		return []string{"api.clients.manage"}
	case under("/users/search"), under("/users/scopus"), under("/users/thaijo"), under("/user-publications"):
		return []string{"ui.page.admin.academic_imports.view"}
	case under("/users"):
		return []string{"users.view", "users.manage"}
	case under("/scopus/config"):
		return []string{"api.clients.manage"}
	case under("/scopus/dashboard"):
		return []string{"ui.page.admin.research_dashboard.view"}
	case under("/scopus/author-roles"), under("/scopus/import"), under("/scopus/metrics"), under("/scopus/conference"), under("/thaijo/import"):
		return []string{"ui.page.admin.academic_imports.view"}
	case under("/scopus"):
		return []string{"scopus.publications.read", "ui.page.admin.scopus.view"}
	case under("/publications"):
		return []string{"scopus.publications.read", "ui.page.admin.scopus.view"}
	case under("/dashboard"):
		return []string{"dashboard.view.admin", "ui.page.admin.dashboard.view"}
	case under("/submissions"), under("/applications"):
		return []string{"submission.read.all", "ui.page.admin.applications.view"}
	case under("/approval-records"):
		return []string{"ui.page.admin.approval_records.view"}
	case under("/legacy-submissions"):
		return []string{"ui.page.admin.applications.view", "ui.page.admin.import_export.view"}
	case under("/import/users"):
		return []string{"users.manage"}
	case under("/import"), under("/import-templates"):
		return []string{"ui.page.admin.import_export.view"}
	case under("/notification-messages"), under("/announcements"):
		return []string{"announcement.manage"}
	case under("/trigger"), under("/kku-people"):
		return []string{"ui.page.admin.academic_imports.view"}
	case under("/projects"):
		return []string{"ui.page.admin.projects.view"}
	case under("/reports"):
		return []string{"report.export", "ui.page.admin.research_dashboard.view"}
	case under("/files"):
		return []string{"users.view", "users.manage"}
	case under("/instructors"), under("/ranking-weights"), under("/ranking-sources"), under("/courses"), under("/audit-logs"):
		return []string{"portal.card.researcher_management.access"}
	case under("/reward-config"):
		return []string{"publication.reward.rate.manage"}
	case under("/years"), under("/installments"), under("/sdgs"), under("/categories"), under("/project-types"), under("/project-budget-plans"), under("/subcategories"), under("/budgets"), under("/funds"), under("/end-of-contract"), under("/document-types"), under("/system-config"):
		return []string{"ui.page.admin.fund_settings.view"}
	default:
		return nil
	}
}
