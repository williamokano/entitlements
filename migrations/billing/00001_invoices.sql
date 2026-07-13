-- +goose Up
-- Billing invoices with line-item snapshots. An issued invoice is a historical
-- fact: every line copies the plan/addon key, version, unit price, quantity and
-- currency at issuance time, so later catalog changes never rewrite it. All
-- money is integer minor units + a currency code; never floats.

-- Per-tenant gapless number sequences. One row per (tenant, kind) — kind is
-- 'invoice' or 'credit_note' — holds the last number assigned. Numbers are
-- allocated with a single atomic upsert inside the issuing transaction
-- (INSERT ... ON CONFLICT DO UPDATE ... RETURNING), so the row lock serializes
-- concurrent issuers per tenant: the sequence is gapless and dup-free even under
-- concurrency, and a rolled-back issue rolls back its increment (no gap).
CREATE TABLE billing.number_sequences (
    tenant_id   uuid   NOT NULL,
    kind        text   NOT NULL,   -- invoice | credit_note
    next_number bigint NOT NULL,
    PRIMARY KEY (tenant_id, kind)
);

CREATE TABLE billing.invoices (
    id              uuid PRIMARY KEY,
    tenant_id       uuid   NOT NULL,
    subscription_id uuid,                       -- the subscription that was billed (nullable)
    number          bigint NOT NULL,            -- per-tenant gapless sequence
    status          text   NOT NULL,            -- draft | open | paid | void | uncollectible
    currency        text   NOT NULL,
    subtotal_minor  bigint NOT NULL,            -- sum of line amounts, minor units
    tax_minor       bigint NOT NULL,            -- from the TaxCalculator, minor units
    total_minor     bigint NOT NULL,            -- subtotal + tax, minor units
    issued_at       timestamptz,                -- set when the invoice opens
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (tenant_id, number)
);

CREATE INDEX invoices_tenant_idx ON billing.invoices (tenant_id, number);

-- Snapshotted line items: all values are copies frozen at issuance.
CREATE TABLE billing.invoice_line_items (
    id                uuid PRIMARY KEY,
    invoice_id        uuid   NOT NULL REFERENCES billing.invoices (id) ON DELETE CASCADE,
    kind              text   NOT NULL,          -- plan | addon
    description       text   NOT NULL,
    plan_or_addon_key text   NOT NULL,          -- snapshotted catalog key
    version           int    NOT NULL,          -- snapshotted catalog version
    unit_price_minor  bigint NOT NULL,          -- snapshotted price, minor units
    quantity          int    NOT NULL,
    amount_minor      bigint NOT NULL,          -- unit_price_minor * quantity
    currency          text   NOT NULL,
    position          int    NOT NULL           -- stable ordering within the invoice
);

CREATE INDEX invoice_line_items_invoice_idx
    ON billing.invoice_line_items (invoice_id, position);

-- Credit notes negate (part of) an invoice: amount_minor is stored negative.
CREATE TABLE billing.credit_notes (
    id           uuid PRIMARY KEY,
    invoice_id   uuid   NOT NULL REFERENCES billing.invoices (id) ON DELETE CASCADE,
    tenant_id    uuid   NOT NULL,
    number       bigint NOT NULL,               -- per-tenant gapless sequence
    amount_minor bigint NOT NULL,               -- negative: credits negate invoice amounts
    currency     text   NOT NULL,
    reason       text   NOT NULL,
    created_at   timestamptz NOT NULL,
    UNIQUE (tenant_id, number)
);

CREATE INDEX credit_notes_invoice_idx ON billing.credit_notes (invoice_id);

-- +goose Down
DROP TABLE billing.credit_notes;
DROP TABLE billing.invoice_line_items;
DROP TABLE billing.invoices;
DROP TABLE billing.number_sequences;
