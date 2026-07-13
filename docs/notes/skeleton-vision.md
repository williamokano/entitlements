# Skeleton vision — investigation notes

> Status: **raw investigation notes**, not decided plan. Dumped from a design
> conversation (2026-07-13) for later work. Nothing here is committed to code yet.
> See also [`deviations-from-vision.md`](deviations-from-vision.md) and
> [`open-questions.md`](open-questions.md).

## What this project actually is

A **kickstarter for building a multi-tenant SaaS** — any vertical: e-commerce,
real estate, food tech, delivery, ride-hailing, gym management, doctor booking.
The vertical doesn't matter; the specific business logic for it lives in the
**`example`/demo module** and gets swapped out wholesale.

The skeleton owns **"the business of running a SaaS."** The example owns **"the
product."** Everything the skeleton ships is the part *every* multi-tenant SaaS
shares regardless of vertical.

## The load-bearing line: platform vs application

Two worlds that look similar and must **not** cross-wire (technically they could
share services; deliberately deferred — "discussion for other things in the
future"):

- **Platform** (the skeleton): the tenant is the **operator's customer**. It
  subscribes to a plan to **use the platform itself**, is metered/limited,
  invoiced, charged, dunned. All operator-scoped.
- **Application** (the example, future/open): the tenant billing **its own**
  customers. May reuse the same shapes one day, but **separate services**.

> "They all will have subscriptions (for using the platform itself), they all
> will have billing, etc. The application COULD also have tenants, subscription
> and billing though, but that's for the future, it's open. I don't want to
> cross share the same services with these things."

## Three identity layers

1. **Operators** — run the platform. Platform-scoped RBAC roles. **Login, not
   public register** (seeded / invited).
2. **Tenant users / staff** — e.g. realtors, tenant admins for a real-estate
   agency. **Platform users**, tenant-scoped via membership + tenant roles.
   Onboarded by **invitation** (this is exactly the F-005 flow).
3. **Application users** — e.g. the "seekers" (a real-estate agency's home
   hunters), gym members, patients, riders. The tenant's **own** end-customers.
   **Application-scoped**, live in the `example` module, **may have their own
   login**. Explicitly **not** platform data / not a platform concern.

> "for a realstate agent, realtors, tenant admins, etc — they are platform users,
> but the seekers they MIGHT have their own login and this is application scoped
> data, not platform data."

## Platform feature set (what the skeleton ships)

All of these are **operator-scoped**, and most should be gated by **authorization
roles**:

- **Shared / cross kernel** — already there, fine as-is.
- **Authz + Authn** — RBAC, login, etc.
- **Tenants** + management.
- **Products / Plans**.
- **Entitlements (features)** — attached to a plan.
- **Metering / Limits (usage quota)** — attached to a plan.
- **Addons** — for metering, or maybe separate features.
- **Subscription** — which plan a tenant is attached to. *(Open: maybe folded
  into billing — see open-questions.)*
- **Platform operators** — login, **no public register**.
- **Billing** — generate invoices, charge, dunning logic; decide whether a
  subscription stays alive; manage subscription lifecycle. *("maybe we don't even
  need subscription per se")*
- **Payments** — external provider: **they** provide an invoice id, invoice items,
  prices, discount, everything; **we** answer whether it was paid or not. (This is
  the existing `PaymentProvider` port, T-026 + dunning T-027.)

## Explicitly dropped

- **API keys** — removed from the platform for now. Whether a module has API keys
  is a **future feature of the module (example)**, not part of the platform /
  "entitlements." Since entitlements is **embedded**, not an entitlements-SaaS,
  the platform doesn't need them. (Retires T-014 + F-006 — see deviations.)

## Why "embedded" matters

Entitlements/metering are **embedded machinery**, called **in-process** by the
example module through the module's `ports` — not a remote API a customer calls.
That is *why* dropping API keys doesn't break metering: there was never meant to
be an external caller for `ConsumeQuota` / `CheckAccess`.
