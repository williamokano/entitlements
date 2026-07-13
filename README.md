# entitlements

A reusable **skeleton for SaaS products**: a Go modular-monolith backend built
with DDD + hexagonal architecture, plus a React admin frontend. It ships seven
backend modules — tenant, authentication, authorization, catalog, subscription,
entitlements, and billing — and an `admin/` SPA built on the Inspinia v5 design
system, so a new SaaS can start from here instead of from scratch.

See [`docs/PLAN.md`](docs/PLAN.md) for the architecture,
[`docs/TASKS.md`](docs/TASKS.md) for the implementation task breakdown
(backend `T` cards + frontend `F` cards), and
[`docs/FRONTEND.md`](docs/FRONTEND.md) for the frontend design system and
conventions.

## Status

**Milestones 1 (platform kernel) and 2 (identity) complete; Milestone 3 (product
core) in progress.** The API boots, runs its migrations on startup, and serves
real endpoints.

Implemented (tasks **T-001 – T-027**):

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
  listing and removal; a user can belong to many tenants). A membership records
  the email it was invited by, so members can be listed by name without the
  tenant module having to look users up in another module.
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
  Also **plan changes** — upgrades (equal-or-higher monthly-equivalent price)
  re-pin immediately, downgrades become a visible, cancelable **scheduled
  change** applied at the period boundary — and **addon attach/detach** with
  quantities, plan-compatibility enforcement, and `plan_changed` /
  `addon_changed` events carrying the data billing will need for proration.
  Background **renewal & trial jobs** (advisory-lock scheduler) emit
  `renewal_due` exactly once per period, apply scheduled downgrades at rollover,
  advance the period on `invoice_paid` (or immediately while `BILLING_DISABLED`),
  and fire `trial_ending` / `trial_ended` — converting or expiring the trial per
  the plan's `card_required` config.
- **Entitlements** module (`/api/v1/entitlements`) — the product core: a dynamic
  **feature registry** (features are data — `key ≈ Stripe lookup_key`, `type:
  boolean | limit | config | enum`, default, `limit_behavior`, `reset_period`,
  `metadata`, and an `active` flag so features are archived, not deleted; a new
  feature needs no deploy). The **resolution pipeline** composes plan-version
  grants → addon deltas (× quantity) → tenant overrides with precedence exactly
  `plan < addon < override` (the canonical "plan 10 + addon 10 ⇒ 20" case). The
  effective set is **materialized per tenant** and rebuilt by idempotent consumers
  of subscription events; on every real change entitlements publishes exactly one
  `EntitlementsSummaryChanged` carrying the full re-resolved set (the phase-2
  webhook feed). Runtime reads — `GET /entitlements` (whole set in one call) and
  `GET /entitlements/{key}` — resolve live, so they are never stale; an
  `ENTITLEMENTS_UNKNOWN_FEATURE_POLICY` (`deny` default) governs keys absent from
  the registry. **Tenant overrides** (T-023) are managed under
  `POST /entitlements/overrides` + `GET /overrides` + `GET|PATCH|DELETE
  /overrides/{id}`: an override sets a feature's effective value outright (it wins
  over plan + addons), carries a mandatory **reason + actor** (actor taken from the
  signed-in principal) so every create/update/delete is written to the audit log,
  and may be **time-bound** via `expires_at` — a recurring expiry job reverts and
  prunes expired overrides (resolution ignores them the instant they lapse, so
  live reads never serve a stale boost), and the `GET /entitlements` response
  surfaces `expires_at` when the winning value is a time-bound override. Every
  override change re-materializes the tenant's effective set (emitting
  `EntitlementsSummaryChanged`) in the same transaction as the write + audit.
  **Usage tracking + quota enforcement** (T-024) meters `limit` features:
  `POST /entitlements/consume` `{key, n}` increments a per-(tenant, feature,
  period) counter through a **single guarded SQL upsert**, so a **hard** limit is
  never exceeded even under maximal concurrency — a breach returns a typed
  quota-exceeded **problem+json (HTTP 422)** — while a **soft** limit always
  consumes and emits one `EntitlementLimitWarning` per crossing.
  `POST /entitlements/release`, `GET /entitlements/usage`, and
  `GET /entitlements/usage/{key}` round out the surface. The period key is derived
  **lazily** from the feature's `reset_period` (`never` | calendar month | the
  subscription's billing cycle), so a new period resets the counter with no
  background job; and a **downgrade** that shrinks a limit below current usage
  emits one `EntitlementExceeded` and never blocks reads (only future consumes are
  gated). (Registry admin CRUD is service-level for now; the runtime HTTP surface
  is the GETs, overrides CRUD, and the usage/consume endpoints.)
