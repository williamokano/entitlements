# Where the built code deviates from the vision — investigation notes

> Status: **raw investigation notes** (2026-07-13). See
> [`skeleton-vision.md`](skeleton-vision.md).

**Core architecture is NOT deviated.** The module layout, global operator-owned
catalog, tenant-subscribes-to-a-plan, billing/dunning, entitlement resolution,
and RBAC already *are* the vision. The items below are things the vision sharpens
that the currently-built code gets wrong or ships extra.

## 1. API keys — drop from the platform
- **Built**: T-014 (auth middleware + tenant API keys, argon2id, scopes,
  `/api/v1/api-keys`) and F-006 (API-keys admin screen).
- **Vision**: not a platform concern. If a vertical wants them, that's the
  `example` module's call.
- **Ripple**: removes the main reason the `machine` / `Authorization: ApiKey`
  principal exists. Reconsider whether the machine principal survives at all (see
  metering, below), or is kept only for future module use.

## 2. Metering is in-process, not an HTTP+key surface
- **Built**: `POST /entitlements/consume`, `/release`, `GET /entitlements/usage`
  as HTTP endpoints behind `requireAuth`.
- **Vision**: entitlements is **embedded**; the example module calls
  `ConsumeQuota` / `CheckAccess` through the port **in-process**. The HTTP
  metering endpoints are redundant (or operator-only for admin/debug).
- Note this is the reason dropping API keys is safe: no external metering caller
  was ever intended.

## 3. No public register on the platform
- **Built**: T-012 ships a public `POST /api/v1/auth/register`.
- **Vision**: operators are seeded; tenant staff arrive by **invitation** (F-005
  flow). Public self-signup, if a vertical wants it, is **application-scoped**
  (example module), not platform.
- Ties into the T-034 / open-questions fork about who creates tenants and how the
  first tenant admin appears.

## 4. Frontend audience is mislabelled
- **Built**: F-008 (catalog admin), F-009 (subscription), F-010 (entitlements
  viewer), F-013 (billing invoices) — all built as **tenant self-service**
  screens behind `RequireTenant` + the tenant switcher.
- **Vision**: these are **operator** screens (catalog/subscription/billing/
  entitlements are operator-owned). The tenant-facing corner of the SPA is just
  membership + account.
- Implication: the admin SPA is really an **operator console** with a small
  tenant-facing area — not a tenant console. Affects how future F-cards are scoped.

## 5. Security gaps already captured (T-032 / T-033 / T-036)
Not new here, but part of the same reconciliation — they were already heading in
the vision's direction:
- **T-036** 🔴 — cross-tenant IDOR: every endpoint trusts client-supplied
  `X-Tenant-ID` with no membership check (proven live). Fix: deny-by-default.
- **T-033** — operator = a **user with platform-scoped RBAC roles** (one
  mechanism, two scopes: platform `tenant_id NULL`, and tenant). Seeded at boot.
- **T-032** — nothing writes a role assignment, so RBAC is unbootstrappable;
  bridge membership→assignment + the seeded operator fixes it.

## Aligned already (no change)
- **Payments** — the user's description ("provider hands back invoice id, items,
  prices, discount; we answer paid/not") **is** the `PaymentProvider` port (T-026)
  + dunning (T-027).
- Modular-monolith + hexagonal + ports-only cross-module access.
- Global, operator-owned catalog; tenant pins a plan version.
