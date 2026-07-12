# ---------------------------------------------------------------------------
# Build stage — compile a static api binary. Migrations are embedded, so the
# binary is self-contained and applies them on startup.
# ---------------------------------------------------------------------------
FROM golang:1.24-alpine AS build
WORKDIR /src

# Cache modules against go.mod/go.sum before copying the full source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
RUN go build -trimpath -o /out/api ./cmd/api

# ---------------------------------------------------------------------------
# Runtime stage — a scratch image with just the static binary. No package
# manager (the alpine CDN is not always reachable), nothing else to attack.
# ---------------------------------------------------------------------------
FROM scratch AS runtime
COPY --from=build /out/api /api

EXPOSE 8080
ENTRYPOINT ["/api"]