- **Billing** module (`/api/v1/billing`) — invoices with **line-item snapshots**
  (T-025). `POST /billing/invoices` issues an invoice from the tenant's live
  subscription: it **copies** the pinned plan version and each attached addon
  (key, version, unit price, quantity, currency) into line items, so an issued
  invoice is a historical fact — a later plan-version publish, rename, or price
  change never rewrites it. Money is integer minor units throughout (never
  floats); totals are `subtotal = Σ(unit × qty)`, `total = subtotal + tax`, with
  tax from a pluggable **`TaxCalculator`** port (no-op default → zero tax). Each
  invoice gets a **per-tenant gapless number** (1, 2, 3…) allocated by an atomic
  counter upsert inside the issuing transaction — no gaps or dupes even under
  concurrent issuance, and independent per tenant. The invoice lifecycle
  `draft → open → paid | void | uncollectible` is a guarded state machine
  (`POST /billing/invoices/{id}/{pay,void,uncollectible}`); paying an invoice
  publishes `billing.invoice_paid` through the outbox so the subscription module
  advances its period. **Credit notes** (`POST /billing/invoices/{id}/credit-notes`
  `{amount_minor, reason}`) reference an invoice and store a negated amount. List
  and get (`GET /billing/invoices[/{id}]`) are tenant-scoped — another tenant's
  invoices are invisible. A **`PaymentProvider`** port (T-026) drives the money
  side: an in-memory **fake adapter** is the default (auto-succeeds; swap in a
  real gateway with `billing.WithPaymentProvider`). When billing is enabled
  (`BILLING_DISABLED=false`) billing runs an **idempotent renewal charge flow** —
  it consumes `subscription.renewal_due`, issues the invoice, charges the
  provider with a `(invoice, attempt)`-stable idempotency key, then publishes
  `billing.invoice_paid` on success (subscription advances) or
  `billing.payment_failed` on decline (invoice stays open). A duplicate renewal
  delivery yields exactly one charge (idempotent consumer + provider key).
  **Payment methods** are stored **tokens only** — a domain guard and a schema
  `token_not_pan` CHECK both refuse a raw card number. A declined renewal charge
  opens a **dunning schedule** (T-027) driven by the jobs runner: it retries the
  charge at the configured offsets (`BILLING_DUNNING_OFFSETS`, default
  `1d,3d,7d`), each retry using a new `(invoice, attempt)` idempotency key, and
  publishes `billing.payment_recovered` when a retry succeeds or
  `billing.dunning_exhausted` when the schedule is used up. The subscription
  module consumes these: `billing.payment_failed` moves it `active → past_due`,
  exhaustion moves it into **grace** (whose length comes from the pinned plan
  version's grace days) and then to **suspended** once grace elapses, and a
  recovery returns it to **active**. A mid-period plan change publishes
  `subscription.plan_changed`, which billing prorates via a selectable
  **`ProrationStrategy`** (`BILLING_PRORATION_STRATEGY`: `immediate_prorated`
  (default) issues a prorated invoice line now, `credit_next_invoice` defers the
  amount to the next invoice, `none` bills nothing) — all with exact integer
  minor-unit arithmetic.
- **Example** module (`/api/v1/example/things`) — a reference tenant-scoped
  slice demonstrating the full hexagonal shape and the outbox → consumer flow.

- **Frontend** (`admin/`) — a React SPA (React 19 + TypeScript + Vite +
  Tailwind 4) on the Inspinia v5 design system, wired to the API (F-001): the
  real app shell lives at `/` behind a working sign-in (`/auth/sign-in` →
  `POST /api/v1/auth/login`, transparent single refresh-and-retry on 401, hard
  logout on refresh reuse), with runtime configuration via
  `public/app-config.js` (`window.__APP_CONFIG__`: API base URL, tenant mode,
  branding) so a built bundle switches backends without a rebuild. The full
  Inspinia theme demo stays browsable under `/demo/*` (droppable from prod
  builds via `VITE_ENABLE_DEMO=false`). The full **auth suite** is live (F-003):
  sign-in, sign-up, forgot/reset password (token from the URL), email
  verification, plus an account **security** area at `/account/security`
  (change password, active-sessions list with "log out other devices", resend
  verification) and a real logout in the TopBar user menu. The **API keys**
  screen is also live (`/api-keys`: list, create with a one-time secret reveal,
  revoke — F-006). **Tenant** screens are live too (F-004): create-tenant
  onboarding (no-tenant empty state), settings at `/tenant` (name/settings edit,
  suspend/reactivate/delete with a confirm), and a **tenant switcher** in the
  TopBar — the api client sends the selected tenant's id as `X-Tenant-ID` (over a
  locally-persisted "known tenants" store, since there is no list-my-tenants
  endpoint yet). The **catalog** admin is live too (F-008, under `/catalog`):
  plans list (status + public badge) and detail with a versions timeline; a
  version editor that is editable **only while draft** (pricing per cycle in
  integer minor units, trial/grace config, feature grants) and read-only once
  published, with publish gated behind an immutability confirm; add-ons
  list/detail/versions with an entitlement-deltas + compatible-plans editor; and
  a public **pricing preview** bound to `GET /catalog/public`. The
  **subscription** screen is live too (F-009, under `/subscription`): a
  current-subscription card (status chip per state, current period, trial
  countdown, cancel-at-period-end and scheduled-change banners) whose lifecycle
  actions are **rendered from the backend state machine** (only legal
  transitions show — pause/resume/reactivate/cancel); an empty-state subscribe
  flow (public plan + billing-cycle picker); a change-plan flow (immediate re-pin
  vs a scheduled-for-period-end downgrade with a cancel-change button); and an
  addons section (quantity steppers, attach from compatible published addons,
  detach with a confirm). The **billing invoices** screen is live too (F-013,
  under `/billing`): a tenant invoice list (number, status, issued date,
  currency, total — all money formatted from integer minor units) and an invoice
  detail with its snapshotted line items, a subtotal/tax/total breakdown,
  lifecycle actions **gated by the backend state machine** (pay/void/mark
  uncollectible, shown per status), and a credit-note affordance (amount +
  mandatory reason) listing existing credit notes with their negated amounts. The
  **entitlements viewer** is live too (F-010, under `/entitlements`): a read-only
  table of the tenant's effective entitlements (feature key, value, and a source
  badge naming the winning `default | plan | addon | override` layer) fed by a
  single `GET /api/v1/entitlements` call — booleans render as on/off chips, limits
  as numbers, config/enum values verbatim — plus a per-feature drill-in reusing
  `GET /api/v1/entitlements/{key}` (value, source, and any override expiry). A
  tenant with no subscription still shows its `default`-sourced rows. Overrides
  CRUD (F-011) and the usage/quota panel (F-012) extend this page. The
  **members** screen is live too (F-005, under `/members`): the tenant's members
  (by email, with a remove that confirms first) and its pending invitations
  (invite by email + role, resend), plus a **public invitation accept/decline
  page** at `/invitations/{tenantId}/{invitationId}` — an invitee who lands on it
  signed out is sent to sign-in and returned to the invitation afterwards. The
  remaining module screen (roles) is a placeholder page until its **F-track**
  card lands — see [`docs/FRONTEND.md`](docs/FRONTEND.md).

