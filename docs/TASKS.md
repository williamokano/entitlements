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
**Amended by F-005**: a membership now stores the email it was invited by
(`tenant.memberships.email`, migration `00003`), so `GET .../members` returns
`{user_id, email, role, status}` and the UI can name its members. Accept() is the
only place a membership is created and it already holds the invited email, so the
tenant module needs no user lookup in another module — no new port, no dependency
on authentication. Added: `TestListMembersCarriesTheInvitedEmail`,
`TestListMembersToleratesAMembershipWithoutAnEmail` (rows predating the column
have an empty email; clients fall back to the user id).

### T-031 · Tenant creator becomes an owner-member · **S** · ✅ DONE (PR #46)
**Depends on**: T-010, T-015, F-005. **Found while building F-005.**
**Problem**: `Accept()` was the *only* path that created a membership, so
creating a tenant left you with **no membership in it** — a freshly created
tenant's members list was empty, and its creator held no role there. The
authorization module seeds the tenant's roles but nothing bound the creator to
`owner`.
**Does *not* fix the roles 403** — I assumed it would and was wrong: authorization
resolves permissions from `authz.role_assignments`, not from the tenant
membership's role name, and nothing ever writes an assignment. That is a separate
defect: see **T-032**. (The `MemberJoined` event this card now publishes for the
owner is exactly the hook T-032 needs.)
**Delivered**: `POST /api/v1/tenants` now reads the principal — when the caller
is an authenticated user they are written as an active `owner` member (with their
email, per F-005) **in the same transaction** as the tenant, and a `MemberJoined`
event is published for them so consumers (seat counts, …) see the owner like any
other member.
**Not a provisioning hook** (the open question in the original card, now settled):
hooks run async off the outbox and `TenantCreated` carries only
`{tenant_id, slug, name}` — no creator — so a hook has no user to bind. It has to
happen in the create use case.
**Tests** (integration): `TestCreatingATenantMakesTheCreatorAnOwnerMember`,
`TestCreatingATenantEmitsMemberJoinedForTheOwner`,
`TestCreateWithACreatorWritesAnOwnerMembership`,
`TestCreateConflictRollsBackTheOwnerMembership` (no orphan membership on a
duplicate slug), `TestCreatingATenantAnonymouslyStillWorksAndHasNoMembers`.

