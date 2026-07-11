// Package id generates UUIDv7 identifiers behind a Generator interface so
// tests can substitute deterministic values.
//
// UUIDv7 is time-ordered, which keeps primary keys index-friendly.
package id

import (
	"encoding/binary"
	"sync"

	"github.com/google/uuid"
)

// Generator produces unique identifiers.
type Generator interface {
	New() uuid.UUID
}

// UUIDv7 is the production Generator. It returns time-ordered UUIDv7 values.
type UUIDv7 struct{}

// New returns a new UUIDv7. It panics only if the system random source fails,
// which is not a recoverable condition.
func (UUIDv7) New() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}

// Sequence is a deterministic Generator for tests: it returns UUIDs whose
// low bits are a monotonically increasing counter starting at 1, so the
// produced values are stable and ordered across a test run. It is safe for
// concurrent use.
type Sequence struct {
	mu sync.Mutex
	n  uint64
}

// NewSequence returns a Sequence generator starting from 0 (first New() is 1).
func NewSequence() *Sequence {
	return &Sequence{}
}

// New returns the next deterministic UUID in the sequence.
func (s *Sequence) New() uuid.UUID {
	s.mu.Lock()
	s.n++
	n := s.n
	s.mu.Unlock()

	var u uuid.UUID
	binary.BigEndian.PutUint64(u[8:], n)
	return u
}
