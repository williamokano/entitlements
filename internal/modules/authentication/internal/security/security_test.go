package security_test

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/authentication/internal/security"
)

func TestArgon2idHashVerifyAndRejectsWrongPassword(t *testing.T) {
	const pw = "correct horse battery staple"
	hash, err := security.HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// The hash is not the plaintext and is in the standard encoded argon2id form.
	if strings.Contains(hash, pw) {
		t.Fatal("hash contains the plaintext password")
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("hash = %q, want standard argon2id encoding", hash)
	}

	// The same password re-hashes to a different value (random salt).
	hash2, _ := security.HashPassword(pw)
	if hash == hash2 {
		t.Fatal("two hashes of the same password are identical (salt not random)")
	}

	// Correct password verifies; wrong password does not.
	ok, err := security.VerifyPassword(pw, hash)
	if err != nil || !ok {
		t.Fatalf("VerifyPassword(correct) = (%v, %v), want (true, nil)", ok, err)
	}
	ok, err = security.VerifyPassword("wrong password", hash)
	if err != nil || ok {
		t.Fatalf("VerifyPassword(wrong) = (%v, %v), want (false, nil)", ok, err)
	}

	// A tampered hash is rejected as malformed.
	if _, err := security.VerifyPassword(pw, "not-a-hash"); err == nil {
		t.Fatal("VerifyPassword on malformed hash = nil error, want error")
	}
}

func TestJWTSignedWithKidAndVerifies(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	const (
		kid    = "key-1"
		issuer = "entitlements"
	)
	signer := security.NewSigner(kid, priv, issuer, 15*time.Minute)
	verifier := security.NewVerifier(map[string]ed25519.PublicKey{kid: pub}, issuer)

	sub := uuid.New()
	sid := uuid.New()
	issuedAt := time.Now()
	raw, err := signer.Sign(sub, sid, "alice@example.com", issuedAt)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	claims, err := verifier.Verify(raw)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != sub.String() {
		t.Fatalf("sub = %q, want %q", claims.Subject, sub)
	}
	if claims.SessionID != sid.String() {
		t.Fatalf("sid = %q, want %q", claims.SessionID, sid)
	}
	if claims.Email != "alice@example.com" {
		t.Fatalf("email = %q, want alice@example.com", claims.Email)
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatal("iat/exp claims missing")
	}

	// An expired token is rejected.
	expiredSigner := security.NewSigner(kid, priv, issuer, -time.Minute)
	expired, _ := expiredSigner.Sign(sub, sid, "", issuedAt.Add(-time.Hour))
	if _, err := verifier.Verify(expired); err == nil {
		t.Fatal("expired token verified, want rejection")
	}

	// A token signed with a different key is rejected (wrong key).
	_, otherPriv, _ := ed25519.GenerateKey(nil)
	forged, _ := security.NewSigner(kid, otherPriv, issuer, time.Minute).Sign(sub, sid, "", issuedAt)
	if _, err := verifier.Verify(forged); err == nil {
		t.Fatal("token signed with wrong key verified, want rejection")
	}

	// An unknown kid is rejected.
	unknownVerifier := security.NewVerifier(map[string]ed25519.PublicKey{"other": pub}, issuer)
	if _, err := unknownVerifier.Verify(raw); err == nil {
		t.Fatal("token with unknown kid verified, want rejection")
	}
}
