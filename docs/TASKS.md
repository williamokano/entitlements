# Implementation Task Breakdown

Companion to [`docs/PLAN.md`](./PLAN.md). Each task is sized to be implementable by one person/agent in one focused session, is self-contained (read PLAN.md section referenced + this task card), and lists hard dependencies. Tasks with no dependency edge between them can be built in parallel.

**Tests are the contract.** Every task card ends with an **Expected tests** list: the concrete test suite the implementation must contain. A task is only done when those tests exist and pass. The suite pins the task's behavior so any future rewrite — of a package or a whole module — is validated by running the same tests green. Test names below are the canonical scenarios; implementers may split/rename within reason but every listed scenario must be covered by a real test.

## Conventions (apply to every task)

- **Read first**: `docs/PLAN.md` — especially "Key Decisions", "Repository Layout", and the module spec for the task at hand.
- **Branching & PRs**: one branch per task, named `feat/T-XXX-short-name`, cut from the latest `main`. Every task lands via a **pull request into `main`** — no direct merges to `main`. `main` stays green (CI must pass before merge).
- **Testing model** (from the beginning, not deferred):
  - **Unit tests** live next to the code (`*_test.go`), run with `make test` (`go test -race ./...`). Pure domain logic is tested exhaustively here.
  - **Integration tests** run against a **real Postgres via testcontainers-go**. They live in `*_integration_test.go` files guarded by `//go:build integration`, and run with `make test-integration` (`go test -race -tags integration ./...`). CI runs both on every PR.
  - The shared helper is `internal/platform/testkit` (lands in **T-003**): starts (and caches per test-binary) a `postgres:16` container, applies all migrations, and hands each test an isolated database/DSN. Every DB-touching task uses it — no hand-rolled containers, no mocked SQL.
  - **E2E tests** (T-028) boot the real composition root over testcontainers and drive real HTTP.
- **Definition of done** (every task):
  1. All **Expected tests** listed on the task card are implemented and green (`make test` + `make test-integration`).
  2. `make lint test build` passes; CI green on the PR.
  3. New code follows the hexagonal layout (`domain` / `app` / `ports` / `adapters`) and the import rules: `domain` imports nothing from other layers/modules; modules import other modules only via their `ports` package.
  4. Migrations live in `migrations/<module>/` and run via the migration runner (and therefore via testkit).
  5. Public behavior documented in the package's doc comment (godoc), not a separate wiki.
  6. The task's **Acceptance criteria** are demonstrably met (each criterion maps to at least one expected test or CI gate).
  7. **Frontend**: if the task adds or changes user-facing endpoints, the corresponding screens in `admin/` (built per [`docs/FRONTEND.md`](./FRONTEND.md)) are part of the task — shipped in the same PR, or split into a paired **F-task** card created/updated in the same PR and referenced from the backend card so it can be built in parallel. A backend task whose screens are neither shipped nor carded is not done.
- **Frontend track**: frontend tasks are the **F-cards** (see "Frontend track" below), branch `feat/F-XXX-short-name`, same one-PR-per-task flow. Their CI gates are `npm run lint`, `npm run build` (tsc), and `npm test` (Vitest + React Testing Library + MSW) in `admin/`.
- **Don't gold-plate**: implement exactly the task's scope. "Out of scope" notes are binding.
- **Money**: always integer minor units (cents) + currency code. Never floats.
- **Time**: always through `platform/clock`, UTC. Never `time.Now()` in domain/app code. Time-dependent tests use `clock.Frozen`.
- **Size legend**: S ≈ ≤ half day, M ≈ one day, L ≈ 1–2 days.

## Dependency graph

```mermaid
graph TD
  T001[T-001 bootstrap] --> T002[T-002 config/id/clock]
  T001 --> T004[T-004 httpx]
  T002 --> T003[T-003 postgres+testkit]
  T003 --> T005[T-005 events+outbox]
  T003 --> T006[T-006 jobs]
  T003 --> T007[T-007 audit+observability]
  T004 --> T007
  T003 --> T008[T-008 idempotency middleware]
  T004 --> T008
  T005 --> T009[T-009 wiring + composition root]
  T004 --> T009
  T009 --> T010[T-010 tenant core]
  T010 --> T011[T-011 tenant middleware/authctx]
  T009 --> T012[T-012 auth: users+password+JWT]
  T012 --> T013[T-013 auth: recovery+verification+sessions]
  T012 --> T014[T-014 auth middleware + API keys]
  T010 --> T015[T-015 membership+invitations]
  T012 --> T015
  T014 --> T016[T-016 authorization RBAC]
  T010 --> T016
  T009 --> T017[T-017 catalog: plans+versions]
  T017 --> T018[T-018 catalog: addons]
  T017 --> T019[T-019 subscription: core state machine]
  T019 --> T020[T-020 subscription: plan/addon changes]
  T018 --> T020
  T006 --> T021[T-021 subscription: renewal+trial jobs]
  T019 --> T021
  T017 --> T022[T-022 entitlements: registry+resolution]
  T019 --> T022
  T022 --> T023[T-023 entitlements: overrides+audit]
  T022 --> T024[T-024 entitlements: usage+quotas]
  T019 --> T025[T-025 billing: invoices+snapshots]
  T025 --> T026[T-026 billing: PaymentProvider+fake+charge flow]
  T021 --> T026
  T026 --> T027[T-027 billing: dunning+proration]
  T015 --> T028[T-028 signup flow wiring + E2E]
  T016 --> T028
  T024 --> T028
  T027 --> T028
  T028 --> T029[T-029 seed data + new-SaaS checklist]
```

---

## Milestone 1 — Foundation

### T-001 · Project bootstrap · **S** · ✅ DONE (PR #1)
**Depends on**: nothing.
**Deliverables**: `go.mod` (module `github.com/williamokano/entitlements`, latest stable Go); directory skeleton from PLAN.md "Repository Layout" with `doc.go` stubs; `Makefile` (`build`, `test`, `test-integration`, `lint`, `generate`, `migrate-up/down`, `run`, `tidy`); `docker-compose.yml` (Postgres 16 + healthcheck); `.golangci.yml` (standard linters, permissive `depguard` until T-009); GitHub Actions CI (`make lint test build` + integration tests); README "Development" section; minimal `cmd/api` with `/healthz` + `/readyz` and graceful shutdown.
**Acceptance criteria**: fresh clone → `docker compose up -d && make lint test build` succeeds; CI green; `/healthz` returns 200.
**Expected tests** (implemented):
- `cmd/api`: `TestHealthEndpoints` — `/healthz` and `/readyz` return 200 with `application/json`.
- `cmd/api`: `TestNewLogger` — known and unknown log levels both yield a usable logger.
- CI gates: `make lint`, `make test`, `make build`, `go mod tidy` drift check.

### T-002 · `platform/config`, `platform/id`, `platform/clock` · **S** · ✅ DONE (PR #2)
**Depends on**: T-001.
**Deliverables**: env-based config loader (generic `Parse[T]`, optional `.env`, fail-fast `required`, `Validate()`); UUIDv7 generator behind `id.Generator` + deterministic `Sequence`; `clock.Clock` with `System` (UTC) + concurrency-safe `Frozen` (`Set`/`Advance`). Config wired into `cmd/api`.
**Acceptance criteria**: defaults let `make run` work with zero env; production + default DSN is rejected; UUIDs are v7, unique, time-ordered; frozen clock is deterministic.
**Expected tests** (implemented; retrofit noted):
- `config`: `TestLoadDefaults` — defaults applied when env unset.
- `config`: `TestLoadEnvOverrides` — env vars override defaults; `IsProduction`.
- `config`: `TestValidateRejectsProductionWithDefaultDSN` — fail-fast cross-field validation.
- `config`: `TestParseRequiredFieldFailsFast` — missing `required` var errors; present value binds.
- `config`: `TestParseLoadsDotEnvFile` — values in a `.env` file are loaded; real env still wins over `.env`. *(retrofit added alongside this document)*
- `id`: `TestUUIDv7UniqueAndOrdered` — 1000 ids: unique, version 7, byte-ordered.
- `id`: `TestSequenceDeterministic` — two fresh sequences produce identical streams; nth value encodes n.
- `id`: `TestSequenceConcurrentUnique` — parallel `New()` never duplicates.
- `clock`: `TestSystemNowIsUTC`, `TestFrozenSetAndNormalizesToUTC`, `TestFrozenAdvance`, `TestFrozenSet`.

