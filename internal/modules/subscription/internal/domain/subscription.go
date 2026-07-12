// Package domain holds the subscription aggregate and its state machine. It is
// pure: only the standard library, uuid, and the platform error taxonomy.
package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// State is a subscription's lifecycle state.
type State string

// Subscription states. paused is a voluntary hold (distinct from suspended,
// which is dunning-driven). canceled and expired are terminal.
const (
	StateTrialing  State = "trialing"
	StateActive    State = "active"
	StatePastDue   State = "past_due"
	StateGrace     State = "grace"
	StateSuspended State = "suspended"
	StatePaused    State = "paused"
	StateCanceled  State = "canceled"
	StateExpired   State = "expired"
)

// Event triggers a transition.
type Event string

// Transition events.
const (
	EventActivate      Event = "activate"       // trial converts / first payment ok
	EventPaymentFailed Event = "payment_failed" // active -> past_due
	EventEnterGrace    Event = "enter_grace"    // past_due -> grace
	EventPaymentOK     Event = "payment_ok"     // past_due/grace -> active
	EventSuspend       Event = "suspend"        // dunning exhausted -> suspended
	EventReactivate    Event = "reactivate"     // suspended -> active
	EventPause         Event = "pause"          // active -> paused
	EventResume        Event = "resume"         // paused -> active
	EventCancel        Event = "cancel"         // -> canceled (immediate)
	EventExpire        Event = "expire"         // trial/grace/suspended -> expired
)

// transitions is the single source of truth for the state machine: for a given
// state, which event leads to which next state. Any (state, event) pair not
// present here is illegal.
var transitions = map[State]map[Event]State{
	StateTrialing: {
		EventActivate: StateActive,
		EventCancel:   StateCanceled,
		EventExpire:   StateExpired,
	},
	StateActive: {
		EventPaymentFailed: StatePastDue,
		EventPause:         StatePaused,
		EventCancel:        StateCanceled,
	},
	StatePastDue: {
		EventEnterGrace: StateGrace,
		EventPaymentOK:  StateActive,
		EventSuspend:    StateSuspended,
		EventCancel:     StateCanceled,
	},
	StateGrace: {
		EventPaymentOK: StateActive,
		EventSuspend:   StateSuspended,
		EventCancel:    StateCanceled,
		EventExpire:    StateExpired,
	},
	StateSuspended: {
		EventReactivate: StateActive,
		EventCancel:     StateCanceled,
		EventExpire:     StateExpired,
	},
	StatePaused: {
		EventResume: StateActive,
		EventCancel: StateCanceled,
	},
	StateCanceled: {}, // terminal
	StateExpired:  {}, // terminal
}

// AllStates and AllEvents enumerate the machine for exhaustive testing.
func AllStates() []State {
	return []State{StateTrialing, StateActive, StatePastDue, StateGrace, StateSuspended, StatePaused, StateCanceled, StateExpired}
}

// AllEvents lists every event.
func AllEvents() []Event {
	return []Event{EventActivate, EventPaymentFailed, EventEnterGrace, EventPaymentOK, EventSuspend, EventReactivate, EventPause, EventResume, EventCancel, EventExpire}
}

// NextState returns the state an event leads to from `from`, and whether the
// transition is legal.
func NextState(from State, event Event) (State, bool) {
	to, ok := transitions[from][event]
	return to, ok
}

// BillingCycle mirrors the catalog's billing cycles.
type BillingCycle string

// Billing cycles.
const (
	CycleMonthly BillingCycle = "monthly"
	CycleAnnual  BillingCycle = "annual"
	CycleCustom  BillingCycle = "custom"
)

// Transition is one recorded state change (append-only history).
type Transition struct {
	ID             uuid.UUID
	SubscriptionID uuid.UUID
	From           State
	To             State
	Event          Event
	Reason         string
	Actor          string
	At             time.Time
}

