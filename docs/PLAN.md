# SaaS Backend Skeleton — Architecture & Implementation Plan

## Context

This repository is a **reusable Go backend skeleton for SaaS products**: a modular monolith using DDD + hexagonal architecture, containing seven modules (tenant, authentication, authorization, catalog, subscription, entitlements, billing). Whenever a new SaaS is started, this repo is the starting point. It must itself be able to become a SaaS later (phase 2: webhooks/API keys for external consumers).

## Key Decisions (defaults — each is deliberately swappable)

| Concern | Decision | Rationale |
|---|---|---|
| Language | Go (latest stable, currently 1.24.x), single `go.mod` | Monolith module-based; one module, packages as boundaries |
| Persistence | PostgreSQL + `pgx/v5` + `sqlc` | Type-safe SQL, no ORM leaking into domain |
| Migrations | `goose` (embedded, per-module folders) | Simple, supports Go + SQL migrations |
| Tenancy | Shared schema, `tenant_id` on every tenant-scoped table, enforced via repository layer + tenant-aware context; optional Postgres RLS later | Simplest to operate for a starter; isolation strategy hidden behind repositories so schema-per-tenant can replace it per-SaaS |
| Transport | REST, stdlib `net/http` (1.22+ method routing), one router per module mounted at composition root | Zero framework lock-in |
| Module comms | Synchronous: small public **ports** (interfaces) each module exposes. Asynchronous: **domain events** on an in-process bus backed by a **transactional outbox** | Outbox makes phase-2 webhooks/broker trivial; events keep billing/entitlements decoupled from subscription |
| IDs | UUIDv7 | Sortable, index-friendly |
| Auth tokens | JWT access (short-lived) + rotating refresh tokens | Standard; provider-agnostic |
| Passwords | argon2id | Current best practice |
| Background work | Minimal scheduler/worker abstraction (Postgres advisory-lock leader election) for renewals, dunning, trial expiry | No external queue dependency in the skeleton |
| Frontend | React 19 + TypeScript + Vite SPA in `admin/`, on the **Inspinia v5** theme as design system; theme demo preserved under `/demo/*` as a component reference | Buy-not-build UI kit; every screen reuses theme components — see `docs/FRONTEND.md` |
| Frontend config | Runtime-injected `window.__APP_CONFIG__` (`app-config.js` rendered from env by the container entrypoint): API base URL, tenant mode (header/subdomain), branding | One generic Docker image per version — deploy-time configuration, nothing baked into the bundle |

## Repository Layout

```
cmd/api/main.go                  # composition root: config, db, bus, mounts all modules
internal/
  platform/                      # shared kernel (NO business logic)
    config/                      # env-based config loading
    httpx/                       # server, middleware (request-id, recovery, auth, tenant), problem+json errors
    postgres/                    # pool, tx manager (UnitOfWork), migration runner
    events/                      # event bus interface, in-proc impl, transactional outbox + relay worker
    jobs/                        # scheduler abstraction + Postgres-locked runner
    id/ clock/                   # UUIDv7 gen, clock abstraction (testability)
    authctx/                     # principal + tenant extraction from context
  modules/
    tenant/
    authentication/
    authorization/
    catalog/
    subscription/
    entitlements/
    billing/
migrations/                      # per-module: migrations/<module>/NNN_*.sql
admin/                           # frontend SPA (React+Vite, Inspinia design system) — see docs/FRONTEND.md
docs/PLAN.md                     # this document
docker-compose.yml               # postgres for local dev
Makefile                         # build, test, lint, sqlc gen, migrate
```

### Per-module internal structure (hexagonal)

```
modules/<name>/
  ports/           # PUBLIC surface for other modules: facade interface + published event types
  internal/        # compiler-private to this module (Go internal/ visibility)
    domain/        # entities, value objects, domain events, domain services, repository interfaces
    service/       # use cases (command/query handlers); owns transaction boundaries
    adapters/
      rest/        # REST handlers (driving adapter)
      postgres/    # repository implementations (driven adapter)
  module.go        # New(app.Deps) *Module — returns its port + http.Handler + event subscriptions
```

**Enforced rules**:
- **Cross-module privacy (compiler-enforced):** `domain`/`service`/`adapters` live under the module's `internal/` directory, so the Go compiler forbids any other module from importing them — a module reaches another module only through its public `ports` package.
- **Domain purity (depguard):** a module's `domain` package may import only the standard library, `uuid`, and `platform/apperr` — never service/adapters, the platform kernel, or other modules.
- **Platform isolation (depguard):** the platform kernel must not import business modules or the composition layer (`internal/app`).
- A CI self-test (`scripts/archcheck.sh`) writes a deliberately illegal import and asserts `make lint` fails, proving the rules actually bite.
- Each module owns its own Postgres schema (`tenant.*`, `billing.*`, …). No cross-module joins; read-your-own-data or call the port.

