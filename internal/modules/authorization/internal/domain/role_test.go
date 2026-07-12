package domain

import "testing"

func TestPermissionMatchingTable(t *testing.T) {
	cases := []struct {
		granted  string
		required string
		want     bool
	}{
		{"role:read", "role:read", true},    // exact match
		{"role:read", "role:write", false},  // different action
		{"role:*", "role:write", true},      // resource wildcard
		{"role:*", "role:read", true},       // resource wildcard
		{"member:*", "role:read", false},    // no cross-resource leak
		{"*:*", "anything:here", true},      // super-permission
		{"role:read", "member:read", false}, // different resource
	}
	for _, c := range cases {
		if got := Grants(c.granted, c.required); got != c.want {
			t.Errorf("Grants(%q, %q) = %v, want %v", c.granted, c.required, got, c.want)
		}
	}
}

func TestPermissionsGrantUnknownDenied(t *testing.T) {
	granted := []string{"role:read", "member:*"}
	if !PermissionsGrant(granted, "member:write") {
		t.Fatal("member:* should grant member:write")
	}
	if PermissionsGrant(granted, "apikey:read") {
		t.Fatal("unknown permission apikey:read should be denied")
	}
	if PermissionsGrant(nil, "role:read") {
		t.Fatal("empty grants should deny")
	}
}

func TestValidatePermissions(t *testing.T) {
	if err := ValidatePermissions([]string{"role:read", "role:*", "*:*"}); err != nil {
		t.Fatalf("valid permissions rejected: %v", err)
	}
	for _, bad := range []string{"role", "role:", ":read", "Role:Read", "role read", "a:b:c"} {
		if err := ValidatePermissions([]string{bad}); err == nil {
			t.Errorf("invalid permission %q accepted", bad)
		}
	}
}
