package httpx_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/httpx"
	"github.com/williamokano/entitlements/internal/platform/id"
)

// buildHandler wraps h in the standard middleware chain with the given logger.
func buildHandler(logger *slog.Logger, h http.Handler) http.Handler {
	return httpx.Chain(h, httpx.DefaultMiddleware(logger, id.UUIDv7{})...)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestRequestIDGeneratedWhenAbsent(t *testing.T) {
	var seen string
	h := buildHandler(discardLogger(), http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFromContext(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if seen == "" {
		t.Fatal("handler saw empty request ID in context")
	}
	if got := rec.Header().Get(httpx.HeaderRequestID); got != seen {
		t.Fatalf("response header %q = %q, want %q", httpx.HeaderRequestID, got, seen)
	}
}

func TestRequestIDEchoedWhenPresent(t *testing.T) {
	const want = "inbound-correlation-id"
	var seen string
	h := buildHandler(discardLogger(), http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(httpx.HeaderRequestID, want)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if seen != want {
		t.Fatalf("context request ID = %q, want %q", seen, want)
	}
	if got := rec.Header().Get(httpx.HeaderRequestID); got != want {
		t.Fatalf("echoed header = %q, want %q", got, want)
	}
}

func TestRecoveryReturns500ProblemJSON(t *testing.T) {
	const secret = "super secret internal detail"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) {
		panic(secret)
	})
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(buildHandler(discardLogger(), mux))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/panic")
	if err != nil {
		t.Fatalf("GET /panic: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != httpx.ProblemContentType {
		t.Fatalf("content-type = %q, want %q", ct, httpx.ProblemContentType)
	}

	var p httpx.Problem
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("body is not valid problem+json: %v (%s)", err, body)
	}
	if p.RequestID == "" {
		t.Error("problem is missing request_id")
	}
	if s := string(body); strings.Contains(s, secret) || strings.Contains(s, "goroutine") {
		t.Fatalf("response body leaked internal detail or stack: %s", s)
	}

	// The server keeps serving after a recovered panic.
	resp2, err := http.Get(srv.URL + "/ok")
	if err != nil {
		t.Fatalf("GET /ok after panic: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("post-panic request status = %d, want 200", resp2.StatusCode)
	}
}

func TestErrorMapperTable(t *testing.T) {
	const secret = "internal cause not for clients"
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not_found", apperr.NotFound("tenant not found"), http.StatusNotFound},
		{"validation", apperr.Validation("email is required"), http.StatusBadRequest},
		{"conflict", apperr.Conflict("slug already taken"), http.StatusConflict},
		{"unauthorized", apperr.Unauthorized("login required"), http.StatusUnauthorized},
		{"forbidden", apperr.Forbidden("insufficient role"), http.StatusForbidden},
		{"internal_apperr", apperr.Internal(errInternal(secret)), http.StatusInternalServerError},
		{"plain_error", errInternal(secret), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/resource", nil)
			httpx.WriteProblem(rec, req, tc.err)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != httpx.ProblemContentType {
				t.Fatalf("content-type = %q, want %q", ct, httpx.ProblemContentType)
			}

			var p httpx.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("invalid problem+json: %v", err)
			}
			if p.Status != tc.wantStatus {
				t.Errorf("problem.status = %d, want %d", p.Status, tc.wantStatus)
			}
			if p.Title != http.StatusText(tc.wantStatus) {
				t.Errorf("problem.title = %q, want %q", p.Title, http.StatusText(tc.wantStatus))
			}
			if tc.wantStatus == http.StatusInternalServerError && strings.Contains(rec.Body.String(), secret) {
				t.Errorf("internal error leaked cause to client: %s", rec.Body.String())
			}
		})
	}
}

