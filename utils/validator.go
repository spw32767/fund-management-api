// utils/validator.go - Input validation
package utils

import (
	"regexp"
	"strings"
)

// ValidateEmail checks if email is valid
func ValidateEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

// ValidatePassword checks password strength
func ValidatePassword(password string) (bool, string) {
	if len(password) < 8 {
		return false, "Password must be at least 8 characters"
	}

	return true, ""
}

// SanitizeInput removes potentially harmful characters
func SanitizeInput(input string) string {
	// Remove leading/trailing spaces
	input = strings.TrimSpace(input)

	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	return input
}