> ⚠️ **REVIEW THIS — deliberate judgment call, easy to change.**
> Tenant creation is currently an **open endpoint**: `cmd/api/app.go` mounts the
> tenant handler unguarded and the auth middleware is permissive ("no credential:
> pass through unauthenticated"), so **anyone can create a tenant anonymously** —
> the README walkthrough and `cmd/api/app_integration_test.go` both do exactly
> that, with no `Authorization` header.
>
> I therefore made owner-binding **opportunistic and non-breaking**: sign in and
> you own what you create; stay anonymous and you get an ownerless tenant, exactly
> as before. Nothing that worked stops working.
>
> The stricter alternative is to **require authentication on `POST
> /api/v1/tenants`**, so every tenant provably has an owner and the ownerless
> state becomes unrepresentable. That is the cleaner invariant, and T-028's own
> acceptance test ("register → create tenant") already assumes it.
>
> **Superseded by T-034**: reviewing this guard turned up that the *entire* tenant
> CRUD surface is unauthenticated — not just create — and that suspend/reactivate
> must be an **operator** action, not a membership one (a tenant that can
> reactivate itself can undo its own suspension). So "require auth on create" is
> no longer a standalone question; it is one row of the T-034 split, which is
> blocked on the operator identity (T-033). This card's opportunistic binding is
> the safe interim state and breaks nothing.

### T-032 · RBAC is unbootstrappable: nothing ever assigns a role · **M** ← blocks all of `/api/v1/roles`
**Depends on**: T-016, T-031. **Found while building F-005/T-031; verified against a live server.**

**Problem.** No user can ever obtain a permission through the API, in any tenant:

- `Service.Check` (authorization) resolves a user's permissions from the union of
  the roles assigned to them in `authz.role_assignments`. It does **not** consult
  the tenant membership's role name — despite what `ports.MembershipReader`'s doc
  comment implies ("authorization resolves a user's role in a tenant through it").
- The **only** thing in the codebase that writes an assignment is
  `POST /api/v1/roles/{id}/assignments` — and that route is itself guarded by
  `role:write`.
- Permissions come only from assignments; assignments can only be made by someone
  who already has a permission. **Chicken-and-egg**: the assignment table starts
  empty and can never become non-empty.

So every `/api/v1/roles/*` route 403s for every user, forever. Seeding the system
roles (the provisioning hook) creates the roles but binds nobody to them. Today
this is only survivable because nothing else is permission-guarded yet — but it
means RBAC (T-016) is effectively dead code from the API's point of view, and the
F-005 invite form only works because it falls back when roles 403.

**Deliverables**: bridge membership → role assignment. The natural seam is the
event: authorization subscribes to `tenant.member.joined` (payload carries
`{tenant_id, user_id, role}`), resolves the role by name within that tenant, and
writes the assignment; `tenant.member.left` unassigns. This covers **both**
entry points into a tenant — the creator (T-031 now publishes `MemberJoined` for
the owner) and every invitee who accepts. Keep it idempotent: the handler runs
through the outbox and can be redelivered.
Per the module-ownership rule, authorization must **not** import `tenant/ports` —
declare the event name and a local payload struct on its own side.
Also fix `ports.MembershipReader`'s misleading doc comment, and decide whether
membership role and assignment are two sources of truth or one (they currently
disagree, silently).

**Expected tests** (integration): a tenant's creator can immediately read
`GET /api/v1/roles` (the 403 is gone); an invitee who accepts gains their role's
permissions; removing a member revokes them; redelivery of `member.joined` does
not duplicate assignments; a member's permissions are the union of their roles'.

### T-033 · Platform operator identity ("global admin") · **L** ← prerequisite for T-034/T-035
**Depends on**: T-012, T-016. **Found while reviewing T-031's guard.**

**The product model these three cards assume** (confirmed, and already what the
code does — stated here because every guard decision below hangs on it):
the **catalog is global** — plans, versions and addons are *the SaaS operator's
offering*, defined once for the whole platform (`catalog/module.go`: "catalog is
global (the SaaS operator's offering), not tenant-scoped"). **A tenant is a
customer** of this SaaS, not an owner of its own catalog. **Subscription** is the
join: a tenant pins a *version* of a global plan (+ addons). **Entitlements are
embedded machinery**, not a product surface — resolved per tenant from
`plan grants + addon deltas + overrides`. So: the operator owns the offering;
tenants consume it. That is exactly why catalog admin (T-035) and tenant
lifecycle (T-034) are *operator* surfaces, while members/API-keys/subscription
are *tenant* surfaces.

**The gap.** The system has **no notion of the SaaS operator as an authenticated
actor**. Authorization's RBAC is entirely *tenant-scoped* — roles live per tenant
— so there is no way to express "this person runs the platform". `PrincipalSystem`
/ `WithSystemContext` cannot fill the role: it is explicitly internal-only ("set
only by internal callers, never derived from a request") and T-028 adds a CI gate
banning it from request handlers.

Consequence: every operator-owned surface is effectively unguarded, because there
was nobody to authorize it *against* —

| Surface | Guard today |
|---|---|
| Tenant CRUD (`/api/v1/tenants`) | **none at all** (see T-034) |
| Catalog admin (`/api/v1/catalog/plans…`) | `requireAuth` only — **any registered user** can create/publish/archive plans in the operator's global catalog (see T-035) |

**Options** (decide before building):

1. **Platform-scoped RBAC + seeded bootstrap admin** *(recommended)* — extend
   authorization with a global, tenant-less scope (`tenant_id IS NULL`, or a
   reserved platform tenant), a `RequirePlatformPermission` middleware, and a
   bootstrap platform-admin **seeded at startup from config** (e.g.
   `PLATFORM_ADMIN_EMAILS`). Reuses "RBAC as data" rather than inventing a
   parallel mechanism, and gives granular operator permissions
   (`tenant:suspend`, `catalog:write`, …). Bonus: a seeded assignment is exactly
   what breaks **T-032**'s bootstrap deadlock, for the platform scope at least.
   Cost: the largest change; touches authorization's schema and its
   tenant-scoping assumptions throughout.
2. **`platform_admin` flag on the authn user** — a boolean/claim plus a
   `RequirePlatformAdmin` middleware. Fastest to land and unblocks the guards
   immediately, but it is a *second* authz mechanism alongside the RBAC the
   system otherwise commits to, and it is all-or-nothing (no granular operator
   permissions).
3. **Operator API keys** — machine principals with platform scopes. Fits the
   existing API-key machinery, but API keys are tenant-bound today, and it gives
   the operator no *human* login path.

**This is a skeleton — ship primitives, not one-off guards.** Adopters bring
their own tenant-scoped modules (the `example` module is the template they copy),
so the deliverable is *reusable* guards, not bespoke checks inside `tenant`:

- `RequirePlatformPermission(perm)` — operator surfaces (tenant lifecycle,
  catalog admin).
- `RequireTenantMembership(roles…)` — tenant-scoped surfaces; the guard an
  adopter puts on their own module's routes.
- Applied at the **composition root**, next to the existing
  `authentication.RequireAuth` on `/api/v1/api-keys`, so a module's routes are
  guarded where they are mounted rather than by hand in every handler.

> ⚠️ **The `example` module has no principal check at all** — and it is precisely
> the file adopters copy to start their own module. Whatever guard this card
> lands, `example` must **demonstrate** it, or the skeleton teaches the insecure
> pattern by default. Make the secure path the path of least resistance.

**Expected tests**: a platform admin can suspend a tenant; a tenant owner cannot;
an anonymous caller cannot; the bootstrap admin is seeded idempotently at startup
and is the only way in on a fresh database; the `example` module rejects an
anonymous caller and a caller from another tenant.

### T-034 · Split the tenant endpoints: self-service vs operator · **M**
**Depends on**: T-031, T-033.

> ⚠️ **Live security hole today.** The whole tenant CRUD surface is
> **unauthenticated**: `POST /`, `GET /{id}`, `PATCH /{id}`, `POST
> /{id}/suspend`, `POST /{id}/reactivate`, `DELETE /{id}` never look at the
> principal. Anyone who knows or guesses a tenant UUID can rename, suspend, or
> soft-delete **any tenant**, with no credentials. The tenant module's handler is
> mounted bare and its CRUD handlers do no check — every *other* module's REST
> adapter checks a principal; tenant CRUD is the outlier.
>
> Note the cause of the confusion: `tenant.WithExempt("/api/v1/tenants", …)`
> exempts these paths from the **tenant-resolution** middleware (you cannot
> resolve a tenant *context* for an operation that manages tenants — chicken and
> egg). That is a different middleware from `authenticate`. Being exempt from
> tenant resolution was conflated with being public.

**The design.** The resource mixes two audiences, so one guard cannot be right
for all of it:

| Route | Audience | Guard |
|---|---|---|
| `POST /tenants` | **open policy question — see below** | authenticated user; creator becomes owner (T-031 ✅) |
| `GET /tenants/{id}` | tenant member (the SPA settings/switcher read it) | active membership in `{id}` |
| `PATCH /tenants/{id}` | tenant owner/admin (edit name/settings) | `owner \| admin` membership in `{id}` |
| `POST /{id}/suspend`, `/reactivate` | **operator only** | platform admin (T-033) |
| `DELETE /{id}` | operator — *or* owner self-serve account closure (**product policy call**) | platform admin (± owner) |

**Why suspend/reactivate must NOT be a membership check** (this was nearly
shipped as one): suspension is the *enforcement* lever — dunning, abuse,
non-payment. If a tenant owner can reactivate their own tenant, the party being
enforced against can trivially undo the enforcement. Same reasoning makes DELETE
sensitive.

**❓ Open policy question — who may create a tenant?** T-031 assumed *self-serve*
(any authenticated user creates their own org and becomes its owner), because
T-028's acceptance test reads "register → create tenant". But a SaaS may instead
be **operator-provisioned** (the global admin creates the tenant for a customer,
then invites their first user as owner) — both are legitimate shapes, and the
skeleton arguably wants to support both:

- **Self-serve** — `POST /tenants` requires an authenticated user; creator becomes
  owner. (What T-031 ships.)
- **Operator-provisioned** — `POST /tenants` requires a platform admin; the
  operator does *not* become a member, and the customer's first owner arrives via
  an invitation. Note this makes T-031's "creator becomes owner" **wrong** for
  that path — an operator creating a hundred tenants must not end up owning them.
- **Both** — allow either principal: a user creating their own org becomes its
  owner; a platform admin creating one for a customer does not.

Decide this before T-034 is built; it is the one row of the table above that
changes T-031's behavior.

**Also decide**: should tenant admin accept a *machine* principal (API key), or
users only? Membership routes are user-only today.

**Migration note**: once `GET/PATCH /{id}` require membership, tenants created
**before T-031** have no owner and become unreachable by their creator. Fresh
databases are fine; a dev DB may need a backfill.

**Expected tests**: anonymous → 401 on every route; a non-member → 403; a member
can read but not update; an owner can update but **cannot** suspend/reactivate;
a platform admin can; the creator can administer the tenant they just made.

### T-035 · Guard the catalog admin surface · **S**
**Depends on**: T-033.
**Problem**: catalog is "the SaaS operator's offering, global, not tenant-scoped"
— but its admin routes (`POST /plans`, `/publish`, `/archive`, addons, versions)
are guarded by `requireAuth` alone, so **any registered user can mutate the
operator's global catalog**: publish plans, archive them, change addon deltas.
The public listing (`GET /catalog/public`) is intentionally open and stays so.
**Deliverables**: guard every catalog *mutation* with the platform-admin
permission from T-033; leave reads/public listing as they are.
**Expected tests**: a plain tenant user gets 403 creating/publishing a plan; a
platform admin succeeds; `GET /catalog/public` stays open to anonymous callers.

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
**Note (implemented)**: exactly-once emission is gated by a `renewal_emitted_period_end` marker on the subscription (not by advisory state), so duplicate ticks and multiple runners still yield one `subscription.renewal_due` per period. `BILLING_DISABLED` (still the default `true`) makes the renewal auto-advance the period; when billing is enabled (`BILLING_DISABLED=false`) the period advances only when the module's idempotent `billing.invoice_paid` consumer fires — and as of T-026 that event is produced by the real charge flow (billing consumes `renewal_due` → issue → charge → `invoice_paid`), so `BILLING_DISABLED=false` now exercises billing end to end. Trial resolution keys off the plan version's `card_required`: no card → convert to active (starting the first paid period at the trial end); card required → expire (no payment method on file yet). `SUBSCRIPTION_TRIAL_ENDING_DAYS` (default 3) sets the pre-expiry `TrialEnding` lead time. Frontend: none (no new user-facing endpoints; renewal/trial are background jobs surfaced through the existing subscription view — F-009).
**Deliverables**: recurring job scanning due subscriptions: emits `SubscriptionRenewalDue`, applies scheduled changes at rollover, advances period only after `InvoicePaid` (config flag `billing.disabled=true` auto-advances; T-026 makes the real charge flow produce `InvoicePaid` when billing is enabled); trial job: `TrialEnding` (configurable days before), `TrialEnded` → convert or expire per plan config.
**Acceptance criteria**: renewal emission is exactly-once per period even with duplicate ticks/replicas; trial events fire at the right frozen-clock moments; the billing-disabled path keeps the module testable standalone.
**Expected tests** (integration, frozen clock):
- `TestRenewalDueEmittedExactlyOncePerPeriod` — duplicate job ticks + idempotent consumer ⇒ one event.
- `TestTwoRunnersOneRenewalEmission` — combined with jobs advisory lock.
- `TestScheduledDowngradeAppliedAtRollover`.
- `TestBillingDisabledAutoAdvancesPeriod` vs `TestPeriodNotAdvancedUntilInvoicePaidWhenBillingEnabled`.
- `TestTrialEndingEmittedConfiguredDaysBefore`.
- `TestTrialEndedConvertsToActiveOrExpiresPerPlanConfig` — both branches.

### T-022 · Entitlements: feature registry + resolution pipeline · **L** ← core of the product · ✅ DONE (PR #31)
**Depends on**: T-017, T-019. **Spec**: PLAN.md §6.

**Implementation notes** (PR #<pending>): shipped `internal/modules/entitlements/`
(hexagonal) + `migrations/entitlements/00001_entitlements.sql` (`features`,
`effective_entitlements`, `tenant_overrides`). Design points worth recording:
- **Reads resolve live, materialization powers the change feed.** `GET
  /entitlements[/{key}]` and `ports.EntitlementsReader` compute the effective set
  live from the registry + subscription/catalog ports + overrides, so responses
  are never stale. `effective_entitlements` is the materialized cache that idempotent
  event consumers rebuild; its only behavioural role is the diff that decides
  whether to publish `EntitlementsSummaryChanged` (exactly one per real change,
  nothing on a no-op).
- **Subscription port extension** (the allowed cross-module change): added
  `ports.AddonAttachment` + `SubscriptionReader.GetAttachedAddons(ctx, subID)`,
  implemented on the subscription service over `repo.ListAddons`. The resolver
  reads it to apply addon deltas × quantity.
- **enum allowed-set** lives in the feature's `metadata.allowed_values` (no extra
  column), read by the domain when constraining enum values.
- **Feature-registry CRUD is service-level** (create/list/get/update/archive),
  exposed via `Module.Service()` for provisioning/seeding; no admin HTTP routes
  were added (registry admin API stays out of the T-022 HTTP surface, which the
  card scopes to the two runtime GETs). Key and type are immutable on update.
- **Consumers**: `subscription.transitioned|plan_changed|addon_changed`, each
  `events.Idempotent`-wrapped, re-materialize the tenant. Catalog/override event
  wiring deferred (overrides land in T-023) — kept scope tight.
- **Config**: `ENTITLEMENTS_UNKNOWN_FEATURE_POLICY` (`deny` default | `allow`).
- Paired frontend: **F-010** (entitlements viewer) added below.

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

### T-023 · Entitlements: tenant overrides + audit · **M** · ✅ DONE (PR #34)
**Depends on**: T-022.
**Deliverables**: override CRUD (feature_key, **absolute `value`** — matching how
T-022's resolver reads overrides; the table has no `delta` column, so overrides
set the effective value outright at precedence plan < addon < override — optional
`expires_at`, mandatory reason + actor) via admin REST; expiry job re-resolving
affected tenants; every change audited; override changes trigger re-materialization.
**Acceptance criteria**: overrides win over plan+addons; expire on time; are fully audited (who/what/when/why); removal restores the pre-override value.
**Delivered**:
- REST under `/api/v1/entitlements` (tenant-scoped, auth-required): `POST /overrides`,
  `GET /overrides`, `GET|PATCH|DELETE /overrides/{id}`. Actor is taken from the
  authenticated principal (never the body); reason is mandatory. The runtime
  `GET /entitlements[/{key}]` reads are unchanged except they now surface
  `expires_at` when the winning value comes from a time-bound override.
- Create/update/delete each run the override write + a `platform/audit` entry +
  a re-materialization in **one `UnitOfWork` transaction** (nested `Do` joins the
  ambient tx), so the override, its audit row, the rebuilt `effective_entitlements`,
  and the `EntitlementsSummaryChanged` event commit together. Audit actions:
  `entitlement.override.{created,updated,deleted,expired}`.
- **Expiry job** `entitlements.override_expiry` (registered via
  `Module.RegisterJobs`, wired in `cmd/api/app.go`, clock-driven). Decision:
  **expired overrides are ignored by resolution *and* deleted by the job**, which
  audits the removal as the `system` actor and re-materializes the affected
  tenants. Resolution already excludes expired rows, so live reads revert at the
  instant of expiry; the job then reverts the stored/materialized set and prunes
  the row.
- No schema change: the T-022 migration already carries `tenant_overrides`
  (value/reason/actor/expires_at). No sqlc drift.
**Expected tests** (integration unless noted) — all implemented and green:
- `TestOverrideBoostsEffectiveValueImmediately`.
- `TestOverrideWithoutReasonOrActorRejected` (unit).
- `TestOverrideExpiryJobRevertsValue` (frozen clock).
- `TestOverrideDeleteRestoresPlanAddonValue`.
- `TestOverrideChangesFullyAudited` — create/update/delete each produce an audit entry with reason + actor.
- `TestTimeBoundOverrideVisibleInEntitlementsResponse` — expiry surfaced to clients.
**Paired frontend**: override admin UI — see **F-011** below (the T-022 viewer
F-010 is read-only; overrides CRUD is a new screen).

### T-024 · Entitlements: usage tracking + quota enforcement · **M** · ✅ DONE (PR #36)
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

**Note (implemented, PR #36)**: usage counters live in `entitlements.usage_counters`
keyed by `(tenant_id, feature_key, period_key)`; `period_key` is derived lazily
from the feature's `reset_period` (`never` | `YYYY-MM` for monthly | the
subscription's current-period start for `billing_cycle`), so a new period is a
new key with a fresh zero counter — no reset job. Hard enforcement is a single
guarded upsert (`INSERT … SELECT … WHERE n ≤ limit ON CONFLICT DO UPDATE …
WHERE used + n ≤ limit RETURNING used`): the guard lives in SQL, so N racing
callers over limit L yield exactly L accepted consumes; a no-row result maps to
the new `apperr.KindQuotaExceeded` (HTTP **422** problem+json). Soft limits
always consume and claim a per-period `warned` latch so exactly one crosser emits
`EntitlementLimitWarning`. Downgrade grace runs inside `Materialize` (already
change-gated + idempotent): a limit that shrinks below current usage emits one
`EntitlementExceeded` and never blocks reads. REST: `POST /entitlements/consume`,
`POST /entitlements/release`, `GET /entitlements/usage`, `GET /entitlements/usage/{key}`.
Other modules consume via `ports.UsageReader` (`Module.UsagePort()`). **Frontend**:
paired **F-012** (usage/quota display on the Entitlements page).

### T-025 · Billing: invoices + line-item snapshots · **M** · ✅ DONE (PR #38)
**Depends on**: T-019. **Spec**: PLAN.md §7.
**Deliverables**: `Invoice` aggregate (`draft→open→paid|void|uncollectible`); line items snapshotting plan/addon name, version, unit price, quantity, currency at issuance (copied values); per-tenant gapless invoice number sequence; totals in minor units; `TaxCalculator` port (no-op default); credit notes; REST list/get.
**Acceptance criteria**: an issued invoice is a historical fact — later catalog changes never alter it; numbering is per-tenant, gapless, concurrency-safe; lifecycle transitions guarded.
**Note (implemented)**: shipped `internal/modules/billing/` (hexagonal) +
`migrations/billing/00001_invoices.sql` (schema `billing`: `invoices`,
`invoice_line_items`, `credit_notes`, and `number_sequences` — a generic
per-`(tenant, kind)` gapless counter serving invoices and credit notes).
Design points:
- **Snapshotting.** `Issue` reads the tenant's live subscription (subscription
  port) + its pinned plan version and attached addon versions (catalog port) and
  **copies** key/version/unit-price/quantity/currency into `invoice_line_items`
  rows. The invoice is read back only from its own rows, so a later plan-version
  publish or price change never rewrites it (proven live + by
  `TestIssuedInvoiceImmuneToCatalogChanges`). Money is integer minor units
  throughout; no floats. Line amount = unit × quantity (exact), subtotal = Σ
  amounts, total = subtotal + tax.
- **Gapless numbering.** `NextNumber` runs one atomic upsert inside the issuing
  `UnitOfWork` tx: `INSERT ... VALUES (tenant, kind, 1) ON CONFLICT (tenant, kind)
  DO UPDATE SET next_number = next_number + 1 RETURNING next_number`. The conflict
  row lock serializes concurrent issuers per tenant → 1,2,3… no gaps/dupes; a
  rolled-back issue rolls back its increment; tenants are independent.
- **TaxCalculator** is a service-level port with a shipped `NoopTaxCalculator`
  (zero tax) default; issuance always invokes it and stores `tax_minor`.
- **Lifecycle** is a guarded transition table in the pure domain.
- **Event seam.** Paying an invoice publishes `billing.invoice_paid` (exact type)
  via the outbox with the `subscription_id`, so the subscription module's existing
  idempotent consumer works. The PaymentProvider/charge flow that drives this
  automatically on renewal landed in T-026.
- REST under `/api/v1/billing/invoices` (tenant-scoped, auth-required):
  `POST` (issue), `GET` (list), `GET /{id}`, `POST /{id}/{pay,void,uncollectible}`,
  `POST|GET /{id}/credit-notes`. `ports.BillingReader` exposes GetInvoice/ListInvoices.
- Wired in `cmd/api/app.go` (takes catalog + subscription ports). No depguard
  change needed (cross-module *ports* imports are already allowed; only the pure
  `domain` package is import-restricted, and it stays pure). No sqlc drift (sqlc
  reads only `migrations/platform`).
- **Paired frontend**: invoices UI is carded as **F-013** below.
**Expected tests** — all implemented and green:
- unit: `TestInvoiceLifecycleTransitionTable` — allowed/denied transitions.
- unit: `TestTotalsSumLineItemsInMinorUnits` — no floats anywhere (assert integer math incl. rounding rules).
- unit: `TestTaxCalculatorPortInvokedNoopDefaultZeroTax`.
- integration: `TestIssuedInvoiceImmuneToCatalogChanges` — publish new plan version + rename plan → invoice lines unchanged.
- integration: `TestPerTenantNumberingGaplessUnderConcurrency` — N concurrent invoices: sequence 1..N, no gaps/dupes; independent per tenant.
- integration: `TestCreditNoteReferencesInvoiceAndNegatesAmounts`.
- integration (HTTP): `TestInvoiceListGetScopedToTenant` — other tenant's invoices invisible.

### T-026 · Billing: `PaymentProvider` port, fake provider, charge flow · **L** · ✅ DONE (PR #NN)
**Depends on**: T-021, T-025.
**Deliverables**: `PaymentProvider` port (`CreateCustomer`, `AttachPaymentMethod`, `Charge(idempotencyKey, …)`, `Refund`, `TranslateWebhook`); `fakeprovider` (auto-succeeds, programmable failures, records calls); payment-method storage (tokens only); idempotent consumer of `SubscriptionRenewalDue`: invoice → charge → `InvoicePaid` | `PaymentFailed`; provider idempotency keys derived from (invoice, attempt); subscription consumes `InvoicePaid` to advance period (removes `billing.disabled` path).
**Note (implemented)**: `PaymentProvider` is a service-level driven port modeled like `TaxCalculator`; the default adapter is `internal/adapters/fakeprovider` (injected by `billing.New`, overridable with `billing.WithPaymentProvider` for a real gateway). Provider idempotency keys are `charge:<invoiceID>:<attempt>` (attempt 1 for the initial renewal; dunning uses higher attempts in T-027). The renewal charge flow is the billing module's idempotent `subscription.renewal_due` consumer (`events.Idempotent`, consumer `"billing"`): it issues the invoice, charges, then pays (publishing `billing.invoice_paid`) or publishes `billing.payment_failed` and leaves the invoice open. Billing only subscribes to `renewal_due` when `BILLING_DISABLED=false`, so exactly one path advances a period: with billing enabled the real charge flow drives it; with `BILLING_DISABLED=true` (still the default) the subscription's renewal job auto-advances as before. Payment methods store **tokens only** — a `NewPaymentMethod` domain guard plus a schema `token_not_pan` CHECK both reject a raw PAN (`migrations/billing/00002_payment_methods.sql`). No sqlc drift (sqlc reads only `migrations/platform`). Frontend: none (no new HTTP endpoints — the charge flow is event-driven; payment-method entry is a Go module method surfaced later by an F-card).
**Acceptance criteria**: duplicate renewal events cause exactly one charge; provider is swappable behind the port (fake proves the contract); raw payment credentials never persisted.
**Expected tests** — all implemented and green:
- integration: `TestRenewalFlowChargeSucceedsInvoicePaidPeriodAdvances` — full loop with fakeprovider.
- integration: `TestRenewalFlowChargeFailsEmitsPaymentFailed` — invoice stays open; subscription hears the event.
- integration: `TestDuplicateRenewalDueDeliveryProducesExactlyOneCharge` — fake records one `Charge` call.
- integration: `TestProviderChargeReceivesStableIdempotencyKey` — same (invoice, attempt) ⇒ same key on retry.
- unit: `TestFakeProviderProgrammableFailuresAndCallRecording` — the test-harness contract itself.
- integration: `TestWebhookTranslationNormalizesProviderEvent` — fake webhook → normalized `ProviderEvent`.
- integration: `TestPaymentMethodStoresTokenOnlyNeverPAN` — schema/repo rejects anything but a token reference.

### T-027 · Billing: dunning + proration · **M** · ✅ DONE (PR #NN)
**Depends on**: T-026.
**Deliverables**: dunning schedule from config (default `+1d,+3d,+7d`) via T-006 jobs: retries charge, emits `PaymentRecovered` | `DunningExhausted`; subscription consumes for `past_due→grace→suspended` (grace length from plan version); `ProrationStrategy` interface with `immediate_prorated` implemented, invoked on `SubscriptionPlanChanged`, producing prorated invoice or credit line.
**Acceptance criteria**: retries happen at exactly the configured offsets; both recovery and exhaustion paths drive the correct subscription states; proration math is exact integer arithmetic.
**Note (implemented)**: A declined renewal charge opens a `billing.dunning` schedule (one row per invoice, `UNIQUE(invoice_id)`) anchored at the failure instant; the schedule stores the highest charge `attempt` (1 = initial renewal, retries use 2,3,…) and `next_retry_at`. The `billing.dunning` job (`RegisterJobs`, advisory-lock elected) scans due schedules and re-charges the same open invoice with the T-026 stable `charge:<invoice>:<attempt>` key at a **new attempt number**, so duplicate ticks and redeliveries never double-charge; retries fire at exactly `failed_at + offsets[]` (offsets from `BILLING_DUNNING_OFFSETS`, default `1d,3d,7d`, day/duration syntax with optional `+`). A retry that succeeds pays the invoice (publishing `billing.invoice_paid` → period advances) and `billing.payment_recovered`; exhaustion publishes `billing.dunning_exhausted`. The subscription module consumes all three (local event-name consts, like `invoice_paid`): `payment_failed` → `past_due`, `dunning_exhausted` → `grace` with `grace_ends_at = now + pinned plan version grace days` (new `subscription.subscriptions.grace_ends_at` column, migration `00004_grace.sql`), a new `subscription.grace` scan job suspends once grace elapses, and `payment_recovered` → `active`. Proration is a service-level `ProrationStrategy` port (modeled like `TaxCalculator`/`PaymentProvider`): `immediate_prorated` (default) issues a one-line prorated invoice now, `credit_next_invoice` persists a `billing.pending_prorations` row that the next `Issue()` drains into a proration line, `none` bills nothing; selected via `BILLING_PRORATION_STRATEGY`, invoked by billing's idempotent `subscription.plan_changed` consumer. Proration is exact integer minor units: `(newUnit-oldUnit) * remainingDays / totalDays`, truncated toward zero (documented, table-tested). Both new billing consumers/jobs only run when `BILLING_DISABLED=false`. Also raised the pgx pool `MaxConns` floor to 16 (`platform/postgres`): the runner holds one advisory-lock connection per concurrently-due job, and the added grace + dunning jobs would starve the pgx default of `max(4, numCPU)` on low-core hosts. No sqlc drift (billing/subscription migrations don't drive sqlc). Frontend: none (event-driven; no new HTTP endpoints). Billing (Milestone 3) is now complete.
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

### T-030 · Postgres Row-Level Security (defense-in-depth tenant isolation) · **L**
**Depends on**: T-010 (tenant context), T-003 (UnitOfWork). **Spec**: [`docs/RUNNING.md`](./RUNNING.md) §7.
**Context**: today tenant isolation is enforced in the app layer (every tenant-scoped repository query filters by the context `tenant_id`). This task adds Postgres RLS as a second, database-enforced layer. The least-privilege roles already exist (`deploy/db/init`): the runtime `entitlements_app` role has **no `BYPASSRLS`**, so policies will bind it.
**Deliverables**:
- A per-transaction tenant GUC: the `UnitOfWork` sets `SET LOCAL app.tenant_id = <ctx tenant>` at transaction start for tenant-scoped work; system-context transactions (`authctx.IsSystem`) skip it. Audit that **every** tenant-scoped read runs inside a transaction (some currently use the ambient pool directly) — route them through the UoW or a read helper that sets the GUC.
- A migration per module enabling `ROW LEVEL SECURITY` + `FORCE ROW LEVEL SECURITY` on every tenant-scoped table, with a `tenant_isolation` policy (`USING` + `WITH CHECK` on `tenant_id = current_setting('app.tenant_id')::uuid`). Platform-shared/tenant-less tables are exempt.
- A bypass path for legitimately cross-tenant work: migrations run as the `BYPASSRLS` owner; background jobs (renewals/trials) that scan all tenants use a system role or `authctx.WithSystemContext` that skips the GUC. Document which operations bypass and why.
- Config flag `RLS_ENABLED` (default off → on once verified) so the switch is reversible.
**Acceptance criteria**: with RLS on, a query for tenant A cannot read/write tenant B's rows even if the app-layer filter is bypassed; cross-tenant jobs and migrations still work; no request path runs a tenant query outside a GUC-bound transaction.
**Expected tests** (integration):
- `TestRLSCrossTenantReadReturnsZeroRows` — set `app.tenant_id` to A, query B's data directly ⇒ 0 rows (DB-enforced, not app-filtered).
- `TestRLSWriteForOtherTenantRejected` — insert/update with a mismatched `tenant_id` ⇒ policy violation.
- `TestSystemContextBypassScansAllTenants` — a system-context/bypass-role scan sees every tenant (renewal job still works).
- `TestForceRLSAppliesToTableOwner` — even a direct owner connection is constrained where `FORCE` is set.
- unit: the UoW sets `SET LOCAL app.tenant_id` for a tenant context and omits it for a system context.

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

### F-003 · Auth screens · **L** *(backend: T-012, T-013 — merged)* · ✅ DONE (PR #32)
**Depends on**: F-001.
**Screens** (adapt `views/auth/basic/*` + `account-settings` security tab): sign-in, sign-up, forgot/reset password (token from URL), email verification (request + confirm + success-mail state), password change, active sessions list + "log out other devices", logout.
**Endpoints**: `POST /api/v1/auth/{register,login,refresh,logout,verify-email/request,verify-email,password/forgot,password/reset,password/change}`, `GET /api/v1/auth/sessions`, `POST /api/v1/auth/sessions/revoke-others`.
**Acceptance criteria**: the full journey register → verify → sign-in → change password → revoke other sessions works against the real API; problem+json details render as form/field errors; 429 (login rate limit) shows a friendly retry message.
**Expected tests** (MSW):
- sign-in success stores the token pair and redirects to `/`; 401 renders the error inline.
- sign-up client-side validation (yup) blocks invalid email/short password before any request.
- reset-password page posts the token from the URL and routes to sign-in on success.
- sessions page lists sessions; revoke-others calls the endpoint and refreshes the list.
**Delivered**: auth screens under `admin/src/views/app/auth/` — `sign-in` (polished: forgot-password link, post-register banner, 429-friendly message), `sign-up` (yup email+password+confirm validation), `forgot-password` (→ `password/forgot`, "if the email exists" success state), `reset-password?token=…` (reads token from URL → `password/reset` → routes to sign-in), `verify-email?token=…` (posts token, success/failure states, adapted from `success-mail`). Account security area at `/account/security` (behind `RequireAuth`): change-password form (`password/change`), active-sessions table with "log out other devices" (`GET /sessions` + `sessions/revoke-others` + refresh), and an authed "resend verification email" action (`verify-email/request`, using the email captured at sign-in). `lib/auth.ts` extended with register/recovery/verify/change/sessions actions (+ `getSessionEmail`); `useAuth` gained `register` and a shared 429→friendly-message mapping; real **logout** wired into the TopBar user menu (`SimpleUserDropdown`); `App.tsx` mounts a `react-toastify` `ToastContainer`. Auth routes added in `routes/index.tsx`; all auth pages redirect already-authenticated users to `/`. Tests (MSW + RTL, all four expected cases): `sign-in/SignIn.test.tsx`, `sign-up/SignUp.test.tsx`, `reset-password/ResetPassword.test.tsx`, `account/security/Sessions.test.tsx`. **Out of scope / follow-ups**: 2FA (theme `two-factor` page is model-ready but no backend endpoint yet); the account area covers security only (personal-info/profile fields deferred to F-004+); no cross-app "unverified email" banner (verification status has no read endpoint — resend lives on the security page).

### F-004 · Tenant screens: onboarding, settings, lifecycle · **M** · ✅ DONE (PR #33) *(backend: T-010, T-011 — merged)*
**Depends on**: F-001.
**Screens**: post-signup create-tenant onboarding (no-tenant empty state), tenant settings (name/slug/settings), danger zone (suspend/reactivate/delete with sweetalert2 confirm), tenant switcher in the TopBar (header mode).
**Endpoints**: `POST /api/v1/tenants`, `GET|PATCH /api/v1/tenants/{id}`, `POST /api/v1/tenants/{id}/{suspend,reactivate}`, `DELETE /api/v1/tenants/{id}`.
**Expected tests** (MSW): create-tenant success routes to the dashboard and subsequent requests carry the tenant header; settings form round-trips a PATCH; delete requires the confirm dialog; switcher swaps the tenant header.

**Delivered**: a persisted **tenant store** (`admin/src/lib/tenant.ts`) holding the "known tenants" list + current tenant id (localStorage, subscribe/get/set like the token store); every created/loaded tenant is remembered. `lib/api.ts` now sends the **current tenant id** as `X-Tenant-ID` in header mode (config `tenantSlug` is only an initial fallback — fixes the F-001 follow-up that sent the slug as the id). Screens under `admin/src/views/app/tenant/`: onboarding empty state (gated by new `RequireTenant`, header mode only), settings (`/tenant` — loads `GET /tenants/{id}`, edits name + settings JSON, PATCH round-trip with toast; slug is read-only since the backend PATCH fixes it), a danger zone (suspend/reactivate/delete via a shared `lib/confirm.ts` sweetalert2 confirm — delete drops the tenant from the store and falls back to onboarding/another tenant), a create-another screen (`/tenant/new`), and a **TenantSwitcher** in the TopBar (header mode; lists known tenants, selecting one sets it current so the next request carries its id, plus a Create action). Routes wired in `routes/index.tsx`; the "Tenant" menu item points at settings. Tests (MSW + RTL): `CreateTenant.test.tsx`, `TenantSettings.test.tsx`, `DangerZone.test.tsx`, `TenantSwitcher.test.tsx`.
**Backend follow-up**: **`GET /api/v1/tenants` (list the current user's tenant memberships) is needed for a full switcher** — the API cannot enumerate a user's tenants today, so the switcher is built over a locally-persisted "known tenants" store (tenants the user created or successfully loaded). Also: the tenant PATCH does not accept a slug change and the tenant response does not echo `settings` (the UI preserves the sent settings locally).

### F-005 · Members & invitations screens · **M** *(backend: T-015 — merged)* · ✅ DONE (PR #44)
**Depends on**: F-001.
**Screens**: members list with remove, invitations list with invite form (email + role) and resend, and the public invitation accept/decline page.
**Endpoints**: `POST|GET /api/v1/tenants/{id}/invitations`, `POST .../invitations/{invId}/{resend,accept,decline}`, `GET /api/v1/tenants/{id}/members`, `DELETE /api/v1/tenants/{id}/members/{userId}`.
**Expected tests** (MSW): members table renders and remove confirms first; invite form validates email; accept page drives the accept endpoint then routes onward (sign-in when anonymous).

**Shipped with a backend change** (amends T-015): `GET .../members` returned only
`{user_id, role, status}`, so the table could only show UUIDs. The tenant module
now stores the email a member was invited by (see T-015's amendment) — no new
port and no dependency on the authentication module.

**Also fixed**: post-sign-in redirect. `RequireAuth` already stashed `state.from`,
but `useAuth.login` hardcoded `navigate('/')` and dropped it, so redirect-back was
dead code. `login` now takes a redirect target and the sign-in page passes
`state.from` — which every guarded route benefits from, not just the invitation
page (it depends on it: an anonymous invitee signs in and lands back on the
invitation).

**Notes**: the invite role picker reads `GET /api/v1/roles` so custom roles are
invitable, falling back to the seeded `owner|admin|member` when roles are
unreadable. That fallback is **load-bearing today, not defensive**: `GET
/api/v1/roles` returns **403 to every user**, including the person doing the
inviting — verified against a live server. The cause is **T-032** (nothing ever
writes a role assignment, so no user holds any permission), not T-031. Without
the fallback the invite form would be broken for everyone. Once T-032 lands, the
picker starts showing the tenant's real roles with no further change.
The accept/decline page is public and takes both ids
(`/invitations/:tenantId/:invId`) because there is no invite token — the backend
authorizes by matching the signed-in user's email. A newly created tenant also
shows an empty members list until someone accepts an invitation (T-031 again).

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

### F-008 · Catalog admin screens · **L** *(backend: T-017, T-018 — merged)* · ✅ DONE (PR #35)
**Depends on**: F-001.
**Screens**: plans list (status, public badge); plan detail with versions timeline; version editor — editable **only while draft** (pricing per cycle in minor units, trial config, grace days, features/limits), publish with immutability warning, archive, public toggle; addons list/detail/versions (entitlement deltas + compatible plans); public pricing preview reusing `pages/pricing` bound to `GET /api/v1/catalog/public`.
**Endpoints**: the full catalog surface (`/api/v1/catalog/plans...`, `/versions/{vid}`, `/public`, `/addons...`, `/addon-versions/{vid}`).
**Expected tests** (MSW): editor fields disabled for a published version; publish requires the confirm; pricing preview renders public plans; addon delta rows validate feature keys and integer amounts (minor units — no floats anywhere in the UI layer either).

### F-009 · Subscription screen · **M** *(backend: T-019, T-020 — merged)* · ✅ DONE (PR #37)
**Depends on**: F-001, F-008 (reuses the plan/pricing picker components).
**Screens**: a "Billing → Subscription" page: current-subscription card (status chip per state, current period, trial countdown, cancel-at-period-end banner); when none, a subscribe flow (public plan + billing-cycle picker); lifecycle actions **rendered from the state machine** so only legal actions show (cancel immediate/at-period-end modal, pause, resume, reactivate). Plus (T-020): a **change-plan flow** (pick plan+cycle; on response show either the new pin or a "scheduled for period end" banner with a cancel-change button) and an **addons section** (attached addons with quantity steppers, attach from compatible published addons, detach with confirm).
**Endpoints**: `POST|GET /api/v1/subscription`, `POST /api/v1/subscription/{cancel,pause,resume,reactivate,change-plan}`, `POST /api/v1/subscription/scheduled-change/cancel`, `POST /api/v1/subscription/addons`, `DELETE /api/v1/subscription/addons/{vid}`.
**Expected tests** (MSW): active shows pause+cancel and not resume; paused shows resume; the cancel modal posts `immediate` true/false correctly; a 409 renders a conflict toast; the empty state shows the plan picker; a downgrade response renders the scheduled-change banner and its cancel button calls the endpoint; addon attach validates quantity and a 400 (incompatible) renders inline.

### F-010 · Entitlements viewer · **S** *(backend: T-022 — merged)* · ✅ DONE (PR #41)
**Depends on**: F-001.
**Screens**: a read-only "Billing → Entitlements" page for the current tenant:
a table of the tenant's effective entitlements (feature key, value, and a source
badge — `plan | addon | override | default`), fed by a single `GET
/api/v1/entitlements` call. Booleans render as on/off chips, limits as numbers,
config/enum as their value. A per-feature drill-in reuses `GET
/api/v1/entitlements/{key}`. No mutation — overrides CRUD is F-paired with T-023.
**Endpoints**: `GET /api/v1/entitlements`, `GET /api/v1/entitlements/{key}`.
**Expected tests** (MSW): the table renders the whole set from one call; the
source badge reflects the winning layer; an empty/no-subscription tenant shows
only `default`-sourced rows.
**Delivered**: a read-only "Billing → Entitlements" surface under
`admin/src/views/app/entitlements/` — `index.tsx` (effective-entitlements table
fed by a SINGLE `GET /api/v1/entitlements` call, rows sorted by feature key),
`detail.tsx` (per-feature drill-in reusing `GET /api/v1/entitlements/{key}`,
shows value + source + override expiry), `api.ts` (typed client + `Source` /
`Entitlement` types), `helpers.ts` (source labels/badge classes, value-kind
inference, `formatValue`; re-uses catalog `errorMessage`/`formatDate`), and
`components/{SourceBadge,ValueCell}.tsx`. Routes `/entitlements` +
`/entitlements/:key` added in `routes/index.tsx` (behind `RequireAuth` /
`RequireTenant`); an "Entitlements" item added to the Billing menu group in
`layouts/components/app-data.ts`. Tests (MSW + RTL) in `Entitlements.test.tsx`:
whole set from one call, per-value-type rendering (bool chip / limit number /
config value), source badge reflects the winning layer, empty/no-subscription
tenant shows only `default` rows, and the drill-in fetches by key + navigates
from a row. **Backend-shape observation** (matters for F-011/F-012): the read
endpoints return only `{ value, source, expires_at? }` per key — NOT the
feature's declared type, so the UI infers how to render a value from its JSON
runtime type (`boolean` → chip, `number` → limit, else config/enum). The map is
nested under `entitlements` for the list and flat (`{ key, value, source,
expires_at? }`) for the single-key read. **For F-011/F-012**: this page and its
`api.ts`/`helpers.ts`/components now exist to extend — F-011 adds the overrides
CRUD surface (refreshing this table on mutation) and F-012 adds the usage/quota
panel; both build on top of this viewer.

### F-011 · Entitlement overrides admin · **S** *(backend: T-023 — merged)*
**Depends on**: F-010.
**Screens**: an "override" management surface on the Entitlements page: list the
tenant's overrides (feature key, value, reason, actor, `expires_at` with a
"time-bound / expired" indicator), a create modal (feature key, value, mandatory
reason, optional expiry), edit and delete with confirm. On any mutation the
effective-entitlements table refreshes so the new winning value + `override`
source badge (and its `expires_at`) show immediately. Surface validation errors
(missing reason) inline; actor is implicit (the signed-in principal).
**Endpoints**: `POST /api/v1/entitlements/overrides`, `GET /api/v1/entitlements/overrides`,
`GET|PATCH|DELETE /api/v1/entitlements/overrides/{id}`.
**Expected tests** (MSW): the list renders overrides with a time-bound badge when
`expires_at` is set; creating without a reason surfaces the 422 inline; a
successful create refreshes the entitlements table to show the `override` source;
delete calls the endpoint and the row reverts to plan/addon/default.

### F-012 · Usage & quota display · **S** *(backend: T-024 — merged)*
**Depends on**: F-010.
**Screens**: a usage panel on the Entitlements page listing each metered (`limit`)
feature with its current `used` / `limit`, period key, and behavior (soft/hard) —
a progress bar per feature, an "over limit" indicator when `used > limit`
(downgrade grace), and an "unlimited" state when no numeric limit applies.
Optionally a "consume"/"release" affordance for demo/testing. Consume surfaces the
`422` quota-exceeded problem+json inline.
**Endpoints**: `GET /api/v1/entitlements/usage`, `GET /api/v1/entitlements/usage/{key}`,
`POST /api/v1/entitlements/consume`, `POST /api/v1/entitlements/release`.
**Expected tests** (MSW): the panel renders used/limit per feature; a feature at or
over its limit shows the over-limit indicator; a consume that returns 422 surfaces
the quota-exceeded message; an unlimited feature renders without a bar.

### F-013 · Billing invoices screen · ✅ **Done** (PR #NN) · **M** *(backend: T-025 — merged)*
**Depends on**: F-009.
**Screens**: a Billing page listing the tenant's invoices (number, status, issued
date, currency, total in minor units formatted to the currency) with a detail view
showing the snapshotted line items (kind, description, key + version, unit price,
quantity, amount) and the subtotal / tax / total breakdown. Actions for the
lifecycle transitions the backend exposes (pay / void / mark uncollectible, shown
per current status) and a "credit note" affordance (amount + mandatory reason) that
lists the invoice's credit notes with their negated amounts. Money is always
rendered from integer minor units; never parse into floats.
**Endpoints**: `GET /api/v1/billing/invoices`, `GET /api/v1/billing/invoices/{id}`,
`POST /api/v1/billing/invoices` (issue), `POST /api/v1/billing/invoices/{id}/{pay,void,uncollectible}`,
`POST|GET /api/v1/billing/invoices/{id}/credit-notes`.
**Expected tests** (MSW): the list renders invoice number/status/total; the detail
view renders snapshotted line items and the subtotal/tax/total; a lifecycle action
updates the shown status; creating a credit note without a reason is blocked and a
created credit note shows a negated amount; another tenant's invoices are not shown.

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
