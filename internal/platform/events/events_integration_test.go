//go:build integration

package events_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

type harness struct {
	pool  *pgxpool.Pool
	uow   *postgres.UnitOfWork
	clk   *clock.Frozen
	out   *events.Outbox
	bus   *events.Bus
	relay *events.Relay
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	pool := testkit.Postgres(t)
	clk := clock.NewFrozen(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	bus := events.NewBus()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	// A fixed one-minute backoff keeps the failure/retry test deterministic
	// under the frozen clock.
	relay := events.NewRelay(pool, bus, clk, logger, events.RelayConfig{
		BatchSize: 50,
		Backoff:   func(int) time.Duration { return time.Minute },
	})
	return &harness{pool: pool, uow: postgres.NewUnitOfWork(pool), clk: clk, out: out(pool, clk), bus: bus, relay: relay}
}

func out(pool *pgxpool.Pool, clk clock.Clock) *events.Outbox {
	return events.NewOutbox(pool, id.UUIDv7{}, clk)
}

func (h *harness) publish(t *testing.T, tenant uuid.UUID, typ string, payload any) events.Event {
	t.Helper()
	e, err := h.out.Publish(context.Background(), events.EventInput{
		TenantID: tenant, Module: "test", Type: typ, Payload: payload,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return e
}

func (h *harness) countUnpublished(t *testing.T) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM platform.outbox WHERE published_at IS NULL`).Scan(&n); err != nil {
		t.Fatalf("count unpublished: %v", err)
	}
	return n
}

func (h *harness) row(t *testing.T, eventID uuid.UUID) (attempts int, published bool) {
	t.Helper()
	var publishedAt *time.Time
	if err := h.pool.QueryRow(context.Background(),
		`SELECT attempts, published_at FROM platform.outbox WHERE id = $1`, eventID).
		Scan(&attempts, &publishedAt); err != nil {
		t.Fatalf("read outbox row: %v", err)
	}
	return attempts, publishedAt != nil
}

func TestAppendInvisibleAfterRollback(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	businessErr := errors.New("business failure")
	err := h.uow.Do(ctx, func(ctx context.Context) error {
		if _, err := h.out.Publish(ctx, events.EventInput{
			TenantID: uuid.New(), Module: "test", Type: "thing.created", Payload: map[string]string{"a": "b"},
		}); err != nil {
			return err
		}
		return businessErr // roll the whole transaction back
	})
	if !errors.Is(err, businessErr) {
		t.Fatalf("Do error = %v, want businessErr", err)
	}

	if n := h.countUnpublished(t); n != 0 {
		t.Fatalf("outbox has %d rows after rollback, want 0", n)
	}
}

func TestAppendPublishedAfterCommit(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	var got events.Event
	h.bus.Subscribe("thing.created", func(_ context.Context, e events.Event) error {
		got = e
		return nil
	})

	published := h.publish(t, uuid.New(), "thing.created", map[string]string{"name": "widget"})

	n, err := h.relay.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if n != 1 {
		t.Fatalf("published = %d, want 1", n)
	}
	if got.ID != published.ID || got.Type != "thing.created" {
		t.Fatalf("handler received %+v, want event %s", got, published.ID)
	}
	var payload map[string]string
	if err := json.Unmarshal(got.Payload, &payload); err != nil || payload["name"] != "widget" {
		t.Fatalf("payload = %s (err %v), want name=widget", got.Payload, err)
	}
	if _, isPublished := h.row(t, published.ID); !isPublished {
		t.Fatal("event not marked published after delivery")
	}
}

func TestRelayRedeliversUnackedAfterRestart(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	var calls int
	h.bus.Subscribe("thing.created", func(_ context.Context, _ events.Event) error {
		calls++
		return nil
	})
	published := h.publish(t, uuid.New(), "thing.created", map[string]int{"n": 1})

	// Simulate a relay that fetched and locked the row but crashed before it
	// could commit the "published" mark: the transaction rolls back, so the
	// event remains unacked.
	_ = h.uow.Do(ctx, func(ctx context.Context) error {
		var gotID uuid.UUID
		if err := postgres.Q(ctx, h.pool).QueryRow(ctx,
			`SELECT id FROM platform.outbox WHERE published_at IS NULL FOR UPDATE SKIP LOCKED LIMIT 1`).
			Scan(&gotID); err != nil {
			return err
		}
		return errors.New("simulated crash before ack")
	})

	if _, isPublished := h.row(t, published.ID); isPublished {
		t.Fatal("event marked published despite simulated crash before ack")
	}
	if calls != 0 {
		t.Fatalf("handler ran %d times during simulated crash, want 0", calls)
	}

	// A fresh relay pass redelivers the unacked event exactly once.
	n, err := h.relay.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if n != 1 || calls != 1 {
		t.Fatalf("redelivery: published=%d calls=%d, want 1/1", n, calls)
	}
	if _, isPublished := h.row(t, published.ID); !isPublished {
		t.Fatal("event not published after redelivery")
	}
}

func TestHandlerFailureIncrementsAttemptsWithBackoff(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	var calls int
	h.bus.Subscribe("thing.created", func(_ context.Context, _ events.Event) error {
		calls++
		if calls == 1 {
			return errors.New("transient failure")
		}
		return nil
	})
	published := h.publish(t, uuid.New(), "thing.created", map[string]int{"n": 1})

	// First pass: the handler fails, so nothing is published and the attempt
	// is recorded with a future next_attempt_at.
	n, err := h.relay.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("ProcessBatch #1: %v", err)
	}
	if n != 0 {
		t.Fatalf("published after failure = %d, want 0", n)
	}
	attempts, isPublished := h.row(t, published.ID)
	if attempts != 1 || isPublished {
		t.Fatalf("after failure: attempts=%d published=%v, want 1/false", attempts, isPublished)
	}

	// The event is not eligible again until the backoff elapses.
	if n, _ := h.relay.ProcessBatch(ctx); n != 0 {
		t.Fatalf("event redelivered before backoff elapsed: published=%d", n)
	}

	// After advancing past the backoff, it is redelivered and succeeds.
	h.clk.Advance(2 * time.Minute)
	n, err = h.relay.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("ProcessBatch #3: %v", err)
	}
	if n != 1 || calls != 2 {
		t.Fatalf("after backoff: published=%d calls=%d, want 1/2", n, calls)
	}
	if _, isPublished := h.row(t, published.ID); !isPublished {
		t.Fatal("event not published after successful retry")
	}
}

func TestIdempotentWrapperDropsDuplicateDelivery(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	var effect int
	handler := events.Idempotent("consumer-a", h.pool, func(_ context.Context, _ events.Event) error {
		effect++
		return nil
	})

	evt := events.Event{
		ID:         uuid.New(),
		OccurredAt: h.clk.Now(),
		TenantID:   uuid.New(),
		Module:     "test",
		Type:       "thing.created",
		Payload:    json.RawMessage(`{}`),
	}

	// Deliver the same event twice, each in its own transaction (as the relay
	// would across two deliveries).
	for i := 0; i < 2; i++ {
		if err := h.uow.Do(ctx, func(ctx context.Context) error {
			return handler(ctx, evt)
		}); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}

	if effect != 1 {
		t.Fatalf("handler effect ran %d times, want 1", effect)
	}

	var processed int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM platform.processed_events WHERE consumer = $1 AND event_id = $2`,
		"consumer-a", evt.ID).Scan(&processed); err != nil {
		t.Fatalf("count processed_events: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed_events rows = %d, want 1", processed)
	}
}

func TestTwoRelaysNoDoubleDispatch(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	const total = 20
	var mu sync.Mutex
	seen := make(map[uuid.UUID]int, total)
	h.bus.Subscribe("thing.created", func(_ context.Context, e events.Event) error {
		mu.Lock()
		seen[e.ID]++
		mu.Unlock()
		return nil
	})

	for i := 0; i < total; i++ {
		h.publish(t, uuid.New(), "thing.created", map[string]int{"n": i})
	}

	// A second relay over the same pool and bus.
	relay2 := events.NewRelay(h.pool, h.bus, h.clk, slog.New(slog.NewJSONHandler(io.Discard, nil)),
		events.RelayConfig{BatchSize: 50})

	var wg sync.WaitGroup
	worker := func(r *events.Relay) {
		defer wg.Done()
		for h.countUnpublished(t) > 0 {
			if _, err := r.ProcessBatch(ctx); err != nil {
				t.Errorf("ProcessBatch: %v", err)
				return
			}
		}
	}
	wg.Add(2)
	go worker(h.relay)
	go worker(relay2)
	wg.Wait()

	if len(seen) != total {
		t.Fatalf("distinct events dispatched = %d, want %d", len(seen), total)
	}
	for evID, count := range seen {
		if count != 1 {
			t.Fatalf("event %s dispatched %d times, want exactly 1", evID, count)
		}
	}
}

func TestEventsDispatchedInPerTenantOrder(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	tenant := uuid.New()
	var order []int
	h.bus.Subscribe("seq", func(_ context.Context, e events.Event) error {
		var p struct {
			N int `json:"n"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		order = append(order, p.N)
		return nil
	})

	// Publish with strictly increasing occurred_at for the same tenant.
	for i := 0; i < 5; i++ {
		h.clk.Advance(time.Second)
		h.publish(t, tenant, "seq", map[string]int{"n": i})
	}

	if _, err := h.relay.ProcessBatch(ctx); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	want := []int{0, 1, 2, 3, 4}
	if len(order) != len(want) {
		t.Fatalf("dispatched %d events, want %d", len(order), len(want))
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("dispatch order = %v, want %v (per-tenant occurred_at order)", order, want)
		}
	}
}
