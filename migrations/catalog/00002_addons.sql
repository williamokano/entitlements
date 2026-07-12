-- +goose Up
-- Addons extend a plan with extra pricing and entitlement deltas. Like plans,
-- they are versioned and a published version is immutable.

CREATE TABLE catalog.addons (
    id         uuid PRIMARY KEY,
    key        text NOT NULL UNIQUE,
    name       text NOT NULL,
    status     text NOT NULL,           -- draft | active | archived
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

-- An addon version freezes its pricing, compatibility, and entitlement deltas.
-- compatible_plan_keys lists the plan keys this addon may attach to; deltas is a
-- JSONB array of {feature_key, kind, amount, value} where kind is limit_delta
-- (amount) or value_override (value).
CREATE TABLE catalog.addon_versions (
    id                   uuid PRIMARY KEY,
    addon_id             uuid NOT NULL REFERENCES catalog.addons (id) ON DELETE CASCADE,
    version              integer NOT NULL,
    status               text NOT NULL,  -- draft | published
    currency             text NOT NULL,
    prices               jsonb NOT NULL DEFAULT '[]'::jsonb,
    quantity_allowed     boolean NOT NULL DEFAULT false,
    compatible_plan_keys text[] NOT NULL DEFAULT '{}',
    deltas               jsonb NOT NULL DEFAULT '[]'::jsonb,
    published_at         timestamptz,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL,
    UNIQUE (addon_id, version)
);

CREATE UNIQUE INDEX addon_versions_single_draft_idx
    ON catalog.addon_versions (addon_id)
    WHERE status = 'draft';

CREATE INDEX addon_versions_addon_idx ON catalog.addon_versions (addon_id);

-- +goose Down
DROP TABLE catalog.addon_versions;
DROP TABLE catalog.addons;
