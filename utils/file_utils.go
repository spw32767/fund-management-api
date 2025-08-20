// utils/file_utils.go
// เพิ่มฟังก์ชันเหล่านี้ใน utils/file_utils.go หรือสร้างไฟล์ใหม่

package utils

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// SanitizeFilename ทำความสะอาดชื่อไฟล์ให้ปลอดภัย
func SanitizeFilename(filename string) string {
	// Remove or replace unsafe characters
	filename = strings.ReplaceAll(filename, " ", "_")
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")
	filename = strings.ReplaceAll(filename, ":", "_")
	filename = strings.ReplaceAll(filename, "*", "_")
	filename = strings.ReplaceAll(filename, "?", "_")
	filename = strings.ReplaceAll(filename, "\"", "_")
	filename = strings.ReplaceAll(filename, "<", "_")
	filename = strings.ReplaceAll(filename, ">", "_")
	filename = strings.ReplaceAll(filename, "|", "_")

	// Remove multiple underscores
	re := regexp.MustCompile(`_+`)
	filename = re.ReplaceAllString(filename, "_")

	// Remove leading/trailing underscores
	filename = strings.Trim(filename, "_")

	// Limit length
	if len(filename) > 100 {
		filename = filename[:100]
	}

	// If empty, generate random name
	if filename == "" {
		filename = GenerateRandomString(10)
	}

	return filename
}

// DefaultString returns default value if input is empty
func DefaultString(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// GenerateRandomString generates a random string of specified length
func GenerateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to a simple method
		return fmt.Sprintf("file_%d", len(charset))
	}

	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b)
}

// GetFileExtension safely gets file extension
func GetFileExtension(filename string) string {
	ext := filepath.Ext(filename)
	return strings.ToLower(ext)
}

// IsAllowedFileType checks if file type is allowed
func IsAllowedFileType(mimeType string) bool {
	allowedTypes := map[string]bool{
		"application/pdf":    true,
		"application/msword": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/vnd.ms-excel": true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
	}
	return allowedTypes[mimeType]
}

// FormatFileSize formats file size in human readable format
func FormatFileSize(size int64) string {
	if size == 0 {
		return "0 B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}
	sizeFloat := float64(size)

	for i, unit := range units {
		if sizeFloat < 1024 || i == len(units)-1 {
			if i == 0 {
				return fmt.Sprintf("%.0f %s", sizeFloat, unit)
			}
			return fmt.Sprintf("%.2f %s", sizeFloat, unit)
		}
		sizeFloat /= 1024
	}

	return fmt.Sprintf("%.2f %s", sizeFloat, units[len(units)-1])
}

// ValidateFileSize checks if file size is within limit
func ValidateFileSize(size int64, maxSizeMB int) bool {
	maxSizeBytes := int64(maxSizeMB * 1024 * 1024)
	return size <= maxSizeBytes
}

// IsSafeFileName checks if filename is safe
func IsSafeFileName(filename string) bool {
	// Check for dangerous patterns
	dangerous := []string{
		"..",
		"./",
		"\\",
		":",
		"*",
		"?",
		"\"",
		"<",
		">",
		"|",
	}

	filename = strings.ToLower(filename)
	for _, pattern := range dangerous {
		if strings.Contains(filename, pattern) {
			return false
		}
	}

	// Check for control characters
	for _, r := range filename {
		if unicode.IsControl(r) {
			return false
		}
	}

	return true
}

// CleanPath cleans and normalizes file path
func CleanPath(path string) string {
	// Replace backslashes with forward slashes
	path = strings.ReplaceAll(path, "\\", "/")

	// Clean the path
	path = filepath.Clean(path)

	// Remove any leading slashes to prevent absolute path
	path = strings.TrimLeft(path, "/")

	return path
}

// GetMimeTypeFromExtension gets MIME type from file extension
func GetMimeTypeFromExtension(ext string) string {
	ext = strings.ToLower(ext)
	mimeTypes := map[string]string{
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
	}

	if mimeType, exists := mimeTypes[ext]; exists {
		return mimeType
	}

	return "application/octet-stream"
}

// ValidateAnnouncementType validates announcement type
func ValidateAnnouncementType(announcementType string) bool {
	validTypes := map[string]bool{
		"general":        true,
		"research_fund":  true,
		"promotion_fund": true,
	}
	return validTypes[announcementType]
}

// ValidatePriority validates priority level
func ValidatePriority(priority string) bool {
	validPriorities := map[string]bool{
		"normal": true,
		"high":   true,
		"urgent": true,
	}
	return validPriorities[priority]
}

// ValidateFormType validates form type
func ValidateFormType(formType string) bool {
	validTypes := map[string]bool{
		"application": true,
		"report":      true,
		"evaluation":  true,
		"guidelines":  true,
		"other":       true,
	}
	return validTypes[formType]
}

// ValidateFundCategory validates fund category
func ValidateFundCategory(category string) bool {
	validCategories := map[string]bool{
		"research_fund":  true,
		"promotion_fund": true,
		"both":           true,
	}
	return validCategories[category]
}

// ValidateStatus validates status
func ValidateStatus(status string) bool {
	validStatuses := map[string]bool{
		"active":   true,
		"inactive": true,
	}
	return validStatuses[status]
}

// ValidateFormStatus validates form status
func ValidateFormStatus(status string) bool {
	validStatuses := map[string]bool{
		"active":   true,
		"inactive": true,
		"archived": true,
	}
	return validStatuses[status]
}
