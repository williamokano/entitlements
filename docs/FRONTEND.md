# Frontend — Design System & Conventions

The frontend lives in [`admin/`](../admin) and is built on the **Inspinia v5
(React edition)** theme, which is the project's **design system**. Every screen
we ship reuses the theme's components, layouts, and styling — we do not invent
new visual language. The implementation task breakdown for the frontend is the
**F-track** in [`docs/TASKS.md`](./TASKS.md).

## 1. What the theme is (analysis)

Inspinia v5.0.0 React edition (WebAppLayers), a commercial admin template:

| Aspect | Stack |
|---|---|
| Framework | React 19 + TypeScript 5.9, Vite 7 |
| Styling | Tailwind CSS 4 (+ `@tailwindcss/forms`/`typography`), Preline UI 4, theme CSS under `admin/src/assets/css` (12 sidebar/topbar color themes, dark mode, RTL) |
| Routing | react-router 7, lazy-loaded route table in `admin/src/routes/index.tsx` (~230 demo routes) |
| Forms | react-hook-form + yup (`@hookform/resolvers`); rich pickers (flatpickr, react-select, choices.js) |
| Tables | `@tanstack/react-table` via `components/table/DataTable.tsx` (+ `TablePagination`, `DeleteConfirmationModal`); DataTables.net for some demos |
| Charts | ApexCharts + ECharts via `components/wrappers/{ApexChart,EChart}.tsx` |
| Feedback | react-toastify (toasts), sweetalert2 (confirms), ladda (loading buttons) |
| Layouts | `layouts/MainLayout.tsx` → Vertical/Horizontal nav, `TopBar`, `Sidenav`, `Customizer`; menu is **data-driven** via `layouts/components/data.ts` (`menuItems: MenuItemType[]`) |
| Auth pages | `views/auth/{basic,card,split}/` — sign-in, sign-up, reset-pass, new-pass, two-factor, lock-screen, login-pin, success-mail, delete-account (3 visual variants each) |
| Demo apps | api-keys, users (contacts/roles/permissions/role-details), invoice, pricing, ecommerce, clients, companies, chat, email, calendar, file-manager, projects, … |
| Other | landing page (`views/landing`), error pages (`views/error`), `hooks/useAuth.ts` (dummy sessionStorage auth), `config/constants.ts` (branding `META_DATA`) |

**Gaps we must fill** (the adaptation points):

- `useAuth` is a hardcoded dummy — no real API calls anywhere in the theme.
- No runtime configuration mechanism (no env handling, no API base URL).
- No test setup (no vitest/RTL), no Dockerfile, no CI wiring.
- Branding is Inspinia's (`META_DATA`, logos).

**Luckily mapped demo pages** — these are the starting point for our screens:

| Our module / endpoints | Theme page to adapt |
|---|---|
| Authentication (login/register/recovery/verify/2FA-ready) | `views/auth/basic/*` |
| Sessions & password change | `views/admin/pages/account-settings` |
| API keys (`/api/v1/api-keys`) | `views/admin/apps/api-keys` (near 1:1) |
| RBAC roles (`/api/v1/roles`) | `views/admin/apps/users/{roles,role-details,permissions}` |
| Tenant members & invitations | `views/admin/apps/users/contacts` |
| Tenants | `views/admin/apps/companies`, `account-settings` |
| Catalog plans/pricing | `views/admin/pages/pricing`, ecommerce product pages |
| Subscription | `views/admin/pages/pricing` + `account-settings` billing patterns |
| Billing invoices (future) | `views/admin/apps/invoice/{list,details,create}` |

## 2. Structure: real app at `/`, theme demo on the side

We keep **every** theme capability browsable as a living reference, but out of
the way of the real product:

```
admin/src/
  views/
    app/            # OUR screens (new) — built from theme components
      auth/         #   sign-in, sign-up, recovery… (adapted from views/auth/basic)
      dashboard/
      tenant/       #   settings, members, invitations
      api-keys/
      roles/
      catalog/
      subscription/
    admin/  auth/  landing/  error/  others/   # theme demo — UNTOUCHED
  routes/
    index.tsx       # real app routes at "/", demo mounted under "/demo/*"
  lib/              # OUR plumbing (new): api client, auth store, runtime config
  components/       # theme shared components — reused as-is, additions allowed
```

Rules:

- **Never edit** files under the demo views except mechanical route-prefix
  fixes; treat the demo as vendored reference. Copy a demo page into
  `views/app/...` and adapt the copy.