func TestRequestLoggingIncludesMethodPathStatusRequestID(t *testing.T) {
	capH := &capturingHandler{}
	logger := slog.New(capH)

	h := buildHandler(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/widgets", nil))

	r, ok := capH.find("http request")
	if !ok {
		t.Fatal("no 'http request' log record emitted")
	}
	attrs := r.attrs
	if attrs["method"] != "GET" {
		t.Errorf("method = %v, want GET", attrs["method"])
	}
	if attrs["path"] != "/widgets" {
		t.Errorf("path = %v, want /widgets", attrs["path"])
	}
	if attrs["status"] != int64(http.StatusTeapot) {
		t.Errorf("status = %v, want %d", attrs["status"], http.StatusTeapot)
	}
	if attrs["request_id"] == nil || attrs["request_id"] == "" {
		t.Errorf("request_id missing from log record")
	}
}

func TestRouterCompositionMountsUnderPrefix(t *testing.T) {
	module := http.NewServeMux()
	module.HandleFunc("GET /things/{id}", func(w http.ResponseWriter, r *http.Request) {
		// The module sees the prefix-stripped path.
		_, _ = io.WriteString(w, r.URL.Path)
	})

	router := httpx.NewRouter()
	router.Mount("/api/v1", module)
	srv := httptest.NewServer(router.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/things/42")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := string(body); got != "/things/42" {
		t.Fatalf("module saw path %q, want /things/42 (prefix stripped)", got)
	}
}

func TestRouterMountServesCollectionAtPrefixRoot(t *testing.T) {
	// A module whose collection endpoint is at its root ("POST /") must be
	// reachable at the bare prefix without a trailing-slash redirect that would
	// drop the request body.
	module := http.NewServeMux()
	module.HandleFunc("POST /", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	module.HandleFunc("GET /{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.PathValue("id"))
	})

	router := httpx.NewRouter()
	router.Mount("/api/v1/things", module)
	srv := httptest.NewServer(router.Handler())
	defer srv.Close()

	// POST to the bare prefix — no redirect, body preserved.
	resp, err := http.Post(srv.URL+"/api/v1/things", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST bare prefix: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/things = %d, want 201 (no redirect)", resp.StatusCode)
	}

	// Sub-resource still routes with the prefix stripped.
	got, err := http.Get(srv.URL + "/api/v1/things/42")
	if err != nil {
		t.Fatalf("GET sub-resource: %v", err)
	}
	body, _ := io.ReadAll(got.Body)
	_ = got.Body.Close()
	if string(body) != "42" {
		t.Fatalf("sub-resource id = %q, want 42", body)
	}
}

func TestGracefulShutdownDrainsInflightRequests(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release // block until the test lets the request finish
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "done")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httpx.NewServer("", h, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ctx, ln) }()

	type result struct {
		status int
		body   string
		err    error
	}
	resCh := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/slow")
		if err != nil {
			resCh <- result{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		resCh <- result{status: resp.StatusCode, body: string(b)}
	}()

	<-started        // request is in-flight inside the handler
	cancel()         // trigger graceful shutdown
	time.Sleep(20 * time.Millisecond)
	close(release)   // let the in-flight request complete

	select {
	case res := <-resCh:
		if res.err != nil {
			t.Fatalf("in-flight request failed during shutdown: %v", res.err)
		}
		if res.status != http.StatusOK || res.body != "done" {
			t.Fatalf("in-flight request = %d %q, want 200 \"done\"", res.status, res.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight request did not complete")
	}

	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after shutdown")
	}
}

// errInternal is a plain error carrying a detail that must never reach clients.
type errInternal string

func (e errInternal) Error() string { return string(e) }

// capturingHandler is a slog.Handler that records emitted records for tests.
type capturingHandler struct {
	mu      sync.Mutex
	records []capturedRecord
}

type capturedRecord struct {
	msg   string
	attrs map[string]any
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	rec := capturedRecord{msg: r.Message, attrs: map[string]any{}}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, rec)
	h.mu.Unlock()
	return nil
}

func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler      { return h }

func (h *capturingHandler) find(msg string) (capturedRecord, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.msg == msg {
			return r, true
		}
	}
	return capturedRecord{}, false
}
