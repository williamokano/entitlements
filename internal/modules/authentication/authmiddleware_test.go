package authentication_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/authentication"
	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
)

// fakeVerifier maps the literal token "good" to a fixed identity; anything else
// fails verification.
type fakeVerifier struct{ userID uuid.UUID }

func (f fakeVerifier) Verify(raw string) (ports.Identity, error) {
	if raw == "good" {
		return ports.Identity{UserID: f.userID, Email: "u@example.com"}, nil
	}
	return ports.Identity{}, apperr.Unauthorized("bad token")
}

// fakeKeys authenticates the literal key "goodkey".
type fakeKeys struct {
	tenantID uuid.UUID
	keyID    uuid.UUID
}

func (f fakeKeys) AuthenticateAPIKey(_ context.Context, raw string) (ports.MachineIdentity, error) {
	if raw == "goodkey" {
		return ports.MachineIdentity{KeyID: f.keyID, TenantID: f.tenantID, Scopes: []string{"things:write"}}, nil
	}
	return ports.MachineIdentity{}, apperr.Unauthorized("invalid api key")
}

func TestBearerTokenMatrix(t *testing.T) {
	userID := uuid.New()
	mw := authentication.NewAuthMiddleware(fakeVerifier{userID: userID}, fakeKeys{})

	// Probe records the principal the middleware set (if any).
	var gotPrincipal authctx.Principal
	var hadPrincipal bool
	probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPrincipal, hadPrincipal = authctx.PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Behind RequireAuth, absence is a 401; a valid token sets a user principal.
	guarded := mw(authentication.RequireAuth(probe))

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
		wantUser   bool
	}{
		{"valid bearer", "Bearer good", http.StatusOK, true},
		{"expired/garbage bearer", "Bearer nope", http.StatusUnauthorized, false},
		{"absent", "", http.StatusUnauthorized, false},
		{"wrong scheme", "Basic abc", http.StatusUnauthorized, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hadPrincipal = false
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			guarded.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantUser && (!hadPrincipal || gotPrincipal.Kind != authctx.PrincipalUser || gotPrincipal.Subject != userID.String()) {
				t.Fatalf("principal = %+v (had=%v), want user %s", gotPrincipal, hadPrincipal, userID)
			}
		})
	}
}

func TestAuthMiddlewareIsPermissiveWithoutGuard(t *testing.T) {
	mw := authentication.NewAuthMiddleware(fakeVerifier{userID: uuid.New()}, fakeKeys{})

	reached := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if _, ok := authctx.PrincipalFromContext(r.Context()); ok {
			t.Error("unauthenticated request carried a principal")
		}
		w.WriteHeader(http.StatusOK)
	}))

	// No Authorization header: passes through so public routes still work.
	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !reached {
		t.Fatalf("unauthenticated request blocked: status=%d reached=%v", rec.Code, reached)
	}
}

func TestApiKeyPrincipalAndTenantSet(t *testing.T) {
	tenantID := uuid.New()
	keyID := uuid.New()
	mw := authentication.NewAuthMiddleware(fakeVerifier{}, fakeKeys{tenantID: tenantID, keyID: keyID})

	var p authctx.Principal
	var tid uuid.UUID
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ = authctx.PrincipalFromContext(r.Context())
		tid, _ = authctx.TenantID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "ApiKey goodkey")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if p.Kind != authctx.PrincipalMachine || p.Subject != keyID.String() || !p.HasScope("things:write") {
		t.Fatalf("principal = %+v, want machine %s with scope", p, keyID)
	}
	if tid != tenantID {
		t.Fatalf("tenant = %s, want %s (api key binds its tenant)", tid, tenantID)
	}
}
