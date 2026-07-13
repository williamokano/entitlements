# Open questions / forks to decide — investigation notes

> Status: **raw investigation notes** (2026-07-13). These are genuine forks the
> product owner has not settled yet; do not guess them. See
> [`skeleton-vision.md`](skeleton-vision.md).

## 1. Subscription vs billing — one module or two?
- Today: **two** modules. `subscription` is a state machine
  (`trialing → active → past_due → grace → suspended → …`); `billing` drives its
  transitions via events (invoice paid/failed → advance/suspend).
- Floated: **"maybe we don't even need subscription per se"** — billing could
  own subscription lifecycle directly.
- Reasonable either way. Decide before reworking either module.

## 2. How does the first tenant admin appear? (who creates a tenant)
- Vision leans **operator-provisioned**: the operator onboards a customer, then
  the customer's first admin arrives by **invitation** (F-005 flow).
- Conflicts with T-028's acceptance test ("register → create tenant" = self-serve
  signup) and with T-031's current "creator becomes owner."
- Options: operator-provisioned only / self-serve signup / both.
- ⚠️ If operator-provisioned, **T-031's "creator becomes owner" is wrong** for
  that path (an operator creating many tenants must not own them) → owner-binding
  becomes conditional on the creator being a non-operator user, or moves to the
  first-owner invitation.

## 3. Application-user auth — scaffold in `example`, or leave open?
- The third identity layer (seekers / patients / riders) is **application-scoped**
  and may have its own login, living in the `example` module.
- Question: does the skeleton **scaffold** an application-user auth pattern in
  `example` (as a reference), or leave it entirely to the adopter?

## 4. Does the machine principal survive at all?
- With API keys dropped (deviation #1) and metering in-process (deviation #2), the
  `Authorization: ApiKey` / `machine` principal has no remaining platform caller.
- Payments are outbound (a `PaymentProvider` port we call) and webhooks are
  inbound + signature-verified — neither needs the machine principal.
- Keep it for future module use, or remove it from the platform for now?

## 5. Platform vs application services — hard boundary later
- Deferred by the product owner: the tenant's own application may *also* have
  tenants/subscription/billing, but those must **not** cross-share the platform's
  services. When application-side billing is tackled, decide the boundary
  (duplicate the shapes in `example`, vs. a shared library, vs. a generic engine).

## 6. Frontend restructure (consequence of deviation #4)
- If the admin SPA is an **operator console** (not a tenant console), the existing
  F-008/09/10/13 screens need re-framing and the tenant-facing area shrinks to
  membership + account. Decide whether/when to restructure, and whether a separate
  tenant-facing UI is even in scope for the skeleton.
