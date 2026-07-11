// Package observability provides the logging, tracing, and metrics baseline:
// a slog logger that attaches correlation IDs from context, and OpenTelemetry
// tracer/meter wiring with an HTTP span-per-request middleware.
package observability

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// LogField extracts an attribute from a context, or reports ok=false when the
// value is absent.
type LogField func(ctx context.Context) (slog.Attr, bool)

// RequestIDField logs the request correlation ID set by httpx middleware.
func RequestIDField(ctx context.Context) (slog.Attr, bool) {
	if id := httpx.RequestIDFromContext(ctx); id != "" {
		return slog.String("request_id", id), true
	}
	return slog.Attr{}, false
}

// TenantIDField logs the resolved tenant ID when present.
func TenantIDField(ctx context.Context) (slog.Attr, bool) {
	if id, ok := authctx.TenantID(ctx); ok {
		return slog.String("tenant_id", id.String()), true
	}
	return slog.Attr{}, false
}

// TraceIDField logs the active OpenTelemetry trace ID when a span is recording.
func TraceIDField(ctx context.Context) (slog.Attr, bool) {
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		return slog.String("trace_id", sc.TraceID().String()), true
	}
	return slog.Attr{}, false
}

// DefaultFields returns the standard correlation fields attached to every log
// line: request ID, tenant ID, and trace ID.
func DefaultFields() []LogField {
	return []LogField{RequestIDField, TenantIDField, TraceIDField}
}

// NewLogger builds a JSON slog logger at the given level whose records are
// enriched with the given context fields. An unrecognized level falls back to
// info.
func NewLogger(w io.Writer, level string, fields ...LogField) *slog.Logger {
	base := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})
	return slog.New(&contextHandler{next: base, fields: fields})
}

// parseLevel maps a level string to a slog.Level, defaulting to info.
func parseLevel(level string) slog.Level {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return slog.LevelInfo
	}
	return lvl
}

// contextHandler enriches each record with attributes pulled from its context.
type contextHandler struct {
	next   slog.Handler
	fields []LogField
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, f := range h.fields {
		if attr, ok := f(ctx); ok {
			r.AddAttrs(attr)
		}
	}
	return h.next.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{next: h.next.WithAttrs(attrs), fields: h.fields}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name), fields: h.fields}
}
