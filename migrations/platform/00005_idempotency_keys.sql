-- +goose Up
-- Stores the outcome of an idempotent mutating request so retries with the same
-- Idempotency-Key replay the original response instead of re-executing.
CREATE TABLE platform.idempotency_keys (
    tenant_id    uuid NOT NULL, -- uuid.Nil for tenant-less endpoints
    idem_key     text NOT NULL,
    route        text NOT NULL, -- "METHOD /path"
    completed    boolean NOT NULL DEFAULT false,
    status_code  integer,
    content_type text,
    body         bytea,
    created_at   timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, idem_key, route)
);

-- Supports pruning expired keys.
CREATE INDEX idempotency_keys_expires_idx ON platform.idempotency_keys (expires_at);

-- +goose Down
DROP TABLE platform.idempotency_keys;
