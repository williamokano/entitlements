package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTenantLifecycleTransitionTable(t *testing.T) {
	cases := []struct {
		from    Status
		action  string
		wantErr bool
		wantTo  Status
	}{
		{StatusActive, "suspend", false, StatusSuspended},
		{StatusActive, "delete", false, StatusDeleted},
		{StatusActive, "reactivate", true, StatusActive},
		{StatusSuspended, "reactivate", false, StatusActive},
		{StatusSuspended, "delete", false, StatusDeleted},
		{StatusSuspended, "suspend", true, StatusSuspended},
		{StatusDeleted, "suspend", true, StatusDeleted},
		{StatusDeleted, "reactivate", true, StatusDeleted},
		{StatusDeleted, "delete", true, StatusDeleted},
	}

	for _, tc := range cases {
		t.Run(string(tc.from)+"_"+tc.action, func(t *testing.T) {
			tn := &Tenant{Status: tc.from}
			var err error
			switch tc.action {
			case "suspend":
				err = tn.Suspend()
			case "reactivate":
				err = tn.Reactivate()
			case "delete":
				err = tn.Delete()
			}
			if tc.wantErr {
				if err == nil {
					t.Fatalf("%s from %s: err = nil, want error", tc.action, tc.from)
				}
				if tn.Status != tc.from {
					t.Fatalf("denied transition changed status to %s, want unchanged %s", tn.Status, tc.from)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s from %s: unexpected err %v", tc.action, tc.from, err)
			}
			if tn.Status != tc.wantTo {
				t.Fatalf("status = %s, want %s", tn.Status, tc.wantTo)
			}
		})
	}
}

func TestSlugNormalizationAndValidation(t *testing.T) {
	valid := map[string]string{
		"acme":        "acme",
		"ACME":        "acme",
		"  Acme-Co  ": "acme-co",
		"a1-b2-c3":    "a1-b2-c3",
	}
	for in, want := range valid {
		got, err := NormalizeSlug(in)
		if err != nil {
			t.Errorf("NormalizeSlug(%q) error = %v, want %q", in, err, want)
			continue
		}
		if got != want {
			t.Errorf("NormalizeSlug(%q) = %q, want %q", in, got, want)
		}
	}

	invalid := []string{
		"ab",          // too short
		"-acme",       // leading hyphen
		"acme-",       // trailing hyphen
		"acme--co",    // double hyphen
		"acme_co",     // underscore
		"acme co",     // space
		"acmé",        // non-ascii
		string(make([]byte, 0)), // empty
	}
	for _, in := range invalid {
		if _, err := NormalizeSlug(in); err == nil {
			t.Errorf("NormalizeSlug(%q) = nil error, want validation error", in)
		}
	}
}

func TestNewTenantDefaults(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tn, err := New(uuid.New(), "Acme", "Acme Inc", nil, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tn.Slug != "acme" || tn.Status != StatusActive || tn.Settings == nil {
		t.Fatalf("unexpected tenant: %+v", tn)
	}
	if _, err := New(uuid.New(), "acme", "  ", nil, now); err == nil {
		t.Fatal("New with blank name = nil error, want validation error")
	}
}