### T-003 · `platform/postgres` + `platform/testkit` — pool, UnitOfWork, migrations, sqlc, testcontainers · **L**
**Depends on**: T-002.
**Deliverables**: pgx/v5 pool from config; `UnitOfWork.Do(ctx, fn)` with the tx traveling in `ctx` so repositories join ambient transactions (nested `Do` joins the outer tx); goose migration runner over embedded per-module FS (`migrations/<module>/`); migration `000` creating per-module schemas; `sqlc.yaml` + `make generate` producing compiling code; **`internal/platform/testkit`**: `testkit.Postgres(t)` starts a `postgres:16` testcontainer once per test binary, applies all migrations, returns an isolated DSN/pool per test (unique database or schema per test for parallelism), with clear failure when Docker is absent.
**Acceptance criteria**: migrations apply cleanly on a fresh container; a failed `Do` rolls back every write inside it; nested `Do` joins (inner writes vanish when outer fails); testkit is the single entry point all later integration tests use.
**Expected tests**:
- `testkit` (integration): `TestPostgresStartsAndMigrates` — container boots, all migrations applied, connection usable.
- `testkit` (integration): `TestPostgresIsolationBetweenTests` — two tests get isolated databases/schemas; writes don't leak.
- `postgres` (integration): `TestUnitOfWorkCommits` — writes inside `Do` visible after it returns nil.
- `postgres` (integration): `TestUnitOfWorkRollsBackOnError` — returned error rolls back all writes.
- `postgres` (integration): `TestUnitOfWorkRollsBackOnPanic` — panic inside `Do` rolls back and re-panics.
- `postgres` (integration): `TestNestedDoJoinsOuterTransaction` — inner `Do` writes roll back when the outer fails.
- `postgres` (integration): `TestQueryOutsideDoRunsNonTransactional` — repositories work without an ambient tx.
- `postgres` (integration): `TestMigrateDownReverts` — down migration reverts the last version.
- CI gate: `make generate` output compiles and `git diff --exit-code` shows no drift.

### T-004 · `platform/httpx` — server, middleware, errors · **M** · ✅ DONE (PR #5)
**Depends on**: T-001. Also adds `internal/platform/apperr` (error taxonomy, net/http-free so the domain can return typed errors).
**Deliverables**: `net/http` server with graceful shutdown; middleware: request-ID (accept inbound or generate), panic recovery, request logging; RFC 7807 `problem+json` mapping from typed app errors (`NotFound`, `Conflict`, `Validation`, `Unauthorized`, `Forbidden`); `/healthz` + `/readyz`; router composition helper for mounting module handlers under prefixes.
**Acceptance criteria**: every response carries a request ID; a panicking handler yields a 500 problem+json (no stack leak) and the server keeps serving; each typed error maps to its status.
**Expected tests** (all `httptest`, unit):
- `TestRequestIDGeneratedWhenAbsent` / `TestRequestIDEchoedWhenPresent` — header round-trip, present in response and logs.
- `TestRecoveryReturns500ProblemJSON` — panic → 500, `application/problem+json`, request ID included, stack trace NOT in body; subsequent request still succeeds.
- `TestErrorMapperTable` — table test: NotFound→404, Validation→400, Conflict→409, Unauthorized→401, Forbidden→403, unknown error→500 (opaque message, no internals leaked); all bodies are valid RFC 7807.
- `TestRequestLoggingIncludesMethodPathStatusRequestID` — via a captured slog handler.
- `TestRouterCompositionMountsUnderPrefix` — module handler mounted at `/api/v1/x` receives stripped/full path as designed.
- `TestGracefulShutdownDrainsInflightRequests` — in-flight request completes during `Shutdown`.

### T-005 · `platform/events` — bus, transactional outbox, idempotent consumers · **L** ← the backbone · ✅ DONE (PR #6)
**Depends on**: T-003.
**Deliverables** (see PLAN.md "Outbox scope note"): `Event` envelope (uuidv7 ID, occurred_at, tenant_id, module, type, JSON payload); `Bus.Subscribe(eventType, handler)`; `Append(ctx, evt)` writing to `platform.outbox` inside the ambient UnitOfWork tx; relay worker polling `FOR UPDATE SKIP LOCKED` in batches, dispatching, marking `published_at`, incrementing `attempts` with backoff on failure; `platform.processed_events` + wrapper making any handler exactly-once-effective; synchronous-dispatch test helper for unit tests of consumers.
**Acceptance criteria**: an event is atomically coupled to its business transaction; delivery is at-least-once and survives relay crashes; wrapped consumers are exactly-once-effective; two relays never double-dispatch.
**Expected tests**:
- unit: `TestSubscribeDispatchesToMatchingHandlers` — type routing, multiple subscribers.
- unit: `TestEventEnvelopeValidation` — missing type/payload rejected at Append.
- integration: `TestAppendInvisibleAfterRollback` — business tx rolls back → no outbox row.
- integration: `TestAppendPublishedAfterCommit` — commit → relay dispatches to subscriber.
- integration: `TestRelayRedeliversUnackedAfterRestart` — kill relay after fetch/before ack; new relay redelivers.
- integration: `TestHandlerFailureIncrementsAttemptsWithBackoff` — failing handler: attempts++, retried later, eventually delivered.
- integration: `TestIdempotentWrapperDropsDuplicateDelivery` — same event delivered twice → handler effect exactly once; `processed_events` row present.
- integration: `TestTwoRelaysNoDoubleDispatch` — two concurrent relays over one outbox: each event dispatched once (SKIP LOCKED).
- integration: `TestEventsDispatchedInPerTenantOrder` — ordering guarantee (at minimum: stable ordering by occurred_at within a tenant) documented and tested.

### T-006 · `platform/jobs` — scheduler + locked runner · **M** · ✅ DONE (PR #7)
**Depends on**: T-003.
**Deliverables**: `jobs.Register(name, interval, fn)`; runner using Postgres advisory locks so exactly one instance executes a job across replicas; per-run timeout; panic isolation; last-run bookkeeping table; clock-driven so tests can use `Frozen`.
**Acceptance criteria**: with N runners on one database, each due job executes exactly once per tick; a panicking or hanging job neither kills the runner nor blocks other jobs.
**Expected tests**:
- integration: `TestTwoRunnersSingleExecutionPerTick` — two runners, one DB: counter increments exactly once per tick.
- integration: `TestJobPanicIsolatedAndRunnerContinues` — panicking job recovered; other jobs and next ticks unaffected.
- integration: `TestPerRunTimeoutCancelsJobContext` — job observing ctx.Done() is canceled at the deadline; recorded as failed run.
- integration: `TestLastRunBookkeepingRecorded` — name, started_at, finished_at, status persisted.
- integration: `TestLockReleasedAfterCrashAllowsNextRun` — advisory lock released on connection drop; job runs again.
- unit: `TestRegisterRejectsDuplicateJobNames`.

