-- +goose Up
-- Entitlements answer "what can this tenant do right now, and how much of it".
--
-- The feature registry is data, not code: a new feature is inserted (never
-- deployed) and immediately participates in resolution with its default. Features
-- are archived, never deleted, so historical entitlements and audit rows stay
-- coherent.

CREATE TABLE entitlements.features (
    id             uuid PRIMARY KEY,
    key            text NOT NULL UNIQUE,   -- stable external id (≈ Stripe lookup_key)
    type           text NOT NULL,          -- boolean | limit | config | enum
    default_value  jsonb NOT NULL DEFAULT 'null'::jsonb,
    description    text NOT NULL DEFAULT '',
    limit_behavior text NOT NULL DEFAULT '', -- soft | hard (limit types); '' otherwise
    reset_period   text NOT NULL DEFAULT 'never', -- billing_cycle | monthly | never
    metadata       jsonb NOT NULL DEFAULT '{}'::jsonb, -- includes allowed_values for enum
    active         boolean NOT NULL DEFAULT true,
    created_at     timestamptz NOT NULL,
    updated_at     timestamptz NOT NULL
);

CREATE INDEX features_active_idx ON entitlements.features (active);

-- The materialized effective set per (tenant, feature): the resolved value and
-- the layer that produced it. Rebuilt by idempotent consumers of subscription /
-- catalog / override events; the diff against this table decides whether an
-- EntitlementsSummaryChanged event is published.
CREATE TABLE entitlements.effective_entitlements (
    tenant_id   uuid NOT NULL,
    feature_key text NOT NULL,
    value       jsonb NOT NULL,
    source      text NOT NULL,   -- plan | addon | override | default
    updated_at  timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, feature_key)
);

CREATE INDEX effective_entitlements_tenant_idx
    ON entitlements.effective_entitlements (tenant_id);

-- Per-tenant manual overrides (support gestures, negotiated contracts). Resolution
-- reads this table; the override CRUD + audit + expiry job land in T-023. An
-- override with a null expires_at never expires; a past expires_at is ignored by
-- resolution.
CREATE TABLE entitlements.tenant_overrides (
    id          uuid PRIMARY KEY,
    tenant_id   uuid NOT NULL,
    feature_key text NOT NULL,
    value       jsonb NOT NULL,
    reason      text NOT NULL DEFAULT '',
    actor       text NOT NULL DEFAULT '',
    expires_at  timestamptz,
    created_at  timestamptz NOT NULL
);

-- Resolution reads the latest live override per (tenant, feature).
CREATE INDEX tenant_overrides_tenant_feature_idx
    ON entitlements.tenant_overrides (tenant_id, feature_key);

-- +goose Down
DROP TABLE entitlements.tenant_overrides;
DROP TABLE entitlements.effective_entitlements;
DROP TABLE entitlements.features;
