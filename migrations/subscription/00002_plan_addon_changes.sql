-- +goose Up
-- Plan changes: an upgrade re-pins the plan version immediately; a downgrade is
-- stored as a scheduled change and applied at the period boundary. Addons attach
-- to a subscription with a quantity, pinned to an addon version.

ALTER TABLE subscription.subscriptions
    ADD COLUMN pending_plan_version_id uuid,
    ADD COLUMN pending_billing_cycle   text;

CREATE TABLE subscription.subscription_addons (
    id               uuid PRIMARY KEY,
    subscription_id  uuid NOT NULL REFERENCES subscription.subscriptions (id) ON DELETE CASCADE,
    addon_version_id uuid NOT NULL,
    quantity         int NOT NULL DEFAULT 1 CHECK (quantity >= 1),
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    UNIQUE (subscription_id, addon_version_id)
);

CREATE INDEX subscription_addons_sub_idx
    ON subscription.subscription_addons (subscription_id);

-- +goose Down
DROP TABLE subscription.subscription_addons;
ALTER TABLE subscription.subscriptions
    DROP COLUMN pending_plan_version_id,
    DROP COLUMN pending_billing_cycle;
