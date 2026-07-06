package routes

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// fileViewURLTTL is how long a signed /view URL stays valid after it is minted.
// The short TTL is for live inline display; the export TTL is for links embedded in
// downloaded spreadsheets/documents that a user may open much later.
const (
	fileViewURLTTL       = 5 * time.Minute
	fileViewExportURLTTL = 7 * 24 * time.Hour
)

// normalizeUploadRelPath converts a raw client-supplied path into the canonical
// relative path used under the upload root, rejecting directory traversal. It is
// shared by the signer (SignFileViewURL) and the /view verifier so both agree on
// the exact string that gets signed.
func normalizeUploadRelPath(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.TrimPrefix(s, "./")
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimPrefix(s, "uploads/")
	if s == "" || strings.Contains(s, "..") {
		return "", errors.New("invalid file path")
	}
	return s, nil
}

// fileViewSigningKey uses JWT_SECRET (validated non-empty at startup) as the HMAC key.
func fileViewSigningKey() []byte {
	return []byte(os.Getenv("JWT_SECRET"))
}

// signFileView returns the hex HMAC-SHA256 signature binding a normalized path to
// an expiry timestamp.
func signFileView(normalizedPath string, exp int64) string {
	mac := hmac.New(sha256.New, fileViewSigningKey())
	mac.Write([]byte(normalizedPath + "\n" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyFileViewSig validates the signature and expiry for a normalized path using
// a constant-time comparison.
func verifyFileViewSig(normalizedPath, expStr, sig string) bool {
	if expStr == "" || sig == "" {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	expected := signFileView(normalizedPath, exp)
	return hmac.Equal([]byte(expected), []byte(sig))
}

// SignFileViewURL issues a short-lived signed URL for inline file viewing. It is
// mounted behind AuthMiddleware, so only authenticated users can mint view URLs;
// this replaces the previously-anonymous /view and static /uploads access.
// NOTE: it authorizes "any logged-in user may view any upload path" — per-object
// ownership scoping is a later hardening step (see Phase 2 in the review).
func SignFileViewURL(c *gin.Context) {
	normalized, err := normalizeUploadRelPath(c.Query("path"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid file path"})
		return
	}

	// purpose=export mints a long-lived link for URLs embedded in downloaded
	// spreadsheets/documents; the default is a short-lived link for live display.
	ttl := fileViewURLTTL
	if c.Query("purpose") == "export" {
		ttl = fileViewExportURLTTL
	}
	exp := time.Now().Add(ttl).Unix()
	sig := signFileView(normalized, exp)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"path":       normalized,
		"url":        fmt.Sprintf("/api/v1/view/%s?exp=%d&sig=%s", normalized, exp, sig),
		"expires_at": exp,
	})
}
