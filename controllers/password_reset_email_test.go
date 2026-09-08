package controllers

import (
	"testing"

	"fund-management-api/models"
)

func TestSendPasswordResetEmailFallsBackToLoginEmail(t *testing.T) {
	originalSendMail := sendMailFunc
	defer func() { sendMailFunc = originalSendMail }()

	var recipients []string
	sendMailFunc = func(to []string, subject, html string) error {
		recipients = append([]string(nil), to...)
		return nil
	}

	t.Setenv("APP_BASE_URL", "https://example.test")
	user := models.User{UserFname: "Test", UserLname: "User", Email: "login@example.test"}
	if err := sendPasswordResetEmail(user, "reset-token"); err != nil {
		t.Fatalf("sendPasswordResetEmail returned error: %v", err)
	}
	if len(recipients) != 1 || recipients[0] != user.Email {
		t.Fatalf("expected login email fallback, got %#v", recipients)
	}
}
