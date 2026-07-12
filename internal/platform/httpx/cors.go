package httpx

import (
	"net/http"
	"strings"
)

// CORS response header names.
const (
	headerACAllowOrigin   = "Access-Control-Allow-Origin"
	headerACAllowMethods  = "Access-Control-Allow-Methods"
	headerACAllowHeaders  = "Access-Control-Allow-Headers"
	headerACExposeHeaders = "Access-Control-Expose-Headers"
	headerACMaxAge        = "Access-Control-Max-Age"
	headerACRequestMethod = "Access-Control-Request-Method"
)

// Request/response headers the browser must be allowed to send and read. The
// API authenticates with a bearer token (not cookies), so credentials mode is
// not enabled and `*` is a valid allow-origin.
const (
	corsAllowMethods  = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowHeaders  = "Authorization, Content-Type, " + HeaderRequestID + ", X-Tenant-ID, " + HeaderIdempotencyKey
	corsExposeHeaders = HeaderRequestID + ", " + HeaderIdempotencyReplayed
	corsMaxAge        = "600"
)

// CORS allows the configured browser origins to call the API cross-origin. The
// admin SPA is served from a different origin than the API, so without this the
// browser blocks every request. It short-circuits preflight (OPTIONS) requests
// with 204 and sets the allow headers on all responses for an allowed origin.
//
// allowedOrigins are matched exactly; a single "*" entry allows any origin.
// Unlisted origins get no CORS headers (the browser then blocks the response),
// so this is not a wildcard by default.
func CORS(allowedOrigins []string) Middleware {
	allowAny := false
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(o)
		if o == "*" {
			allowAny = true
		}
		if o != "" {
			allowed[o] = true
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAny || allowed[origin]) {
				if allowAny {
					w.Header().Set(headerACAllowOrigin, "*")
				} else {
					// Echo the specific origin and vary on it so caches don't
					// serve one origin's response to another.
					w.Header().Set(headerACAllowOrigin, origin)
					w.Header().Add("Vary", "Origin")
				}
				w.Header().Set(headerACAllowMethods, corsAllowMethods)
				w.Header().Set(headerACAllowHeaders, corsAllowHeaders)
				w.Header().Set(headerACExposeHeaders, corsExposeHeaders)
				w.Header().Set(headerACMaxAge, corsMaxAge)
			}

			// Preflight: a browser OPTIONS carrying Access-Control-Request-Method.
			// Answer it here — it must not reach auth/tenant/handler layers.
			if r.Method == http.MethodOptions && r.Header.Get(headerACRequestMethod) != "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
