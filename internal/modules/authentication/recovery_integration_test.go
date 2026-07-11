//go:build integration

package authentication_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/entitlements/internal/modules/authentication"
	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
)

// recordingSender captures every email the service sends.
type recordingSender struct {
	mu   sync.Mutex
	sent []ports.Email
}

func (s *recordingSender) Send(_ context.Context, msg ports.Email) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, msg)
	return nil
}

func (s *recordingSender) last() (ports.Email, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sent) == 0 {
		return ports.Email{}, false
	}
	return s.sent[len(s.sent)-1], true
}

func (s *recordingSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

// tokenRE extracts the token query parameter from a link in an email body.
var tokenRE = regexp.MustCompile(`token=([^\s&]+)`)

func tokenFromEmail(t *testing.T, body string) string {
	t.Helper()
	m := tokenRE.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no token link in email body: %q", body)
	}
	raw, err := url.QueryUnescape(m[1])
	if err != nil {
		t.Fatalf("unescape token: %v", err)
	}
	return raw
}

func TestEmailSenderPortReceivesRenderedTokenLink(t *testing.T) {
	sender := &recordingSender{}
	mod, _, _ := newModule(t, authentication.WithEmailSender(sender))
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	if status, _ := post(t, srv.URL+"/register", `{"email":"link@example.com","password":"verification-pass"}`); status != http.StatusCreated {
		t.Fatal("register, want 201")
	}
	if status, _ := post(t, srv.URL+"/verify-email/request", `{"email":"link@example.com"}`); status != http.StatusAccepted {
		t.Fatal("verify-email request, want 202")
	}

	msg, ok := sender.last()
	if !ok {
		t.Fatal("email sender received no message")
	}
	if !strings.Contains(msg.TextBody, "/verify-email?token=") || msg.To != "link@example.com" {
		t.Fatalf("email = %+v, want a verify-email link to the user", msg)
	}
}

func TestEmailVerificationFlow(t *testing.T) {
	// Frozen clock so we can push past the token's expiry deterministically.
	frozen := clock.NewFrozen(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	sender := &recordingSender{}
	mod, deps := newModuleWithClock(t, frozen, authentication.WithEmailSender(sender))
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	if status, _ := post(t, srv.URL+"/register", `{"email":"verify@example.com","password":"verification-pass"}`); status != http.StatusCreated {
		t.Fatal("register, want 201")
	}
	if status, _ := post(t, srv.URL+"/verify-email/request", `{"email":"verify@example.com"}`); status != http.StatusAccepted {
		t.Fatal("request verification, want 202")
	}
	msg, _ := sender.last()
	token := tokenFromEmail(t, msg.TextBody)

	// First use verifies (204); user is now marked verified.
	if status, _ := post(t, srv.URL+"/verify-email", `{"token":"`+token+`"}`); status != http.StatusNoContent {
		t.Fatal("first verify, want 204")
	}
	var verifiedAt *time.Time
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT email_verified_at FROM authn.users WHERE email = 'verify@example.com'`).Scan(&verifiedAt); err != nil {
		t.Fatalf("query verified: %v", err)
	}
	if verifiedAt == nil {
		t.Fatal("email_verified_at not set after verification")
	}

	// Second use of the same token is rejected (single-use).
	if status, _ := post(t, srv.URL+"/verify-email", `{"token":"`+token+`"}`); status != http.StatusUnauthorized {
		t.Fatal("second verify, want 401 (single-use)")
	}

	// A fresh token that has expired is rejected.
	if status, _ := post(t, srv.URL+"/verify-email/request", `{"email":"verify@example.com"}`); status != http.StatusAccepted {
		t.Fatal("request 2nd verification, want 202")
	}
	msg2, _ := sender.last()
	expiredToken := tokenFromEmail(t, msg2.TextBody)
	frozen.Advance(48 * time.Hour) // past the 24h verification TTL
	if status, _ := post(t, srv.URL+"/verify-email", `{"token":"`+expiredToken+`"}`); status != http.StatusUnauthorized {
		t.Fatal("expired verify, want 401")
	}
}

func TestRecoveryRequestForUnknownEmailIndistinguishable(t *testing.T) {
	sender := &recordingSender{}
	mod, _, _ := newModule(t, authentication.WithEmailSender(sender))
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	if status, _ := post(t, srv.URL+"/register", `{"email":"known@example.com","password":"known-user-pass"}`); status != http.StatusCreated {
		t.Fatal("register, want 201")
	}

	knownStatus, knownBody := post(t, srv.URL+"/password/forgot", `{"email":"known@example.com"}`)
	unknownStatus, unknownBody := post(t, srv.URL+"/password/forgot", `{"email":"ghost@example.com"}`)
	if knownStatus != http.StatusAccepted || unknownStatus != http.StatusAccepted {
		t.Fatalf("known=%d unknown=%d, want both 202", knownStatus, unknownStatus)
	}
	if knownBody != unknownBody {
		t.Fatalf("response bodies differ (enumeration leak): %q vs %q", knownBody, unknownBody)
	}
	// Only the known email actually produced a message.
	if sender.count() != 1 {
		t.Fatalf("emails sent = %d, want exactly 1 (only for the known address)", sender.count())
	}
}

func TestRecoveryFlowResetsPasswordAndRevokesAllSessions(t *testing.T) {
	sender := &recordingSender{}
	mod, _, _ := newModule(t, authentication.WithEmailSender(sender))
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	const email = "reset@example.com"
	const oldPass = "old-password-value"
	if status, _ := post(t, srv.URL+"/register", `{"email":"`+email+`","password":"`+oldPass+`"}`); status != http.StatusCreated {
		t.Fatal("register, want 201")
	}
	// Two live sessions.
	_, b1 := post(t, srv.URL+"/login", `{"email":"`+email+`","password":"`+oldPass+`"}`)
	_, b2 := post(t, srv.URL+"/login", `{"email":"`+email+`","password":"`+oldPass+`"}`)
	s1, s2 := decodeTokens(t, b1), decodeTokens(t, b2)

	// Request + perform reset.
	if status, _ := post(t, srv.URL+"/password/forgot", `{"email":"`+email+`"}`); status != http.StatusAccepted {
		t.Fatal("forgot, want 202")
	}
	msg, _ := sender.last()
	token := tokenFromEmail(t, msg.TextBody)
	const newPass = "brand-new-password"
	if status, _ := post(t, srv.URL+"/password/reset", `{"token":"`+token+`","password":"`+newPass+`"}`); status != http.StatusNoContent {
		t.Fatal("reset, want 204")
	}

	// Old password no longer works; new one does.
	if status, _ := post(t, srv.URL+"/login", `{"email":"`+email+`","password":"`+oldPass+`"}`); status != http.StatusUnauthorized {
		t.Fatal("login with old password, want 401")
	}
	if status, _ := post(t, srv.URL+"/login", `{"email":"`+email+`","password":"`+newPass+`"}`); status != http.StatusOK {
		t.Fatal("login with new password, want 200")
	}
	// Both prior sessions are dead (all sessions revoked on reset).
	for i, s := range []tokenResponse{s1, s2} {
		if status, _ := post(t, srv.URL+"/refresh", `{"refresh_token":"`+s.RefreshToken+`"}`); status != http.StatusUnauthorized {
			t.Fatalf("prior session %d refresh, want 401", i)
		}
	}
}

func TestPasswordChangeRequiresCurrentPassword(t *testing.T) {
	mod, bus, deps := newModule(t)
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	var changed int
	var mu sync.Mutex
	bus.Subscribe(ports.EventPasswordChanged, func(_ context.Context, _ events.Event) error {
		mu.Lock()
		changed++
		mu.Unlock()
		return nil
	})
	relay := events.NewRelay(deps.Pool, bus, deps.Clock, deps.Logger, events.RelayConfig{})

	const email = "change@example.com"
	const pass = "current-password-1"
	if status, _ := post(t, srv.URL+"/register", `{"email":"`+email+`","password":"`+pass+`"}`); status != http.StatusCreated {
		t.Fatal("register, want 201")
	}
	_, body := post(t, srv.URL+"/login", `{"email":"`+email+`","password":"`+pass+`"}`)
	tokens := decodeTokens(t, body)

	// Wrong current password → 401.
	if status := postAuth(t, srv.URL+"/password/change", tokens.AccessToken, `{"current_password":"WRONG","new_password":"next-password-2"}`); status != http.StatusUnauthorized {
		t.Fatalf("change with wrong current password = %d, want 401", status)
	}
	// Correct current password → 204.
	if status := postAuth(t, srv.URL+"/password/change", tokens.AccessToken, `{"current_password":"`+pass+`","new_password":"next-password-2"}`); status != http.StatusNoContent {
		t.Fatalf("change with correct password, want 204")
	}

	// PasswordChanged event emitted and an audit entry written.
	if _, err := relay.ProcessBatch(context.Background()); err != nil {
		t.Fatalf("relay: %v", err)
	}
	mu.Lock()
	got := changed
	mu.Unlock()
	if got < 1 {
		t.Fatalf("password.changed events = %d, want >= 1", got)
	}
	var auditCount int
	if err := deps.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM platform.audit_log WHERE action = 'authn.password.changed'`).Scan(&auditCount); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditCount < 1 {
		t.Fatal("no audit entry for password change")
	}
}

