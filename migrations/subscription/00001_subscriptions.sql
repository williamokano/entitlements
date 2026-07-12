-- +goose Up
-- A subscription is a tenant's commitment to a pinned plan version on a billing
-- cycle, with an explicit lifecycle state machine. A tenant may hold at most one
-- non-terminal subscription at a time.

CREATE TABLE subscription.subscriptions (
    id                   uuid PRIMARY KEY,
    tenant_id            uuid NOT NULL,
    plan_version_id      uuid NOT NULL,
    billing_cycle        text NOT NULL,   -- monthly | annual | custom
    status               text NOT NULL,   -- trialing | active | past_due | grace | suspended | paused | canceled | expired
    current_period_start timestamptz NOT NULL,
    current_period_end   timestamptz NOT NULL,
    trial_ends_at        timestamptz,
    cancel_at_period_end boolean NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL
);

-- At most one non-terminal (live) subscription per tenant.
CREATE UNIQUE INDEX subscriptions_one_live_per_tenant_idx
    ON subscription.subscriptions (tenant_id)
    WHERE status NOT IN ('canceled', 'expired');

CREATE INDEX subscriptions_tenant_idx ON subscription.subscriptions (tenant_id);

-- Append-only audit of every state transition.
CREATE TABLE subscription.subscription_transitions (
    id              uuid PRIMARY KEY,
    subscription_id uuid NOT NULL REFERENCES subscription.subscriptions (id) ON DELETE CASCADE,
    from_state      text NOT NULL,   -- "" for creation
    to_state        text NOT NULL,
    event           text NOT NULL,
    reason          text NOT NULL DEFAULT '',
    actor           text NOT NULL DEFAULT '',
    at              timestamptz NOT NULL
);

CREATE INDEX subscription_transitions_sub_idx
    ON subscription.subscription_transitions (subscription_id, at);

-- +goose Down
DROP TABLE subscription.subscription_transitions;
DROP TABLE subscription.subscriptions;
