package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAuthTokenRedeemableRespectsConsumptionAndExpiry(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tok := NewAuthToken(uuid.New(), uuid.New(), PurposeEmailVerification, "raw-token", t0, t0.Add(time.Hour))

	if tok.Hash == "raw-token" || tok.Hash == "" {
		t.Fatalf("token stored raw or empty hash: %q", tok.Hash)
	}
	// Fresh and unexpired → redeemable.
	if !tok.Redeemable(t0) {
		t.Fatal("fresh token not redeemable")
	}
	// After expiry → not redeemable.
	if tok.Redeemable(t0.Add(2 * time.Hour)) {
		t.Fatal("expired token still redeemable")
	}
	// Consumed → not redeemable, even before expiry.
	consumed := t0.Add(10 * time.Minute)
	tok.ConsumedAt = &consumed
	if tok.Redeemable(t0.Add(20 * time.Minute)) {
		t.Fatal("consumed token still redeemable")
	}
}
