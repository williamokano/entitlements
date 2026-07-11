package httpx

import (
	"net/http"
	"strings"
)

// Router is a thin wrapper over http.ServeMux that applies a middleware chain
// to every route and supports mounting module handlers under a path prefix.
type Router struct {
	mux *http.ServeMux
	mws []Middleware
}

// NewRouter returns a Router whose routes are wrapped by mws (first outermost).
func NewRouter(mws ...Middleware) *Router {
	return &Router{mux: http.NewServeMux(), mws: mws}
}

// Handle registers a handler for a pattern (Go 1.22 method-aware syntax, e.g.
// "GET /tenants/{id}").
func (r *Router) Handle(pattern string, h http.Handler) {
	r.mux.Handle(pattern, h)
}

// HandleFunc registers a handler function for a pattern.
func (r *Router) HandleFunc(pattern string, h http.HandlerFunc) {
	r.mux.Handle(pattern, h)
}

// Mount attaches a module handler under prefix, stripping the prefix before
// the handler sees the request. A request to "<prefix>/foo" reaches h as
// "/foo", so modules register prefix-relative routes and the composition root
// decides where they live.
//
// Both the exact prefix and its subtree are registered, so a module's
// collection endpoint at its root (e.g. "POST /") is reachable at "<prefix>"
// itself without a trailing-slash redirect that would drop the request body.
func (r *Router) Mount(prefix string, h http.Handler) {
	prefix = "/" + strings.Trim(prefix, "/")
	stripped := http.StripPrefix(prefix, ensureLeadingSlash(h))
	r.mux.Handle(prefix, stripped)
	r.mux.Handle(prefix+"/", stripped)
}

// ensureLeadingSlash restores the "/" that StripPrefix removes when the request
// path equals the mount prefix exactly, so the module's root routes match.
func ensureLeadingSlash(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		h.ServeHTTP(w, r)
	})
}

// Handler returns the composed http.Handler: the mux wrapped in the router's
// middleware chain.
func (r *Router) Handler() http.Handler {
	return Chain(r.mux, r.mws...)
}
