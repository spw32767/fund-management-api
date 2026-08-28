package services

import "testing"

func TestPermissionImplicationsReturnsDefensiveCopy(t *testing.T) {
	first := PermissionImplications()
	targets := first["ui.page.admin.access_control.view"]
	if len(targets) != 1 || targets[0] != "access.view" {
		t.Fatalf("unexpected access-control implications: %#v", targets)
	}

	first["ui.page.admin.access_control.view"][0] = "access.manage"
	first["new.permission"] = []string{"other.permission"}

	second := PermissionImplications()
	if second["ui.page.admin.access_control.view"][0] != "access.view" {
		t.Fatal("caller mutation changed the authorization implication catalog")
	}
	if _, exists := second["new.permission"]; exists {
		t.Fatal("caller-added implication leaked into the authorization catalog")
	}
}

func TestApplyPermissionImplicationsRespectsExplicitDeny(t *testing.T) {
	set := map[string]struct{}{
		"ui.page.admin.access_control.view": {},
	}
	denied := map[string]struct{}{
		"access.view": {},
	}

	applyPermissionImplications(set, denied)

	if _, exists := set["access.view"]; exists {
		t.Fatal("explicitly denied permission was restored by implication")
	}
	if _, exists := set["portal.admin.access"]; !exists {
		t.Fatal("admin page permission did not imply admin portal access")
	}
}

func TestResolveRolePermissionCodesUsesDefaultCatalogWithoutDatabase(t *testing.T) {
	service := &AuthorizationService{}
	permissions, err := service.ResolveRolePermissionCodes(101, 3)
	if err != nil {
		t.Fatalf("ResolveRolePermissionCodes returned an error: %v", err)
	}

	found := map[string]bool{}
	for _, code := range permissions {
		found[code] = true
	}
	for _, expected := range []string{"access.manage", "ui.page.admin.access_control.view", "access.view", "portal.admin.access"} {
		if !found[expected] {
			t.Fatalf("expected default admin role permission %q", expected)
		}
	}
}
