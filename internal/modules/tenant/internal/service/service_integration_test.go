//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	pgadapter "github.com/williamokano/entitlements/internal/modules/tenant/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/tenant/internal/service"
	"github.com/williamokano/entitlements/internal/modules/tenant/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

func newService(t *testing.T) (*service.Service, *pgxpool.Pool) {
	t.Helper()
	pool := testkit.Postgres(t)
	uow := postgres.NewUnitOfWork(pool)
	ids := id.UUIDv7{}
	clk := clock.System
	svc := service.New(uow, events.NewOutbox(pool, ids, clk), pgadapter.New(pool), pgadapter.NewMemberships(pool), ids, clk)
	return svc, pool
}

func TestCreateTenantPersistsAndEmitsTenantCreated(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	info, err := svc.Create(ctx, "Acme-Co", "Acme Inc", map[string]any{"tier": "gold"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Slug != "acme-co" || info.Status != ports.StatusActive {
		t.Fatalf("info = %+v, want slug acme-co, status active", info)
	}

	// Row persisted.
	var name, status string
	if err := pool.QueryRow(ctx,
		`SELECT name, status FROM tenant.tenants WHERE id = $1`, info.ID).Scan(&name, &status); err != nil {
		t.Fatalf("read tenant row: %v", err)
	}
	if name != "Acme Inc" || status != "active" {
		t.Fatalf("row = (%s, %s), want (Acme Inc, active)", name, status)
	}

	// Outbox event emitted for this tenant.
	var eventType string
	var tenantID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT event_type, tenant_id FROM platform.outbox WHERE event_type = $1 AND tenant_id = $2`,
		ports.EventTenantCreated, info.ID).Scan(&eventType, &tenantID); err != nil {
		t.Fatalf("read outbox event: %v", err)
	}
	if tenantID != info.ID {
		t.Fatalf("event tenant_id = %s, want %s", tenantID, info.ID)
	}
}

// T-031: the creator becomes the tenant's owner, written in the same
// transaction as the tenant itself.
func TestCreateWithACreatorWritesAnOwnerMembership(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	creator := &service.Creator{UserID: uuid.New(), Email: "founder@example.com"}
	info, err := svc.Create(ctx, "acme", "Acme", nil, creator)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var (
		userID uuid.UUID
		email  string
		role   string
		status string
	)
	if err := pool.QueryRow(ctx,
		`SELECT user_id, email, role, status FROM tenant.memberships WHERE tenant_id = $1`,
		info.ID).Scan(&userID, &email, &role, &status); err != nil {
		t.Fatalf("read owner membership: %v", err)
	}
	if userID != creator.UserID || email != "founder@example.com" || role != "owner" || status != "active" {
		t.Fatalf("owner membership = (%s, %s, %s, %s), want the creator as an active owner", userID, email, role, status)
	}
}

// A conflicting create must not leave an orphan membership behind: the tenant,
// its owner and the event share one transaction.
func TestCreateConflictRollsBackTheOwnerMembership(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	first := &service.Creator{UserID: uuid.New(), Email: "first@example.com"}
	if _, err := svc.Create(ctx, "dup", "First", nil, first); err != nil {
		t.Fatalf("first create: %v", err)
	}

	second := &service.Creator{UserID: uuid.New(), Email: "second@example.com"}
	if _, err := svc.Create(ctx, "dup", "Second", nil, second); err == nil {
		t.Fatal("duplicate slug create = nil error, want conflict")
	}

	var memberships int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM tenant.memberships WHERE user_id = $1`, second.UserID).Scan(&memberships); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if memberships != 0 {
		t.Fatalf("memberships for the failed creator = %d, want 0 (rolled back)", memberships)
	}
}

func TestSlugUniquenessReturnsConflict(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, "dup", "First", nil, nil); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.Create(ctx, "DUP", "Second", nil, nil) // normalizes to same slug
	if err == nil {
		t.Fatal("duplicate slug create = nil error, want conflict")
	}
	if apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("duplicate slug kind = %v, want conflict", apperr.KindOf(err))
	}
}

func TestSoftDeleteExcludesTenantFromReads(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	info, err := svc.Create(ctx, "goodbye", "Bye", nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(ctx, info.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := svc.GetByID(ctx, info.ID); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("GetByID after delete kind = %v, want not_found", apperr.KindOf(err))
	}
	if _, err := svc.GetBySlug(ctx, "goodbye"); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("GetBySlug after delete kind = %v, want not_found", apperr.KindOf(err))
	}

	// The row still exists, marked deleted.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM tenant.tenants WHERE id = $1`, info.ID).Scan(&status); err != nil {
		t.Fatalf("read soft-deleted row: %v", err)
	}
	if status != "deleted" {
		t.Fatalf("status = %s, want deleted (row should persist)", status)
	}
}

func TestSettingsJSONBRoundTrip(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	settings := map[string]any{
		"branding": map[string]any{"color": "#fff", "logo": "x.png"},
		"limits":   map[string]any{"seats": float64(10)},
		"flags":    []any{"beta", "invoicing"},
	}
	info, err := svc.Create(ctx, "nested", "Nested", settings, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	assertSettings := func(stage string) {
		var raw []byte
		if err := pool.QueryRow(ctx, `SELECT settings FROM tenant.tenants WHERE id = $1`, info.ID).Scan(&raw); err != nil {
			t.Fatalf("%s: read settings: %v", stage, err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("%s: unmarshal settings: %v", stage, err)
		}
		if !reflect.DeepEqual(got, settings) {
			t.Fatalf("%s: settings = %#v, want %#v", stage, got, settings)
		}
	}

	assertSettings("after create")

	// Round-trip through an update as well.
	if _, err := svc.Update(ctx, info.ID, "", settings); err != nil {
		t.Fatalf("Update: %v", err)
	}
	assertSettings("after update")
}
