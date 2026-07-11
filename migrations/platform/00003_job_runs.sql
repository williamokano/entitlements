-- +goose Up
-- Bookkeeping for the scheduled-job runner: one row per execution attempt.
CREATE TABLE platform.job_runs (
    id          uuid PRIMARY KEY,
    job_name    text NOT NULL,
    started_at  timestamptz NOT NULL,
    finished_at timestamptz,
    status      text NOT NULL, -- 'running' | 'succeeded' | 'failed'
    error       text
);

-- Supports the "when did this job last run?" due-check.
CREATE INDEX job_runs_name_started_idx
    ON platform.job_runs (job_name, started_at DESC);

-- +goose Down
DROP TABLE platform.job_runs;
