-- +goose Up
-- Dunning grace window. When billing exhausts its charge retries it publishes
-- billing.dunning_exhausted; the subscription moves past_due → grace and records
-- grace_ends_at = now + the pinned plan version's grace days. The grace scan then
-- suspends subscriptions whose grace_ends_at has passed. nil except while in grace.

ALTER TABLE subscription.subscriptions
    ADD COLUMN grace_ends_at timestamptz;

-- The grace scan finds grace subscriptions whose deadline has passed.
CREATE INDEX subscriptions_grace_idx ON subscription.subscriptions (grace_ends_at)
    WHERE status = 'grace';

-- +goose Down
DROP INDEX subscription.subscriptions_grace_idx;
ALTER TABLE subscription.subscriptions
    DROP COLUMN grace_ends_at;
