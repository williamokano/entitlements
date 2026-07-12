package security

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	apiKeyPrefixLabel = "ak"
	apiKeyPrefixBytes = 8  // → 16 hex chars of public prefix
	apiKeySecretBytes = 24 // → 192 bits of secret entropy
	apiKeySeparator   = "."
)

// APIKeyMaterial is a freshly minted API key. Prefix is public (stored, used for
// lookup); Secret is shown to the caller exactly once and only its hash is
// stored; Full is the value the client presents in the Authorization header.
type APIKeyMaterial struct {
	Prefix string
	Secret string
	Full   string
}

// GenerateAPIKey mints a new API key. The presented form is "<prefix>.<secret>";
// the prefix is "ak_<hex>" and the secret is high-entropy base64url (so it never
// contains the "." separator).
func GenerateAPIKey() (APIKeyMaterial, error) {
	prefixRaw := make([]byte, apiKeyPrefixBytes)
	if _, err := rand.Read(prefixRaw); err != nil {
		return APIKeyMaterial{}, fmt.Errorf("security: read api key prefix: %w", err)
	}
	secretRaw := make([]byte, apiKeySecretBytes)
	if _, err := rand.Read(secretRaw); err != nil {
		return APIKeyMaterial{}, fmt.Errorf("security: read api key secret: %w", err)
	}
	prefix := apiKeyPrefixLabel + "_" + hex.EncodeToString(prefixRaw)
	secret := base64.RawURLEncoding.EncodeToString(secretRaw)
	return APIKeyMaterial{
		Prefix: prefix,
		Secret: secret,
		Full:   prefix + apiKeySeparator + secret,
	}, nil
}

// ParseAPIKey splits a presented "<prefix>.<secret>" value. It returns ok=false
// for anything that is not well-formed.
func ParseAPIKey(raw string) (prefix, secret string, ok bool) {
	prefix, secret, found := strings.Cut(raw, apiKeySeparator)
	if !found || prefix == "" || secret == "" {
		return "", "", false
	}
	return prefix, secret, true
}
