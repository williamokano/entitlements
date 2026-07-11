package httpx

import "context"

type ctxKey int

const requestIDKey ctxKey = iota

// WithRequestID stores the request ID in the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext returns the request ID stored in the context, or "" if
// none is present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}
