-- +goose Up
-- Identity lives in the authn schema (AUTHORIZATION is a reserved word, so the
-- authentication module uses the short name authn). Users are global — a user
-- can belong to many tenants via membership (tenant module) — so nothing here
-- is tenant-scoped.

-- A global user. Email is unique among non-deleted users, stored lowercased by
-- the domain so uniqueness is case-insensitive without a citext extension.
CREATE TABLE authn.users (
    id         uuid PRIMARY KEY,
    email      text NOT NULL,
    status     text NOT NULL, -- active | disabled
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX users_active_email_idx
    ON authn.users (email)
    WHERE status <> 'deleted';

-- Credentials are modeled as factors so password is just the first kind; totp
-- and webauthn slot in later without a schema change. `secret` holds the
-- factor's verifier (an argon2id hash for password) — never a plaintext secret.
CREATE TABLE authn.credentials (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES authn.users (id) ON DELETE CASCADE,
    type       text NOT NULL, -- password | totp | webauthn
    secret     text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (user_id, type)
);

-- Refresh tokens form per-login families. Rotation marks the presented token
-- `rotated` and issues a child; presenting an already-used token (reuse) lets
-- the service revoke the whole family. Only the SHA-256 hash is stored — the
-- raw token value never touches the database.
CREATE TABLE authn.refresh_tokens (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES authn.users (id) ON DELETE CASCADE,
    family_id  uuid NOT NULL,
    parent_id  uuid REFERENCES authn.refresh_tokens (id),
    token_hash text NOT NULL UNIQUE,
    status     text NOT NULL, -- active | rotated | revoked
    issued_at  timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE INDEX refresh_tokens_family_idx ON authn.refresh_tokens (family_id);
CREATE INDEX refresh_tokens_user_idx ON authn.refresh_tokens (user_id);

-- +goose Down
DROP TABLE authn.refresh_tokens;
DROP TABLE authn.credentials;
DROP TABLE authn.users;
