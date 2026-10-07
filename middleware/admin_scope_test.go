package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDelegatedAdminPermissionIsLimitedToItsArea(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		method      string
		path        string
		roleID      int
		permissions []string
		wantStatus  int
	}{
		{"access viewer can read access catalog", http.MethodGet, "/api/v1/admin/access/permissions", 1, []string{"ui.page.admin.access_control.view"}, http.StatusNoContent},
		{"researcher manager cannot read access catalog", http.MethodGet, "/api/v1/admin/access/permissions", 1, []string{"portal.card.researcher_management.access"}, http.StatusForbidden},
		{"access viewer cannot read Scopus secret", http.MethodGet, "/api/v1/admin/scopus/config", 1, []string{"ui.page.admin.access_control.view"}, http.StatusForbidden},
		{"API key manager can read Scopus config", http.MethodGet, "/api/v1/admin/scopus/config", 1, []string{"api.clients.manage"}, http.StatusNoContent},
		{"access viewer cannot import users", http.MethodPost, "/api/v1/admin/import/users", 1, []string{"ui.page.admin.access_control.view"}, http.StatusForbidden},
		{"fund settings viewer cannot modify years", http.MethodPut, "/api/v1/admin/years/1", 1, []string{"ui.page.admin.fund_settings.view"}, http.StatusForbidden},
		{"projects viewer cannot create projects", http.MethodPost, "/api/v1/admin/projects", 1, []string{"ui.page.admin.projects.view"}, http.StatusForbidden},
		{"read permission cannot write role grants", http.MethodPut, "/api/v1/admin/access/roles/3/permissions", 1, []string{"access.view"}, http.StatusForbidden},
		{"manager can write role grants", http.MethodPut, "/api/v1/admin/access/roles/3/permissions", 1, []string{"access.manage"}, http.StatusNoContent},
		{"admin can use legacy endpoint", http.MethodGet, "/api/v1/admin/files/stats", 3, []string{"portal.admin.access"}, http.StatusNoContent},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			admin := router.Group("/api/v1/admin")
			admin.Use(func(c *gin.Context) {
				c.Set("userID", 101)
				c.Set("roleID", tc.roleID)
				c.Set("permissions", tc.permissions)
				c.Next()
			}, RequireAdminAreaPermission())
			admin.Handle(tc.method, "/access/permissions", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			admin.Handle(tc.method, "/scopus/config", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			admin.Handle(tc.method, "/import/users", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			admin.Handle(tc.method, "/access/roles/:id/permissions", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			admin.Handle(tc.method, "/files/stats", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			admin.Handle(tc.method, "/years/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			admin.Handle(tc.method, "/projects", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.wantStatus, response.Body.String())
			}
		})
	}
}
