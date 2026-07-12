-- +goose Up
-- Email verification, password recovery, and session management build on the
-- T-012 identity tables.

-- Track whether (and when) a user's email was verified.
ALTER TABLE authn.users ADD COLUMN email_verified_at timestamptz;

-- Single-use, expiring tokens for email verification and password reset. Only
-- the SHA-256 hash of the token is stored; the raw value is emailed to the user
-- and never persisted. `purpose` scopes a token so a verification token can
-- never be redeemed as a reset token. `consumed_at` enforces single use.
CREATE TABLE authn.auth_tokens (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES authn.users (id) ON DELETE CASCADE,
    purpose    text NOT NULL, -- email_verification | password_reset
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL
);

CREATE INDEX auth_tokens_user_purpose_idx ON authn.auth_tokens (user_id, purpose);

-- +goose Down
DROP TABLE authn.auth_tokens;
ALTER TABLE authn.users DROP COLUMN email_verified_at;
