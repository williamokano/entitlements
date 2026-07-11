// Package clock abstracts the current time behind a Clock interface with a
// real implementation and a frozen implementation for deterministic tests.
//
// Domain and application code must read time through this package (UTC),
// never time.Now directly.
package clock

import (
	"sync"
	"time"
)

// Clock reports the current time. Implementations must return UTC.
type Clock interface {
	Now() time.Time
}

// System is the production Clock, backed by the wall clock in UTC.
var System Clock = systemClock{}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// Frozen is a Clock whose time only moves when explicitly told to. It is safe
// for concurrent use and is intended for deterministic tests.
type Frozen struct {
	mu  sync.RWMutex
	now time.Time
}

// NewFrozen returns a Frozen clock set to t (normalized to UTC).
func NewFrozen(t time.Time) *Frozen {
	return &Frozen{now: t.UTC()}
}

// Now returns the frozen time.
func (f *Frozen) Now() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.now
}

// Set moves the clock to t (normalized to UTC).
func (f *Frozen) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t.UTC()
}

// Advance moves the clock forward by d and returns the new time.
func (f *Frozen) Advance(d time.Duration) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	return f.now
}
