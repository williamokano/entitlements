# Migrations

Database migrations live here, one subdirectory per module, applied by the
goose-based migration runner (`internal/platform/postgres`, wired in T-003).

```
migrations/
  <module>/NNN_description.sql
```

Each module owns its own Postgres schema (e.g. `tenant`, `billing`). The
`platform` schema holds shared infrastructure tables (outbox,
processed_events, audit_log, idempotency_keys). Migration `000` creates the
schemas.

Conventions:
- Sequential zero-padded prefixes per module (`001_`, `002_`, …).
- Forward and rollback in the same file using goose `-- +goose Up` /
  `-- +goose Down` annotations.
- Money columns are `BIGINT` minor units plus a `TEXT` currency code.
- Timestamps are `TIMESTAMPTZ`, written in UTC.
