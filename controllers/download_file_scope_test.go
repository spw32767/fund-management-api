package controllers

import (
	"fund-management-api/config"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

// Department reviewers must be able to open files attached to submissions they
// review (uploaded by other users); plain users stay limited to own/public files.
func TestDownloadFileScopeByPermission(t *testing.T) {
	for _, tc := range []struct {
		name        string
		permissions []string
		pattern     string
	}{
		{"dept head reads submission attachments", []string{"submission.read.department"},
			`file_uploads.*uploaded_by = \? OR is_public = \? OR EXISTS \(SELECT 1 FROM submission_documents sd JOIN submissions s .*deleted_at IS NULL WHERE sd.file_id = file_uploads.file_id\)`},
		{"plain user limited to own or public", nil,
			`file_uploads.*\(uploaded_by = \? OR is_public = \?\) ORDER BY`},
		{"full reader unrestricted", []string{"submission.read.all"},
			`file_uploads.*delete_at IS NULL ORDER BY`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, state, cleanup := newScriptedGormDB(t, []*queryStep{{
				kind:    stepQuery,
				pattern: regexp.MustCompile(tc.pattern),
				columns: []string{"file_id"},
			}})
			defer cleanup()
			original := config.DB
			config.DB = db
			defer func() { config.DB = original }()

			router := gin.New()
			router.GET("/files/managed/:id/download", func(c *gin.Context) {
				c.Set("userID", 7)
				c.Set("permissions", tc.permissions)
				DownloadFile(c)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/files/managed/42/download", nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if err := state.verifyComplete(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