The admin SPA also ships as a **generic Docker image** (F-002): one image built
once, configured entirely at container start via environment variables (API URL,
tenant mode, branding, demo toggle) — see *Running the admin SPA in Docker*
below. `docker compose up` now brings up Postgres, the API, and the SPA together.

Not yet implemented: the remaining frontend module screens (members, roles, the
overrides admin, and the usage/quota panel — F-005, F-007, F-011, F-012). See
[`docs/TASKS.md`](docs/TASKS.md) for the full plan.
(Note: the tenant creator is not yet auto-assigned the `owner` role, so an
initial role assignment currently has to be bootstrapped out of band — see the
T-016 follow-up in the tasks doc.)

## Try it locally

**Want the whole product (Postgres + API + admin SPA) in one command, with a
guided walkthrough, database-role setup, and the RLS hardening path?** See the
complete guide: **[`docs/RUNNING.md`](docs/RUNNING.md)**.

```bash
docker compose up --build     # Postgres + API + SPA → API :8080, SPA :3000
# …or just the DB for the Go dev loop:
docker compose up -d postgres # Postgres on :5432 (credentials match the app default)
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

# Invite someone into the tenant. The invitee needs no account yet — the
# invitation is by email, and becomes a membership when they accept.
curl -sX POST localhost:8080/api/v1/tenants/<tenant-uuid>/invitations \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer <access-token>' \
  -d '{"email":"teammate@example.com","role":"member"}'
# → {"id":"<invitation-uuid>","email":"teammate@example.com","role":"member",
#    "status":"pending","expires_at":"…"}

# The invitee registers + logs in as teammate@example.com, then accepts with
# THEIR access token: the backend authorizes by matching the caller's email to
# the invitation (there is no invite token), which is why the accept link carries
# the tenant and invitation ids: /invitations/<tenant-uuid>/<invitation-uuid>
curl -sX POST \
  localhost:8080/api/v1/tenants/<tenant-uuid>/invitations/<invitation-uuid>/accept \
  -H 'Authorization: Bearer <invitee-access-token>'
# → 201 {"user_id":"…","email":"teammate@example.com","role":"member","status":"active"}

# Members are listed with the email they were invited by
curl -s localhost:8080/api/v1/tenants/<tenant-uuid>/members \
  -H 'Authorization: Bearer <access-token>'
# → {"members":[{"user_id":"…","email":"teammate@example.com","role":"member","status":"active"}]}
# NOTE: the tenant's *creator* is not a member yet (T-031) — only accepting an
# invitation creates a membership, so a brand-new tenant lists no members.

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

# Read the tenant's effective entitlements in a single call (resolved live from
# the pinned plan version's grants + attached addon deltas + tenant overrides).
# The canonical case: a plan granting seats=10 plus an addon with a +10 limit
# delta at quantity 1 resolves to 20, source "addon":
curl -s localhost:8080/api/v1/entitlements \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>'
# → {"entitlements":{"seats":{"value":20,"source":"addon"}, …}}
curl -s localhost:8080/api/v1/entitlements/seats \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>'
# → {"key":"seats","value":20,"source":"addon"}

# Set a manual override (wins over plan + addons). Reason is mandatory; the actor
# is taken from the caller's principal. Add "expires_at":"<RFC3339>" to make it
# time-bound — the expiry job reverts and prunes it once it lapses.
curl -sX POST localhost:8080/api/v1/entitlements/overrides \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>' \
  -d '{"feature_key":"seats","value":50,"reason":"enterprise negotiation"}'
# → {"id":"…","feature_key":"seats","value":50,"reason":"…","actor":"…", …}
curl -s localhost:8080/api/v1/entitlements/seats \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>'
# → {"key":"seats","value":50,"source":"override"}
curl -sX DELETE localhost:8080/api/v1/entitlements/overrides/<override-id> \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>'
# → 204; the effective value reverts to plan + addons (20, source "addon")

# Meter a `limit` feature. Consume increments a per-(tenant, feature, period)
# counter through a single guarded upsert, so a hard limit is never exceeded even
# under concurrency. Say the feature `api_calls` (type limit, limit_behavior hard)
# resolves to 3 for this tenant:
curl -sX POST localhost:8080/api/v1/entitlements/consume \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>' \
  -d '{"key":"api_calls","n":1}'
# → 200 {"key":"api_calls","used":1,"limit":3,"behavior":"hard","period":"never",…}
# … three consumes succeed (used 1 → 2 → 3); the fourth breaches the hard limit:
# → 422 application/problem+json
#   {"title":"Unprocessable Entity","status":422,"detail":"quota exceeded for feature api_calls", …}

# Give capacity back (floored at zero), then read usage — for one feature or all.
curl -sX POST localhost:8080/api/v1/entitlements/release \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>' \
  -d '{"key":"api_calls","n":2}'                                   # → used back to 1
curl -s localhost:8080/api/v1/entitlements/usage/api_calls \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>'
# → {"key":"api_calls","used":1,"limit":3,"behavior":"hard","period":"never", …}
curl -s localhost:8080/api/v1/entitlements/usage \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>'
# → {"usage":[{"key":"api_calls","used":1,"limit":3, …}]}

# Issue an invoice from the tenant's live subscription. Its line items snapshot
# the pinned plan version (and any attached addons): key, version, unit price,
# quantity, currency — all copied, so later catalog changes never rewrite it. The
# invoice gets a per-tenant gapless number (1, 2, 3…) and integer minor-unit totals.
curl -sX POST localhost:8080/api/v1/billing/invoices \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>'
# → 201 {"id":"<invoice-uuid>","number":1,"status":"open","currency":"USD",
#        "subtotal_minor":1000,"tax_minor":0,"total_minor":1000,
#        "line_items":[{"kind":"plan","key":"pro","version":1,
#                       "unit_price_minor":1000,"quantity":1,"amount_minor":1000, …}]}

curl -s localhost:8080/api/v1/billing/invoices \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>'
# → {"invoices":[{ …the invoice above… }]}   (tenant-scoped — other tenants invisible)

# Publish a new plan version / change the price in the catalog, then re-read the
# invoice: it is unchanged (a historical fact).
curl -s localhost:8080/api/v1/billing/invoices/<invoice-uuid> \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>'
# → still {"line_items":[{"key":"pro","version":1,"unit_price_minor":1000, …}], …}

# Lifecycle: pay the invoice (publishes billing.invoice_paid, which the
# subscription module consumes to advance its period), then credit it.
curl -sX POST localhost:8080/api/v1/billing/invoices/<invoice-uuid>/pay \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>'
# → {"status":"paid", …}
curl -sX POST localhost:8080/api/v1/billing/invoices/<invoice-uuid>/credit-notes \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer <access-token>' -H 'X-Tenant-ID: <tenant-uuid>' \
  -d '{"amount_minor":1000,"reason":"refund"}'
# → 201 {"invoice_id":"<invoice-uuid>","number":1,"amount_minor":-1000,"reason":"refund", …}
```

