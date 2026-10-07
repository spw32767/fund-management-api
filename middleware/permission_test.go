package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequirePermissionUsesEffectiveAuthPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		permissions []string
		wantStatus  int
	}{
		{
			name:        "delegated teacher can access researcher management",
			permissions: []string{"portal.card.researcher_management.access"},
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "teacher without grant is denied",
			permissions: []string{"portal.member.access"},
			wantStatus:  http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/researcher-management/instructors", func(c *gin.Context) {
				c.Set("userID", 101)
				c.Set("roleID", 1)
				c.Set("permissions", tc.permissions)
				c.Next()
			}, RequirePermission("portal.card.researcher_management.access"), func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/researcher-management/instructors", nil)
			router.ServeHTTP(response, request)

			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.wantStatus, response.Body.String())
			}
		})
	}
}