- All demo routes move under the `/demo/*` prefix (one mechanical pass in
  F-001). A build-time flag (`VITE_ENABLE_DEMO`, default on in dev) lets a
  production build drop the demo bundle entirely.
- The sidebar menu for the real app is a new `menuItems` data file; the demo
  keeps its own menu inside `/demo`.
- Branding (`config/constants.ts` `META_DATA`, `AppLogo`/`AuthLogo`) is
  parameterized from runtime config, not hardcoded.

## 3. Runtime configuration (one generic Docker image)

The image must be **built once and configured at deploy time** — backend URL,
tenant-resolution mode, branding, etc. are injected when the container starts,
never baked into the JS bundle.

- `admin/public/app-config.js` defines `window.__APP_CONFIG__`:

  ```js
  window.__APP_CONFIG__ = {
    apiBaseUrl: "http://localhost:8080",   // Go API origin
    tenantMode: "header",                  // "header" | "subdomain"
    tenantSlug: "",                        // fixed tenant when mode=header (else resolved from subdomain)
    appName: "Entitlements",
    enableDemo: false,
  }
  ```

- `index.html` loads it before the bundle; `src/lib/config.ts` exposes a typed
  `appConfig()` accessor with dev fallbacks from `import.meta.env` (`VITE_API_BASE_URL`, …).
- The Docker entrypoint regenerates `app-config.js` from environment variables
  (`API_BASE_URL`, `TENANT_MODE`, `TENANT_SLUG`, `APP_NAME`, …) via a template +
  `envsubst`, then starts nginx. Same image → any environment.
- **Tenant mode** mirrors the backend's `tenant.ResolveMiddleware`:
  - `subdomain`: the SPA is served on `*.example.com`; the backend resolves the
    tenant from the Host header — the client sends requests to the same origin
    (or forwards the host).
  - `header`: the client sends `X-Tenant-ID` explicitly (from config or the
    tenant switcher — the backend header carries the tenant UUID).

## 4. API client & auth conventions

- `src/lib/api.ts` — a single fetch wrapper: prefixes `apiBaseUrl`, sets
  `Content-Type`, attaches `Authorization: Bearer <access>` from the auth store,
  injects the tenant header per tenant mode, and parses **RFC 7807
  problem+json** errors into a typed `ApiError{status, title, detail}` that
  pages render via toasts/inline form errors.
- Auth store (`src/lib/tokens.ts` + `src/lib/auth.ts` + reworked
  `hooks/useAuth.ts`): keeps the access/refresh token pair (localStorage), the
  api client transparently calls `POST /api/v1/auth/refresh` on 401 once
  (rotation-aware — a rejected refresh means hard logout), exposes
  `login/logout/isAuthenticated` (register etc. arrive with F-003). The demo
  keeps the theme's dummy hook as `hooks/useDemoAuth.ts`.
- Idempotency: mutating requests that the backend guards (per T-008) send an
  `Idempotency-Key` header (uuid per logical submit).
- Protected routes: a `RequireAuth` wrapper around the app layout redirects to
  `/auth/sign-in`; auth pages redirect authenticated users to `/`.
- Forms use react-hook-form + yup like the theme's `auth/**/components/Form.tsx`.

## 5. Testing (tests are the contract — frontend too)

- **Unit/component tests**: Vitest + React Testing Library + **MSW** (mock
  service worker) to fake the API at the network layer — no hand-mocked fetch.
  Live next to the code (`*.test.tsx`), run with `npm test` → CI job.
- Every F-task card in `docs/TASKS.md` lists expected tests, same contract as
  backend tasks.
- Gates on every PR touching `admin/` (path-filtered CI job): `npm run lint`,
  `npm run build` (twice — with and without `VITE_ENABLE_DEMO`), `npm test`.
  Note: the vendored theme is neither lint- nor `tsc`-clean, so eslint ignores
  the vendored demo views + a few theme-shared modules (list in
  `admin/eslint.config.js`) and the build is pure `vite build` without a `tsc`
  pass — OUR code must lint clean.
- E2E (Playwright against the real Go API) arrives with the hardening milestone.

## 6. Definition of done — frontend rule

From now on, **a backend task that adds or changes user-facing endpoints is not
done until the corresponding screens exist** (or, when built in parallel, an
F-task card for them is created/updated in the same PR and referenced by the
backend card). The F-track cards for already-shipped endpoints (T-010–T-019)
exist precisely because those tasks predate this rule — they are the catch-up
backlog and can be implemented in parallel with backend work.