A `limit` feature is registered service-level for now (registry admin CRUD has no
HTTP surface yet). A soft-limit feature never refuses a consume; it emits one
`EntitlementLimitWarning` the first time usage crosses the limit. Period reset is
lazy: for a `monthly` or `billing_cycle` feature the counter starts fresh in each
new period with no background job.

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
- Node 20+ (frontend only)

### Frontend (admin/)

```bash
cd admin
npm install
npm run dev        # Vite dev server — real app at /, theme demo at /demo
npm test           # Vitest + React Testing Library + MSW
npm run lint       # eslint (our code; the vendored theme is excluded)
npm run build      # production bundle (VITE_ENABLE_DEMO=false drops the demo)
```

The SPA is wired to the API (F-001): point `admin/public/app-config.js`
(`window.__APP_CONFIG__.apiBaseUrl`, default `http://localhost:8080`) at a
running backend, register a user (`POST /api/v1/auth/register`, then verify —
see the walkthrough above), and sign in at `/auth/sign-in`. Anonymous visitors
are redirected to the sign-in page; the vendored Inspinia demo stays fully
browsable under `/demo` and doubles as the component reference for building
our screens. See [`docs/FRONTEND.md`](docs/FRONTEND.md) for structure, runtime
configuration, and testing conventions.

