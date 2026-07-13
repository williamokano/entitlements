-- +goose Up
-- Dunning schedules: when a renewal charge is declined, one row tracks the retry
-- schedule for the open invoice. attempt is the highest charge-attempt number
-- already made (1 = the initial renewal charge; retries use 2, 3, …), failed_at
-- anchors the offsets, and next_retry_at is when the dunning job should retry.
-- status moves active → recovered (a retry succeeded) or active → exhausted (all
-- retries failed). One schedule per invoice.
CREATE TABLE billing.dunning (
    id              uuid PRIMARY KEY,
    tenant_id       uuid   NOT NULL,
    invoice_id      uuid   NOT NULL REFERENCES billing.invoices (id) ON DELETE CASCADE,
    subscription_id uuid,
    status          text   NOT NULL,       -- active | recovered | exhausted
    attempt         int    NOT NULL,       -- highest charge-attempt number made
    failed_at       timestamptz NOT NULL,  -- schedule anchor (initial failure)
    next_retry_at   timestamptz,           -- when the next retry is due (null when terminal)
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (invoice_id)
);

-- The dunning job scans for active schedules that are due.
CREATE INDEX dunning_due_idx ON billing.dunning (next_retry_at)
    WHERE status = 'active';

-- Pending prorations: a deferred plan-change adjustment (the credit_next_invoice
-- strategy) that the next issued invoice for the subscription drains into a
-- proration line. applied_invoice_id is set when it is consumed, so a proration
-- is billed exactly once. All money is integer minor units + a currency code.
CREATE TABLE billing.pending_prorations (
    id                 uuid PRIMARY KEY,
    tenant_id          uuid   NOT NULL,
    subscription_id    uuid   NOT NULL,
    description        text   NOT NULL,
    plan_or_addon_key  text   NOT NULL,   -- snapshotted plan key for the line
    version            int    NOT NULL,   -- snapshotted plan version for the line
    amount_minor       bigint NOT NULL,   -- signed: positive charge, negative credit
    currency           text   NOT NULL,
    applied_invoice_id uuid,              -- set when drained into an invoice
    created_at         timestamptz NOT NULL
);

CREATE INDEX pending_prorations_pending_idx
    ON billing.pending_prorations (subscription_id)
    WHERE applied_invoice_id IS NULL;

-- +goose Down
DROP TABLE billing.pending_prorations;
DROP TABLE billing.dunning;
