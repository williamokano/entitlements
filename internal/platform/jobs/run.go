package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// Run polls for due jobs until ctx is canceled. Errors from a batch are logged
// and retried on the next tick rather than stopping the runner.
func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		if err := r.RunDue(ctx); err != nil && !errors.Is(err, context.Canceled) {
			r.logger.ErrorContext(ctx, "jobs runner tick failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// RunDue runs every job that is currently due, once. Jobs run concurrently so a
// slow or hanging job does not delay the others. It is exported so tests can
// drive the runner deterministically. RunDue returns nil even if individual
// jobs fail (failures are recorded in platform.job_runs); it returns an error
// only for an infrastructure failure that prevents scheduling.
func (r *Runner) RunDue(ctx context.Context) error {
	jobs := r.snapshot()

	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j *job) {
			defer wg.Done()
			if err := r.runJobIfDue(ctx, j); err != nil {
				r.logger.ErrorContext(ctx, "jobs: scheduling failure", "job", j.name, "error", err)
			}
		}(j)
	}
	wg.Wait()
	return nil
}

// runJobIfDue elects a leader via advisory lock and executes the job if it is
// due. Returns an error only for infrastructure failures (not job failures).
func (r *Runner) runJobIfDue(ctx context.Context, j *job) error {
	due, err := r.isDue(ctx, j, r.pool)
	if err != nil {
		return err
	}
	if !due {
		return nil
	}

	// Acquire the job's advisory lock on a dedicated connection so lock and
	// unlock happen on the same session; a lost connection auto-releases it.
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, j.lockKey).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil // another runner is executing this job
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, j.lockKey)
	}()

	// Re-check due-ness under the lock: another runner may have just finished.
	due, err = r.isDue(ctx, j, r.pool)
	if err != nil {
		return err
	}
	if !due {
		return nil
	}

	r.execute(ctx, j)
	return nil
}

// isDue reports whether the job's interval has elapsed since its last run.
func (r *Runner) isDue(ctx context.Context, j *job, q postgres.Querier) (bool, error) {
	var last *time.Time
	err := q.QueryRow(ctx,
		`SELECT max(started_at) FROM platform.job_runs WHERE job_name = $1`, j.name).
		Scan(&last)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if last == nil {
		return true, nil // never run
	}
	return !r.clk.Now().Before(last.Add(j.interval)), nil
}

// execute records a run, invokes the job with a per-run timeout and panic
// recovery, and records the outcome.
func (r *Runner) execute(ctx context.Context, j *job) {
	runID := r.gen.New()
	started := r.clk.Now().UTC()

	if _, err := r.pool.Exec(ctx,
		`INSERT INTO platform.job_runs (id, job_name, started_at, status) VALUES ($1, $2, $3, 'running')`,
		runID, j.name, started); err != nil {
		r.logger.ErrorContext(ctx, "jobs: record run start failed", "job", j.name, "error", err)
		return
	}

	runErr := r.invoke(ctx, j)

	status := "succeeded"
	var errText *string
	if runErr != nil {
		status = "failed"
		s := runErr.Error()
		errText = &s
	}

	finished := r.clk.Now().UTC()
	if _, err := r.pool.Exec(context.WithoutCancel(ctx),
		`UPDATE platform.job_runs SET finished_at = $2, status = $3, error = $4 WHERE id = $1`,
		runID, finished, status, errText); err != nil {
		r.logger.ErrorContext(ctx, "jobs: record run finish failed", "job", j.name, "error", err)
	}
}

// invoke runs the job's function under a real-time timeout, converting a panic
// into an error so it never crashes the runner.
func (r *Runner) invoke(ctx context.Context, j *job) (err error) {
	runCtx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()

	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("jobs: panic in %q: %v", j.name, rec)
		}
	}()
	return j.fn(runCtx)
}
