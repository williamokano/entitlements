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
	svc := service.New(uow, events.NewOutbox(pool, ids, clk), pgadapter.New(pool), ids, clk)
	return svc, pool
}

func TestCreateTenantPersistsAndEmitsTenantCreated(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	info, err := svc.Create(ctx, "Acme-Co", "Acme Inc", map[string]any{"tier": "gold"})
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

func TestSlugUniquenessReturnsConflict(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, "dup", "First", nil); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.Create(ctx, "DUP", "Second", nil) // normalizes to same slug
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

	info, err := svc.Create(ctx, "goodbye", "Bye", nil)
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
	info, err := svc.Create(ctx, "nested", "Nested", settings)
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
