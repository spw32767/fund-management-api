package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"fund-management-api/middleware"
	"fund-management-api/services"
	"github.com/gin-gonic/gin"
)

// Exercise every registered admin area with the Admin permission bundle and
// a teacher identity. This catches unmapped endpoints formerly hidden by the
// primary-admin bypass, without running endpoint business logic or touching DB.
func TestAdminBundleCoversEveryRegisteredAdminArea(t *testing.T) {
	gin.SetMode(gin.TestMode)
	source := gin.New()
	SetupRoutes(source)
	permissions, err := services.NewAuthorizationService(nil).ResolvePermissionCodes(7, 3)
	if err != nil {
		t.Fatal(err)
	}
	protected := gin.New()
	protected.Use(func(c *gin.Context) {
		c.Set("userID", 7)
		c.Set("roleID", 1)
		c.Set("permissions", permissions)
	}, middleware.RequireAdminAreaPermission())
	var adminRoutes []gin.RouteInfo
	for _, route := range source.Routes() {
		if strings.HasPrefix(route.Path, "/api/v1/admin/") {
			adminRoutes = append(adminRoutes, route)
			protected.Handle(route.Method, route.Path, func(c *gin.Context) { c.Status(http.StatusNoContent) })
		}
	}
	if len(adminRoutes) < 100 {
		t.Fatal("admin route inventory unexpectedly empty")
	}
	params := regexp.MustCompile(`:[^/]+|\*[^/]+`)
	for _, route := range adminRoutes {
		response := httptest.NewRecorder()
		path := params.ReplaceAllString(route.Path, "1")
		protected.ServeHTTP(response, httptest.NewRequest(route.Method, path, nil))
		if response.Code != http.StatusNoContent {
			t.Errorf("Admin grant blocked for teacher: %s %s: %d %s", route.Method, route.Path, response.Code, response.Body.String())
		}
	}
}

// TestSetupRoutesNoConflict ensures the full route tree registers without a
// gin/httprouter panic (e.g. a wildcard param colliding with a static segment).
func TestSetupRoutesNoConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetupRoutes panicked (route conflict?): %v", r)
		}
	}()
	router := gin.New()
	SetupRoutes(router)
	for _, path := range []string{"/api/v1/admin/scopus/dashboard/faculty-insights", "/api/v1/admin/scopus/dashboard/faculty-insights/drilldown"} {
		found := false
		for _, route := range router.Routes() {
			if route.Method == "GET" && route.Path == path {
				found = true
			}
		}
		if !found {
			t.Errorf("faculty insights GET route missing: %s", path)
		}
	}
}

// TestFileViewSignRoundTrip verifies that a signed path validates and that
// tampering or expiry is rejected.
func TestFileViewSignRoundTrip(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-for-signing")

	path := "users/123/submissions/456/doc.pdf"
	exp := time.Now().Unix() + 300
	sig := signFileView(path, exp)
	expStr := strconv.FormatInt(exp, 10)

	if !verifyFileViewSig(path, expStr, sig) {
		t.Fatal("valid signature should verify")
	}
	if verifyFileViewSig(path+"x", expStr, sig) {
		t.Fatal("tampered path must not verify")
	}
	if verifyFileViewSig(path, strconv.FormatInt(time.Now().Unix()-10, 10), sig) {
		t.Fatal("expired/mismatched exp must not verify")
	}
	if verifyFileViewSig(path, expStr, sig+"00") {
		t.Fatal("tampered signature must not verify")
	}
}
