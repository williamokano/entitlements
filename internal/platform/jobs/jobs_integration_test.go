//go:build integration

package jobs_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/jobs"
	"github.com/williamokano/entitlements/internal/platform/testkit"
)

func newRunner(t *testing.T, pool *pgxpool.Pool, clk clock.Clock) *jobs.Runner {
	t.Helper()
	return jobs.NewRunner(pool, id.UUIDv7{}, clk, discardLogger(), time.Millisecond)
}

func lastRun(t *testing.T, pool *pgxpool.Pool, name string) (status string, started time.Time, finished *time.Time, errText *string, ok bool) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT status, started_at, finished_at, error
		 FROM platform.job_runs WHERE job_name = $1 ORDER BY started_at DESC LIMIT 1`, name).
		Scan(&status, &started, &finished, &errText)
	if err != nil {
		return "", time.Time{}, nil, nil, false
	}
	return status, started, finished, errText, true
}

func runCount(t *testing.T, pool *pgxpool.Pool, name string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM platform.job_runs WHERE job_name = $1`, name).Scan(&n); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	return n
}

func TestTwoRunnersSingleExecutionPerTick(t *testing.T) {
	pool := testkit.Postgres(t)
	clk := clock.NewFrozen(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	var counter atomic.Int64
	inc := func(context.Context) error { counter.Add(1); return nil }

	// Two runners over the same database, both registering the same job.
	rA := newRunner(t, pool, clk)
	rB := newRunner(t, pool, clk)
	for _, r := range []*jobs.Runner{rA, rB} {
		if err := r.Register("tick", time.Minute, inc); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	runBoth := func() {
		var wg sync.WaitGroup
		wg.Add(2)
		for _, r := range []*jobs.Runner{rA, rB} {
			go func(r *jobs.Runner) {
				defer wg.Done()
				if err := r.RunDue(ctx); err != nil {
					t.Errorf("RunDue: %v", err)
				}
			}(r)
		}
		wg.Wait()
	}

	runBoth()
	if got := counter.Load(); got != 1 {
		t.Fatalf("after first tick, executions = %d, want 1", got)
	}

	// Not yet due again at the same instant.
	runBoth()
	if got := counter.Load(); got != 1 {
		t.Fatalf("job ran again before interval elapsed: executions = %d, want 1", got)
	}

	// After the interval elapses, exactly one more execution.
	clk.Advance(time.Minute)
	runBoth()
	if got := counter.Load(); got != 2 {
		t.Fatalf("after second tick, executions = %d, want 2", got)
	}
	if n := runCount(t, pool, "tick"); n != 2 {
		t.Fatalf("job_runs rows = %d, want 2", n)
	}
}

func TestJobPanicIsolatedAndRunnerContinues(t *testing.T) {
	pool := testkit.Postgres(t)
	clk := clock.NewFrozen(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()
	r := newRunner(t, pool, clk)

	var counter atomic.Int64
	if err := r.Register("boom", time.Minute, func(context.Context) error {
		panic("kaboom")
	}); err != nil {
		t.Fatalf("register boom: %v", err)
	}
	if err := r.Register("safe", time.Minute, func(context.Context) error {
		counter.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("register safe: %v", err)
	}

	// RunDue must not propagate the panic and must still run the sibling job.
	if err := r.RunDue(ctx); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if counter.Load() != 1 {
		t.Fatalf("sibling job executions = %d, want 1", counter.Load())
	}

	status, _, finished, errText, ok := lastRun(t, pool, "boom")
	if !ok || status != "failed" || finished == nil || errText == nil || !strings.Contains(*errText, "panic") {
		t.Fatalf("panic run bookkeeping = (%s, finished=%v, err=%v), want failed/panic", status, finished, errText)
	}

	// The next tick is unaffected: the runner keeps working.
	clk.Advance(time.Minute)
	if err := r.RunDue(ctx); err != nil {
		t.Fatalf("RunDue after panic: %v", err)
	}
	if counter.Load() != 2 {
		t.Fatalf("sibling job executions after next tick = %d, want 2", counter.Load())
	}
}

func TestPerRunTimeoutCancelsJobContext(t *testing.T) {
	pool := testkit.Postgres(t)
	clk := clock.NewFrozen(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()
	r := newRunner(t, pool, clk)

	if err := r.Register("slow", time.Minute, func(ctx context.Context) error {
		<-ctx.Done() // block until the per-run timeout cancels us
		return ctx.Err()
	}, jobs.WithTimeout(50*time.Millisecond)); err != nil {
		t.Fatalf("register: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = r.RunDue(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunDue did not return; per-run timeout failed to cancel the job")
	}

	status, _, finished, errText, ok := lastRun(t, pool, "slow")
	if !ok || status != "failed" || finished == nil || errText == nil {
		t.Fatalf("timeout run bookkeeping = (%s, finished=%v, err=%v), want failed", status, finished, errText)
	}
	if !strings.Contains(*errText, "deadline") {
		t.Fatalf("timeout run error = %q, want to mention deadline", *errText)
	}
}

func TestLastRunBookkeepingRecorded(t *testing.T) {
	pool := testkit.Postgres(t)
	clk := clock.NewFrozen(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()
	r := newRunner(t, pool, clk)

	if err := r.Register("book", time.Minute, noop); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.RunDue(ctx); err != nil {
		t.Fatalf("RunDue: %v", err)
	}

	status, started, finished, errText, ok := lastRun(t, pool, "book")
	if !ok {
		t.Fatal("no job_runs row recorded")
	}
	if status != "succeeded" {
		t.Errorf("status = %q, want succeeded", status)
	}
	if started.IsZero() {
		t.Error("started_at not recorded")
	}
	if finished == nil {
		t.Error("finished_at not recorded")
	}
	if errText != nil {
		t.Errorf("error = %q, want nil for a successful run", *errText)
	}
}

func TestLockReleasedAfterCrashAllowsNextRun(t *testing.T) {
	pool := testkit.Postgres(t)
	ctx := context.Background()

	const key int64 = 424242

	// Acquire the advisory lock on connection A.
	connA, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	var locked bool
	if err := connA.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked); err != nil {
		t.Fatalf("lock on A: %v", err)
	}
	if !locked {
		t.Fatal("could not acquire lock on A")
	}

	// A different connection cannot take the lock while A holds it.
	connB, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire B: %v", err)
	}
	var gotB bool
	if err := connB.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&gotB); err != nil {
		t.Fatalf("lock attempt on B: %v", err)
	}
	if gotB {
		t.Fatal("B acquired a lock already held by A")
	}
	connB.Release()

	// Capture A's backend PID so the crash can be made deterministic.
	var pidA int
	if err := connA.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pidA); err != nil {
		t.Fatalf("read A backend pid: %v", err)
	}

	// Simulate a crash of A: close its connection without unlocking. Postgres
	// releases a session's advisory locks when its backend terminates, but that
	// happens asynchronously after the socket drops. Force-terminate the backend
	// and then poll until the lock becomes acquirable, rather than assuming the
	// release is instantaneous.
	hijacked := connA.Hijack()
	_ = hijacked.Close(ctx)

	connC, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire C: %v", err)
	}
	defer connC.Release()

	_, _ = connC.Exec(ctx, `SELECT pg_terminate_backend($1)`, pidA)

	var gotC bool
	for waited := time.Duration(0); waited < 5*time.Second; waited += 50 * time.Millisecond {
		if err := connC.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&gotC); err != nil {
			t.Fatalf("lock attempt on C: %v", err)
		}
		if gotC {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !gotC {
		t.Fatal("lock not released within 5s after simulated crash of the holder")
	}
	_, _ = connC.Exec(ctx, `SELECT pg_advisory_unlock($1)`, key)
}