func TestSessionListAndRevokeOthersKeepsCurrent(t *testing.T) {
	mod, _, _ := newModule(t)
	srv := httptest.NewServer(mod.Handler())
	defer srv.Close()

	const email = "sessions@example.com"
	const pass = "sessions-password"
	if status, _ := post(t, srv.URL+"/register", `{"email":"`+email+`","password":"`+pass+`"}`); status != http.StatusCreated {
		t.Fatal("register, want 201")
	}
	// Three sessions on three "devices".
	var sessions []tokenResponse
	for i := 0; i < 3; i++ {
		_, b := post(t, srv.URL+"/login", `{"email":"`+email+`","password":"`+pass+`"}`)
		sessions = append(sessions, decodeTokens(t, b))
	}
	current := sessions[0]

	// List shows 3 sessions.
	var listed struct {
		Sessions []struct {
			Current bool `json:"current"`
		} `json:"sessions"`
	}
	body := getAuth(t, srv.URL+"/sessions", current.AccessToken)
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if len(listed.Sessions) != 3 {
		t.Fatalf("listed %d sessions, want 3", len(listed.Sessions))
	}

	// Revoke others from the current session.
	if status := postAuth(t, srv.URL+"/sessions/revoke-others", current.AccessToken, ``); status != http.StatusNoContent {
		t.Fatalf("revoke-others, want 204")
	}

	// Current session still refreshes; the others are dead.
	if status, _ := post(t, srv.URL+"/refresh", `{"refresh_token":"`+current.RefreshToken+`"}`); status != http.StatusOK {
		t.Fatal("current session refresh after revoke-others, want 200")
	}
	for i, s := range sessions[1:] {
		if status, _ := post(t, srv.URL+"/refresh", `{"refresh_token":"`+s.RefreshToken+`"}`); status != http.StatusUnauthorized {
			t.Fatalf("other session %d refresh, want 401", i+1)
		}
	}
}

// --- auth'd HTTP helpers ---

func postAuth(t *testing.T, url, accessToken, body string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func getAuth(t *testing.T, url, accessToken string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
