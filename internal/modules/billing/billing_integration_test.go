//go:build integration

package billing_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	"github.com/williamokano/entitlements/internal/modules/billing"
	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	subports "github.com/williamokano/entitlements/internal/modules/subscription/ports"
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

// fakeCatalog serves a mutable plan version and addon versions so tests can
// change the catalog after issuance and prove the invoice is unaffected.
type fakeCatalog struct {
	mu     sync.Mutex
	planPV catalogports.PlanVersionInfo
	addons map[uuid.UUID]catalogports.AddonVersionInfo
}

func (f *fakeCatalog) GetPlanVersion(_ context.Context, id uuid.UUID) (catalogports.PlanVersionInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pv := f.planPV
	pv.ID = id
	return pv, nil
}

func (f *fakeCatalog) GetAddonVersion(_ context.Context, id uuid.UUID) (catalogports.AddonVersionInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if av, ok := f.addons[id]; ok {
		return av, nil
	}
	return catalogports.AddonVersionInfo{
		ID: id, AddonKey: "seats", Version: 1, Status: "published", Currency: "USD",
		Prices: []catalogports.PriceInfo{{Cycle: "monthly", AmountMinor: 500}},
	}, nil
}

func (f *fakeCatalog) setPlan(pv catalogports.PlanVersionInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.planPV = pv
}

// fakeSubs serves a canned live subscription per tenant and its attached addons.
type fakeSubs struct {
	planVersionID uuid.UUID
	cycle         string
	addons        []subports.AddonAttachment
	subByTenant   map[uuid.UUID]uuid.UUID
	mu            sync.Mutex
}

func (f *fakeSubs) GetLiveForTenant(_ context.Context, tenantID uuid.UUID) (subports.SubscriptionInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subByTenant == nil {
		f.subByTenant = map[uuid.UUID]uuid.UUID{}
	}
	subID, ok := f.subByTenant[tenantID]
	if !ok {
		subID = uuid.New()
		f.subByTenant[tenantID] = subID
	}
	return subports.SubscriptionInfo{
		ID: subID, TenantID: tenantID, PlanVersionID: f.planVersionID,
		BillingCycle: f.cycle, Status: "active",
	}, nil
}

func (f *fakeSubs) GetAttachedAddons(_ context.Context, _ uuid.UUID) ([]subports.AddonAttachment, error) {
	return f.addons, nil
}

func defaultCatalog() *fakeCatalog {
	return &fakeCatalog{
		planPV: catalogports.PlanVersionInfo{
			PlanKey: "pro", Version: 1, Status: "published", Currency: "USD",
			Prices: []catalogports.PriceInfo{{Cycle: "monthly", AmountMinor: 1000}},
		},
		addons: map[uuid.UUID]catalogports.AddonVersionInfo{},
	}
}

func ctxFor(tenantID uuid.UUID) context.Context {
	ctx := authctx.WithTenantID(context.Background(), tenantID)
	return authctx.WithPrincipal(ctx, authctx.Principal{Kind: authctx.PrincipalUser, Subject: uuid.NewString()})
}

