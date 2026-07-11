-- +goose Up
-- The transactional outbox: events are appended in the same transaction as the
-- business state change, then delivered at-least-once by the relay worker.
CREATE TABLE platform.outbox (
    id              uuid PRIMARY KEY,
    occurred_at     timestamptz NOT NULL,
    tenant_id       uuid NOT NULL, -- uuid.Nil (all-zeros) means platform-wide / no tenant
    module          text NOT NULL,
    event_type      text NOT NULL,
    payload         jsonb NOT NULL,
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL,
    published_at    timestamptz,
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- Supports the relay's poll for the next eligible unpublished event.
CREATE INDEX outbox_pending_idx
    ON platform.outbox (next_attempt_at, occurred_at, id)
    WHERE published_at IS NULL;

-- Makes each consumer's handling of an event exactly-once-effective: the
-- Idempotent wrapper inserts (consumer, event_id) in the delivery transaction.
CREATE TABLE platform.processed_events (
    consumer     text NOT NULL,
    event_id     uuid NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);

-- +goose Down
DROP TABLE platform.processed_events;
DROP TABLE platform.outbox;
