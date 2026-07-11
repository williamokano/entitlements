// Package security holds the authentication module's cryptographic primitives:
// argon2id password hashing and JWT signing/verification. It lives outside the
// domain because it imports crypto dependencies (golang.org/x/crypto,
// golang-jwt) that depguard forbids the pure domain from importing.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// argon2idParams are the argon2id cost parameters. They are encoded into every
// hash so a stored hash remains verifiable even if these defaults change later.
type argon2idParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLen     uint32
	keyLen      uint32
}

// defaultParams follow current OWASP argon2id guidance (19 MiB, t=2, p=1).
var defaultParams = argon2idParams{
	memory:      19 * 1024,
	iterations:  2,
	parallelism: 1,
	saltLen:     16,
	keyLen:      32,
}

// ErrMalformedHash indicates a stored hash that cannot be parsed.
var ErrMalformedHash = errors.New("security: malformed argon2id hash")

// HashPassword derives an argon2id hash in the standard encoded form
// ($argon2id$v=19$m=...,t=...,p=...$salt$hash). The plaintext is never stored.
func HashPassword(plaintext string) (string, error) {
	if plaintext == "" {
		return "", apperr.Validation("password must not be empty")
	}
	p := defaultParams
	salt := make([]byte, p.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("security: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(plaintext), salt, p.iterations, p.memory, p.parallelism, p.keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.iterations, p.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether plaintext matches the encoded argon2id hash,
// using a constant-time comparison. A malformed hash returns an error.
func VerifyPassword(plaintext, encoded string) (bool, error) {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	candidate := argon2.IDKey([]byte(plaintext), salt, p.iterations, p.memory, p.parallelism, uint32(len(key)))
	return subtle.ConstantTimeCompare(key, candidate) == 1, nil
}

func decodeHash(encoded string) (argon2idParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argon2idParams{}, nil, nil, ErrMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argon2idParams{}, nil, nil, ErrMalformedHash
	}
	var p argon2idParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.parallelism); err != nil {
		return argon2idParams{}, nil, nil, ErrMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2idParams{}, nil, nil, ErrMalformedHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2idParams{}, nil, nil, ErrMalformedHash
	}
	return p, salt, key, nil
}
