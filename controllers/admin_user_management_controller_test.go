package controllers

import (
	"strings"
	"testing"

	"fund-management-api/models"
)

func validAdminUserRequest() adminUserWriteRequest {
	return adminUserWriteRequest{
		Prefix: "ดร.", UserFname: "สมชาย", UserLname: "ใจดี",
		Email: "teacher@kku.ac.th", RoleID: 1,
		TemporaryPassword: "temporary123",
	}
}

func TestValidateAdminUserRequestAcceptsCreatePayload(t *testing.T) {
	req := validAdminUserRequest()
	req.DateOfEmployment = "2026-09-01"

	date, validationErr := validateAdminUserRequest(req, true)
	if validationErr != nil {
		t.Fatalf("expected valid request, got %#v", validationErr)
	}
	if date == nil || date.Format("2006-01-02") != "2026-09-01" {
		t.Fatalf("unexpected employment date: %v", date)
	}
}

func TestValidateAdminUserRequestRejectsWeakTemporaryPassword(t *testing.T) {
	req := validAdminUserRequest()
	req.TemporaryPassword = "short"

	_, validationErr := validateAdminUserRequest(req, true)
	if validationErr == nil || validationErr["field"] != "temporary_password" {
		t.Fatalf("expected temporary_password error, got %#v", validationErr)
	}
}

func TestValidateAdminUserRequestDoesNotRequirePasswordOnUpdate(t *testing.T) {
	req := validAdminUserRequest()
	req.TemporaryPassword = ""

	_, validationErr := validateAdminUserRequest(req, false)
	if validationErr != nil {
		t.Fatalf("expected update without password to be valid, got %#v", validationErr)
	}
}

func TestAdminUserRequestNormalizeTrimsAndLowercasesEmail(t *testing.T) {
	req := validAdminUserRequest()
	req.Email = "  New.Teacher@KKU.AC.TH  "
	req.UserFname = "  สมชาย  "
	req.normalize()

	if req.Email != "new.teacher@kku.ac.th" {
		t.Fatalf("unexpected normalized email: %q", req.Email)
	}
	if req.UserFname != "สมชาย" {
		t.Fatalf("unexpected normalized name: %q", req.UserFname)
	}
}

func TestAdminUserAuditValuesNeverContainPassword(t *testing.T) {
	hash := "secret-hash"
	values := adminUserAuditValues(models.User{
		UserID: 1, UserFname: "Test", UserLname: "User", Email: "test@example.com",
		Password: &hash, RoleID: 1, PositionID: 1,
	})

	for key, value := range values {
		if strings.Contains(strings.ToLower(key), "password") || value == hash {
			t.Fatalf("audit values leaked password data: %s=%v", key, value)
		}
	}
}

func TestParsePositiveQueryIntUsesSafeFallback(t *testing.T) {
	if got := parsePositiveQueryInt("12", 1); got != 12 {
		t.Fatalf("expected 12, got %d", got)
	}
	for _, input := range []string{"", "0", "-1", "not-a-number"} {
		if got := parsePositiveQueryInt(input, 20); got != 20 {
			t.Fatalf("expected fallback for %q, got %d", input, got)
		}
	}
}
