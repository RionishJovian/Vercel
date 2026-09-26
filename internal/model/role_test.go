package model

import "testing"

func TestHasAtLeastRole(t *testing.T) {
	cases := []struct {
		role, min UserRole
		want      bool
	}{
		{RoleOwner, RoleOwner, true},
		{RoleOwner, RoleAdmin, true},
		{RoleAdmin, RoleOwner, false},
		{RoleOperator, RoleTechnician, true},
		{RoleViewer, RoleOperator, false},
		{RoleSupport, RoleViewer, true},
		{RoleTechnician, RoleOperator, false},
		{RoleDeveloper, RoleOwner, true}, // system role passes everything
		{UserRole("NOPE"), RoleViewer, false},
	}
	for _, c := range cases {
		if got := HasAtLeastRole(c.role, c.min); got != c.want {
			t.Fatalf("HasAtLeastRole(%s,%s) = %v, want %v", c.role, c.min, got, c.want)
		}
	}
}

func TestIsTenantRole(t *testing.T) {
	for _, r := range []UserRole{RoleOwner, RoleAdmin, RoleOperator, RoleTechnician, RoleViewer, RoleSupport} {
		if !IsTenantRole(r) {
			t.Fatalf("%s must be a grantable tenant role", r)
		}
	}
	if IsTenantRole(RoleDeveloper) {
		t.Fatal("DEVELOPER must NOT be a grantable tenant role")
	}
	if IsTenantRole(UserRole("GHOST")) {
		t.Fatal("unknown role must not be grantable")
	}
}