**Composition root** (`cmd/api`): `config → observability → Postgres pool → migrations → build `app.Deps` → wire each `app.Module` (mount its handler under `/api/v1/<name>`, register its subscriptions on the bus) → start the outbox relay + job runner → serve`. The `example` module is a bootable reference implementation of this pattern (delete it and `migrations/example/` for a real SaaS).

## Module Specifications

### 1. Tenant
The root of isolation; everything else is tenant-scoped.
- Tenant lifecycle: create, update, suspend, soft-delete (status: `active | suspended | deleted`), slug + UUID.
- Extensible metadata (`settings JSONB`) so each SaaS adds fields without schema changes.
- **Membership**: users ↔ tenants (a user can belong to multiple tenants) with role reference; invitation flow (invite by email, accept/decline, expiry).
- **Provisioning pipeline**: `TenantCreated` event drives hookable onboarding steps (seed default roles, create trial subscription, …) — each SaaS registers its own steps at the composition root. This is how "tenant creation is customizable per SaaS" is achieved.
- Tenant resolution middleware: subdomain / header / JWT claim → `authctx`; repositories refuse to run without a tenant in context (except explicitly tenant-less admin ops).

### 2. Authentication
Identity only — no permissions (that's authorization) and no membership (that's tenant).
- Register (email + password), email verification, login, logout, password recovery (single-use expiring tokens), password change.
- JWT access tokens + rotating refresh tokens with revocation (token family detection for reuse attacks).
- **`IdentityProvider` port** so password auth is just the first implementation; OAuth2/OIDC and SAML slot in later. User record supports multiple linked credentials.
- **2FA-ready model**: credentials modeled as *factors* (`password`, later `totp`, `webauthn`); login flow returns `mfa_required` challenge state even though only password exists initially.
- Gap-closing additions: login rate limiting + account lockout hooks, security audit events (`UserRegistered`, `LoginFailed`, `PasswordChanged`), session listing/revocation ("log out other devices").

### 3. Authorization
Simple dynamic RBAC, replaceable later.
- Role CRUD per tenant (roles are data, not code) + permission strings (`resource:action` convention).
- **System roles** seeded per tenant (`owner`, `admin`, `member`) that can't be deleted; custom roles unlimited.
- Assign/unassign roles to a user *within a tenant* (same user, different roles per tenant).
- Query: get user's roles, get effective permissions; `Check(ctx, permission)` helper + HTTP middleware.
- Port designed so a policy engine (ABAC/Cedar/OPA) can replace the RBAC impl without touching callers.

### 4. Catalog
The product definition: plans, prices, addons, and what features they carry.
- Plan CRUD with **versioning**: published plan versions are immutable; subscriptions pin a plan version (protects existing customers when a plan changes — "grandfathering" for free).
- Plan lifecycle: `draft → active → archived`; public vs hidden plans (hidden = custom enterprise deals).
- Per plan version: pricing per **billing cycle** (monthly/annual/custom interval), currency, trial config (enabled, days, card required?), grace period days, feature/limit composition.
- **Addon catalog**: addons with their own pricing, declared compatible plans, and the entitlement deltas they apply (e.g. `max_configs +10`).
- Gap-closing additions: plan display metadata (ordering, highlight), setup fees, coupons/discount codes (deferred to phase 2, but invoice line items modeled so discounts fit later).

### 5. Subscription — a separate module
It owns a genuine state machine that is neither "what you get" (entitlements) nor "what you pay" (billing). Keeping it separate keeps both neighbors simple.
- Create subscription for a tenant against a plan version + billing cycle (with or without trial).
- **State machine**: `trialing → active → past_due → grace → suspended → canceled | expired`, with guarded transitions and an audit trail of every transition.
- Current period tracking (start/end, next renewal at), renewal handled via scheduled job emitting `SubscriptionRenewalDue` exactly once per period (gated by a per-subscription marker, so duplicate ticks / multiple runners don't double-emit). The period advances only when billing confirms payment via an idempotent `billing.invoice_paid` consumer; a `BILLING_DISABLED` flag (default on until the billing module lands) auto-advances so the module is testable standalone. Trials resolve on their own scheduled job (`TrialEnding` a configurable number of days before, then convert-or-expire at the boundary per the plan version's `card_required`).
- Plan changes: upgrade (immediate) / downgrade (**scheduled change** applied at period end by default), with proration policy delegated to billing.
- Addon attach/detach (quantity-aware, e.g. extra seats ×3).
- Cancel (immediate or at period end), reactivate, pause/resume.
- Trial handling: `TrialEnding` (n days before) and `TrialEnded` events; converts or expires per plan config.
- Published events: `SubscriptionCreated/Activated/Renewed/PlanChanged/AddonChanged/Canceled/Suspended/TrialEnding` — these are what entitlements and billing react to.

