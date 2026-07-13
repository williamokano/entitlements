package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// TestOverrideWithoutReasonOrActorRejected proves an override cannot be created
// (or updated) without both a reason and an actor: an entitlement override must
// always be accountable.
func TestOverrideWithoutReasonOrActorRejected(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	tenant := uuid.New()

	cases := []struct {
		name    string
		key     string
		reason  string
		actor   string
		wantErr bool
	}{
		{name: "valid", key: "seats", reason: "negotiated contract", actor: "user/42", wantErr: false},
		{name: "missing reason", key: "seats", reason: "", actor: "user/42", wantErr: true},
		{name: "missing actor", key: "seats", reason: "negotiated contract", actor: "", wantErr: true},
		{name: "missing both", key: "seats", reason: "", actor: "", wantErr: true},
		{name: "missing feature key", key: "", reason: "negotiated contract", actor: "user/42", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewOverride(uuid.New(), tenant, tc.key, int64(50), tc.reason, tc.actor, nil, now)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NewOverride(%q, reason=%q, actor=%q) = nil error, want validation error", tc.key, tc.reason, tc.actor)
				}
				if apperr.KindOf(err) != apperr.KindValidation {
					t.Fatalf("error kind = %s, want validation", apperr.KindOf(err))
				}
				return
			}
			if err != nil {
				t.Fatalf("NewOverride: unexpected error %v", err)
			}
		})
	}

	// An update that drops the reason or actor is rejected just like a create.
	o, err := NewOverride(uuid.New(), tenant, "seats", int64(50), "initial", "user/1", nil, now)
	if err != nil {
		t.Fatalf("seed override: %v", err)
	}
	if err := o.Update(int64(75), "", "user/1", nil); err == nil {
		t.Fatal("Update with empty reason = nil error, want validation error")
	}
	if err := o.Update(int64(75), "raise", "", nil); err == nil {
		t.Fatal("Update with empty actor = nil error, want validation error")
	}
	if err := o.Update(int64(75), "raise", "user/1", nil); err != nil {
		t.Fatalf("valid Update: unexpected error %v", err)
	}
}