// Subscription is a tenant's commitment to a pinned plan version.
type Subscription struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	PlanVersionID      uuid.UUID
	BillingCycle       BillingCycle
	Status             State
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	TrialEndsAt        *time.Time
	CancelAtPeriodEnd  bool
	Scheduled          *ScheduledChange
	CreatedAt          time.Time
	UpdatedAt          time.Time

	// pending is the transition produced by the last Create/Apply call, to be
	// persisted and published by the service. It is not part of the stored state.
	pending *Transition
	// pendingChange is the plan change produced by the last
	// ChangePlanNow/ApplyScheduledChange call, for the service to publish.
	pendingChange *PlanChange
}

// Create builds a subscription in its initial state. When trialDays > 0 it
// starts trialing with the trial period; otherwise it starts active with a full
// billing period. The initial transition (from "") is recorded as pending.
func Create(id, tenantID, planVersionID uuid.UUID, cycle BillingCycle, trialDays int, now time.Time, actor string) (*Subscription, error) {
	if !validCycle(cycle) {
		return nil, apperr.Validation(fmt.Sprintf("invalid billing cycle %q", cycle))
	}
	s := &Subscription{
		ID:                 id,
		TenantID:           tenantID,
		PlanVersionID:      planVersionID,
		BillingCycle:       cycle,
		CurrentPeriodStart: now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if trialDays > 0 {
		end := now.AddDate(0, 0, trialDays)
		s.Status = StateTrialing
		s.CurrentPeriodEnd = end
		s.TrialEndsAt = &end
	} else {
		s.Status = StateActive
		s.CurrentPeriodEnd = periodEnd(now, cycle)
	}
	s.pending = &Transition{
		ID: id, SubscriptionID: id, From: "", To: s.Status, Event: "created", Actor: actor, At: now,
	}
	return s, nil
}

// Apply moves the subscription through the state machine. An illegal (state,
// event) pair is rejected and the state is left unchanged. On success it records
// a pending transition.
func (s *Subscription) Apply(event Event, reason, actor string, transitionID uuid.UUID, now time.Time) error {
	to, ok := NextState(s.Status, event)
	if !ok {
		return apperr.Conflict(fmt.Sprintf("cannot apply %q to a %s subscription", event, s.Status))
	}
	from := s.Status
	s.Status = to
	s.UpdatedAt = now
	s.pending = &Transition{
		ID: transitionID, SubscriptionID: s.ID, From: from, To: to, Event: event, Reason: reason, Actor: actor, At: now,
	}
	return nil
}

// ScheduleCancelAtPeriodEnd marks the subscription to cancel at its period
// boundary without changing state now. Only a live (non-terminal) subscription
// can be scheduled.
func (s *Subscription) ScheduleCancelAtPeriodEnd() error {
	if s.Terminal() {
		return apperr.Conflict("subscription is already terminal")
	}
	s.CancelAtPeriodEnd = true
	return nil
}

// Terminal reports whether the subscription is in a terminal state.
func (s *Subscription) Terminal() bool {
	return s.Status == StateCanceled || s.Status == StateExpired
}

// PendingTransition returns and clears the transition produced by the last
// Create/Apply, for the service to persist and publish.
func (s *Subscription) PendingTransition() *Transition {
	t := s.pending
	s.pending = nil
	return t
}

func validCycle(c BillingCycle) bool {
	return c == CycleMonthly || c == CycleAnnual || c == CycleCustom
}

// periodEnd computes a billing period's end. Custom falls back to monthly until
// a plan defines a custom interval (out of scope here).
func periodEnd(start time.Time, cycle BillingCycle) time.Time {
	switch cycle {
	case CycleAnnual:
		return start.AddDate(1, 0, 0)
	default:
		return start.AddDate(0, 1, 0)
	}
}

// Repository persists subscriptions, their transitions, and their addons.
type Repository interface {
	Create(ctx context.Context, s *Subscription) error
	Update(ctx context.Context, s *Subscription) error
	GetByID(ctx context.Context, id uuid.UUID) (*Subscription, error)
	GetLiveForTenant(ctx context.Context, tenantID uuid.UUID) (*Subscription, error)
	AppendTransition(ctx context.Context, t *Transition) error
	ListTransitions(ctx context.Context, subscriptionID uuid.UUID) ([]*Transition, error)
	AddonRepository
}