// TestIssuedInvoiceImmuneToCatalogChanges issues an invoice, then publishes a new
// plan version and renames the plan; the stored invoice's lines are unchanged.
func TestIssuedInvoiceImmuneToCatalogChanges(t *testing.T) {
	deps := newDeps(t)
	cat := defaultCatalog()
	addonID := uuid.New()
	cat.addons[addonID] = catalogports.AddonVersionInfo{
		ID: addonID, AddonKey: "seats", Version: 1, Status: "published", Currency: "USD",
		Prices: []catalogports.PriceInfo{{Cycle: "monthly", AmountMinor: 500}},
	}
	subs := &fakeSubs{
		planVersionID: uuid.New(), cycle: "monthly",
		addons: []subports.AddonAttachment{{AddonVersionID: addonID, Quantity: 3}},
	}
	mod := billing.New(deps, cat, subs)
	tenantID := uuid.New()
	ctx := ctxFor(tenantID)

	issued, err := issue(ctx, mod)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// plan 1000*1 + addon 500*3 = 2500.
	assertContains(t, issued, `"total_minor":2500`)
	assertContains(t, issued, `"key":"pro"`)
	assertContains(t, issued, `"version":1`)

	// Now the catalog changes: new plan version, renamed key, new price.
	cat.setPlan(catalogports.PlanVersionInfo{
		PlanKey: "pro-renamed", Version: 2, Status: "published", Currency: "USD",
		Prices: []catalogports.PriceInfo{{Cycle: "monthly", AmountMinor: 9999}},
	})
	cat.addons[addonID] = catalogports.AddonVersionInfo{
		ID: addonID, AddonKey: "seats-renamed", Version: 5, Status: "published", Currency: "USD",
		Prices: []catalogports.PriceInfo{{Cycle: "monthly", AmountMinor: 7777}},
	}

	// Re-read the stored invoice: it still reflects the snapshot at issuance.
	id := extractID(t, issued)
	got, err := mod.Port().GetInvoice(ctx, id)
	if err != nil {
		t.Fatalf("get invoice: %v", err)
	}
	if got.TotalMinor != 2500 {
		t.Fatalf("total changed after catalog change: got %d, want 2500", got.TotalMinor)
	}
	// The full view still carries the original keys/versions/prices.
	view, err := getInvoiceView(ctx, mod, id)
	if err != nil {
		t.Fatalf("get view: %v", err)
	}
	assertContains(t, view, `"key":"pro"`)
	assertContains(t, view, `"key":"seats"`)
	assertContains(t, view, `"unit_price_minor":1000`)
	assertContains(t, view, `"unit_price_minor":500`)
	if strings.Contains(view, "pro-renamed") || strings.Contains(view, "seats-renamed") || strings.Contains(view, "9999") {
		t.Fatalf("invoice was rewritten by catalog change: %s", view)
	}
}

// TestPerTenantNumberingGaplessUnderConcurrency issues N invoices concurrently
// for one tenant and asserts the numbers are exactly 1..N with no gaps or dupes;
// a second tenant's sequence is independent.
func TestPerTenantNumberingGaplessUnderConcurrency(t *testing.T) {
	deps := newDeps(t)
	cat := defaultCatalog()
	subs := &fakeSubs{planVersionID: uuid.New(), cycle: "monthly"}
	mod := billing.New(deps, cat, subs)

	tenantA := uuid.New()
	const n = 25
	numbers := make([]int64, n)
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body, err := issue(ctxFor(tenantA), mod)
			if err != nil {
				errs <- err
				return
			}
			numbers[i] = extractNumber(t, body)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent issue: %v", err)
	}

	sort.Slice(numbers, func(i, j int) bool { return numbers[i] < numbers[j] })
	for i := 0; i < n; i++ {
		if numbers[i] != int64(i+1) {
			t.Fatalf("numbers not gapless 1..%d: got %v", n, numbers)
		}
	}

	// A second tenant's sequence starts fresh at 1.
	tenantB := uuid.New()
	b1, err := issue(ctxFor(tenantB), mod)
	if err != nil {
		t.Fatalf("tenantB issue: %v", err)
	}
	if got := extractNumber(t, b1); got != 1 {
		t.Fatalf("tenantB first invoice number = %d, want 1 (independent per tenant)", got)
	}
}

// TestCreditNoteReferencesInvoiceAndNegatesAmounts issues and pays an invoice,
// then creates a credit note for its full total and asserts it references the
// invoice and stores a negated amount.
func TestCreditNoteReferencesInvoiceAndNegatesAmounts(t *testing.T) {
	deps := newDeps(t)
	cat := defaultCatalog()
	subs := &fakeSubs{planVersionID: uuid.New(), cycle: "monthly"}
	mod := billing.New(deps, cat, subs)
	tenantID := uuid.New()
	ctx := ctxFor(tenantID)

	body, err := issue(ctx, mod)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	id := extractID(t, body)
	total := extractInt(t, body, "total_minor") // 1000

	// Pay it so it is creditable, then credit the full total.
	srv := serverFor(mod.Handler(), tenantID)
	defer srv.Close()
	if code, resp := post(t, srv.URL+"/invoices/"+id.String()+"/pay", ""); code != http.StatusOK {
		t.Fatalf("pay = %d %s", code, resp)
	}
	code, resp := post(t, srv.URL+"/invoices/"+id.String()+"/credit-notes", `{"amount_minor":`+itoa(total)+`,"reason":"refund"}`)
	if code != http.StatusCreated {
		t.Fatalf("credit note = %d %s", code, resp)
	}
	assertContains(t, resp, `"invoice_id":"`+id.String()+`"`)
	assertContains(t, resp, `"amount_minor":-`+itoa(total))
}

