-- +goose Up
-- The catalog is the SaaS operator's product offering (global, not tenant-scoped
-- — tenants subscribe to it). A plan has a lifecycle and many versions; a
-- published version is immutable so existing subscribers who pinned it are never
-- affected by later changes.

CREATE TABLE catalog.plans (
    id         uuid PRIMARY KEY,
    key        text NOT NULL UNIQUE,     -- stable external identifier
    name       text NOT NULL,
    status     text NOT NULL,            -- draft | active | archived
    public     boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

-- A plan version freezes pricing + trial/grace + feature grants at publish time.
-- prices is a JSONB array of {cycle, amount_minor}; feature_grants is a JSONB
-- object of free-form feature_key -> value. Money is always integer minor units.
CREATE TABLE catalog.plan_versions (
    id             uuid PRIMARY KEY,
    plan_id        uuid NOT NULL REFERENCES catalog.plans (id) ON DELETE CASCADE,
    version        integer NOT NULL,
    status         text NOT NULL,        -- draft | published
    currency       text NOT NULL,
    prices         jsonb NOT NULL DEFAULT '[]'::jsonb,
    trial_enabled  boolean NOT NULL DEFAULT false,
    trial_days     integer NOT NULL DEFAULT 0,
    card_required  boolean NOT NULL DEFAULT false,
    grace_days     integer NOT NULL DEFAULT 0,
    feature_grants jsonb NOT NULL DEFAULT '{}'::jsonb,
    published_at   timestamptz,
    created_at     timestamptz NOT NULL,
    updated_at     timestamptz NOT NULL,
    UNIQUE (plan_id, version)
);

-- At most one draft version per plan (the "next" being edited).
CREATE UNIQUE INDEX plan_versions_single_draft_idx
    ON catalog.plan_versions (plan_id)
    WHERE status = 'draft';

CREATE INDEX plan_versions_plan_idx ON catalog.plan_versions (plan_id);

-- +goose Down
DROP TABLE catalog.plan_versions;
DROP TABLE catalog.plans;
