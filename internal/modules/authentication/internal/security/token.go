package security

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims are the access-token claims. Standard registered claims (sub, iat, exp)
// carry identity and validity; sid identifies the session (refresh-token family)
// so session management works from the access token alone. Downstream services
// verify these offline with the published verification key.
type Claims struct {
	jwt.RegisteredClaims
	Email     string `json:"email,omitempty"`
	SessionID string `json:"sid,omitempty"`
}

// Signer mints signed access tokens with an EdDSA (Ed25519) key. The key id
// (kid) is written into each token header so verifiers can select the matching
// public key from a published set.
type Signer struct {
	kid        string
	privateKey ed25519.PrivateKey
	issuer     string
	ttl        time.Duration
}

// NewSigner builds a Signer. ttl is the access-token lifetime.
func NewSigner(kid string, priv ed25519.PrivateKey, issuer string, ttl time.Duration) *Signer {
	return &Signer{kid: kid, privateKey: priv, issuer: issuer, ttl: ttl}
}

// PublicKey returns the verification key, so it can be published (e.g. via a
// JWKS endpoint) for offline verification.
func (s *Signer) PublicKey() ed25519.PublicKey {
	return s.privateKey.Public().(ed25519.PublicKey)
}

// KeyID returns the signer's kid.
func (s *Signer) KeyID() string { return s.kid }

// TTL returns the access-token lifetime.
func (s *Signer) TTL() time.Duration { return s.ttl }

// Sign issues an access token for a subject on a session, valid for the signer's
// ttl from issuedAt.
func (s *Signer) Sign(subject, sessionID uuid.UUID, email string, issuedAt time.Time) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject.String(),
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(s.ttl)),
		},
		Email:     email,
		SessionID: sessionID.String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.kid
	signed, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", fmt.Errorf("security: sign token: %w", err)
	}
	return signed, nil
}

// ErrInvalidToken is returned when a token fails verification for any reason
// (bad signature, expired, wrong key, malformed).
var ErrInvalidToken = errors.New("security: invalid token")

// Verifier validates access tokens offline against a set of public keys keyed
// by kid. New keys can be added for rotation without invalidating live tokens.
type Verifier struct {
	keys   map[string]ed25519.PublicKey
	issuer string
}

// NewVerifier builds a Verifier over a kid -> public key map.
func NewVerifier(keys map[string]ed25519.PublicKey, issuer string) *Verifier {
	return &Verifier{keys: keys, issuer: issuer}
}

// Verify parses and validates a token, returning its claims. It enforces the
// EdDSA algorithm and selects the public key by the token's kid header.
func (v *Verifier) Verify(raw string) (*Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %s", ErrInvalidToken, token.Method.Alg())
		}
		kid, _ := token.Header["kid"].(string)
		key, ok := v.keys[kid]
		if !ok {
			return nil, fmt.Errorf("%w: unknown key id %q", ErrInvalidToken, kid)
		}
		return key, nil
	},
		jwt.WithValidMethods([]string{"EdDSA"}),
		jwt.WithIssuer(v.issuer),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	return &claims, nil
}
