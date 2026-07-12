-- Least-privilege role setup for the entitlements database.
--
-- This runs once, on first initialization of an empty data volume, via the
-- Postgres image's /docker-entrypoint-initdb.d hook (as the superuser that the
-- image creates). It creates two login roles that split DDL from DML:
--
--   entitlements_owner  — owns and migrates the schema (CREATE/ALTER/DROP). The
--                         API applies migrations as this role
--                         (MIGRATION_DATABASE_URL). Objects it creates during
--                         migration become owned by it.
--
--   entitlements_app    — the runtime role the API serves requests as
--                         (DATABASE_URL). It gets DML only (SELECT/INSERT/
--                         UPDATE/DELETE) and can never change the schema. It is
--                         deliberately NOT a superuser and does NOT have
--                         BYPASSRLS, so once Row-Level Security is enabled
--                         (see docs/RUNNING.md → "Row-Level Security") its
--                         queries are constrained by the policies.
--
-- Passwords here are DEV defaults — override them (and prefer a secret manager)
-- for anything real. They must match the DSNs in docker-compose.roles.yml.

CREATE ROLE entitlements_owner LOGIN PASSWORD 'owner';
CREATE ROLE entitlements_app   LOGIN PASSWORD 'app';

-- The owner needs to create schemas/tables in this database.
GRANT CONNECT, CREATE ON DATABASE entitlements TO entitlements_owner;
GRANT USAGE, CREATE ON SCHEMA public          TO entitlements_owner;

-- The app connects and reads/writes but never creates.
GRANT CONNECT ON DATABASE entitlements TO entitlements_app;
GRANT USAGE   ON SCHEMA public         TO entitlements_app;

-- Everything the owner creates from now on (every module's schema, tables, and
-- sequences, created during migration) automatically grants the app role the
-- privileges below — so no per-table grant step is ever needed, including for
-- future migrations.
ALTER DEFAULT PRIVILEGES FOR ROLE entitlements_owner
    GRANT USAGE ON SCHEMAS TO entitlements_app;
ALTER DEFAULT PRIVILEGES FOR ROLE entitlements_owner
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO entitlements_app;
ALTER DEFAULT PRIVILEGES FOR ROLE entitlements_owner
    GRANT USAGE, SELECT ON SEQUENCES TO entitlements_app;
