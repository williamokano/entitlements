# Running the stack locally

A complete guide to bring up **the whole product — Postgres + Go API + React
admin SPA — with Docker Compose**, exercise it end to end, and understand the
database roles (and the Row-Level-Security hardening path). For the pure Go dev
loop (`make run`, tests) see the [README](../README.md#development).

- [1. TL;DR](#1-tldr)
- [2. What comes up](#2-what-comes-up)
- [3. First run, step by step](#3-first-run-step-by-step)
- [4. A guided walkthrough](#4-a-guided-walkthrough)
- [5. How the database is created & migrated](#5-how-the-database-is-created--migrated)
- [6. Database roles & least privilege](#6-database-roles--least-privilege)
- [7. Row-Level Security (RLS)](#7-row-level-security-rls)
- [8. Running the published images (GHCR)](#8-running-the-published-images-ghcr)
- [9. Configuration reference](#9-configuration-reference)
- [10. Reset & troubleshooting](#10-reset--troubleshooting)

---

## 1. TL;DR

```bash
docker compose up --build
```

- **Admin SPA** → http://localhost:3000
- **API** → http://localhost:8080 (health: `GET /healthz`)
- **Postgres** → localhost:5432 (`entitlements` / `entitlements`)

The API applies all migrations on startup, so the database is ready as soon as
`api` reports healthy. Open the SPA, sign up, and you're in. Stop with
`Ctrl-C`; wipe everything with `docker compose down -v`.

> **Prerequisites:** Docker + the Compose plugin. Nothing else — Go and Node are
> only needed for local development, not for running the stack.

---

## 2. What comes up

`docker compose up` starts three services (see [`docker-compose.yml`](../docker-compose.yml)):

| Service | Image / build | Port | Role |
|---|---|---|---|
| `postgres` | `postgres:16-alpine` | 5432 | Database. Data persists in the `pgdata` volume. |
| `api` | built from [`Dockerfile`](../Dockerfile) (Go, `scratch` runtime) | 8080 | The modular-monolith backend. Applies migrations on boot. |
| `admin` | built from [`admin/Dockerfile`](../admin/Dockerfile) (nginx) | 3000 | The React SPA. One generic image, configured at container start. |

**How the SPA reaches the API:** the SPA runs in *your browser*, so it calls the
API at the **host** address, not the compose service name. That's why
`admin`'s `API_BASE_URL` is `http://localhost:8080` (the API's published port),
not `http://api:8080`. Change it (and the other `admin` env vars) to point a
single built image at any backend — see [§8](#8-running-the-published-images-ghcr)
and [§9](#9-configuration-reference).

---

## 3. First run, step by step

```bash
# 1. Build images and start everything (first build pulls base images + compiles).
docker compose up --build

# 2. In another terminal, confirm the API is healthy and migrated.
curl -s localhost:8080/healthz          # {"status":"ok"}

# 3. Open the SPA.
open http://localhost:3000              # (or just visit it in a browser)
```

On the sign-in screen, create an account and sign in — the SPA talks to the real
API (`POST /api/v1/auth/register` + `/login`, with transparent refresh-token
rotation). Then visit **API Keys** in the sidebar to create a tenant API key
(one-time secret reveal). The rest of the module screens arrive with their
F-track tasks (see [`docs/TASKS.md`](./TASKS.md)); the full Inspinia component
demo stays browsable under **/demo** when the demo bundle is enabled.

To keep the demo in the built image, start with:

```bash
docker compose build --build-arg VITE_ENABLE_DEMO=true admin && docker compose up
# and set ENABLE_DEMO=true in the admin service env
```

---

## 4. A guided walkthrough

The API is usable directly with `curl` — handy for seeing the whole product flow
before the screens exist. Auth is global (no tenant header); everything else is
tenant-scoped via `X-Tenant-ID`.

```bash
API=http://localhost:8080/api/v1
J='content-type: application/json'

# --- Identity: register + login ---
curl -sX POST $API/auth/register -H "$J" \
  -d '{"email":"me@example.com","password":"a-strong-password"}'
TOKEN=$(curl -sX POST $API/auth/login -H "$J" \
  -d '{"email":"me@example.com","password":"a-strong-password"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')
AUTH="authorization: Bearer $TOKEN"

# --- Tenant: create one, capture its id ---
TENANT=$(curl -sX POST $API/tenants -H "$J" -H "$AUTH" \
  -d '{"name":"Acme","slug":"acme"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
T="x-tenant-id: $TENANT"

# --- Catalog: a plan with a published, priced version ---
CREATED=$(curl -sX POST $API/catalog/plans -H "$J" -H "$AUTH" \
  -d '{"key":"pro","name":"Pro"}')
PLAN=$(echo "$CREATED"    | python3 -c 'import sys,json;print(json.load(sys.stdin)["plan"]["id"])')
VER=$(echo "$CREATED"     | python3 -c 'import sys,json;print(json.load(sys.stdin)["version"]["id"])')
curl -sX PATCH $API/catalog/plans/$PLAN/versions/$VER -H "$J" -H "$AUTH" \
  -d '{"currency":"USD","prices":[{"cycle":"monthly","amount_minor":1000}]}' >/dev/null
curl -sX POST  $API/catalog/plans/$PLAN/versions/$VER/publish -H "$AUTH" >/dev/null

# --- Subscription: subscribe the tenant to that version ---
curl -sX POST $API/subscription -H "$J" -H "$AUTH" -H "$T" \
  -d "{\"plan_version_id\":\"$VER\",\"cycle\":\"monthly\"}"

# --- API keys: mint one, then call the API as a machine (tenant derived from the key) ---
KEY=$(curl -sX POST $API/api-keys -H "$J" -H "$AUTH" -H "$T" \
  -d '{"name":"ci","scopes":["things:write"]}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["api_key"])')
curl -sX POST $API/example/things -H "$J" -H "authorization: ApiKey $KEY" \
  -d '{"name":"via-key"}'
```

Everything above is also doable from the SPA as those screens land.

---

## 5. How the database is created & migrated

- The `postgres` service creates the database (`POSTGRES_DB=entitlements`) on
  first start of an empty `pgdata` volume.
- **The API owns its schema.** On every boot it runs the embedded migrations
  (each module has its own `goose_<module>_version` table), so there is **no
  separate migrate step** — start the API and the schema is current. Migrations
  are idempotent; restarting is safe.
- Migrations run with the **migration DSN**: `MIGRATION_DATABASE_URL` if set,
  otherwise `DATABASE_URL`. In the default single-role compose these are the
  same role; the least-privilege overlay ([§6](#6-database-roles--least-privilege))
  splits them.

To reset the database completely, drop the volume: `docker compose down -v`.

---

## 6. Database roles & least privilege

The base compose uses **one** database role for simplicity. For a
production-shaped setup you want two roles — one that can change the schema and
one that only reads/writes data — so a compromised app process can't alter or
drop tables. That split is provided as an overlay.

```bash
# Fresh volume required (role init runs only on first DB init):
docker compose down -v
docker compose -f docker-compose.yml -f docker-compose.roles.yml up --build
```

This wires:

| Role | Used for | Privileges |
|---|---|---|
| `entitlements_owner` | migrations (`MIGRATION_DATABASE_URL`) — DDL | creates/owns every schema, table, sequence |
| `entitlements_app` | runtime (`DATABASE_URL`) — DML | `SELECT/INSERT/UPDATE/DELETE` only; **no** DDL, **not** a superuser, **no `BYPASSRLS`** |

The roles are created by [`deploy/db/init/10-roles.sql`](../deploy/db/init/10-roles.sql),
which the Postgres image runs once on first init. The clever bit is
`ALTER DEFAULT PRIVILEGES FOR ROLE entitlements_owner … TO entitlements_app`:
every object the owner creates *in the future* (i.e. during each migration,
including migrations you haven't written yet) automatically grants the app role
schema `USAGE` + table DML. No per-table grant step, ever.

You can verify the separation:

```bash
# The app role can read data…
docker compose exec postgres psql -U entitlements_app -d entitlements \
  -c 'select count(*) from tenant.tenants;'
# …but cannot change the schema:
docker compose exec postgres psql -U entitlements_app -d entitlements \
  -c 'create table public.nope(x int);'      # ERROR: permission denied for schema public
```

> Passwords in the init script and overlay are **dev defaults** (`owner` / `app`).
> Replace them (and source them from a secret manager) for anything real.

---

## 7. Row-Level Security (RLS)

**Status — read this first.** RLS is **not yet enforced by the application.**
Today, tenant isolation is enforced in the **app layer**: every tenant-scoped
repository query filters by the `tenant_id` resolved from the request context
(JWT claim / `X-Tenant-ID` header / subdomain), and the compiler + `depguard`
keep that logic centralized. This is robust for a single trusted app process,
which is what the skeleton is. Wiring Postgres RLS as a **defense-in-depth second
layer** is tracked as **T-030** in [`docs/TASKS.md`](./TASKS.md). This section
documents the target design and the exact role setup it needs, so the
least-privilege roles above are already RLS-ready (the app role has **no
`BYPASSRLS`**).

### The model RLS will use

1. **A per-transaction tenant GUC.** The app sets a session variable at the
   start of each tenant-scoped transaction:

   ```sql
   SET LOCAL app.tenant_id = '<tenant-uuid>';
   ```

   `SET LOCAL` is scoped to the transaction, so it can't leak across pooled
   connections — which is why every tenant-scoped write already goes through the
   `UnitOfWork`, the natural place to set it.

2. **A policy per tenant-scoped table**, keyed off that GUC. Example migration
   for one table (the pattern T-030 will apply across the board):

   ```sql
   ALTER TABLE tenant.some_table ENABLE ROW LEVEL SECURITY;
   ALTER TABLE tenant.some_table FORCE ROW LEVEL SECURITY;  -- applies to the table owner too

   CREATE POLICY tenant_isolation ON tenant.some_table
     USING      (tenant_id = current_setting('app.tenant_id')::uuid)
     WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
   ```

   `USING` filters reads/updates/deletes; `WITH CHECK` stops a row being written
   for a different tenant.

3. **The runtime role is subject to RLS.** `entitlements_app` is a plain,
   non-superuser role **without `BYPASSRLS`** — so the policies actually bind it.
   (Postgres skips RLS for superusers, for `BYPASSRLS` roles, and — unless you
   add `FORCE ROW LEVEL SECURITY` — for the table's owner. That's why the policy
   above forces it and why the app never connects as the owner.)

### Bypass — for the operations that genuinely span tenants

Some work is legitimately cross-tenant or tenant-less: applying migrations,
background jobs that scan every tenant (renewals, trials), platform-admin tooling,
and seed scripts. Those must **not** be constrained by a single tenant's policy.
Two complementary mechanisms:

- **A dedicated bypass role.** Give the migration/owner role (or a separate
  `entitlements_system` role) the `BYPASSRLS` attribute:

  ```sql
  ALTER ROLE entitlements_owner BYPASSRLS;   -- migrations & schema ops ignore policies
  -- or a purpose-built system role for cross-tenant background work:
  CREATE ROLE entitlements_system LOGIN PASSWORD '…' BYPASSRLS;
  ```

  Connect the migration path (and, if you want DB-level enforcement for jobs, the
  job runner) with this role. This is the "bypass sometimes for some operations"
  case: the app role never bypasses; a narrow, audited system role does.

- **App-level system context.** The codebase already models tenant-less
  operations with `authctx.WithSystemContext` / `IsSystem`. When T-030 lands, a
  system-context transaction will skip the `SET LOCAL app.tenant_id` (and use the
  bypass role where a DB-level bypass is required), so cross-tenant jobs keep
  working while ordinary requests stay policy-bound.

### Why it isn't on by default yet

Turning RLS on safely means: setting the GUC on **every** code path that touches
tenant data (not just writes), auditing that no tenant query runs outside a
transaction, `FORCE`-ing RLS so even accidental owner connections are covered,
and a test that a cross-tenant read returns zero rows at the database layer. That
is a focused, test-backed change — hence its own task rather than a flag flip.

---

## 8. Running the published images (GHCR)

CI publishes **two images** from this monorepo on every push to `main`:

- `ghcr.io/williamokano/entitlements/api` — the Go backend
- `ghcr.io/williamokano/entitlements/admin` — the admin SPA

Both are tagged `latest` and `sha-<commit>`. To run the stack from the published
images instead of building locally, add the GHCR overlay:

```bash
docker compose -f docker-compose.yml -f docker-compose.ghcr.yml up
# pin a specific build:
IMAGE_TAG=sha-abc1234 docker compose -f docker-compose.yml -f docker-compose.ghcr.yml up
```

The overlay ([`docker-compose.ghcr.yml`](../docker-compose.ghcr.yml)) swaps the
`build:` for `image:` on `api` and `admin`. The images are **public** if the
repository packages are set public; otherwise `docker login ghcr.io` first with a
PAT that has `read:packages`.

**The admin image is generic — configured at container start.** The same image
serves any environment; only the runtime env changes:

```bash
docker run -p 3000:80 \
  -e API_BASE_URL=https://api.your-saas.com \
  -e TENANT_MODE=subdomain \
  -e APP_NAME="Your SaaS" \
  ghcr.io/williamokano/entitlements/admin:latest
```

The entrypoint renders `/app-config.js` (`window.__APP_CONFIG__`) from those
variables via `envsubst`, then starts nginx — nothing environment-specific is
baked into the JS bundle.

---

## 9. Configuration reference

**API** (see [`internal/platform/config`](../internal/platform/config)):

| Env var | Default | Purpose |
|---|---|---|
| `APP_ENV` | `development` | `development` \| `production`. Production forbids the default DB URL. |
| `PORT` | `8080` | HTTP listen port. |
| `DATABASE_URL` | local dev DSN | Runtime connection (least-privilege app role in the roles overlay). |
| `MIGRATION_DATABASE_URL` | *(empty → uses `DATABASE_URL`)* | DSN migrations run with (privileged owner role). |
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error`. |
| `BILLING_DISABLED` | `true` | Auto-advance subscription periods without a billing module (until T-025/026). |
| `SUBSCRIPTION_TRIAL_ENDING_DAYS` | `3` | Days before trial end to emit `trial_ending`. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | *(empty → no export)* | OTLP/HTTP collector URL for traces/metrics. |
| `OTEL_SERVICE_NAME` | `entitlements` | Service name in telemetry. |

**Admin SPA** (rendered into `window.__APP_CONFIG__` at container start):

| Env var | Default | Purpose |
|---|---|---|
| `API_BASE_URL` | `http://localhost:8080` | Origin the browser calls the API at. |
| `TENANT_MODE` | `header` | `header` (send `X-Tenant-ID`) \| `subdomain` (backend resolves from Host). |
| `TENANT_SLUG` | *(empty)* | Fixed tenant when `TENANT_MODE=header`. |
| `APP_NAME` | `Entitlements` | Branding shown in the UI. |
| `ENABLE_DEMO` | `false` | Expose the Inspinia demo under `/demo` at runtime (also needs the demo built into the bundle via the `VITE_ENABLE_DEMO` build arg). |

---

## 10. Reset & troubleshooting

| Symptom | Fix |
|---|---|
| Schema/roles changes didn't take effect | Role init and `POSTGRES_*` only apply on a **fresh** volume: `docker compose down -v` then up. |
| `admin` can't reach the API | `API_BASE_URL` must be the **host** origin the browser can reach (e.g. `http://localhost:8080`), not `http://api:8080`. |
| API exits on boot with a DB error | Postgres wasn't ready or the DSN/role is wrong. The `api` service waits on the postgres healthcheck; check `docker compose logs postgres`. |
| Port already in use | Something else is on 3000/8080/5432 — stop it or remap the `ports:` in compose. |
| Want just the DB (run API via `make run`) | `docker compose up -d postgres`, then `make run`. |
| Start over from scratch | `docker compose down -v && docker compose up --build`. |
