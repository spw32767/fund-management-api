package routes

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestSetupRoutesNoConflict ensures the full route tree registers without a
// gin/httprouter panic (e.g. a wildcard param colliding with a static segment).
func TestSetupRoutesNoConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetupRoutes panicked (route conflict?): %v", r)
		}
	}()
	SetupRoutes(gin.New())
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
