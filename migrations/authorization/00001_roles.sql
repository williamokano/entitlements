-- +goose Up
-- Dynamic RBAC: roles are tenant-scoped data (created at runtime, no deploy).
-- The authorization module uses the short schema name authz (AUTHORIZATION is a
-- reserved SQL word).

-- A role is a named set of permission strings ("resource:action", with a
-- "resource:*" wildcard). System roles (owner/admin/member) are seeded per
-- tenant and cannot be edited or deleted.
CREATE TABLE authz.roles (
    id          uuid PRIMARY KEY,
    tenant_id   uuid NOT NULL,
    name        text NOT NULL,
    permissions text[] NOT NULL DEFAULT '{}',
    system      boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    UNIQUE (tenant_id, name)
);

-- Assignment of a role to a user within a tenant. The same user can hold
-- different roles in different tenants.
CREATE TABLE authz.role_assignments (
    id         uuid PRIMARY KEY,
    tenant_id  uuid NOT NULL,
    user_id    uuid NOT NULL,
    role_id    uuid NOT NULL REFERENCES authz.roles (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    UNIQUE (tenant_id, user_id, role_id)
);

CREATE INDEX role_assignments_user_idx ON authz.role_assignments (tenant_id, user_id);

-- +goose Down
DROP TABLE authz.role_assignments;
DROP TABLE authz.roles;
