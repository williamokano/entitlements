package security

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// opaqueTokenBytes is the entropy (256 bits) of an opaque refresh token.
const opaqueTokenBytes = 32

// GenerateOpaqueToken returns a URL-safe, high-entropy random token suitable for
// use as a refresh token. Only its hash is ever stored (see
// domain.HashRefreshToken).
func GenerateOpaqueToken() (string, error) {
	buf := make([]byte, opaqueTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("security: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
