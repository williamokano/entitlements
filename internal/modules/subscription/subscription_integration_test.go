//go:build integration

package subscription_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/modules/subscription"
	subpg "github.com/williamokano/entitlements/internal/modules/subscription/internal/adapters/postgres"
	subdomain "github.com/williamokano/entitlements/internal/modules/subscription/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

func newDeps(t *testing.T) app.Deps {
	t.Helper()
	pool := testkit.Postgres(t)
	ids := id.UUIDv7{}
	clk := clock.System
	return app.Deps{
		Pool:       pool,
		UnitOfWork: postgres.NewUnitOfWork(pool),
		Outbox:     events.NewOutbox(pool, ids, clk),
		Bus:        events.NewBus(),
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Clock:      clk,
		IDs:        ids,
	}
}

// fakeCatalog returns a canned published plan version for any id.
type fakeCatalog struct{ trialDays int }

func (f fakeCatalog) GetPlanVersion(_ context.Context, id uuid.UUID) (catalogports.PlanVersionInfo, error) {
	return catalogports.PlanVersionInfo{
		ID: id, PlanID: uuid.New(), PlanKey: "pro", Version: 1, Status: "published",
		Currency: "USD",
		Prices:   []catalogports.PriceInfo{{Cycle: "monthly", AmountMinor: 1000}},
		TrialEnabled: f.trialDays > 0, TrialDays: f.trialDays,
	}, nil
}

// serverFor wraps a module handler with a resolved tenant + user principal.
func serverFor(h http.Handler, tenantID, userID uuid.UUID) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := authctx.WithTenantID(r.Context(), tenantID)
		ctx = authctx.WithPrincipal(ctx, authctx.Principal{Kind: authctx.PrincipalUser, Subject: userID.String()})
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
}

func post(t *testing.T, url, body string) (int, string) { return do(t, http.MethodPost, url, body) }
func get(t *testing.T, url string) (int, string)        { return do(t, http.MethodGet, url, "") }

func do(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestSubscriptionEndpointsCreateGetCancel(t *testing.T) {
	deps := newDeps(t)
	mod := subscription.New(deps, fakeCatalog{trialDays: 14})
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	pv := uuid.New()
	status, body := post(t, srv.URL, `{"plan_version_id":"`+pv.String()+`","cycle":"monthly"}`)
	if status != http.StatusCreated {
		t.Fatalf("create: %d (%s)", status, body)
	}
	if !strings.Contains(body, `"status":"trialing"`) {
		t.Fatalf("expected trialing (14-day trial), got %s", body)
	}

	// Get returns the live subscription.
	if status, gb := get(t, srv.URL); status != http.StatusOK || !strings.Contains(gb, `"status":"trialing"`) {
		t.Fatalf("get = %d %s", status, gb)
	}

	// Cancel at period end keeps it live.
	if status, cb := post(t, srv.URL+"/cancel", `{"immediate":false}`); status != http.StatusOK || !strings.Contains(cb, `"cancel_at_period_end":true`) {
		t.Fatalf("cancel-at-period-end = %d %s", status, cb)
	}
	if _, gb := get(t, srv.URL); strings.Contains(gb, `"status":"canceled"`) {
		t.Fatalf("subscription canceled immediately on at-period-end cancel: %s", gb)
	}

	// Immediate cancel terminates it.
	if status, _ := post(t, srv.URL+"/cancel", `{"immediate":true,"reason":"done"}`); status != http.StatusOK {
		t.Fatalf("cancel immediate status")
	}
	if status, _ := get(t, srv.URL); status != http.StatusNotFound {
		t.Fatalf("get after immediate cancel = %d, want 404 (no live subscription)", status)
	}
}

func TestSecondActiveSubscriptionForTenantRejected409(t *testing.T) {
	deps := newDeps(t)
	mod := subscription.New(deps, fakeCatalog{})
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != http.StatusCreated {
		t.Fatal("first create, want 201")
	}
	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != http.StatusConflict {
		t.Fatal("second create, want 409")
	}
}

func TestEveryTransitionRecordsHistoryAndOutboxEventAtomically(t *testing.T) {
	deps := newDeps(t)
	mod := subscription.New(deps, fakeCatalog{})
	tenantID, userID := uuid.New(), uuid.New()
	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()
	ctx := context.Background()

	// Create (active) then pause → two transitions, two outbox events.
	if status, _ := post(t, srv.URL, `{"plan_version_id":"`+uuid.NewString()+`","cycle":"monthly"}`); status != http.StatusCreated {
		t.Fatal("create")
	}
	if status, _ := post(t, srv.URL+"/pause", ``); status != http.StatusOK {
		t.Fatal("pause")
	}

	var subID uuid.UUID
	if err := deps.Pool.QueryRow(ctx, `SELECT id FROM subscription.subscriptions WHERE tenant_id = $1`, tenantID).Scan(&subID); err != nil {
		t.Fatalf("find subscription: %v", err)
	}
	var transitions, outboxEvents int
	if err := deps.Pool.QueryRow(ctx, `SELECT count(*) FROM subscription.subscription_transitions WHERE subscription_id = $1`, subID).Scan(&transitions); err != nil {
		t.Fatalf("count transitions: %v", err)
	}
	if err := deps.Pool.QueryRow(ctx, `SELECT count(*) FROM platform.outbox WHERE tenant_id = $1 AND event_type = 'subscription.transitioned'`, tenantID).Scan(&outboxEvents); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if transitions != 2 || outboxEvents != 2 {
		t.Fatalf("transitions=%d outbox=%d, want 2 and 2", transitions, outboxEvents)
	}

	// Atomicity: a transaction that appends a transition then fails rolls back —
	// leaving neither the row nor any partial state.
	repo := subpg.New(deps.Pool)
	before := transitions
	_ = deps.UnitOfWork.Do(ctx, func(ctx context.Context) error {
		_ = repo.AppendTransition(ctx, &subdomain.Transition{
			ID: uuid.New(), SubscriptionID: subID, From: subdomain.StatePaused, To: subdomain.StateActive, Event: subdomain.EventResume, At: deps.Clock.Now(),
		})
		return errors.New("boom") // force rollback
	})
	var after int
	if err := deps.Pool.QueryRow(ctx, `SELECT count(*) FROM subscription.subscription_transitions WHERE subscription_id = $1`, subID).Scan(&after); err != nil {
		t.Fatalf("count after rollback: %v", err)
	}
	if after != before {
		t.Fatalf("rollback left a transition row: before=%d after=%d", before, after)
	}
}

func TestUnauthenticatedSubscriptionRouteRejected(t *testing.T) {
	deps := newDeps(t)
	mod := subscription.New(deps, fakeCatalog{})
	// Tenant set but no principal.
	tenantID := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mod.Handler().ServeHTTP(w, r.WithContext(authctx.WithTenantID(r.Context(), tenantID)))
	}))
	defer srv.Close()
	if status, _ := get(t, srv.URL); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated get = %d, want 401", status)
	}
}
