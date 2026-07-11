-- +goose Up
-- Tenants are the root of isolation. The `tenant` schema was created by the
-- platform schema migration; this adds the table.
CREATE TABLE tenant.tenants (
    id         uuid PRIMARY KEY,
    slug       text NOT NULL,
    name       text NOT NULL,
    status     text NOT NULL, -- active | suspended | deleted
    settings   jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

-- Slugs are unique among non-deleted tenants, so a slug is reusable once its
-- tenant is soft-deleted.
CREATE UNIQUE INDEX tenants_active_slug_idx
    ON tenant.tenants (slug)
    WHERE status <> 'deleted';

-- +goose Down
DROP TABLE tenant.tenants;
