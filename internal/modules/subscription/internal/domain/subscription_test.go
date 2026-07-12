package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newActive(t *testing.T) *Subscription {
	t.Helper()
	s, err := Create(uuid.New(), uuid.New(), uuid.New(), CycleMonthly, 0, t0, "tester")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return s
}

func TestTransitionMatrixExhaustive(t *testing.T) {
	for _, from := range AllStates() {
		for _, ev := range AllEvents() {
			want, legal := NextState(from, ev)

			s := newActive(t)
			s.Status = from // force the starting state
			s.pending = nil
			err := s.Apply(ev, "r", "a", uuid.New(), t0)

			if legal {
				if err != nil {
					t.Errorf("(%s,%s) legal but Apply errored: %v", from, ev, err)
					continue
				}
				if s.Status != want {
					t.Errorf("(%s,%s) → %s, want %s", from, ev, s.Status, want)
				}
				if tr := s.PendingTransition(); tr == nil || tr.From != from || tr.To != want || tr.Event != ev {
					t.Errorf("(%s,%s) transition not recorded correctly: %+v", from, ev, tr)
				}
			} else {
				if apperr.KindOf(err) != apperr.KindConflict {
					t.Errorf("(%s,%s) illegal but err = %v (want conflict)", from, ev, err)
				}
				if s.Status != from {
					t.Errorf("(%s,%s) illegal but state changed to %s", from, ev, s.Status)
				}
			}
		}
	}
}

func TestCreateWithTrialSetsTrialingAndPeriodFromPlanConfig(t *testing.T) {
	s, err := Create(uuid.New(), uuid.New(), uuid.New(), CycleMonthly, 14, t0, "a")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.Status != StateTrialing {
		t.Fatalf("status = %s, want trialing", s.Status)
	}
	if s.TrialEndsAt == nil || !s.TrialEndsAt.Equal(t0.AddDate(0, 0, 14)) {
		t.Fatalf("trial ends = %v, want +14d", s.TrialEndsAt)
	}
	if !s.CurrentPeriodEnd.Equal(t0.AddDate(0, 0, 14)) {
		t.Fatalf("period end = %v, want trial boundary", s.CurrentPeriodEnd)
	}
	if tr := s.PendingTransition(); tr == nil || tr.To != StateTrialing || tr.From != "" {
		t.Fatalf("creation transition = %+v", tr)
	}
}

func TestCreateWithoutTrialGoesActive(t *testing.T) {
	s, _ := Create(uuid.New(), uuid.New(), uuid.New(), CycleAnnual, 0, t0, "a")
	if s.Status != StateActive {
		t.Fatalf("status = %s, want active", s.Status)
	}
	if s.TrialEndsAt != nil {
		t.Fatal("trial set on a non-trial subscription")
	}
	if !s.CurrentPeriodEnd.Equal(t0.AddDate(1, 0, 0)) {
		t.Fatalf("annual period end = %v, want +1y", s.CurrentPeriodEnd)
	}
}

func TestCancelImmediateTransitionsNow(t *testing.T) {
	s := newActive(t)
	s.pending = nil
	if err := s.Apply(EventCancel, "user", "a", uuid.New(), t0); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if s.Status != StateCanceled {
		t.Fatalf("status = %s, want canceled", s.Status)
	}
}

func TestCancelAtPeriodEndKeepsActiveUntilBoundary(t *testing.T) {
	s := newActive(t)
	if err := s.ScheduleCancelAtPeriodEnd(); err != nil {
		t.Fatalf("schedule cancel: %v", err)
	}
	if s.Status != StateActive {
		t.Fatalf("status = %s, want still active", s.Status)
	}
	if !s.CancelAtPeriodEnd {
		t.Fatal("cancel_at_period_end not set")
	}
	// A terminal subscription cannot be scheduled.
	s.Status = StateCanceled
	if apperr.KindOf(s.ScheduleCancelAtPeriodEnd()) != apperr.KindConflict {
		t.Fatal("scheduling cancel on terminal subscription not rejected")
	}
}

func TestPauseResumeRoundTrip(t *testing.T) {
	s := newActive(t)
	if err := s.Apply(EventPause, "", "a", uuid.New(), t0); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if s.Status != StatePaused {
		t.Fatalf("status = %s, want paused", s.Status)
	}
	if err := s.Apply(EventResume, "", "a", uuid.New(), t0); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if s.Status != StateActive {
		t.Fatalf("status = %s, want active", s.Status)
	}
}