### T-007 · `platform/audit` + observability baseline · **M** · ✅ DONE (PR #8)
**Depends on**: T-003, T-004. Also adds `internal/platform/observability` and minimal `authctx` tenant-context helpers (T-011 extends authctx).
**Deliverables**: `audit.Writer.Record(ctx, Entry{Actor, TenantID, Action, Resource, Before, After, Reason})` persisting to `platform.audit_log` (append-only) within the ambient tx; slog JSON handler with request/tenant/trace IDs auto-attached from context; OpenTelemetry Tracer/Meter wiring (no-op exporters by default, OTLP via config); HTTP middleware creating a span per request.
**Acceptance criteria**: audit entries commit/roll back with the business tx; every request produces one structured log line and one span carrying the request ID.
**Expected tests**:
- integration: `TestAuditEntryVisibleOnlyAfterCommit` — entry written in a rolled-back tx never appears.
- integration: `TestAuditEntryPersistsAllFields` — actor/tenant/action/resource/before/after/reason round-trip (JSONB).
- integration: `TestAuditLogIsAppendOnly` — updates/deletes rejected (trigger or revoked grants).
- unit: `TestSlogAttachesRequestAndTenantIDsFromContext` — captured handler shows both attrs.
- unit: `TestHTTPMiddlewareCreatesSpanPerRequest` — in-memory span exporter records one span with route + status.
- unit: `TestUnknownLogLevelFallsBackToInfo`.

### T-008 · `platform/httpx` idempotency-key middleware · **S** · ✅ DONE (PR #9)
**Depends on**: T-003, T-004.
**Deliverables**: middleware honoring `Idempotency-Key` on mutating methods: first call stores response (status + body hash) in `platform.idempotency_keys` keyed by (tenant, key, route) with TTL; replay returns the stored response without re-executing the handler; concurrent duplicate gets 409.
**Acceptance criteria**: at-most-once handler execution per (tenant, key, route) within the TTL; GETs unaffected.
**Expected tests** (integration unless noted):
- `TestDuplicatePOSTReturnsStoredResponseHandlerRunsOnce` — same key twice: identical status/body, handler counter == 1.
- `TestConcurrentDuplicateReturns409` — two simultaneous requests, same key: one wins, other 409.
- `TestDifferentKeyExecutesHandlerAgain`.
- `TestKeyScopedByTenantAndRoute` — same key, different tenant or route → both execute.
- `TestExpiredKeyReexecutes` — frozen clock past TTL → handler runs again.
- unit: `TestGETRequestsBypassIdempotency`.

### T-009 · Module wiring pattern + composition root + arch enforcement · **M** · ✅ DONE (PR #10)
**Depends on**: T-004, T-005. Module boundaries are compiler-enforced via Go `internal/` (domain/service/adapters private; only `ports` public); depguard enforces domain purity + platform isolation; `scripts/archcheck.sh` self-tests the guard.
**Deliverables**: `Module` contract — `module.go` exposes `Wire(Deps) (Ports, http.Handler, []events.Subscription)`; `cmd/api/main.go` composition root (config → postgres → migrations → bus/relay → jobs → mount modules → serve); a minimal `example` module (one entity, one endpoint, one event) proving the pattern end-to-end, kept as living documentation; finalize `depguard` rules from PLAN.md "Enforced rules"; `scripts/archcheck.sh` self-test that verifies the lint actually rejects illegal imports.
**Acceptance criteria**: `make run` boots the full composition root; the example module works over real HTTP + outbox; an illegal cross-module import fails `make lint` (proven by the self-test, not by trust).
**Expected tests**:
- integration: `TestCompositionRootBootsAndServesHealthz` — start the wired server (testkit DB), `/healthz` 200, clean shutdown.
- integration: `TestExampleModuleEndpointPersistsAndResponds` — POST → 201 → row exists (via its repo).
- integration: `TestExampleModuleEventFlowsThroughOutboxToConsumer` — the example event reaches its subscriber exactly once.
- CI gate: `scripts/archcheck.sh` — writes a temp file importing another module's `domain`, asserts `make lint` fails, removes it; wired as a CI step.
- CI gate: depguard active (no longer permissive) and green on the real tree.

---

## Milestone 2 — Identity core

### T-010 · Tenant module core · **M** · ✅ DONE (PR #11)
**Depends on**: T-009. **Spec**: PLAN.md §1. First real business module — the template for the rest (built alongside the `example` reference).
**Deliverables**: `Tenant` aggregate (uuid, slug, name, status `active|suspended|deleted`, `settings JSONB`); create/update/suspend/soft-delete use cases; slug uniqueness + normalization; provisioning pipeline (ordered hook registry on `TenantCreated`; ship a logging no-op hook); publishes `TenantCreated/Suspended/Deleted`; REST CRUD under `/api/v1/tenants`; `ports.TenantReader` (GetByID/GetBySlug, status).
**Acceptance criteria**: lifecycle transitions guarded; soft-deleted tenants invisible to normal reads; provisioning hooks run in registration order after commit; every state change emits its event via the outbox.
**Expected tests**:
- unit: `TestTenantLifecycleTransitionTable` — exhaustive: allowed (active→suspended, suspended→active, active/suspended→deleted) and denied (deleted→anything) transitions.
- unit: `TestSlugNormalizationAndValidation` — case-folding, allowed charset, length bounds; invalid slugs rejected.
- integration: `TestCreateTenantPersistsAndEmitsTenantCreated` — row + outbox event with tenant ID.
- integration: `TestSlugUniquenessReturnsConflict` — duplicate slug → typed Conflict → HTTP 409.
- integration: `TestProvisioningHooksRunInOrderAfterCommit` — two recording hooks; order preserved; not run on rollback.
- integration: `TestSoftDeleteExcludesTenantFromReads` — GetByID/GetBySlug return NotFound; row still present with status deleted.
- integration: `TestSettingsJSONBRoundTrip` — arbitrary nested settings survive write/read.
- integration (HTTP): `TestTenantCRUDEndpoints` — 201 create, 200 get/update, 409 dup slug, 404 missing, suspend/delete paths.

### T-011 · Tenant resolution middleware + `platform/authctx` · **S** · ✅ DONE (PR #12)
**Depends on**: T-010.
**Deliverables**: `authctx` carrying `Principal` and `TenantID`; middleware resolving tenant from (in order) JWT claim → `X-Tenant-ID` header → subdomain, validating via `ports.TenantReader` (must be `active`); `authctx.MustTenant(ctx)` guard used by all tenant-scoped repos; explicit `WithSystemContext` escape hatch.
**Acceptance criteria**: precedence order is exactly claim > header > subdomain; non-active tenants are rejected; tenant-scoped repositories hard-fail without a tenant in context.
**Expected tests** (httptest matrix, unit + integration for the reader):
- `TestTenantResolutionPrecedenceClaimOverHeaderOverSubdomain` — all three present → claim wins; pairwise cases.
- `TestTenantFromHeader`, `TestTenantFromSubdomain` — each source alone resolves.
- `TestSuspendedTenantRejected403`, `TestDeletedOrUnknownTenantRejected` (404/403 as designed).
- `TestMissingTenantOnScopedRouteReturns400`.
- `TestMustTenantFailsWithoutTenant` — returns/panics with a typed error, never a zero tenant ID.
- `TestWithSystemContextBypassesTenantGuard` — and is rejected on normal request paths (guard against leaks).

