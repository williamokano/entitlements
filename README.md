# entitlements

A reusable **Go backend skeleton for SaaS products**: a modular monolith built
with DDD + hexagonal architecture. It ships seven modules — tenant,
authentication, authorization, catalog, subscription, entitlements, and
billing — so a new SaaS can start from here instead of from scratch.

See [`docs/PLAN.md`](docs/PLAN.md) for the architecture and
[`docs/TASKS.md`](docs/TASKS.md) for the implementation task breakdown.

## Status

**Milestones 1 (platform kernel) and 2 (identity) complete; Milestone 3 (product
core) in progress.** The API boots, runs its migrations on startup, and serves
real endpoints.

Implemented (tasks **T-001 – T-019**):

- **Platform kernel** — config, UUIDv7 IDs, clock, Postgres pool + UnitOfWork
  (tx-in-context), migration runner, transactional outbox + relay worker,
  idempotent event bus, advisory-lock job runner, append-only audit log,
  structured logging, OpenTelemetry tracing, RFC 7807 problem+json errors, and
  an idempotency-key middleware.
- **Tenant** module (`/api/v1/tenants`) — lifecycle CRUD (create / update /
  suspend / reactivate / soft-delete), tenant-resolution middleware
  (claim → `X-Tenant-ID` header → subdomain), an event-driven provisioning
  pipeline, and **membership + invitations** (invite by email — including
  not-yet-registered users — accept / decline / resend with expiry, member
  listing and removal; a user can belong to many tenants).
- **Authentication** module (`/api/v1/auth`) — register, login, JWT access
  tokens (EdDSA, offline-verifiable) + rotating refresh tokens with
  family-reuse detection, logout, email verification, password recovery,
  password change, and session listing / revoke-others.
- **Auth middleware + API keys** — a global middleware authenticates each
  request: `Authorization: Bearer <jwt>` → a user principal, or
  `Authorization: ApiKey <key>` → a machine principal (with scopes) whose tenant
  is derived from the key. Tenant-scoped, per-tenant API keys with argon2id-hashed
  secrets (shown once), scopes, `last_used_at`, and immediate revocation are
  managed under `/api/v1/api-keys`.
- **Authorization** module (`/api/v1/roles`) — dynamic RBAC: tenant-scoped roles
  as data (`resource:action` permissions with a `resource:*` wildcard),
  owner/admin/member seeded per tenant (system roles are immutable), role CRUD,
  assign/unassign to users, and a `RequirePermission` middleware that enforces
  permissions at the HTTP layer. The same user can hold different roles in
  different tenants.
- **Catalog** module (`/api/v1/catalog`) — the SaaS operator's product offering
  (global, not tenant-scoped): plans with immutable published **versions**
  (publishing freezes a version so existing subscribers are never affected by
  later changes), per-cycle pricing in integer minor units, trial/grace config,
  and free-form feature grants. Admin CRUD + an open public plan listing. Also
  **addons** — versioned like plans (immutable once published), with their own
  pricing, a compatible-plan list (a shared compatibility helper other modules
  reuse), a quantity-allowed flag, and entitlement **deltas**
  (`feature_key → limit delta | value override`).
- **Subscription** module (`/api/v1/subscription`) — a tenant's commitment to a
  pinned plan version, driven by an explicit state machine
  (`trialing → active → past_due → grace → suspended | paused → canceled |
  expired`); one live subscription per tenant, guarded transitions with a full
  history trail and an event per transition, trial/period tracking, and
  create / cancel (immediate or at period end) / reactivate / pause / resume.
- **Example** module (`/api/v1/example/things`) — a reference tenant-scoped
  slice demonstrating the full hexagonal shape and the outbox → consumer flow.

Not yet implemented: subscription **renewals & plan changes** (T-020/021),
entitlements, and billing (Milestone 3). See [`docs/TASKS.md`](docs/TASKS.md) for
the full plan.
(Note: the tenant creator is not yet auto-assigned the `owner` role, so an
initial role assignment currently has to be bootstrapped out of band — see the
T-016 follow-up in the tasks doc.)

## Try it locally

```bash
docker compose up -d          # Postgres on :5432 (credentials match the app default)
make run                      # applies migrations on startup, serves on :8080
```

