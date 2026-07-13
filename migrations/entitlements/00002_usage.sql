-- +goose Up
-- Usage counters answer "how much of a metered feature has this tenant consumed
-- in the current period". They are the enforcement substrate for ConsumeQuota:
-- a single guarded upsert increments `used` and is rejected atomically when it
-- would breach a hard limit, so N concurrent callers can never exceed it.
--
-- A counter is keyed by (tenant_id, feature_key, period_key). period_key is
-- derived from the feature's reset_period and the relevant boundary — 'never'
-- for a non-resetting feature, 'YYYY-MM' for monthly, or the subscription's
-- current-period start for billing_cycle. A new period is simply a new key: the
-- absent row reads as zero, so period reset is lazy and needs no background job.
--
-- `warned` is a once-per-period latch: the consume that first crosses a soft
-- limit claims it (UPDATE ... WHERE warned = false) so exactly one concurrent
-- caller emits the EntitlementLimitWarning event.
CREATE TABLE entitlements.usage_counters (
    tenant_id   uuid NOT NULL,
    feature_key text NOT NULL,
    period_key  text NOT NULL,
    used        bigint NOT NULL DEFAULT 0,
    warned      boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, feature_key, period_key)
);

CREATE INDEX usage_counters_tenant_idx
    ON entitlements.usage_counters (tenant_id);

-- +goose Down
DROP TABLE entitlements.usage_counters;
