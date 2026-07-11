package tenant

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/tenant/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// HeaderTenantID is the request header carrying the tenant identifier.
const HeaderTenantID = "X-Tenant-ID"

// resolveConfig holds the resolution middleware's options.
type resolveConfig struct {
	exempt []string
}

// ResolveOption customizes the resolution middleware.
type ResolveOption func(*resolveConfig)

// WithExempt marks path prefixes that do not require a tenant (e.g. health
// probes and tenant-admin routes).
func WithExempt(prefixes ...string) ResolveOption {
	return func(c *resolveConfig) { c.exempt = append(c.exempt, prefixes...) }
}

// ResolveMiddleware resolves the tenant for each request and puts it in the
// context, so downstream tenant-scoped code can read it via authctx.
//
// The tenant identifier is taken, in order, from the auth-layer tenant claim,
// the X-Tenant-ID header (a UUID), or the request subdomain (a slug). It is
// validated through the TenantReader: an unknown or soft-deleted tenant yields
// 404, a non-active (e.g. suspended) tenant yields 403, and a request to a
// non-exempt route without any tenant identifier yields 400. Requests to exempt
// prefixes, and system-privileged contexts, pass through untouched.
func ResolveMiddleware(reader ports.TenantReader, opts ...ResolveOption) httpx.Middleware {
	var cfg resolveConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if authctx.IsSystem(ctx) || cfg.isExempt(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			info, err := resolveTenant(ctx, r, reader)
			if err != nil {
				httpx.WriteProblem(w, r, err)
				return
			}
			if info.Status != ports.StatusActive {
				httpx.WriteProblem(w, r, apperr.Forbidden("tenant is not active"))
				return
			}
			next.ServeHTTP(w, r.WithContext(authctx.WithTenantID(ctx, info.ID)))
		})
	}
}

func (c resolveConfig) isExempt(path string) bool {
	for _, p := range c.exempt {
		if path == p || strings.HasPrefix(path, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

// resolveTenant applies the claim → header → subdomain precedence.
func resolveTenant(ctx context.Context, r *http.Request, reader ports.TenantReader) (ports.TenantInfo, error) {
	if claim, ok := authctx.TenantClaim(ctx); ok {
		return byUUID(ctx, reader, claim)
	}
	if header := r.Header.Get(HeaderTenantID); header != "" {
		return byUUID(ctx, reader, header)
	}
	if slug := subdomainSlug(r.Host); slug != "" {
		return reader.GetBySlug(ctx, slug)
	}
	return ports.TenantInfo{}, apperr.Validation("tenant not specified")
}

func byUUID(ctx context.Context, reader ports.TenantReader, raw string) (ports.TenantInfo, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return ports.TenantInfo{}, apperr.Validation("invalid tenant identifier")
	}
	return reader.GetByID(ctx, id)
}

// subdomainSlug returns the first label of the host as a tenant slug when the
// host looks like <slug>.<domain>.<tld>. It returns "" for bare domains, IPs,
// and the conventional "www" label.
func subdomainSlug(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// An IP literal (e.g. 127.0.0.1) is not a subdomain; its dot-separated
	// octets must not be mistaken for a <slug>.<domain> host.
	if net.ParseIP(host) != nil {
		return ""
	}
	labels := strings.Split(host, ".")
	if len(labels) < 3 {
		return ""
	}
	if labels[0] == "www" {
		return ""
	}
	return labels[0]
}