### T-012 · Authentication: users, password factor, JWT + refresh rotation · **L** · ✅ DONE (PR #13)
**Depends on**: T-009. **Spec**: PLAN.md §2.
**Deliverables**: global `User` (id, email unique, status) with credentials as **factors** (`password` first; table shaped for `totp`/`webauthn`); argon2id hashing; register + login use cases; JWT access (short TTL, `kid` header) + opaque refresh tokens (hashed at rest) with rotation and family-reuse detection (reuse → revoke family); logout; REST under `/api/v1/auth`; publishes `UserRegistered`, `LoginSucceeded/Failed`; rate-limit hook interface (in-memory default).
**Acceptance criteria**: passwords never stored or logged in clear; refresh rotation invalidates the old token; reuse of a rotated token kills the whole family; JWTs verify offline with the published key.
**Expected tests**:
- unit: `TestArgon2idHashVerifyAndRejectsWrongPassword` — hash≠plaintext, verify roundtrip, tampered hash fails.
- unit: `TestJWTSignedWithKidAndVerifies` — claims (sub, exp, iat), expiry rejected, wrong key rejected.
- unit: `TestRefreshRotationInvalidatesPreviousToken`.
- unit: `TestRefreshReuseRevokesEntireFamily` — old token reuse → family revoked, active descendant no longer works.
- integration (HTTP): `TestRegisterLoginRefreshLogoutFlow` — full happy path with real DB.
- integration: `TestRegisterDuplicateEmailConflict409`.
- integration: `TestLoginWrongPassword401AndLoginFailedEventEmitted` — constant-shape error (no user enumeration).
- integration: `TestRefreshTokensHashedAtRest` — raw token value absent from DB.
- integration: `TestRateLimitHookInvokedOnLogin` — recording hook sees attempts.

### T-013 · Authentication: email verification, recovery, session management · **M** · ✅ DONE (PR #14)
**Depends on**: T-012.
**Deliverables**: `EmailSender` port + dev adapter (logs the link); email verification (single-use expiring token); password recovery (request → token → reset, invalidates all sessions); password change (re-auth required); session listing + revoke-others; publishes `PasswordChanged/Recovered`; audit entries via `platform/audit`.
**Acceptance criteria**: all tokens single-use and expiring; recovery/reset revokes every live session; flows never reveal whether an email exists.
**Expected tests** (integration unless noted):
- `TestEmailVerificationFlow` — token verifies once; second use rejected; expired token rejected (frozen clock).
- `TestRecoveryFlowResetsPasswordAndRevokesAllSessions` — old password fails, old refresh tokens dead.
- `TestRecoveryRequestForUnknownEmailIndistinguishable` — same response shape/status either way.
- `TestPasswordChangeRequiresCurrentPassword` — wrong current password → 401/403; success emits `PasswordChanged` + audit entry.
- `TestSessionListAndRevokeOthersKeepsCurrent` — N sessions → revoke-others → only current refresh works.
- unit: `TestEmailSenderPortReceivesRenderedTokenLink` — recording sender.

### T-014 · Auth middleware + API keys (machine auth) · **M** · ✅ DONE (PR #17)
**Depends on**: T-012.
**Deliverables**: HTTP middleware validating `Authorization: Bearer` JWT → `Principal{Kind: user}`; API keys per tenant (prefix + argon2id-hashed secret shown once, scopes, `last_used_at`, revocation); middleware branch for `Authorization: ApiKey …` → `Principal{Kind: machine, Scopes}`; REST key management.
**Acceptance criteria**: both credential kinds populate `authctx` uniformly; key secrets irrecoverable after creation; revocation is immediate.
**Expected tests**:
- unit: `TestBearerTokenMatrix` — valid → principal set; expired/garbage/absent → 401; wrong signature → 401.
- integration: `TestAPIKeyAuthSetsMachinePrincipalWithScopes`.
- integration: `TestRevokedAPIKeyRejected401`.
- integration: `TestAPIKeySecretShownOnceAndHashedAtRest` — create response contains secret; DB stores hash; GET never returns it.
- integration: `TestAPIKeyLastUsedAtUpdatedOnUse`.
- integration (HTTP): `TestAPIKeyCRUDEndpoints` — create/list/revoke, tenant-scoped.

### T-015 · Tenant membership + invitations · **M** · ✅ DONE (PR #18)
**Depends on**: T-010, T-012. **Spec**: PLAN.md §1.
**Deliverables**: membership (user↔tenant, role reference, status); invitations by email (existing or future user) with accept/decline, expiry, resend; publishes `MemberJoined/Left`, `InvitationSent`; REST under `/api/v1/tenants/{id}/members|invitations`; `ports.MembershipReader`.
**Acceptance criteria**: a user can belong to multiple tenants; invitations expire; accepting an invite for a not-yet-registered email works after registration.
**Expected tests** (integration):
- `TestInviteNewEmailThenRegisterThenAcceptBecomesMember` — the deferred-user path.
- `TestInviteExistingUserAcceptFlow` + `MemberJoined` event emitted.
- `TestInvitationExpiryRejected` (frozen clock) and `TestResendExtendsExpiry`.
- `TestDeclineInvitationLeavesNoMembership`.
- `TestDuplicatePendingInvitationConflict`.
- `TestUserInMultipleTenantsIndependentMemberships`.
- `TestRemoveMemberEmitsMemberLeftAndRevokesAccess`.
- `TestMembershipReaderPortReturnsRoleRef`.

### T-016 · Authorization module (dynamic RBAC) · **M** · ✅ DONE (PR #19)
**Depends on**: T-010, T-014. **Spec**: PLAN.md §3.
**Follow-up**: the tenant creator is not yet auto-assigned the `owner` role
(tenant creation does not capture a creating user), so the first role assignment
must be bootstrapped out of band. Wire owner-on-create (or bridge
`tenant.member.joined` role names → assignments) in a later task.
**Deliverables**: `Role` (tenant-scoped, name, permissions `[]string` as `resource:action`, `system bool`); seed `owner/admin/member` via a provisioning hook; role CRUD (system roles immutable); assign/unassign to members; `ports.Authorizer.Check(ctx, permission)` + `RequirePermission(perm)` middleware with `resource:*` wildcard; replaceable behind port.
**Acceptance criteria**: roles are data (create at runtime, no deploy); same user can hold different roles in different tenants; system roles undeletable; permission checks enforce at the HTTP layer.
**Expected tests**:
- unit: `TestPermissionMatchingTable` — exact match, `resource:*` wildcard, no cross-resource leak, unknown permission denied.
- integration: `TestSystemRolesSeededOnTenantProvisioning` — new tenant has owner/admin/member.
- integration: `TestSystemRoleDeleteOrEditRejected409`.
- integration: `TestCustomRoleCreatedAtRuntimeGrantsAccess` — create role → assign → protected endpoint 200.
- integration: `TestUnassignRevokesAccess403`.
- integration: `TestSameUserDifferentRolesPerTenant` — admin in tenant A, member in tenant B; checks differ accordingly.
- integration: `TestRequirePermissionMiddlewareDenies403WithProblemJSON`.

---

## Milestone 3 — Product core

### T-017 · Catalog: plans, versions, pricing · **L** · ✅ DONE (PR #20)
**Depends on**: T-009. **Spec**: PLAN.md §4.
**Deliverables**: `Plan` + immutable `PlanVersion` (publish = new version; subscriptions pin versions); lifecycle `draft→active→archived`, public/hidden; per version: prices per billing cycle (`monthly|annual|custom`) in minor units + currency, trial config (enabled, days, card_required), grace days, feature grants (`feature_key → value`, free-form keys); REST admin CRUD + public "list active public plans"; `ports.CatalogReader.GetPlanVersion(id)`; publishes `PlanVersionPublished/PlanArchived`.
**Acceptance criteria**: a published version can never change; new versions never mutate what existing subscribers pinned; public listing hides drafts/hidden plans; prices only in integer minor units.
**Expected tests**:
- unit: `TestPublishFreezesVersionMutationsRejected` — edit on published version → typed error.
- unit: `TestPlanLifecycleTransitionTable` — draft→active→archived; illegal moves rejected.
- unit: `TestPriceValidationMinorUnitsAndCurrency` — negative/float-ish inputs rejected; currency required.
- unit: `TestTrialAndGraceConfigValidation` — negative days rejected; trial off ⇒ days ignored.
- integration: `TestPlanCRUDAndPublishCreatesNewImmutableVersion` — v1 pinned data intact after v2 published.
- integration: `TestPublicListingExcludesHiddenAndDraft`.
- integration: `TestGetPlanVersionPortReturnsFrozenSnapshot`.
- integration: `TestPlanVersionPublishedEventEmitted`.

