# SaaS backend skeleton — developer tasks.
# Targets that depend on tooling not yet wired print a notice and succeed, so
# the interface is stable from T-001 onward (see docs/TASKS.md).

GO             ?= go
GOLANGCI_LINT  ?= golangci-lint
SQLC           ?= $(shell command -v sqlc 2>/dev/null || echo $(shell $(GO) env GOPATH)/bin/sqlc)
BIN_DIR        ?= bin
APP            ?= api

# Postgres DSN used by migrate targets (wired in T-003).
DATABASE_URL   ?= postgres://entitlements:entitlements@localhost:5432/entitlements?sslmode=disable

.DEFAULT_GOAL := help

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile all packages and the api binary
	$(GO) build ./...
	$(GO) build -o $(BIN_DIR)/$(APP) ./cmd/api

.PHONY: run
run: ## Run the api server
	$(GO) run ./cmd/api

.PHONY: test
test: ## Run unit tests with the race detector
	$(GO) test -race ./...

.PHONY: test-integration
test-integration: ## Run unit + integration tests (testcontainers; requires Docker)
	$(GO) test -race -tags integration ./...

.PHONY: tidy
tidy: ## Tidy and verify go.mod / go.sum
	$(GO) mod tidy
	$(GO) mod verify

.PHONY: archcheck
archcheck: ## Prove the architecture lint rejects an illegal import
	./scripts/archcheck.sh

.PHONY: lint
lint: ## Run golangci-lint (falls back to go vet if not installed)
	@if command -v $(GOLANGCI_LINT) >/dev/null 2>&1; then \
		$(GOLANGCI_LINT) run ./...; \
	else \
		echo "golangci-lint not found; running 'go vet' instead"; \
		$(GO) vet ./...; \
	fi

.PHONY: generate
generate: ## Generate sqlc code
	$(SQLC) generate

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations for every module
	DATABASE_URL=$(DATABASE_URL) $(GO) run ./cmd/migrate up

.PHONY: migrate-down
migrate-down: ## Roll back the last migration of one module (MODULE=<name>)
	@test -n "$(MODULE)" || (echo "usage: make migrate-down MODULE=<module>"; exit 1)
	DATABASE_URL=$(DATABASE_URL) $(GO) run ./cmd/migrate down $(MODULE)

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
