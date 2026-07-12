-- +goose Up
-- Renewal and trial job bookkeeping. renewal_emitted_period_end records the
-- period boundary for which a SubscriptionRenewalDue was already published, so
-- emission is exactly-once per period even under duplicate job ticks. The trial
-- flags make TrialEnding fire once and TrialEnded resolve once.

ALTER TABLE subscription.subscriptions
    ADD COLUMN renewal_emitted_period_end timestamptz,
    ADD COLUMN trial_ending_emitted       boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE subscription.subscriptions
    DROP COLUMN renewal_emitted_period_end,
    DROP COLUMN trial_ending_emitted;