```bash
# Health
curl -s localhost:8080/healthz            # {"status":"ok"}

# Register + login (auth is global — no tenant header needed)
curl -sX POST localhost:8080/api/v1/auth/register \
  -H 'content-type: application/json' \
  -d '{"email":"me@example.com","password":"a-strong-password"}'

curl -sX POST localhost:8080/api/v1/auth/login \
  -H 'content-type: application/json' \
  -d '{"email":"me@example.com","password":"a-strong-password"}'
# → {"access_token":"…","refresh_token":"…","token_type":"Bearer", …}

# Create a tenant, then call a tenant-scoped route with its id
curl -sX POST localhost:8080/api/v1/tenants \
  -H 'content-type: application/json' -d '{"slug":"acme","name":"Acme"}'
# → {"id":"<tenant-uuid>", …}

curl -sX POST localhost:8080/api/v1/example/things \
  -H 'content-type: application/json' -H 'X-Tenant-ID: <tenant-uuid>' \
  -d '{"name":"widget"}'

# Mint a tenant API key (needs a user access token + the tenant), then use it —
# a machine call derives its tenant from the key, so no X-Tenant-ID is needed
curl -sX POST localhost:8080/api/v1/api-keys \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>' \
  -d '{"name":"ci","scopes":["things:write"]}'
# → {"id":"…","prefix":"ak_…","api_key":"ak_….<secret>", …}   (secret shown once)

curl -sX POST localhost:8080/api/v1/example/things \
  -H 'content-type: application/json' \
  -H 'Authorization: ApiKey ak_….<secret>' \
  -d '{"name":"via-key"}'
```

Verification and password-reset emails are **logged, not sent** by the dev
`EmailSender` — grab the link from `make run`'s output (a `dev email (not
delivered)` line carrying `…/verify-email?token=…`). The JWT signing key is
ephemeral unless you set `AUTH_JWT_ED25519_SEED` (a base64-encoded 32-byte
seed), so tokens do not survive a restart otherwise.

## Development

### Prerequisites

- Go 1.24+
- Docker + Docker Compose (for Postgres)
- [`golangci-lint`](https://golangci-lint.run/) v2 (optional locally; `make
  lint` falls back to `go vet` if it is not installed)

### Build and test

```bash
docker compose up -d          # Postgres (integration tests + local run)
make build
make lint
make test                     # unit tests, race detector
make test-integration         # + testcontainers-backed integration tests
```

The server listens on `:8080` by default; override with the `PORT`
environment variable. It applies its migrations on startup, so `make run`
against a running Postgres is all that is needed — no separate migrate step.

### Common Make targets

| Target         | Description                                          |
| -------------- | ---------------------------------------------------- |
| `make build`   | Compile all packages and the `api` binary            |
| `make run`     | Run the API server (migrates on startup)             |
| `make test`    | Run unit tests with the race detector                |
| `make test-integration` | Run unit + integration tests (testcontainers; requires Docker) |
| `make lint`    | Run golangci-lint (falls back to `go vet`)           |
| `make archcheck` | Prove the linter rejects illegal cross-layer imports |
| `make tidy`    | Tidy and verify `go.mod` / `go.sum`                  |
| `make generate`| Regenerate sqlc code from queries + schema           |
| `make migrate-up` / `migrate-down` | Apply / roll back migrations manually    |

## Contributing

Work is tracked as tasks (`T-XXX`) in [`docs/TASKS.md`](docs/TASKS.md). Each
task lands via its own pull request: branch `feat/T-XXX-short-name` cut from
`main`, then open a PR into `main`. Direct pushes to `main` are not used.

**Tests are the contract**: every task card in `docs/TASKS.md` lists the
expected tests that prove the task is done. A task is only complete when that
suite exists and passes (`make test` for unit, `make test-integration` for
testcontainers-backed integration tests — both run in CI). This keeps the
skeleton rewrite-safe: reimplement any package and the suite tells you whether
behavior survived.

**Keep the docs in sync.** Part of every task's definition of done is updating
the tracking docs so they never drift from the code:

- Mark the task ✅ done in [`docs/TASKS.md`](docs/TASKS.md) (with its PR number).
- Refresh the **Status** and, where relevant, the **Try it locally** sections of
  this README when a task adds or changes user-visible behavior (new endpoints,
  new modules, new run/config requirements).
- Update [`docs/PLAN.md`](docs/PLAN.md) when a task changes architecture,
  decisions, or scope from what the plan describes.

Nothing ships without the docs reflecting it. See
[`CLAUDE.md`](CLAUDE.md) for the agent-facing version of this checklist.
