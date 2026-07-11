# Migrations

Database migrations live here, one subdirectory per module, applied by the
goose-based migration runner (`internal/platform/postgres`, wired in T-003).

```
migrations/
  <module>/NNN_description.sql
```

Each module owns its own Postgres schema (e.g. `tenant`, `billing`; the
authentication/authorization modules use `authn`/`authz` because
`AUTHORIZATION` is a reserved SQL word). The `platform` schema holds shared
infrastructure tables (outbox, processed_events, audit_log,
idempotency_keys). The first platform migration creates all schemas, and the
platform module always migrates first; each module's migrations are versioned
independently (goose table `goose_<module>_version`).

Conventions:
- Sequential zero-padded prefixes per module (`001_`, `002_`, …).
- Forward and rollback in the same file using goose `-- +goose Up` /
  `-- +goose Down` annotations.
- Money columns are `BIGINT` minor units plus a `TEXT` currency code.
- Timestamps are `TIMESTAMPTZ`, written in UTC.
