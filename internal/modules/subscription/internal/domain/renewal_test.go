package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func newActiveMonthly(t *testing.T) *Subscription {
	t.Helper()
	s, err := Create(uuid.New(), uuid.New(), uuid.New(), CycleMonthly, 0, t0, "tester")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	s.PendingTransition()
	return s
}

func TestRenewalDueOnlyAfterPeriodEndAndOncePerPeriod(t *testing.T) {
	s := newActiveMonthly(t) // period t0 .. t0+1mo
	if s.RenewalDue(t0.Add(24 * time.Hour)) {
		t.Fatal("renewal due mid-period")
	}
	end := s.CurrentPeriodEnd
	if !s.RenewalDue(end) {
		t.Fatal("renewal not due at period end")
	}
	s.MarkRenewalEmitted(end)
	if s.RenewalDue(end.Add(time.Hour)) {
		t.Fatal("renewal due again after emission for the same period")
	}
}

func TestRenewalNotDueForTrialingOrTerminal(t *testing.T) {
	s := newActiveMonthly(t)
	for _, st := range []State{StateTrialing, StatePaused, StateSuspended, StateCanceled, StateExpired} {
		s.Status = st
		if s.RenewalDue(s.CurrentPeriodEnd.Add(time.Hour)) {
			t.Fatalf("renewal due in state %s", st)
		}
	}
}

func TestAdvancePeriodMovesWindowAndClearsMarker(t *testing.T) {
	s := newActiveMonthly(t)
	oldEnd := s.CurrentPeriodEnd
	s.MarkRenewalEmitted(oldEnd)
	if s.AdvancePeriod(t0.Add(24 * time.Hour)) {
		t.Fatal("advanced before period end")
	}
	if !s.AdvancePeriod(oldEnd) {
		t.Fatal("did not advance at period end")
	}
	if !s.CurrentPeriodStart.Equal(oldEnd) {
		t.Fatalf("new start = %v, want %v", s.CurrentPeriodStart, oldEnd)
	}
	if !s.CurrentPeriodEnd.Equal(periodEnd(oldEnd, CycleMonthly)) {
		t.Fatalf("new end = %v", s.CurrentPeriodEnd)
	}
	if s.RenewalEmittedPeriodEnd != nil {
		t.Fatal("renewal marker not cleared after advance")
	}
}

func TestAdvancePeriodAppliesScheduledChange(t *testing.T) {
	s := newActiveMonthly(t)
	newPV := uuid.New()
	if err := s.ScheduleChange(newPV, CycleAnnual, t0); err != nil {
		t.Fatalf("ScheduleChange: %v", err)
	}
	if !s.AdvancePeriod(s.CurrentPeriodEnd) {
		t.Fatal("did not advance")
	}
	if s.PlanVersionID != newPV || s.BillingCycle != CycleAnnual || s.Scheduled != nil {
		t.Fatalf("scheduled change not applied at rollover: pv=%s cycle=%s scheduled=%v", s.PlanVersionID, s.BillingCycle, s.Scheduled)
	}
	if c := s.PendingPlanChange(); c == nil || c.Timing != TimingPeriodEnd {
		t.Fatalf("plan change = %+v", c)
	}
}

func TestTrialEndingWindowAndOnce(t *testing.T) {
	end := t0.AddDate(0, 0, 14)
	s, err := Create(uuid.New(), uuid.New(), uuid.New(), CycleMonthly, 14, t0, "tester")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !s.TrialEndsAt.Equal(end) {
		t.Fatalf("trial end = %v, want %v", s.TrialEndsAt, end)
	}
	// 3-day window opens at end-3d.
	if s.TrialEndingDue(end.AddDate(0, 0, -4), 3) {
		t.Fatal("trial-ending due outside the window")
	}
	if !s.TrialEndingDue(end.AddDate(0, 0, -2), 3) {
		t.Fatal("trial-ending not due inside the window")
	}
	s.MarkTrialEndingEmitted(end.AddDate(0, 0, -2))
	if s.TrialEndingDue(end.AddDate(0, 0, -1), 3) {
		t.Fatal("trial-ending due again after emission")
	}
	// At/after the end it is no longer "ending" — it has ended.
	if s.TrialEndingDue(end, 3) {
		t.Fatal("trial-ending due at the trial end")
	}
	if !s.TrialEnded(end) {
		t.Fatal("trial not ended at the boundary")
	}
}

func TestBeginPeriodAfterTrialStartsFullCycle(t *testing.T) {
	s, err := Create(uuid.New(), uuid.New(), uuid.New(), CycleMonthly, 14, t0, "tester")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	trialEnd := s.CurrentPeriodEnd
	s.BeginPeriodAfterTrial(trialEnd)
	if !s.CurrentPeriodStart.Equal(trialEnd) || !s.CurrentPeriodEnd.Equal(periodEnd(trialEnd, CycleMonthly)) {
		t.Fatalf("period = %v..%v", s.CurrentPeriodStart, s.CurrentPeriodEnd)
	}
	if s.TrialEndsAt != nil {
		t.Fatal("trial marker not cleared")
	}
}
