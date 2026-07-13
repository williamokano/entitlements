//go:build integration

package entitlements_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/modules/entitlements/internal/service"
	"github.com/williamokano/entitlements/internal/modules/entitlements/ports"
	subports "github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/clock"
)

// TestConsumeQuotaConcurrencyNeverExceedsHardLimit races N goroutines consuming
// one unit each against a hard limit L. The single guarded upsert guarantees
// exactly L accepted consumes and N-L rejections, and the counter lands at L —
// no over-consumption under maximal concurrency.
func TestConsumeQuotaConcurrencyNeverExceedsHardLimit(t *testing.T) {
	deps := newDeps(t)
	mod := modFor(deps, fakeCatalog{}, &fakeSubs{hasSub: false}, "deny")
	tenantID := uuid.New()
	const limit, callers = 20, 200
	seedFeature(t, mod, service.FeatureInput{Key: "api_calls", Type: "limit", DefaultValue: int64(limit), LimitBehavior: "hard"})

	var ok, exceeded int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := mod.Service().ConsumeQuota(context.Background(), tenantID, "api_calls", 1)
			switch {
			case err == nil:
				atomic.AddInt64(&ok, 1)
			case apperr.KindOf(err) == apperr.KindQuotaExceeded:
				atomic.AddInt64(&exceeded, 1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if ok != limit {
		t.Fatalf("accepted consumes = %d, want %d", ok, limit)
	}
	if exceeded != callers-limit {
		t.Fatalf("rejected consumes = %d, want %d", exceeded, callers-limit)
	}
	if used := usedInDB(t, deps.Pool, tenantID, "api_calls"); used != limit {
		t.Fatalf("counter used = %d, want %d (hard limit breached)", used, limit)
	}
}

// TestSoftLimitConsumesAndWarnsOncePerCrossing: a soft limit never refuses a
// consume, and the single consume that crosses the limit emits exactly one
// EntitlementLimitWarning — even when many callers cross concurrently.
func TestSoftLimitConsumesAndWarnsOncePerCrossing(t *testing.T) {
	deps := newDeps(t)
	mod := modFor(deps, fakeCatalog{}, &fakeSubs{hasSub: false}, "deny")
	tenantID := uuid.New()
	const limit, callers = 3, 25
	seedFeature(t, mod, service.FeatureInput{Key: "events", Type: "limit", DefaultValue: int64(limit), LimitBehavior: "soft"})

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := mod.Service().ConsumeQuota(context.Background(), tenantID, "events", 1); err != nil {
				t.Errorf("soft consume should never fail: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	// All consumes landed (soft never blocks).
	if used := usedInDB(t, deps.Pool, tenantID, "events"); used != callers {
		t.Fatalf("counter used = %d, want %d", used, callers)
	}
	// Exactly one warning across the whole crossing.
	if n := countEvents(t, deps.Pool, tenantID, ports.EventEntitlementLimitWarning); n != 1 {
		t.Fatalf("limit_warning events = %d, want 1", n)
	}
}

// TestReleaseQuotaFreesCapacity: releasing units under a hard limit frees room
// for further consumes; releases floor at zero.
func TestReleaseQuotaFreesCapacity(t *testing.T) {
	deps := newDeps(t)
	mod := modFor(deps, fakeCatalog{}, &fakeSubs{hasSub: false}, "deny")
	tenantID := uuid.New()
	ctx := context.Background()
	seedFeature(t, mod, service.FeatureInput{Key: "seats", Type: "limit", DefaultValue: int64(3), LimitBehavior: "hard"})

	for i := 0; i < 3; i++ {
		if _, err := mod.Service().ConsumeQuota(ctx, tenantID, "seats", 1); err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
	}
	// At the limit: the next consume is refused.
	if _, err := mod.Service().ConsumeQuota(ctx, tenantID, "seats", 1); apperr.KindOf(err) != apperr.KindQuotaExceeded {
		t.Fatalf("consume at limit err = %v, want quota_exceeded", err)
	}

	// Release two, then two consumes fit again.
	u, err := mod.Service().ReleaseQuota(ctx, tenantID, "seats", 2)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if u.Used != 1 {
		t.Fatalf("used after release = %d, want 1", u.Used)
	}
	for i := 0; i < 2; i++ {
		if _, err := mod.Service().ConsumeQuota(ctx, tenantID, "seats", 1); err != nil {
			t.Fatalf("consume after release %d: %v", i, err)
		}
	}
	// Back at the limit again.
	if _, err := mod.Service().ConsumeQuota(ctx, tenantID, "seats", 1); apperr.KindOf(err) != apperr.KindQuotaExceeded {
		t.Fatalf("consume at limit (2) err = %v, want quota_exceeded", err)
	}

	// Over-release floors at zero.
	u, err = mod.Service().ReleaseQuota(ctx, tenantID, "seats", 999)
	if err != nil {
		t.Fatalf("over-release: %v", err)
	}
	if u.Used != 0 {
		t.Fatalf("used after over-release = %d, want 0", u.Used)
	}
}

// TestLazyPeriodResetOnFirstAccessAfterBoundary: with a frozen clock, both a
// monthly feature and a billing_cycle feature reset the moment their period
// boundary passes — no background job. The first access in the new period sees a
// zero counter simply because it maps to a new period key.
func TestLazyPeriodResetOnFirstAccessAfterBoundary(t *testing.T) {
	clk := clock.NewFrozen(time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	deps := frozenDeps(t, clk)
	pv := uuid.New()
	subs := &fakeSubs{hasSub: true, sub: subports.SubscriptionInfo{
		ID: uuid.New(), PlanVersionID: pv,
		CurrentPeriodStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		CurrentPeriodEnd:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	}}
	mod := modFor(deps, fakeCatalog{}, subs, "deny")
	tenantID := uuid.New()
	ctx := context.Background()
	seedFeature(t, mod, service.FeatureInput{Key: "monthly_calls", Type: "limit", DefaultValue: int64(2), LimitBehavior: "hard", ResetPeriod: "monthly"})
	seedFeature(t, mod, service.FeatureInput{Key: "cycle_calls", Type: "limit", DefaultValue: int64(2), LimitBehavior: "hard", ResetPeriod: "billing_cycle"})

	// Fill both to their limit in the January period.
	for _, key := range []string{"monthly_calls", "cycle_calls"} {
		for i := 0; i < 2; i++ {
			if _, err := mod.Service().ConsumeQuota(ctx, tenantID, key, 1); err != nil {
				t.Fatalf("consume %s: %v", key, err)
			}
		}
		if _, err := mod.Service().ConsumeQuota(ctx, tenantID, key, 1); apperr.KindOf(err) != apperr.KindQuotaExceeded {
			t.Fatalf("%s at limit err = %v, want quota_exceeded", key, err)
		}
	}

	// Cross both boundaries: a new calendar month and a new billing cycle.
	clk.Set(time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC))
	subs.sub.CurrentPeriodStart = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	subs.sub.CurrentPeriodEnd = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	// First access in the new period sees a fresh (zero) counter — reset, no job.
	for _, key := range []string{"monthly_calls", "cycle_calls"} {
		u, err := mod.Service().GetUsage(ctx, tenantID, key)
		if err != nil {
			t.Fatalf("get usage %s: %v", key, err)
		}
		if u.Used != 0 {
			t.Fatalf("%s used after boundary = %d, want 0 (period did not reset)", key, u.Used)
		}
		if _, err := mod.Service().ConsumeQuota(ctx, tenantID, key, 1); err != nil {
			t.Fatalf("consume %s in new period: %v", key, err)
		}
		u, _ = mod.Service().GetUsage(ctx, tenantID, key)
		if u.Used != 1 {
			t.Fatalf("%s used in new period = %d, want 1", key, u.Used)
		}
	}
}

// TestDowngradeBelowUsageEmitsExceededAndKeepsServing: when a plan change shrinks
// a limit below the tenant's current usage, re-materialization emits one
// EntitlementExceeded event; reads keep serving the current usage, and only
// future consumes are gated by the new, smaller limit.
func TestDowngradeBelowUsageEmitsExceededAndKeepsServing(t *testing.T) {
	deps := newDeps(t)
	pvBig, pvSmall := uuid.New(), uuid.New()
	cat := fakeCatalog{grants: map[uuid.UUID]map[string]any{
		pvBig:   {"api_calls": float64(10)},
		pvSmall: {"api_calls": float64(5)},
	}}
	subs := &fakeSubs{hasSub: true, sub: subports.SubscriptionInfo{ID: uuid.New(), PlanVersionID: pvBig}}
	mod := modFor(deps, cat, subs, "deny")
	tenantID := uuid.New()
	ctx := context.Background()
	seedFeature(t, mod, service.FeatureInput{Key: "api_calls", Type: "limit", DefaultValue: int64(0), LimitBehavior: "hard"})

	// Baseline: effective limit 10. Consume 8.
	if err := mod.Materialize(ctx, tenantID); err != nil {
		t.Fatalf("baseline materialize: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := mod.Service().ConsumeQuota(ctx, tenantID, "api_calls", 1); err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
	}

	// Downgrade to the plan granting 5 (below the 8 already used).
	subs.sub.PlanVersionID = pvSmall
	deliverPlanChanged(t, mod, tenantID)

	if n := countEvents(t, deps.Pool, tenantID, ports.EventEntitlementExceeded); n != 1 {
		t.Fatalf("exceeded events = %d, want 1", n)
	}

	// Reads keep serving: usage is still visible, now against the smaller limit.
	u, err := mod.Service().GetUsage(ctx, tenantID, "api_calls")
	if err != nil {
		t.Fatalf("get usage after downgrade: %v", err)
	}
	if u.Used != 8 || u.Limit != 5 {
		t.Fatalf("usage after downgrade = used %d / limit %d, want 8 / 5", u.Used, u.Limit)
	}

	// Future consumes are gated by the new limit.
	if _, err := mod.Service().ConsumeQuota(ctx, tenantID, "api_calls", 1); apperr.KindOf(err) != apperr.KindQuotaExceeded {
		t.Fatalf("consume after downgrade err = %v, want quota_exceeded", err)
	}

	// Re-materializing the same (already-downgraded) state emits nothing new.
	deliverPlanChanged(t, mod, tenantID)
	if n := countEvents(t, deps.Pool, tenantID, ports.EventEntitlementExceeded); n != 1 {
		t.Fatalf("exceeded events after no-op re-materialize = %d, want 1", n)
	}
}

// TestConsumeEndpointAndUsageEndpoint exercises the HTTP surface: consume climbs
// usage and returns 200, a breach returns a typed quota-exceeded problem+json,
// and the usage endpoints report used/limit.
func TestConsumeEndpointAndUsageEndpoint(t *testing.T) {
	deps := newDeps(t)
	mod := modFor(deps, fakeCatalog{}, &fakeSubs{hasSub: false}, "deny")
	tenantID, userID := uuid.New(), uuid.New()
	seedFeature(t, mod, service.FeatureInput{Key: "api_calls", Type: "limit", DefaultValue: int64(3), LimitBehavior: "hard"})

	srv := serverFor(mod.Handler(), tenantID, userID)
	defer srv.Close()

	// Three consumes succeed and usage climbs.
	for want := int64(1); want <= 3; want++ {
		status, body := postJSON(t, srv.URL+"/consume", map[string]any{"key": "api_calls", "n": 1})
		if status != http.StatusOK {
			t.Fatalf("consume %d = %d (%s)", want, status, body)
		}
		var resp struct {
			Used  int64 `json:"used"`
			Limit int64 `json:"limit"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("unmarshal consume: %v", err)
		}
		if resp.Used != want || resp.Limit != 3 {
			t.Fatalf("consume %d => used %d / limit %d", want, resp.Used, resp.Limit)
		}
	}

	// The fourth consume breaches the hard limit: 422 problem+json.
	status, body, ctype := postJSONFull(t, srv.URL+"/consume", map[string]any{"key": "api_calls", "n": 1})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("over-limit consume = %d (%s), want 422", status, body)
	}
	if ctype != "application/problem+json" {
		t.Fatalf("content-type = %q, want application/problem+json", ctype)
	}
	var prob struct {
		Status int    `json:"status"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal([]byte(body), &prob); err != nil || prob.Status != 422 {
		t.Fatalf("problem body = %s (err %v)", body, err)
	}

	// GET /usage/{key} and GET /usage report used/limit.
	status, body = get(t, srv.URL+"/usage/api_calls")
	if status != http.StatusOK {
		t.Fatalf("GET usage/api_calls = %d (%s)", status, body)
	}
	var one struct {
		Used  int64 `json:"used"`
		Limit int64 `json:"limit"`
	}
	if err := json.Unmarshal([]byte(body), &one); err != nil || one.Used != 3 || one.Limit != 3 {
		t.Fatalf("usage/api_calls = %s (err %v), want used 3 limit 3", body, err)
	}

	status, body = get(t, srv.URL+"/usage")
	if status != http.StatusOK {
		t.Fatalf("GET usage = %d (%s)", status, body)
	}
	var all struct {
		Usage []struct {
			Key  string `json:"key"`
			Used int64  `json:"used"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(body), &all); err != nil {
		t.Fatalf("unmarshal usage list: %v (%s)", err, body)
	}
	if len(all.Usage) != 1 || all.Usage[0].Key != "api_calls" || all.Usage[0].Used != 3 {
		t.Fatalf("usage list = %s, want one api_calls used 3", body)
	}
}

// --- helpers ---

func usedInDB(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, key string) int64 {
	t.Helper()
	var used int64
	err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(sum(used), 0) FROM entitlements.usage_counters WHERE tenant_id = $1 AND feature_key = $2`,
		tenantID, key).Scan(&used)
	if err != nil {
		t.Fatalf("read usage for %s: %v", key, err)
	}
	return used
}

func countEvents(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, eventType string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM platform.outbox WHERE tenant_id = $1 AND event_type = $2`,
		tenantID, eventType).Scan(&n); err != nil {
		t.Fatalf("count %s events: %v", eventType, err)
	}
	return n
}

func postJSON(t *testing.T, url string, payload any) (int, string) {
	t.Helper()
	status, body, _ := postJSONFull(t, url, payload)
	return status, body
}

func postJSONFull(t *testing.T, url string, payload any) (int, string, string) {
	t.Helper()
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header.Get("Content-Type")
}
