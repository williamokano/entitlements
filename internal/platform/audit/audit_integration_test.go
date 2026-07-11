//go:build integration

package audit_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/platform/audit"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

func frozen() *clock.Frozen {
	return clock.NewFrozen(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

func countAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform.audit_log`).Scan(&n); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	return n
}

func TestAuditEntryVisibleOnlyAfterCommit(t *testing.T) {
	pool := testkit.Postgres(t)
	w := audit.NewWriter(pool, id.UUIDv7{}, frozen())
	uow := postgres.NewUnitOfWork(pool)
	ctx := context.Background()

	// A record written in a rolled-back transaction must not persist.
	rollbackErr := errors.New("business failure")
	err := uow.Do(ctx, func(ctx context.Context) error {
		if err := w.Record(ctx, audit.Entry{Actor: "system", Action: "thing.created", Resource: "thing/1"}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("Do error = %v, want rollbackErr", err)
	}
	if n := countAudit(t, ctx, pool); n != 0 {
		t.Fatalf("audit rows after rollback = %d, want 0", n)
	}

	// A committed record persists.
	if err := w.Record(ctx, audit.Entry{Actor: "system", Action: "thing.created", Resource: "thing/1"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if n := countAudit(t, ctx, pool); n != 1 {
		t.Fatalf("audit rows after commit = %d, want 1", n)
	}
}

func TestAuditEntryPersistsAllFields(t *testing.T) {
	pool := testkit.Postgres(t)
	w := audit.NewWriter(pool, id.UUIDv7{}, frozen())
	ctx := context.Background()

	tenant := uuid.New()
	entry := audit.Entry{
		Actor:    "user/42",
		TenantID: tenant,
		Action:   "tenant.suspended",
		Resource: "tenant/" + tenant.String(),
		Before:   map[string]any{"status": "active"},
		After:    map[string]any{"status": "suspended"},
		Reason:   "non-payment",
	}
	if err := w.Record(ctx, entry); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var (
		actor, action, resource, reason string
		gotTenant                       uuid.UUID
		beforeJSON, afterJSON           []byte
	)
	err := pool.QueryRow(ctx,
		`SELECT actor, tenant_id, action, resource, reason, before, after
		 FROM platform.audit_log LIMIT 1`).
		Scan(&actor, &gotTenant, &action, &resource, &reason, &beforeJSON, &afterJSON)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}

	if actor != entry.Actor || action != entry.Action || resource != entry.Resource ||
		reason != entry.Reason || gotTenant != tenant {
		t.Fatalf("scalar fields mismatch: actor=%s tenant=%s action=%s resource=%s reason=%s",
			actor, gotTenant, action, resource, reason)
	}

	var gotBefore, gotAfter map[string]any
	if err := json.Unmarshal(beforeJSON, &gotBefore); err != nil || gotBefore["status"] != "active" {
		t.Fatalf("before jsonb = %s (err %v), want status=active", beforeJSON, err)
	}
	if err := json.Unmarshal(afterJSON, &gotAfter); err != nil || gotAfter["status"] != "suspended" {
		t.Fatalf("after jsonb = %s (err %v), want status=suspended", afterJSON, err)
	}
}

func TestAuditLogIsAppendOnly(t *testing.T) {
	pool := testkit.Postgres(t)
	w := audit.NewWriter(pool, id.UUIDv7{}, frozen())
	ctx := context.Background()

	if err := w.Record(ctx, audit.Entry{Actor: "system", Action: "thing.created", Resource: "thing/1"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE platform.audit_log SET reason = 'tampered'`); err == nil {
		t.Fatal("UPDATE on audit_log succeeded, want rejection")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM platform.audit_log`); err == nil {
		t.Fatal("DELETE on audit_log succeeded, want rejection")
	}

	// The row is still present and unchanged.
	var reason string
	if err := pool.QueryRow(ctx, `SELECT reason FROM platform.audit_log LIMIT 1`).Scan(&reason); err != nil {
		t.Fatalf("read after rejected mutations: %v", err)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want unchanged empty string", reason)
	}
}
