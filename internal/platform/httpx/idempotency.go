package httpx

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// HeaderIdempotencyKey is the request header carrying the client's idempotency
// key. HeaderIdempotencyReplayed marks a replayed (cached) response.
const (
	HeaderIdempotencyKey      = "Idempotency-Key"
	HeaderIdempotencyReplayed = "Idempotency-Replayed"
)

// Idempotency returns middleware that makes mutating requests carrying an
// Idempotency-Key header execute at most once per (tenant, key, route) within
// ttl. The first request runs the handler and stores its response; a retry
// replays the stored response without re-executing; a concurrent duplicate
// (the first is still in flight) gets 409. Requests without the header, and
// non-mutating methods, pass straight through.
func Idempotency(pool *pgxpool.Pool, clk clock.Clock, ttl time.Duration) Middleware {
	uow := postgres.NewUnitOfWork(pool)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(HeaderIdempotencyKey)
			if key == "" || !isMutatingMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			tenant, _ := authctx.TenantID(r.Context()) // uuid.Nil when absent
			route := r.Method + " " + r.URL.Path

			reserved, stored, err := reserveIdempotencyKey(r.Context(), uow, pool, clk, ttl, tenant, key, route)
			if err != nil {
				WriteProblem(w, r, apperr.Internal(err))
				return
			}

			if !reserved {
				if stored.completed {
					writeStoredResponse(w, stored)
					return
				}
				// The original request is still in flight.
				WriteProblem(w, r, apperr.Conflict("a request with this Idempotency-Key is already in progress"))
				return
			}

			// We reserved the key: run the handler, capture the response, store
			// it, then flush it to the client.
			rec := &responseCapture{header: make(http.Header), status: http.StatusOK}
			next.ServeHTTP(rec, r)

			if err := completeIdempotencyKey(r.Context(), pool, tenant, key, route, rec); err != nil {
				WriteProblem(w, r, apperr.Internal(err))
				return
			}
			flushCapturedResponse(w, rec)
		})
	}
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// storedResponse is a previously captured response for an idempotency key.
type storedResponse struct {
	completed   bool
	status      int
	contentType string
	body        []byte
}

// reserveIdempotencyKey atomically claims the key for this request. It returns
// reserved=true when this request is the first (and must run the handler);
// otherwise it returns the existing record so the caller can replay it or
// reject the concurrent duplicate. An expired record is discarded first so the
// key can be reused after its TTL.
func reserveIdempotencyKey(ctx context.Context, uow *postgres.UnitOfWork, pool *pgxpool.Pool, clk clock.Clock, ttl time.Duration, tenant uuid.UUID, key, route string) (reserved bool, stored storedResponse, err error) {
	now := clk.Now().UTC()
	expires := now.Add(ttl)

	err = uow.Do(ctx, func(ctx context.Context) error {
		q := postgres.Q(ctx, pool)

		if _, e := q.Exec(ctx,
			`DELETE FROM platform.idempotency_keys
			 WHERE tenant_id = $1 AND idem_key = $2 AND route = $3 AND expires_at <= $4`,
			tenant, key, route, now); e != nil {
			return e
		}

		tag, e := q.Exec(ctx,
			`INSERT INTO platform.idempotency_keys (tenant_id, idem_key, route, completed, created_at, expires_at)
			 VALUES ($1, $2, $3, false, $4, $5)
			 ON CONFLICT (tenant_id, idem_key, route) DO NOTHING`,
			tenant, key, route, now, expires)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 1 {
			reserved = true
			return nil
		}

		var (
			completed bool
			status    *int
			ctype     *string
			body      []byte
		)
		if e := q.QueryRow(ctx,
			`SELECT completed, status_code, content_type, body
			 FROM platform.idempotency_keys
			 WHERE tenant_id = $1 AND idem_key = $2 AND route = $3`,
			tenant, key, route).Scan(&completed, &status, &ctype, &body); e != nil {
			return e
		}
		stored = storedResponse{completed: completed, body: body}
		if status != nil {
			stored.status = *status
		}
		if ctype != nil {
			stored.contentType = *ctype
		}
		return nil
	})
	return reserved, stored, err
}

// completeIdempotencyKey records the captured response so future retries can
// replay it. It uses a cancel-free context so the record persists even if the
// client has disconnected.
func completeIdempotencyKey(ctx context.Context, pool *pgxpool.Pool, tenant uuid.UUID, key, route string, rec *responseCapture) error {
	_, err := pool.Exec(context.WithoutCancel(ctx),
		`UPDATE platform.idempotency_keys
		 SET completed = true, status_code = $4, content_type = $5, body = $6
		 WHERE tenant_id = $1 AND idem_key = $2 AND route = $3`,
		tenant, key, route, rec.status, rec.header.Get("Content-Type"), rec.buf.Bytes())
	return err
}

func writeStoredResponse(w http.ResponseWriter, s storedResponse) {
	if s.contentType != "" {
		w.Header().Set("Content-Type", s.contentType)
	}
	w.Header().Set(HeaderIdempotencyReplayed, "true")
	w.WriteHeader(s.status)
	_, _ = w.Write(s.body)
}

func flushCapturedResponse(w http.ResponseWriter, rec *responseCapture) {
	for k, vs := range rec.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rec.status)
	_, _ = w.Write(rec.buf.Bytes())
}

// responseCapture buffers a handler's response so it can be stored and then
// written to the real client.
type responseCapture struct {
	header http.Header
	status int
	wrote  bool
	buf    bytes.Buffer
}

func (c *responseCapture) Header() http.Header { return c.header }

func (c *responseCapture) WriteHeader(code int) {
	if !c.wrote {
		c.status = code
		c.wrote = true
	}
}

func (c *responseCapture) Write(p []byte) (int, error) {
	c.wrote = true
	return c.buf.Write(p)
}
