package authentication

import (
	"context"
	"sync"
	"time"

	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/clock"
)

// memoryRateLimiter is the default in-memory ports.RateLimiter: a fixed-window
// counter that allows up to `maxAttempts` attempts per `window` per key. It is a
// single-process default; production deployments inject a shared-store limiter
// via WithRateLimiter.
type memoryRateLimiter struct {
	clk         clock.Clock
	maxAttempts int
	window      time.Duration

	mu      sync.Mutex
	windows map[string]*counter
}

type counter struct {
	count int
	reset time.Time
}

// NewMemoryRateLimiter builds an in-memory rate limiter allowing maxAttempts attempts
// per window per key.
func NewMemoryRateLimiter(clk clock.Clock, maxAttempts int, window time.Duration) ports.RateLimiter {
	return &memoryRateLimiter{clk: clk, maxAttempts: maxAttempts, window: window, windows: map[string]*counter{}}
}

// Allow records an attempt for key and blocks once the window's limit is hit.
// The block maps to HTTP 403 today; a dedicated 429 taxonomy kind can replace it
// without changing callers.
func (l *memoryRateLimiter) Allow(_ context.Context, key string) error {
	now := l.clk.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	c, ok := l.windows[key]
	if !ok || now.After(c.reset) {
		l.windows[key] = &counter{count: 1, reset: now.Add(l.window)}
		return nil
	}
	if c.count >= l.maxAttempts {
		return apperr.Forbidden("too many attempts, try again later")
	}
	c.count++
	return nil
}
