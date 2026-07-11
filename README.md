# entitlements

A reusable **Go backend skeleton for SaaS products**: a modular monolith built
with DDD + hexagonal architecture. It ships seven modules — tenant,
authentication, authorization, catalog, subscription, entitlements, and
billing — so a new SaaS can start from here instead of from scratch.

See [`docs/PLAN.md`](docs/PLAN.md) for the architecture and
[`docs/TASKS.md`](docs/TASKS.md) for the implementation task breakdown.

## Status

Bootstrapping (task **T-001**). The repository currently provides the module
skeleton, build tooling, and a minimal API server exposing health probes. The
platform kernel and modules are implemented in subsequent tasks.

## Development

### Prerequisites

- Go 1.24+
- Docker + Docker Compose (for Postgres)
- [`golangci-lint`](https://golangci-lint.run/) v2 (optional locally; `make
  lint` falls back to `go vet` if it is not installed)

### Getting started

```bash
# Start Postgres (used from T-003 onward)
docker compose up -d

# Build, lint, and test
make build
make lint
make test

# Run the API server
make run

# Check it is alive
curl -s localhost:8080/healthz
# {"status":"ok"}
```

The server listens on `:8080` by default; override with the `PORT`
environment variable.

### Common Make targets

| Target         | Description                                          |
| -------------- | ---------------------------------------------------- |
| `make build`   | Compile all packages and the `api` binary            |
| `make run`     | Run the API server                                   |
| `make test`    | Run unit tests with the race detector                |
| `make test-integration` | Run unit + integration tests (testcontainers; requires Docker) |
| `make lint`    | Run golangci-lint (falls back to `go vet`)           |
| `make tidy`    | Tidy and verify `go.mod` / `go.sum`                  |
| `make generate`| Generate sqlc code (wired in T-003)                  |
| `make migrate-up` / `migrate-down` | Apply / roll back migrations (wired in T-003) |

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