### 6. Entitlements
The answer to "what can this tenant do right now, and how much of it".
- **Dynamic feature registry** (features are data, not enums): `key` (the stable
  external identifier — the analog of Stripe's `lookup_key`), type (`boolean |
  limit | config-value | enum`), default value, description, arbitrary
  `metadata` (JSONB, for per-feature integration data), and an `active` flag so
  features are **archived, not deleted** — historical entitlements and audit
  entries stay coherent. New features are inserted, never deployed.
- **Resolution pipeline** with explicit precedence: plan-version base grants → addon deltas → tenant overrides ⇒ *effective entitlements*. Resolved set is cached (materialized per tenant) and invalidated by subscription/catalog events. Each time a tenant's effective set changes, entitlements publishes an **`EntitlementsSummaryChanged`** event carrying the full re-resolved set for that tenant — the analog of Stripe's `entitlements.active_entitlement_summary.updated`, and the feed the phase-2 webhooks module forwards to external consumers.
- **Overrides per tenant**: manual grants/boosts (support gestures, negotiated contracts), each with optional expiry (time-bound overrides) and a reason + actor for audit.
- Addon-driven limit extension: the "plan allows 10, pay to extend to 20" case = addon carrying `limit_delta` on a feature key.
- **Usage tracking**: counters per (tenant, feature) with reset periods (`billing-cycle | monthly | never`), so limits are enforceable: `CheckAccess(feature)`, `ConsumeQuota(feature, n)` (atomic check-and-increment), `GetUsage`.
- Gap-closing additions:
  - **Soft vs hard limits** (warn + event vs block) per feature.
  - **Downgrade grace**: when effective limits shrink below current usage, emit `EntitlementExceeded` instead of breaking the tenant; enforcement policy configurable.
  - **Unknown-feature policy** (default-deny vs default-allow) configurable per SaaS.
  - Feature bundles/dependencies (feature X implies Y) — model now, implement minimal.
  - Full **audit log** of entitlement changes (who/what/when/why).
  - Admin API: feature CRUD, tenant override CRUD; runtime API: get-all-entitlements (single call for frontends), check-one, consume.

**Benchmark vs Stripe Billing Entitlements.** This module is a superset of
Stripe's model. Stripe Entitlements is boolean-only (`feature` + `lookup_key`),
derives active entitlements from catalog product-features + subscription status,
exposes them per customer plus an `active_entitlement_summary` webhook, and does
**no** runtime enforcement, numeric limits, or per-customer overrides (numeric
usage lives in a separate, disconnected Meters API). We cover all of that —
`key`≈`lookup_key`, feature `metadata`, archive-not-delete, subscription-derived
resolution, the `EntitlementsSummaryChanged` event, get-all/check-one — and add
what Stripe lacks in Entitlements: numeric `limit`/`config`/`enum` types, addon
limit deltas, per-tenant overrides with expiry, soft/hard limits, downgrade
grace, feature dependencies, and first-class runtime enforcement
(`CheckAccess` / `ConsumeQuota`). Stripe-Meters-style aggregation of raw usage
events (sum/count/max/unique) is the one thing we defer to the optional phase-2
usage-metering ingestion endpoint; the enforcement path uses counters instead.

### 7. Billing
Money side: invoices, payment execution, dunning. Never decides subscription policy — it reports outcomes as events; subscription reacts.
- **Invoice generation** from a subscription at billing time with **line-item snapshots**: plan name, version, prices, addons, quantities, currency frozen at issuance — later catalog changes never rewrite history.
- Invoice lifecycle: `draft → open → paid | void | uncollectible`; per-tenant invoice number sequences; credit notes for refunds.
- **`PaymentProvider` port** (the core abstraction): `CreateCustomer`, `AttachPaymentMethod`, `Charge`, `Refund`, plus a normalized inbound-webhook translation interface. Skeleton ships a `fakeprovider` (auto-succeeds, test hooks for failure) — Stripe/Adyen/etc. are per-SaaS adapters.
- Payment method management (tokenized references only; raw card data never touches this system).
- **Dunning**: configurable retry schedule for failed charges (e.g. +1d, +3d, +7d), emitting `PaymentFailed/PaymentRecovered/DunningExhausted` — subscription consumes these for `past_due/grace/suspended` transitions.
- Proration calculation for mid-cycle plan changes (strategy interface: `immediate-prorated | none | credit-next-invoice`).
- Gap-closing additions: idempotency keys on all money operations, tax as a pluggable `TaxCalculator` port (skeleton: no-op), refund flow, billing anchor dates.

## Cross-cutting flows (validate the seams)

1. **Signup**: `auth.Register` → `tenant.Create` → `TenantCreated` → provisioning hooks create trial subscription (catalog default plan) + seed roles → `SubscriptionCreated` → entitlements materializes effective set.
2. **Renewal**: jobs runner → `SubscriptionRenewalDue` → billing snapshots invoice → `PaymentProvider.Charge` → `InvoicePaid` → subscription advances period ‖ `PaymentFailed` → dunning → `past_due`.
3. **Upgrade with addon**: subscription applies change → billing prorates → `SubscriptionPlanChanged` → entitlements re-resolves (10 → 20).

## Outbox scope note (phase 1, not phase 2)

The transactional outbox is **part of phase 1's foundation** — only the external *webhooks module* on top of it is phase 2. Concrete phase-1 outbox shape (~1 migration + ~300 lines of Go):
- `platform_outbox` table: `id (uuidv7), occurred_at, tenant_id, module, event_type, payload jsonb, published_at, attempts`.
- `events.Append(ctx, tx, evt)` — called inside the same transaction as the state change, so events are never lost or phantom.
- A relay worker polling `FOR UPDATE SKIP LOCKED` batches and dispatching to in-process subscribers (at-least-once).
- `platform_processed_events` (`consumer, event_id`) so every consumer is idempotent — this companion piece is mandatory for correctness and included in phase 1.

## Additional phase-1 inclusions (pulled forward / made explicit)

- **API keys / machine auth**: hashed keys, tenant-bound, scoped; a second credential type in the auth middleware chain. Cheap now, disruptive to retrofit — and required the day this skeleton is sold as a service.
- **Audit log as a platform package**: entitlements, billing, and auth all independently need who/what/when/why trails; one `platform/audit` writer + table shared by all modules.
- **Observability baseline**: `slog` structured logging with request/tenant/trace IDs, OpenTelemetry trace + metric hooks, `/healthz` + `/readyz` endpoints.
- **Idempotency keys** on mutating HTTP endpoints (header-based), not just billing internals.

## Frontend (admin SPA)

Full details in [`docs/FRONTEND.md`](./FRONTEND.md); the task breakdown is the
**F-track** in [`docs/TASKS.md`](./TASKS.md). The essentials:

- **Design system**: the Inspinia v5 React theme (React 19, TS, Vite 7,
  Tailwind 4, react-router 7, react-hook-form + yup, TanStack Table, Apex/ECharts).
  Screens are adapted copies of theme pages under `admin/src/views/app/`; the
  complete theme demo stays browsable under `/demo/*` (droppable from prod
  builds) as a living component reference. Demo views are treated as vendored —
  never edited, only copied.
- **Backend integration**: one fetch-based API client (`src/lib/api.ts`) —
  bearer token + transparent refresh rotation (hard logout on refresh reuse),
  tenant header or subdomain mode mirroring `tenant.ResolveMiddleware`,
  RFC 7807 problem+json parsed into typed errors, idempotency keys on mutations.
- **Deployability**: a single generic Docker image (multi-stage build → nginx).
  The entrypoint renders `app-config.js` (`window.__APP_CONFIG__`) from
  environment variables at container start — API base URL, tenant mode,
  branding — so the same image serves any environment or white-label domain.
- **Definition of done**: backend tasks that add or change user-facing
  endpoints include their screens (in-task or as a paired F-card in the same
  PR). Frontend tests (Vitest + RTL + MSW) are contract, like backend tests.

## Phase 2 (explicitly out of scope now, but the outbox makes it cheap)

- **Webhooks module**: external subscribers register endpoints per event type; signed (HMAC) delivery with retries/backoff and dead-lettering — feeds directly off the phase-1 outbox; estimated 2–3 days incremental once the outbox exists.
- Admin API surface, usage-metering ingestion endpoint, coupons, SSO (OAuth/OIDC/SAML) and TOTP implementations behind the already-built factor model.

## Implementation Milestones

1. **Foundation**: `go.mod`, platform kernel (config, httpx, postgres, events+outbox+processed-events, jobs, id/clock, audit, observability), module wiring pattern, docker-compose, Makefile, CI (lint + arch-lint + tests).
2. **Identity core**: tenant → authentication → authorization (in that order; auth middleware ties them together).
3. **Catalog**, then **Subscription** (state machine + scheduler), then **Entitlements** (resolution + usage), then **Billing** (invoices, fake provider, dunning).
4. **Hardening**: idempotency, audit trails, seed/demo data, integration tests (testcontainers), example "new SaaS" checklist in README.
5. **Frontend lane** (parallel with 3–4): F-001 foundation → F-002 Docker/infra → F-003…F-009 screens for the shipped endpoints; from T-020 on, screens ship with their backend task.