### T-018 · Catalog: addons · **M** · ✅ DONE (PR #21)
**Depends on**: T-017. **Spec**: PLAN.md §4.
**Deliverables**: `Addon` with own pricing per cycle, compatible-plan list, entitlement deltas (`feature_key → limit_delta | value override`), quantity-allowed flag; versioned like plans; REST CRUD; exposed via `ports.CatalogReader`; compatibility helper other modules reuse.
**Acceptance criteria**: addons declare compatibility and deltas; incompatible pairings are rejected by the shared helper; addon versions immutable once published.
**Expected tests**:
- unit: `TestAddonCompatibilityHelperAcceptsAndRejects` — compatible plan ok; incompatible → typed error.
- unit: `TestAddonDeltaValidation` — delta on limit-type key; value override; malformed rejected.
- unit: `TestQuantityFlagEnforcedAtValidation` — quantity >1 rejected when flag off.
- integration: `TestAddonCRUDAndVersionImmutability`.
- integration: `TestAddonExposedViaCatalogReaderWithDeltas`.

### T-019 · Subscription: core state machine · **L** · ✅ DONE (PR #22)
**Depends on**: T-017. **Spec**: PLAN.md §5.
**Note**: a voluntary `paused` state was added (distinct from dunning-driven
`suspended`) so pause/resume have a target; pause is legal only from `active`.
**Deliverables**: `Subscription` aggregate (tenant_id — one active per tenant, pinned plan_version_id, billing cycle); explicit transition table for `trialing→active→past_due→grace→suspended→canceled|expired`; transition history (from, to, reason, actor, at); period tracking via `clock`; use cases: create (trial or direct per plan config), cancel (immediate | at period end), reactivate, pause/resume; events for every transition; REST under `/api/v1/subscription`; `ports.SubscriptionReader`.
**Out of scope**: renewals (T-021), plan changes (T-020), billing calls.
**Acceptance criteria**: every allowed/denied transition encoded in one table and tested exhaustively; every transition is historied and emits an event atomically; a tenant cannot hold two active subscriptions.
**Expected tests**:
- unit: `TestTransitionMatrixExhaustive` — table-driven over ALL (state × event) pairs; illegal → typed error, state unchanged.
- unit: `TestCreateWithTrialSetsTrialingAndPeriodFromPlanConfig` (frozen clock).
- unit: `TestCreateWithoutTrialGoesActive`.
- unit: `TestCancelAtPeriodEndKeepsActiveUntilBoundary` vs `TestCancelImmediateTransitionsNow`.
- unit: `TestPauseResumeRoundTrip`.
- integration: `TestEveryTransitionRecordsHistoryAndOutboxEventAtomically` — rollback leaves neither.
- integration: `TestSecondActiveSubscriptionForTenantRejected409`.
- integration (HTTP): `TestSubscriptionEndpointsCreateGetCancel`.

### T-020 · Subscription: plan changes + addon attach/detach · **M** · ✅ DONE (PR #24)
**Depends on**: T-018, T-019.
**Deliverables**: upgrade (immediate re-pin, `SubscriptionPlanChanged` with old/new refs); downgrade as scheduled change applied at period end; cancel-scheduled-change; addon attach/detach with quantity, compatibility via catalog port; `SubscriptionAddonChanged` events. Proration not computed here — events carry data for billing.
**Note (implemented)**: upgrade-vs-downgrade is decided by comparing monthly-equivalent prices (annual ÷ 12); equal-or-higher applies immediately, lower is scheduled. `Module.ApplyScheduledChange` is the period-boundary hook T-021 calls. Frontend: covered by the F-009 card (extended in this PR).
**Acceptance criteria**: upgrades take effect immediately; downgrades change nothing until applied; scheduled changes are visible and cancelable; incompatible addons rejected.
**Expected tests**:
- unit: `TestUpgradeRepinsImmediatelyAndEmitsOldNewRefs`.
- unit: `TestDowngradeStoredAsScheduledChangeNoImmediateEffect`.
- unit: `TestCancelScheduledChangeRestoresNoPendingState`.
- unit: `TestAddonAttachRejectsIncompatiblePlan` + `TestAddonQuantityRules`.
- integration: `TestScheduledChangePersistedAndVisibleViaREST`.
- integration: `TestApplyScheduledChangeAtBoundaryRepinsAndEmits` (invoked via the hook T-021 will call).
- integration: `TestAddonAttachDetachEmitsSubscriptionAddonChanged`.

### T-021 · Subscription: renewal + trial jobs · **M** · ✅ DONE (PR #26)
**Depends on**: T-006, T-019 (T-020 for scheduled-change application).
**Note (implemented)**: exactly-once emission is gated by a `renewal_emitted_period_end` marker on the subscription (not by advisory state), so duplicate ticks and multiple runners still yield one `subscription.renewal_due` per period. `BILLING_DISABLED` (default `true` until T-026) makes the renewal auto-advance the period; when billing is enabled the period advances only when the module's idempotent `billing.invoice_paid` consumer fires. Trial resolution keys off the plan version's `card_required`: no card → convert to active (starting the first paid period at the trial end); card required → expire (no payment method on file yet). `SUBSCRIPTION_TRIAL_ENDING_DAYS` (default 3) sets the pre-expiry `TrialEnding` lead time. Frontend: none (no new user-facing endpoints; renewal/trial are background jobs surfaced through the existing subscription view — F-009).
**Deliverables**: recurring job scanning due subscriptions: emits `SubscriptionRenewalDue`, applies scheduled changes at rollover, advances period only after `InvoicePaid` (config flag `billing.disabled=true` auto-advances until T-026); trial job: `TrialEnding` (configurable days before), `TrialEnded` → convert or expire per plan config.
**Acceptance criteria**: renewal emission is exactly-once per period even with duplicate ticks/replicas; trial events fire at the right frozen-clock moments; the billing-disabled path keeps the module testable standalone.
**Expected tests** (integration, frozen clock):
- `TestRenewalDueEmittedExactlyOncePerPeriod` — duplicate job ticks + idempotent consumer ⇒ one event.
- `TestTwoRunnersOneRenewalEmission` — combined with jobs advisory lock.
- `TestScheduledDowngradeAppliedAtRollover`.
- `TestBillingDisabledAutoAdvancesPeriod` vs `TestPeriodNotAdvancedUntilInvoicePaidWhenBillingEnabled`.
- `TestTrialEndingEmittedConfiguredDaysBefore`.
- `TestTrialEndedConvertsToActiveOrExpiresPerPlanConfig` — both branches.

