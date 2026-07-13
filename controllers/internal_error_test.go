package controllers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// leakyErr mimics what a GORM/MySQL driver error string looks like — the kind of
// internal detail we must never leak to the browser.
var leakyErr = errors.New("Error 1054: Unknown column 'r.role' in 'group statement' [SELECT r.role_id FROM roles r]")

func runInternalError(mode string) *httptest.ResponseRecorder {
	gin.SetMode(mode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/access-control/roles", nil)

	InternalError(c, "access_control", leakyErr)
	return w
}

// Production (release) mode: the client must get a generic message and NONE of the
// raw error detail.
func TestInternalError_ReleaseModeDoesNotLeak(t *testing.T) {
	w := runInternalError(gin.ReleaseMode)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	body := w.Body.String()
	for _, leak := range []string{"1054", "Unknown column", "roles", "SELECT"} {
		if strings.Contains(body, leak) {
			t.Errorf("release response LEAKS %q: %s", leak, body)
		}
	}
	if !strings.Contains(body, "เกิดข้อผิดพลาดภายในระบบ") {
		t.Errorf("expected generic message, got: %s", body)
	}
}

// Dev (debug) mode: full detail is still returned so developers keep their visibility.
func TestInternalError_DebugModeKeepsDetail(t *testing.T) {
	w := runInternalError(gin.DebugMode)

	body := w.Body.String()
	if !strings.Contains(body, "Unknown column") {
		t.Errorf("debug mode should expose detail for developers, got: %s", body)
	}
}