### Running the admin SPA in Docker

The SPA builds into **one generic image** — nothing environment-specific is
baked into the JS bundle. Build it once, then point it at any backend by setting
environment variables at container start; an entrypoint renders
`app-config.js` from them (via `envsubst`) before nginx boots.

```bash
# Build once (add --build-arg VITE_ENABLE_DEMO=true to keep the /demo bundle)
docker build -t entitlements-admin ./admin

# Run, injecting configuration for this environment
docker run -d -p 3000:80 \
  -e API_BASE_URL=https://api.example.com \
  -e TENANT_MODE=header \
  -e APP_NAME="Entitlements" \
  entitlements-admin
# → http://localhost:3000  (deep links work via SPA history fallback)
```

Recognised variables (all optional; defaults shown):

| Variable        | Default                 | Purpose                                    |
| --------------- | ----------------------- | ------------------------------------------ |
| `API_BASE_URL`  | `http://localhost:8080` | Origin of the Go API                       |
| `TENANT_MODE`   | `header`                | `header` or `subdomain` tenant resolution  |
| `TENANT_SLUG`   | *(empty)*               | Fixed tenant (header mode)                 |
| `APP_NAME`      | `Entitlements`          | Branding / page title                      |
| `ENABLE_DEMO`   | `false`                 | Expose the vendored Inspinia demo          |

For **subdomain** tenant mode the SPA is served on `*.example.com` and the
backend resolves the tenant from the `Host` header; point wildcard DNS (or
`/etc/hosts` entries like `acme.localhost`) at the container.

Or bring up the whole stack (Postgres + API + SPA) with `docker compose up`
(SPA on `:3000`, API on `:8080`). CI publishes **both** images to GHCR on `main`
(tagged `latest` + commit `sha`): `ghcr.io/williamokano/entitlements/admin` and
`ghcr.io/williamokano/entitlements/api`. Run the stack from the published images
with the GHCR overlay — `docker compose -f docker-compose.yml -f
docker-compose.ghcr.yml up`. Full instructions, plus a least-privilege database
role setup and the Row-Level-Security path, are in
**[`docs/RUNNING.md`](docs/RUNNING.md)**.

### Build and test

```bash
docker compose up -d postgres # Postgres (integration tests + local run)
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
