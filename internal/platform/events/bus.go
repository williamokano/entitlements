package events

import (
	"context"
	"sync"
)

// Handler processes a delivered event. Returning an error causes the relay to
// roll back the delivery transaction and retry later, so handlers must be safe
// to re-run (wrap them with Idempotent for exactly-once effect).
type Handler func(ctx context.Context, e Event) error

// Bus routes events to the handlers subscribed to their type. It is safe for
// concurrent use.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewBus returns an empty Bus.
func NewBus() *Bus {
	return &Bus{handlers: make(map[string][]Handler)}
}

// Subscribe registers h to receive events of the given type. Multiple handlers
// may subscribe to the same type; they are invoked in registration order.
func (b *Bus) Subscribe(eventType string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], h)
}

// Dispatch invokes every handler subscribed to e.Type, in order, stopping at
// the first error. It runs synchronously in the caller's context (and, when
// called by the relay, inside the delivery transaction), which also makes it
// the synchronous-dispatch helper used to unit-test consumers. Events with no
// subscribers are a no-op.
func (b *Bus) Dispatch(ctx context.Context, e Event) error {
	b.mu.RLock()
	handlers := b.handlers[e.Type]
	b.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
