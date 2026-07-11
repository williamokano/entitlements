package httpx

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/id"
)

// HeaderRequestID is the header used to receive and echo the request ID.
const HeaderRequestID = "X-Request-Id"

// Middleware wraps an http.Handler with additional behavior.
type Middleware func(http.Handler) http.Handler

// DefaultMiddleware returns the standard middleware chain, outermost first:
// request ID, panic recovery, then request logging.
func DefaultMiddleware(logger *slog.Logger, gen id.Generator) []Middleware {
	return []Middleware{
		RequestID(gen),
		Recovery(logger),
		Logging(logger),
	}
}

// RequestID assigns each request a correlation ID: it reuses an inbound
// X-Request-Id header when present, otherwise generates one. The ID is placed
// in the request context and echoed in the response header.
func RequestID(gen id.Generator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := r.Header.Get(HeaderRequestID)
			if rid == "" {
				rid = gen.New().String()
			}
			w.Header().Set(HeaderRequestID, rid)
			next.ServeHTTP(w, r.WithContext(WithRequestID(r.Context(), rid)))
		})
	}
}

// Recovery converts a panic in a downstream handler into a 500 problem+json
// response and logs the panic with its stack trace. The stack is never written
// to the response body.
func Recovery(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.ErrorContext(r.Context(), "panic recovered",
						"panic", rec,
						"stack", string(debug.Stack()),
						"request_id", RequestIDFromContext(r.Context()),
						"method", r.Method,
						"path", r.URL.Path,
					)
					WriteProblem(w, r, apperr.Internal(nil))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Logging emits one structured log line per request with method, path,
// status, request ID, and duration.
func Logging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"request_id", RequestIDFromContext(r.Context()),
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// statusRecorder captures the response status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.wrote = true // implicit 200
	}
	return s.ResponseWriter.Write(b)
}

// Chain applies middlewares to h, with the first middleware outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
