-- +goose Up
-- Tokenized payment methods. Storage is tokens only: a row holds an opaque
-- provider customer reference and payment-method token plus non-sensitive
-- display metadata (brand, last four). A raw card number (PAN) must never be
-- stored — tokenization happens at the gateway and only the token crosses into
-- this system. The token_not_pan CHECK is the schema-level backstop: it rejects
-- any token that is a bare 12–19 digit run (the shape of a real PAN), even if a
-- caller bypasses the domain guard.
CREATE TABLE billing.payment_methods (
    id           uuid PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    customer_ref text        NOT NULL,            -- provider customer reference (opaque)
    token        text        NOT NULL,            -- provider payment-method token (never a PAN)
    brand        text,                            -- display only (e.g. "visa")
    last4        text,                            -- display only, last four digits
    created_at   timestamptz NOT NULL,
    CONSTRAINT token_not_pan CHECK (
        replace(replace(token, ' ', ''), '-', '') !~ '^[0-9]{12,19}$'
    ),
    CONSTRAINT customer_ref_not_pan CHECK (
        replace(replace(customer_ref, ' ', ''), '-', '') !~ '^[0-9]{12,19}$'
    ),
    CONSTRAINT last4_digits CHECK (last4 IS NULL OR last4 ~ '^[0-9]{1,4}$')
);

CREATE INDEX payment_methods_tenant_idx ON billing.payment_methods (tenant_id, created_at DESC);

-- +goose Down
DROP TABLE billing.payment_methods;
