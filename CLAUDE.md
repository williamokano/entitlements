# CLAUDE.md

Guidance for Claude (and any agent) working in this repository.

## What this is

A reusable **skeleton for SaaS products**: a Go modular-monolith backend with
DDD + hexagonal architecture, plus a React SPA frontend in `admin/` built on the
Inspinia v5 theme as design system. Architecture lives in
[`docs/PLAN.md`](docs/PLAN.md); the implementation task breakdown lives in
[`docs/TASKS.md`](docs/TASKS.md) (backend `T-XXX` cards + frontend `F-XXX`
cards); frontend conventions live in [`docs/FRONTEND.md`](docs/FRONTEND.md).

## Workflow

- **One PR per task.** Branch `feat/T-XXX-short-name` (backend) or
  `feat/F-XXX-short-name` (frontend) cut from the latest `main`, open a PR into
  `main`, merge on green. No direct pushes to `main`.
- **Tests are the contract.** Every task card in `docs/TASKS.md` lists the
  expected tests. A task is done only when that suite exists and passes:
  `make test` (unit) and `make test-integration` (testcontainers) for backend;
  `npm run lint`, `npm run build`, `npm test` in `admin/` for frontend. All run
  in CI and must be green before merge.
- **Frontend is part of a task's definition of done.** A backend task that adds
  or changes user-facing endpoints must ship the corresponding `admin/` screens
  in the same PR, **or** create/update a paired F-task card in `docs/TASKS.md`
  (referenced from the backend card) so the screens can be built in parallel.
  Screens follow `docs/FRONTEND.md`: reuse Inspinia components, copy demo pages
  into `admin/src/views/app/` (never edit the demo under `/demo/*`), use the
  shared API client + runtime config.
- **Verify before committing:** `make generate` (regenerate sqlc if a migration
  touched the platform schema — CI gates drift), `make lint`, `make archcheck`,
  `make tidy`, then the full test suites. For anything with a runtime surface,
  also do a live boot check against a real Postgres.

## Tracking is part of every task's definition of done

After each task, before opening its PR, update the tracking docs so they never
drift from the code. This is not optional — treat it as part of the task:

1. **`docs/TASKS.md`** — mark the task ✅ done (with its PR number). If the work
   revealed new/changed/removed follow-up work, update the affected cards.
2. **`README.md`** — update the **Status** section, and the **Try it locally**
   walkthrough when the task adds or changes user-visible behavior: new
   endpoints, new modules, new run/config requirements, or changed defaults.
3. **`docs/PLAN.md`** — update it whenever the task changes architecture, a key
   decision, module scope, or anything else the plan describes. Keep the plan a
   truthful description of the system, not just the original intent.

"If required" means: if the task changed what that document describes, it must
be updated in the same PR. When in doubt, update it. A task whose code is merged
but whose docs still describe the old world is not finished.

## Conventions

- Go 1.24. Modules live under `internal/modules/<name>/` with a hexagonal layout
  (`internal/domain`, `internal/service`, `internal/adapters`, public `ports`).
  Cross-module access is via a module's `ports` package only — the compiler and
  `depguard` enforce this (`make archcheck` proves the linter rejects illegal
  imports).
- The pure `domain` layer may import only the standard library, `uuid`, and
  `platform/apperr`. Crypto and other heavy dependencies live outside it.
- Money is integer minor units + currency code, never floats. Time goes through
  `platform/clock` (UTC); never call `time.Now()` in domain/app code — use
  `clock.Frozen` in time-dependent tests.
- Migrations live in `migrations/<module>/NNNNN_*.sql` and run via the embedded
  runner (and on server startup). The API applies migrations itself; there is no
  separate migrate step for a normal run.
- Frontend: React 19 + TypeScript + Vite in `admin/`. The Inspinia theme demo is
  vendored reference — keep it untouched under the demo routes; real screens are
  adapted copies in `admin/src/views/app/`. All deploy-time configuration
  (API base URL, tenant mode, branding) goes through runtime
  `window.__APP_CONFIG__`, never baked into the bundle. Frontend tests are
  Vitest + React Testing Library + MSW.
- GitHub access: use the GitHub MCP tools (`mcp__github__*`), not the `gh` CLI.
