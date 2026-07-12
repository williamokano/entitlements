-- +goose Up
-- Membership binds a (global) user to a tenant with a role reference. A user can
-- belong to many tenants, so uniqueness is per (tenant_id, user_id). The role is
-- a soft reference (role name) until the authorization module (T-016) makes
-- roles first-class rows.
CREATE TABLE tenant.memberships (
    id         uuid PRIMARY KEY,
    tenant_id  uuid NOT NULL REFERENCES tenant.tenants (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL,
    role       text NOT NULL,
    status     text NOT NULL, -- active | removed
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (tenant_id, user_id)
);

CREATE INDEX memberships_user_idx ON tenant.memberships (user_id) WHERE status = 'active';

-- Invitations are by email so a not-yet-registered person can be invited; they
-- are accepted later by the authenticated user whose email matches. At most one
-- pending invitation per (tenant, email).
CREATE TABLE tenant.invitations (
    id         uuid PRIMARY KEY,
    tenant_id  uuid NOT NULL REFERENCES tenant.tenants (id) ON DELETE CASCADE,
    email      text NOT NULL,
    role       text NOT NULL,
    status     text NOT NULL, -- pending | accepted | declined | revoked
    invited_by uuid,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX invitations_pending_email_idx
    ON tenant.invitations (tenant_id, email)
    WHERE status = 'pending';

CREATE INDEX invitations_tenant_idx ON tenant.invitations (tenant_id);

-- +goose Down
DROP TABLE tenant.invitations;
DROP TABLE tenant.memberships;
