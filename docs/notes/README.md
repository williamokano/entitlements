# docs/notes — investigation notes

Raw, un-decided design notes captured for later investigation. **Not** finalized
plan; the source of truth for planned work stays in [`../PLAN.md`](../PLAN.md) and
[`../TASKS.md`](../TASKS.md).

Dumped 2026-07-13 from a design conversation about what this skeleton is meant to be.

- [`skeleton-vision.md`](skeleton-vision.md) — what the project is: a kickstarter
  for any multi-tenant SaaS; the platform-vs-application split; three identity
  layers; the platform feature set; API keys dropped.
- [`deviations-from-vision.md`](deviations-from-vision.md) — where the currently
  built code diverges from that vision (API keys, in-process metering, no public
  register, frontend audience), plus what's already aligned.
- [`open-questions.md`](open-questions.md) — genuine forks not yet decided
  (subscription vs billing, who creates a tenant, application-user auth, the
  machine principal, service boundaries, frontend restructure).