### T-022 · Entitlements: feature registry + resolution pipeline · **L** ← core of the product
**Depends on**: T-017, T-019. **Spec**: PLAN.md §6.
**Deliverables**: feature registry CRUD (`key` — the stable external identifier, the analog of Stripe's `lookup_key`; `type: boolean|limit|config|enum`, default, description, `limit_behavior: soft|hard`, `reset_period`, arbitrary `metadata` JSONB, and an `active` flag so features are **archived, not deleted**); resolution pipeline plan grants → addon deltas (×quantity) → tenant overrides ⇒ effective set; materialized `effective_entitlements` rebuilt by idempotent consumers of subscription/catalog/override events; on every change to a tenant's effective set, publish `EntitlementsSummaryChanged` carrying the full re-resolved set (the analog of Stripe's `active_entitlement_summary.updated`, and the phase-2 webhook feed); unknown-feature policy (`default_deny|default_allow`) from config; runtime REST `GET /entitlements`, `GET /entitlements/{key}`; `ports.EntitlementsReader.Get`.
**Out of scope**: overrides CRUD (T-023), usage/consume (T-024).
**Acceptance criteria**: precedence is exactly plan < addon < override; the canonical "plan 10 + addon 10 ⇒ 20" case holds; materialization converges after any relevant event; unknown keys resolve per policy; archived features stop granting but keep existing rows/audit coherent; a change to a tenant's effective set emits exactly one `EntitlementsSummaryChanged` with the full set.
**Stripe parity note**: `key`≈`lookup_key`, feature `metadata`, archive-not-delete, and the summary event bring the module to full parity with Stripe Billing Entitlements; the numeric/limit/override capabilities below are beyond what Stripe Entitlements offers (see PLAN.md §6 benchmark).
**Expected tests**:
- unit: `TestResolutionPrecedenceTable` — table-driven: plan only; plan+addon; plan+override; all three (override wins); boolean/config/enum types.
- unit: `TestAddonLimitDeltaTimesQuantity` — the 10 + (addon ×1, ×2) cases; **explicitly asserts 10→20**.
- unit: `TestUnknownFeaturePolicyDenyAndAllow`.
- unit: `TestFeatureTypeValidation` — delta on boolean rejected; enum values constrained.
- integration: `TestMaterializedSetRebuiltOnSubscriptionPlanChanged` — event → new effective values readable.
- integration: `TestMaterializationIdempotentUnderDuplicateEvents`.
- integration: `TestFeatureRegistryCRUDNewFeatureNeedsNoDeploy` — insert feature row → appears in resolution with default.
- integration: `TestArchivedFeatureStopsGrantingButKeepsHistory` — archived (`active=false`) feature drops out of resolution; existing effective rows/audit remain.
- integration: `TestEntitlementsSummaryChangedEmittedOnEffectiveSetChange` — a relevant event → exactly one `EntitlementsSummaryChanged` carrying the full re-resolved set; no-op change emits nothing.
- integration (HTTP): `TestGetAllEntitlementsSingleCall` + `TestGetSingleEntitlement`.

### T-023 · Entitlements: tenant overrides + audit · **M**
**Depends on**: T-022.
**Deliverables**: override CRUD (feature_key, value or delta, optional `expires_at`, mandatory reason + actor) via admin REST; expiry job re-resolving affected tenants; every change audited; override changes trigger re-materialization.
**Acceptance criteria**: overrides win over plan+addons; expire on time; are fully audited (who/what/when/why); removal restores the pre-override value.
**Expected tests** (integration unless noted):
- `TestOverrideBoostsEffectiveValueImmediately`.
- `TestOverrideWithoutReasonOrActorRejected` (unit).
- `TestOverrideExpiryJobRevertsValue` (frozen clock).
- `TestOverrideDeleteRestoresPlanAddonValue`.
- `TestOverrideChangesFullyAudited` — create/update/delete each produce an audit entry with reason + actor.
- `TestTimeBoundOverrideVisibleInEntitlementsResponse` — expiry surfaced to clients.

### T-024 · Entitlements: usage tracking + quota enforcement · **M**
**Depends on**: T-022.
**Deliverables**: usage counters per (tenant, feature, period); `ConsumeQuota(ctx, key, n)` — single-statement atomic check-and-increment honoring hard limits (typed `QuotaExceeded`) and soft limits (consume + `EntitlementLimitWarning` event); `ReleaseQuota`, `GetUsage`; lazy period reset per feature `reset_period`; downgrade grace: shrunk limit below usage ⇒ `EntitlementExceeded` event, never blocks reads; REST consume/usage endpoints.
**Acceptance criteria**: hard limits are never exceeded even under maximal concurrency; soft limits warn exactly once per crossing; period reset needs no background job; downgrade never breaks a tenant.
**Expected tests**:
- integration: `TestConsumeQuotaConcurrencyNeverExceedsHardLimit` — N goroutines racing over limit L: exactly L consumed, rest get `QuotaExceeded`.
- integration: `TestSoftLimitConsumesAndWarnsOncePerCrossing`.
- integration: `TestReleaseQuotaFreesCapacity`.
- integration: `TestLazyPeriodResetOnFirstAccessAfterBoundary` (frozen clock; `billing_cycle` and `monthly`).
- integration: `TestDowngradeBelowUsageEmitsExceededAndKeepsServing` — reads still succeed; event emitted once.
- unit: `TestConsumeValidation` — n<=0 rejected; unknown feature per policy.
- integration (HTTP): `TestConsumeEndpointAndUsageEndpoint` — 200 consume, 409/422 on exceeded (typed problem+json).

### T-025 · Billing: invoices + line-item snapshots · **M**
**Depends on**: T-019. **Spec**: PLAN.md §7.
**Deliverables**: `Invoice` aggregate (`draft→open→paid|void|uncollectible`); line items snapshotting plan/addon name, version, unit price, quantity, currency at issuance (copied values); per-tenant gapless invoice number sequence; totals in minor units; `TaxCalculator` port (no-op default); credit notes; REST list/get.
**Acceptance criteria**: an issued invoice is a historical fact — later catalog changes never alter it; numbering is per-tenant, gapless, concurrency-safe; lifecycle transitions guarded.
**Expected tests**:
- unit: `TestInvoiceLifecycleTransitionTable` — allowed/denied transitions.
- unit: `TestTotalsSumLineItemsInMinorUnits` — no floats anywhere (assert integer math incl. rounding rules).
- unit: `TestTaxCalculatorPortInvokedNoopDefaultZeroTax`.
- integration: `TestIssuedInvoiceImmuneToCatalogChanges` — publish new plan version + rename plan → invoice lines unchanged.
- integration: `TestPerTenantNumberingGaplessUnderConcurrency` — N concurrent invoices: sequence 1..N, no gaps/dupes; independent per tenant.
- integration: `TestCreditNoteReferencesInvoiceAndNegatesAmounts`.
- integration (HTTP): `TestInvoiceListGetScopedToTenant` — other tenant's invoices invisible.

### T-026 · Billing: `PaymentProvider` port, fake provider, charge flow · **L**
**Depends on**: T-021, T-025.
**Deliverables**: `PaymentProvider` port (`CreateCustomer`, `AttachPaymentMethod`, `Charge(idempotencyKey, …)`, `Refund`, `TranslateWebhook`); `fakeprovider` (auto-succeeds, programmable failures, records calls); payment-method storage (tokens only); idempotent consumer of `SubscriptionRenewalDue`: invoice → charge → `InvoicePaid` | `PaymentFailed`; provider idempotency keys derived from (invoice, attempt); subscription consumes `InvoicePaid` to advance period (removes `billing.disabled` path).
**Acceptance criteria**: duplicate renewal events cause exactly one charge; provider is swappable behind the port (fake proves the contract); raw payment credentials never persisted.
**Expected tests**:
- integration: `TestRenewalFlowChargeSucceedsInvoicePaidPeriodAdvances` — full loop with fakeprovider.
- integration: `TestRenewalFlowChargeFailsEmitsPaymentFailed` — invoice stays open; subscription hears the event.
- integration: `TestDuplicateRenewalDueDeliveryProducesExactlyOneCharge` — fake records one `Charge` call.
- integration: `TestProviderChargeReceivesStableIdempotencyKey` — same (invoice, attempt) ⇒ same key on retry.
- unit: `TestFakeProviderProgrammableFailuresAndCallRecording` — the test-harness contract itself.
- integration: `TestWebhookTranslationNormalizesProviderEvent` — fake webhook → normalized `ProviderEvent`.
- integration: `TestPaymentMethodStoresTokenOnlyNeverPAN` — schema/repo rejects anything but a token reference.

### T-027 · Billing: dunning + proration · **M**
**Depends on**: T-026.
**Deliverables**: dunning schedule from config (default `+1d,+3d,+7d`) via T-006 jobs: retries charge, emits `PaymentRecovered` | `DunningExhausted`; subscription consumes for `past_due→grace→suspended` (grace length from plan version); `ProrationStrategy` interface with `immediate_prorated` implemented, invoked on `SubscriptionPlanChanged`, producing prorated invoice or credit line.
**Acceptance criteria**: retries happen at exactly the configured offsets; both recovery and exhaustion paths drive the correct subscription states; proration math is exact integer arithmetic.
**Expected tests**:
- integration (frozen clock): `TestDunningRetriesAtConfiguredOffsets` — charges attempted at +1d/+3d/+7d only.
- integration: `TestPaymentRecoveredMidDunningStopsRetriesAndReactivates` — past_due → active.
- integration: `TestDunningExhaustedDrivesGraceThenSuspended` — grace length taken from the pinned plan version.
- unit: `TestProrationImmediateMathTable` — table: mid-period upgrade charges remaining-days difference; boundary cases (first/last day, same price); integer-only assertions.
- unit: `TestProrationStrategyNoneAndCreditNextInvoice` — strategy selection honored.
- integration: `TestPlanChangeTriggersProratedInvoiceLine`.

---

## Milestone 4 — Hardening & assembly

### T-028 · Signup flow wiring + cross-cutting E2E suite · **L**
**Depends on**: T-015, T-016, T-024, T-027.
**Deliverables**: composition-root provisioning hooks (seed system roles, create trial subscription on default plan); E2E suite (testcontainers + real HTTP against the wired binary) for the three PLAN.md flows; lint grep-gate that `WithSystemContext` never appears in request handlers.
**Acceptance criteria**: the three flows work end-to-end through the real stack — no fakes except the payment provider.
**Expected tests** (E2E, integration tag):
- `TestE2ESignup` — register → create tenant → provisioning creates trial subscription + roles → `GET /entitlements` returns the default plan's materialized set.
- `TestE2ERenewalSuccess` — advance frozen clock to renewal → invoice created with snapshots → fake charge succeeds → period advances → invoice `paid`.
- `TestE2ERenewalFailureDunning` — programmed charge failure → `past_due` → retries → exhaustion → `suspended`.
- `TestE2EUpgradeWithAddonRaisesLimit10To20` — attach addon → entitlements endpoint shows 20 → prorated line item exists.
- CI gate: `scripts/no-system-context-in-handlers.sh` green.

### T-029 · Seed/demo data + "new SaaS" checklist · **S**
**Depends on**: T-028.
**Deliverables**: `make seed` — demo features, two plans (free/pro) + one addon, demo tenant/user; README "Starting a new SaaS from this skeleton" checklist; documented phase-2 backlog (webhooks module, admin API, metering, coupons, SSO/TOTP); `docs/demo.sh` curl walkthrough.
**Acceptance criteria**: a fresh clone reaches a working demo (signup → entitlements → renewal) with three commands; seeding is idempotent.
**Expected tests**:
- integration: `TestSeedIsIdempotent` — running seed twice yields identical state, no duplicates.
- integration: `TestSeededDataSupportsDemoFlow` — seeded plan/tenant/user can execute the signup E2E path.
- CI gate: `docs/demo.sh` executed against a seeded compose stack (smoke).

---

## Frontend track (F-tasks)

The frontend lives in `admin/` on the **Inspinia v5 React** theme — the
project's design system. Read [`docs/FRONTEND.md`](./FRONTEND.md) first: it
holds the theme analysis, the keep-the-demo-on-the-side structure, runtime
configuration, API-client conventions, and the testing stack (Vitest + RTL +
MSW). The theme's full demo stays browsable under `/demo/*` as a living
component reference; our screens are adapted copies under `src/views/app/`.

F-003…F-009 are the **catch-up backlog** for endpoints that shipped before the
frontend rule (T-010–T-019): each maps to a merged backend task, so they can be
implemented in parallel — with each other and with backend lanes. From T-020 on,
new backend tasks carry their frontend per DoD item 7 (in-task or as a new
paired F-card).

### F-001 · Frontend foundation: runtime config, API client, auth store, app shell, demo aside · **L** · ✅ DONE (PR #25)
**Depends on**: backend T-013/T-011 (endpoints exist — already merged). Blocks every other F-task.
**Deliverables**:
- Demo preserved on the side: all theme routes re-mounted under `/demo/*` (mechanical prefix pass; demo views untouched); `VITE_ENABLE_DEMO` flag drops the demo bundle from production builds.
- Runtime config: `public/app-config.js` → `window.__APP_CONFIG__` (`apiBaseUrl`, `tenantMode: header|subdomain`, `tenantSlug`, `appName`, `enableDemo`); typed accessor `src/lib/config.ts` with `import.meta.env.VITE_*` dev fallbacks; branding (`META_DATA`, `AppLogo`) reads from it.
- API client `src/lib/api.ts`: base-URL prefixing, `Authorization: Bearer`, tenant header per tenant mode, RFC 7807 problem+json → typed `ApiError`, `Idempotency-Key` helper for mutating calls.
- Real auth store replacing the dummy `useAuth`: access+refresh pair, transparent single retry via `POST /auth/refresh` on 401, hard logout when refresh is rejected (rotation/reuse), `RequireAuth` route wrapper.
- App shell: `MainLayout` with our own `menuItems` (Dashboard, Tenant, Members, API Keys, Roles, Catalog, Subscription), placeholder pages, demo link.
- Test setup (Vitest + RTL + MSW) and an `admin/`-path-filtered CI job running lint + build + test.
**Acceptance criteria**: editing `app-config.js` in a **built** bundle switches backends without rebuilding; anonymous users are redirected to sign-in; demo fully browsable under `/demo`; CI job green.
**Expected tests**:
- unit: `config` prefers `window.__APP_CONFIG__`, falls back to `VITE_*` in dev.
- unit: api client attaches bearer + tenant header (header mode) and parses problem+json into `ApiError{status,title,detail}`.
- unit: a 401 triggers exactly one refresh-and-retry; a failed refresh clears the session (hard logout).
- component: `RequireAuth` redirects anonymous to `/auth/sign-in` and renders children when authenticated.

**Implementation notes** (PR #25): the vendored theme is not lint-/typecheck-clean, so `eslint` ignores the vendored demo views + a few theme-shared modules (documented in `admin/eslint.config.js`) and `npm run build` stays pure `vite build` (no `tsc`) — our own code lints clean. The demo keeps its dummy auth as `hooks/useDemoAuth.ts`; a minimal real sign-in page ships at `/auth/sign-in` (full auth suite is F-003).

### F-002 · Frontend infra: generic Docker image, nginx, compose, CI publish · **M** · ✅ DONE (PR #28)
**Depends on**: F-001.
**Delivered**: `admin/Dockerfile` (node build → `nginx:alpine`) with `.dockerignore`; `docker-entrypoint.sh` renders `app-config.js` from `app-config.js.template` via the `envsubst` that ships with `nginx:alpine` (no `apk add` — the alpine CDN can be unreachable), then `exec nginx`; `nginx.conf` with SPA history fallback, gzip, immutable `/assets/` caching and no-cache on `index.html`/`app-config.js`; a root `Dockerfile` for the Go API plus `api` + `admin` services in `docker-compose.yml`; CI `admin-image` job builds + smoke-tests the image on admin PRs and pushes GHCR tags (`sha` + `latest`) on `main`; unit test `admin/src/lib/appConfigTemplate.test.ts` proves every documented variable renders with defaults.
**Deliverables**: multi-stage `admin/Dockerfile` (node build → nginx-alpine); entrypoint renders `app-config.js` from env (`API_BASE_URL`, `TENANT_MODE`, `TENANT_SLUG`, `APP_NAME`, `ENABLE_DEMO`, …) via template + `envsubst` then starts nginx — **one generic image, config injected at start, nothing baked in**; `nginx.conf` with SPA history fallback, gzip, immutable asset caching, no-cache on `index.html`/`app-config.js`; `admin` service in `docker-compose.yml` wired to the api service; CI builds the image on PRs and pushes tagged (`sha` + `latest`) images on `main`.
**Acceptance criteria**: the same image serves two different backends by changing `API_BASE_URL` only; `docker compose up` yields SPA + API + Postgres working together; subdomain tenant mode documented (wildcard DNS/hosts note).
**Expected tests**:
- CI smoke: build image, `docker run -e API_BASE_URL=http://smoke-test`, assert `/app-config.js` contains it and `/` serves the SPA (200 + history fallback on a deep link).
- unit: entrypoint template renders every documented variable with defaults.

### F-003 · Auth screens · **L** *(backend: T-012, T-013 — merged)*
**Depends on**: F-001.
**Screens** (adapt `views/auth/basic/*` + `account-settings` security tab): sign-in, sign-up, forgot/reset password (token from URL), email verification (request + confirm + success-mail state), password change, active sessions list + "log out other devices", logout.
**Endpoints**: `POST /api/v1/auth/{register,login,refresh,logout,verify-email/request,verify-email,password/forgot,password/reset,password/change}`, `GET /api/v1/auth/sessions`, `POST /api/v1/auth/sessions/revoke-others`.
**Acceptance criteria**: the full journey register → verify → sign-in → change password → revoke other sessions works against the real API; problem+json details render as form/field errors; 429 (login rate limit) shows a friendly retry message.
**Expected tests** (MSW):
- sign-in success stores the token pair and redirects to `/`; 401 renders the error inline.
- sign-up client-side validation (yup) blocks invalid email/short password before any request.
- reset-password page posts the token from the URL and routes to sign-in on success.
- sessions page lists sessions; revoke-others calls the endpoint and refreshes the list.

### F-004 · Tenant screens: onboarding, settings, lifecycle · **M** *(backend: T-010, T-011 — merged)*
**Depends on**: F-001.
**Screens**: post-signup create-tenant onboarding (no-tenant empty state), tenant settings (name/slug/settings), danger zone (suspend/reactivate/delete with sweetalert2 confirm), tenant switcher in the TopBar (header mode).
**Endpoints**: `POST /api/v1/tenants`, `GET|PATCH /api/v1/tenants/{id}`, `POST /api/v1/tenants/{id}/{suspend,reactivate}`, `DELETE /api/v1/tenants/{id}`.
**Expected tests** (MSW): create-tenant success routes to the dashboard and subsequent requests carry the tenant header; settings form round-trips a PATCH; delete requires the confirm dialog; switcher swaps the tenant header.

### F-005 · Members & invitations screens · **M** *(backend: T-015 — merged)*
**Depends on**: F-001.
**Screens** (adapt `users/contacts` + `DataTable`): members list with remove, invitations list with invite form (email + role) and resend, and the public invitation accept/decline page.
**Endpoints**: `POST|GET /api/v1/tenants/{id}/invitations`, `POST .../invitations/{invId}/{resend,accept,decline}`, `GET /api/v1/tenants/{id}/members`, `DELETE /api/v1/tenants/{id}/members/{userId}`.
**Expected tests** (MSW): members table renders and remove confirms first; invite form validates email; accept page drives the accept endpoint then routes onward (sign-in when anonymous).

### F-006 · API keys screen · **S** · ✅ DONE (PR #27) *(backend: T-014 — merged)*
**Depends on**: F-001.
**Screens**: adapt the theme's near-1:1 `apps/api-keys` page — list (prefix, scopes, created/last-used), create modal (name + scopes) with **one-time secret reveal** + copy, revoke with confirm.
**Endpoints**: `POST|GET /api/v1/api-keys`, `DELETE /api/v1/api-keys/{id}`.
**Expected tests** (MSW): create shows the secret exactly once (not re-rendered after close); list renders; revoke removes the row after confirm.

### F-007 · Roles & permissions screens · **M** *(backend: T-016 — merged)*
**Depends on**: F-001.
**Screens** (adapt `users/roles`, `role-details`, `permissions`): roles list (system roles badged, not deletable), create/edit role with a `resource:action` permission editor, role details with assignments (assign user, unassign).
**Endpoints**: `GET|POST /api/v1/roles`, `GET|PATCH|DELETE /api/v1/roles/{id}`, `POST /api/v1/roles/{id}/assignments`, `DELETE /api/v1/roles/{id}/assignments/{userId}`.
**Expected tests** (MSW): system roles hide delete; create posts the permissions array; a 403 from the permission guard renders an access-denied state, not a crash.

### F-008 · Catalog admin screens · **L** *(backend: T-017, T-018 — merged)*
**Depends on**: F-001.
**Screens**: plans list (status, public badge); plan detail with versions timeline; version editor — editable **only while draft** (pricing per cycle in minor units, trial config, grace days, features/limits), publish with immutability warning, archive, public toggle; addons list/detail/versions (entitlement deltas + compatible plans); public pricing preview reusing `pages/pricing` bound to `GET /api/v1/catalog/public`.
**Endpoints**: the full catalog surface (`/api/v1/catalog/plans...`, `/versions/{vid}`, `/public`, `/addons...`, `/addon-versions/{vid}`).
**Expected tests** (MSW): editor fields disabled for a published version; publish requires the confirm; pricing preview renders public plans; addon delta rows validate feature keys and integer amounts (minor units — no floats anywhere in the UI layer either).

### F-009 · Subscription screen · **M** *(backend: T-019, T-020 — merged)*
**Depends on**: F-001, F-008 (reuses the plan/pricing picker components).
**Screens**: a "Billing → Subscription" page: current-subscription card (status chip per state, current period, trial countdown, cancel-at-period-end banner); when none, a subscribe flow (public plan + billing-cycle picker); lifecycle actions **rendered from the state machine** so only legal actions show (cancel immediate/at-period-end modal, pause, resume, reactivate). Plus (T-020): a **change-plan flow** (pick plan+cycle; on response show either the new pin or a "scheduled for period end" banner with a cancel-change button) and an **addons section** (attached addons with quantity steppers, attach from compatible published addons, detach with confirm).
**Endpoints**: `POST|GET /api/v1/subscription`, `POST /api/v1/subscription/{cancel,pause,resume,reactivate,change-plan}`, `POST /api/v1/subscription/scheduled-change/cancel`, `POST /api/v1/subscription/addons`, `DELETE /api/v1/subscription/addons/{vid}`.
**Expected tests** (MSW): active shows pause+cancel and not resume; paused shows resume; the cancel modal posts `immediate` true/false correctly; a 409 renders a conflict toast; the empty state shows the plan picker; a downgrade response renders the scheduled-change banner and its cancel button calls the endpoint; addon attach validates quantity and a 400 (incompatible) renders inline.

---

## Suggested parallel lanes

| Lane | Tasks in order |
|---|---|
| A (platform) | T-001 → T-002 → T-003 → T-005 → T-006 |
| B (platform) | T-004 → T-007 → T-008 (after T-003) |
| — sync point — | T-009 (needs A+B) |
| C (identity) | T-010 → T-011 → T-015 |
| D (identity) | T-012 → T-013 / T-014 → T-016 |
| E (product) | T-017 → T-018 → T-020 |
| F (product) | T-019 → T-021 → T-025 → T-026 → T-027 |
| G (product) | T-022 → T-023 / T-024 |
| H (frontend) | F-001 → F-002, then F-003…F-009 in any order (fully parallel with backend lanes) |
| — final — | T-028 → T-029 |
