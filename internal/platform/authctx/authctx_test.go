package authctx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/authctx"
)

func TestMustTenantFailsWithoutTenant(t *testing.T) {
	// No tenant in context.
	id, err := authctx.MustTenant(context.Background())
	if !errors.Is(err, authctx.ErrNoTenant) {
		t.Fatalf("err = %v, want ErrNoTenant", err)
	}
	if id != uuid.Nil {
		t.Fatalf("id = %s, want zero uuid on error", id)
	}

	// A zero tenant value still counts as absent (never returned as valid).
	ctx := authctx.WithTenantID(context.Background(), uuid.Nil)
	if _, err := authctx.MustTenant(ctx); !errors.Is(err, authctx.ErrNoTenant) {
		t.Fatalf("MustTenant with nil tenant err = %v, want ErrNoTenant", err)
	}

	// A real tenant is returned.
	want := uuid.New()
	got, err := authctx.MustTenant(authctx.WithTenantID(context.Background(), want))
	if err != nil || got != want {
		t.Fatalf("MustTenant = (%s, %v), want (%s, nil)", got, err, want)
	}
}

func TestSystemContext(t *testing.T) {
	if authctx.IsSystem(context.Background()) {
		t.Fatal("fresh context reported as system")
	}
	if !authctx.IsSystem(authctx.WithSystemContext(context.Background())) {
		t.Fatal("WithSystemContext not detected by IsSystem")
	}
}

func TestTenantClaimAndPrincipalRoundTrip(t *testing.T) {
	ctx := authctx.WithTenantClaim(context.Background(), "acme")
	if v, ok := authctx.TenantClaim(ctx); !ok || v != "acme" {
		t.Fatalf("TenantClaim = (%q, %v), want (acme, true)", v, ok)
	}
	if _, ok := authctx.TenantClaim(authctx.WithTenantClaim(context.Background(), "")); ok {
		t.Fatal("empty tenant claim reported present")
	}

	p := authctx.Principal{Kind: authctx.PrincipalMachine, Subject: "key-1", Scopes: []string{"a", "b"}}
	got, ok := authctx.PrincipalFromContext(authctx.WithPrincipal(context.Background(), p))
	if !ok || got.Kind != p.Kind || got.Subject != p.Subject {
		t.Fatalf("PrincipalFromContext = (%+v, %v), want (%+v, true)", got, ok, p)
	}
	if !got.HasScope("a") || !got.HasScope("b") || got.HasScope("c") {
		t.Fatalf("scopes = %v, want [a b]", got.Scopes)
	}
}
