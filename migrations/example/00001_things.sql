-- +goose Up
-- The example module owns its own schema, demonstrating the per-module schema
-- convention. This module is reference/documentation only — delete it (and this
-- migration) when starting a real SaaS (see docs "Starting a new SaaS").
CREATE SCHEMA IF NOT EXISTS example;

CREATE TABLE example.things (
    id            uuid PRIMARY KEY,
    tenant_id     uuid NOT NULL,
    name          text NOT NULL,
    process_count integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP SCHEMA example CASCADE;
