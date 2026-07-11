// Package jobs is a minimal recurring-job scheduler for the monolith.
//
// A Runner executes registered jobs on their interval. Across multiple
// replicas, a Postgres advisory lock elects a single executor per job per tick,
// so a job runs exactly once per interval regardless of how many runners are
// live. Each execution is bounded by a per-run timeout, isolated from panics,
// and recorded in platform.job_runs.
//
// Scheduling decisions (is a job due?) read the injected clock.Clock, so tests
// drive them deterministically with clock.Frozen; the per-run timeout uses the
// real wall clock, because jobs need genuine cancellation.
package jobs

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"

	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/id"
)

// Func is the work a job performs. It must observe ctx cancellation so the
// per-run timeout can stop it.
type Func func(ctx context.Context) error

// defaultTimeout bounds a single run when a job does not specify its own.
const defaultTimeout = 5 * time.Minute

type job struct {
	name     string
	interval time.Duration
	timeout  time.Duration
	lockKey  int64
	fn       Func
}

// Option customizes a registered job.
type Option func(*job)

// WithTimeout overrides the per-run timeout for a job.
func WithTimeout(d time.Duration) Option {
	return func(j *job) {
		if d > 0 {
			j.timeout = d
		}
	}
}

// Runner schedules and executes registered jobs.
type Runner struct {
	pool         *pgxpool.Pool
	gen          id.Generator
	clk          clock.Clock
	logger       *slog.Logger
	pollInterval time.Duration

	mu   sync.Mutex
	jobs map[string]*job
}

// NewRunner builds a Runner. pollInterval controls how often Run wakes to check
// for due jobs (individual jobs still fire only on their own interval).
func NewRunner(pool *pgxpool.Pool, gen id.Generator, clk clock.Clock, logger *slog.Logger, pollInterval time.Duration) *Runner {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &Runner{
		pool:         pool,
		gen:          gen,
		clk:          clk,
		logger:       logger,
		pollInterval: pollInterval,
		jobs:         make(map[string]*job),
	}
}

// Register adds a job that runs fn every interval. It returns an error if a job
// with the same name is already registered.
func (r *Runner) Register(name string, interval time.Duration, fn Func, opts ...Option) error {
	if name == "" {
		return fmt.Errorf("jobs: job name must not be empty")
	}
	if interval <= 0 {
		return fmt.Errorf("jobs: interval for %q must be positive", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.jobs[name]; exists {
		return fmt.Errorf("jobs: job %q already registered", name)
	}

	j := &job{
		name:     name,
		interval: interval,
		timeout:  defaultTimeout,
		lockKey:  lockKey(name),
		fn:       fn,
	}
	for _, opt := range opts {
		opt(j)
	}
	r.jobs[name] = j
	return nil
}

// snapshot returns the registered jobs without holding the lock during
// execution.
func (r *Runner) snapshot() []*job {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*job, 0, len(r.jobs))
	for _, j := range r.jobs {
		out = append(out, j)
	}
	return out
}

// lockKey derives a stable 64-bit Postgres advisory-lock key from a job name.
func lockKey(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return int64(h.Sum64())
}
