-- +goose Up
-- API keys are a second credential kind for machine (server-to-server) auth.
-- Each key is bound to a tenant and carries scopes. Only the argon2id hash of
-- the secret is stored; the raw secret is shown once at creation. The public
-- `prefix` identifies the key for O(1) lookup before the (single) hash verify.
CREATE TABLE authn.api_keys (
    id           uuid PRIMARY KEY,
    tenant_id    uuid NOT NULL,
    name         text NOT NULL,
    prefix       text NOT NULL UNIQUE,
    secret_hash  text NOT NULL,
    scopes       text[] NOT NULL DEFAULT '{}',
    last_used_at timestamptz,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL
);

CREATE INDEX api_keys_tenant_idx ON authn.api_keys (tenant_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE authn.api_keys;
