package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
	"github.com/williamokano/entitlements/internal/platform/observability"
)

func TestSlogAttachesRequestAndTenantIDsFromContext(t *testing.T) {
	var buf bytes.Buffer
	logger := observability.NewLogger(&buf, "info",
		observability.RequestIDField, observability.TenantIDField)

	tenant := uuid.New()
	ctx := authctx.WithTenantID(httpx.WithRequestID(context.Background(), "req-123"), tenant)

	logger.InfoContext(ctx, "something happened")

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line is not JSON: %v (%s)", err, buf.String())
	}
	if line["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want req-123", line["request_id"])
	}
	if line["tenant_id"] != tenant.String() {
		t.Errorf("tenant_id = %v, want %s", line["tenant_id"], tenant)
	}
}

func TestHTTPMiddlewareCreatesSpanPerRequest(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	h := observability.TracingMiddleware(tp)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/widgets", nil))

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	span := spans[0]

	attrs := map[string]string{}
	var statusCode int64
	for _, a := range span.Attributes() {
		switch a.Key {
		case "http.request.method", "http.route":
			attrs[string(a.Key)] = a.Value.AsString()
		case "http.response.status_code":
			statusCode = a.Value.AsInt64()
		}
	}
	if attrs["http.request.method"] != http.MethodGet {
		t.Errorf("span method = %q, want GET", attrs["http.request.method"])
	}
	if attrs["http.route"] != "/widgets" {
		t.Errorf("span route = %q, want /widgets", attrs["http.route"])
	}
	if statusCode != int64(http.StatusTeapot) {
		t.Errorf("span status_code = %d, want %d", statusCode, http.StatusTeapot)
	}
}

func TestUnknownLogLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := observability.NewLogger(&buf, "nonsense")

	ctx := context.Background()
	if !logger.Enabled(ctx, slog.LevelInfo) {
		t.Error("info not enabled after unknown level; want info fallback")
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		t.Error("debug enabled after unknown level; want info fallback (debug off)")
	}
}