// TestInvoiceListGetScopedToTenant proves another tenant's invoices are invisible.
func TestInvoiceListGetScopedToTenant(t *testing.T) {
	deps := newDeps(t)
	cat := defaultCatalog()
	subs := &fakeSubs{planVersionID: uuid.New(), cycle: "monthly"}
	mod := billing.New(deps, cat, subs)

	tenantA, tenantB := uuid.New(), uuid.New()

	srvA := serverFor(mod.Handler(), tenantA)
	defer srvA.Close()
	code, body := post(t, srvA.URL+"/invoices", "")
	if code != http.StatusCreated {
		t.Fatalf("issue for A = %d %s", code, body)
	}
	idA := extractID(t, body)

	// Tenant A sees its invoice.
	if code, lb := get(t, srvA.URL+"/invoices"); code != http.StatusOK || !strings.Contains(lb, idA.String()) {
		t.Fatalf("A list = %d %s", code, lb)
	}

	// Tenant B's list is empty and cannot GET A's invoice.
	srvB := serverFor(mod.Handler(), tenantB)
	defer srvB.Close()
	code, lb := get(t, srvB.URL+"/invoices")
	if code != http.StatusOK {
		t.Fatalf("B list = %d %s", code, lb)
	}
	if strings.Contains(lb, idA.String()) {
		t.Fatalf("tenant B sees tenant A's invoice: %s", lb)
	}
	if code, gb := get(t, srvB.URL+"/invoices/"+idA.String()); code != http.StatusNotFound {
		t.Fatalf("B get A invoice = %d %s, want 404", code, gb)
	}
}

// --- helpers ---

func issue(ctx context.Context, mod *billing.Module) (string, error) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mod.Handler().ServeHTTP(w, r.WithContext(ctx))
	}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/invoices", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return string(b), errStatus(resp.StatusCode, string(b))
	}
	return string(b), nil
}

func getInvoiceView(ctx context.Context, mod *billing.Module, id uuid.UUID) (string, error) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mod.Handler().ServeHTTP(w, r.WithContext(ctx))
	}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/invoices/"+id.String(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func serverFor(h http.Handler, tenantID uuid.UUID) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := authctx.WithTenantID(r.Context(), tenantID)
		ctx = authctx.WithPrincipal(ctx, authctx.Principal{Kind: authctx.PrincipalUser, Subject: uuid.NewString()})
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
}

func post(t *testing.T, url, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(http.MethodPost, url, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func assertContains(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("expected %q in %s", want, body)
	}
}

func extractID(t *testing.T, body string) uuid.UUID {
	t.Helper()
	var m struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode id: %v (%s)", err, body)
	}
	return m.ID
}

func extractNumber(t *testing.T, body string) int64 {
	t.Helper()
	var m struct {
		Number int64 `json:"number"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode number: %v (%s)", err, body)
	}
	return m.Number
}

func extractInt(t *testing.T, body, field string) int64 {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	var n int64
	if err := json.Unmarshal(m[field], &n); err != nil {
		t.Fatalf("decode %s: %v", field, err)
	}
	return n
}

func itoa(n int64) string {
	return strings.TrimSpace(string(func() []byte {
		b, _ := json.Marshal(n)
		return b
	}()))
}

type statusErr struct {
	code int
	body string
}

func (e statusErr) Error() string { return e.body }

func errStatus(code int, body string) error { return statusErr{code: code, body: body} }
