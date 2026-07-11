package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func testEvent(t string) Event {
	return Event{
		ID:         uuid.New(),
		OccurredAt: time.Now(),
		Module:     "test",
		Type:       t,
		Payload:    json.RawMessage(`{"k":"v"}`),
	}
}

func TestSubscribeDispatchesToMatchingHandlers(t *testing.T) {
	bus := NewBus()

	var aCalls, bCalls, otherCalls int
	bus.Subscribe("thing.created", func(context.Context, Event) error { aCalls++; return nil })
	bus.Subscribe("thing.created", func(context.Context, Event) error { bCalls++; return nil })
	bus.Subscribe("thing.deleted", func(context.Context, Event) error { otherCalls++; return nil })

	if err := bus.Dispatch(context.Background(), testEvent("thing.created")); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if aCalls != 1 || bCalls != 1 {
		t.Fatalf("matching handlers called %d/%d times, want 1/1", aCalls, bCalls)
	}
	if otherCalls != 0 {
		t.Fatalf("non-matching handler called %d times, want 0", otherCalls)
	}

	// An event with no subscribers is a no-op.
	if err := bus.Dispatch(context.Background(), testEvent("unknown.type")); err != nil {
		t.Fatalf("Dispatch of unsubscribed type: %v", err)
	}
}

func TestEventEnvelopeValidation(t *testing.T) {
	base := func() Event {
		return Event{
			ID:         uuid.New(),
			OccurredAt: time.Now(),
			Module:     "test",
			Type:       "thing.created",
			Payload:    json.RawMessage(`{}`),
		}
	}

	cases := map[string]func(*Event){
		"missing id":        func(e *Event) { e.ID = uuid.Nil },
		"missing occurred":  func(e *Event) { e.OccurredAt = time.Time{} },
		"missing module":    func(e *Event) { e.Module = "" },
		"missing type":      func(e *Event) { e.Type = "" },
		"missing payload":   func(e *Event) { e.Payload = nil },
		"invalid json body": func(e *Event) { e.Payload = json.RawMessage(`{not json`) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e := base()
			mutate(&e)
			if err := e.validate(); err == nil {
				t.Fatalf("validate() = nil, want error for %q", name)
			}
		})
	}

	if err := base().validate(); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
}
