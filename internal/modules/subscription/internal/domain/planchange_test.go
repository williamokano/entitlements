package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

func TestUpgradeRepinsImmediatelyAndEmitsOldNewRefs(t *testing.T) {
	s := newActive(t)
	s.PendingPlanChange() // drop the creation transition side effects
	oldPV, oldCycle := s.PlanVersionID, s.BillingCycle
	newPV := uuid.New()
	now := t0.Add(time.Hour)

	if err := s.ChangePlanNow(newPV, CycleAnnual, now); err != nil {
		t.Fatalf("ChangePlanNow: %v", err)
	}
	if s.PlanVersionID != newPV || s.BillingCycle != CycleAnnual {
		t.Fatalf("not re-pinned: pv=%s cycle=%s", s.PlanVersionID, s.BillingCycle)
	}
	if s.Scheduled != nil {
		t.Fatal("immediate change must clear any scheduled change")
	}
	change := s.PendingPlanChange()
	if change == nil {
		t.Fatal("no pending plan change recorded")
	}
	if change.OldPlanVersionID != oldPV || change.NewPlanVersionID != newPV {
		t.Fatalf("refs: old=%s new=%s", change.OldPlanVersionID, change.NewPlanVersionID)
	}
	if change.OldCycle != oldCycle || change.NewCycle != CycleAnnual || change.Timing != TimingImmediate {
		t.Fatalf("change = %+v", change)
	}
	if s.PendingPlanChange() != nil {
		t.Fatal("pending change must clear after read")
	}
}

func TestDowngradeStoredAsScheduledChangeNoImmediateEffect(t *testing.T) {
	s := newActive(t)
	oldPV, oldCycle := s.PlanVersionID, s.BillingCycle
	newPV := uuid.New()

	if err := s.ScheduleChange(newPV, CycleMonthly, t0.Add(time.Hour)); err != nil {
		t.Fatalf("ScheduleChange: %v", err)
	}
	if s.PlanVersionID != oldPV || s.BillingCycle != oldCycle {
		t.Fatal("scheduling must not change the current pin")
	}
	if s.Scheduled == nil || s.Scheduled.PlanVersionID != newPV || s.Scheduled.BillingCycle != CycleMonthly {
		t.Fatalf("scheduled = %+v", s.Scheduled)
	}
	if got := s.PendingPlanChange(); got != nil && got.Timing != "" && got.NewPlanVersionID == newPV {
		t.Fatal("scheduling must not record an applied plan change")
	}
}

func TestCancelScheduledChangeRestoresNoPendingState(t *testing.T) {
	s := newActive(t)
	if err := s.CancelScheduledChange(t0); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("cancel with nothing scheduled = %v, want conflict", err)
	}
	if err := s.ScheduleChange(uuid.New(), CycleMonthly, t0); err != nil {
		t.Fatalf("ScheduleChange: %v", err)
	}
	if err := s.CancelScheduledChange(t0.Add(time.Minute)); err != nil {
		t.Fatalf("CancelScheduledChange: %v", err)
	}
	if s.Scheduled != nil {
		t.Fatal("scheduled change not cleared")
	}
}

func TestApplyScheduledChangeRepinsAndRecordsPeriodEndTiming(t *testing.T) {
	s := newActive(t)
	s.PendingPlanChange()
	if s.ApplyScheduledChange(t0) {
		t.Fatal("apply with nothing scheduled must be a no-op")
	}
	newPV := uuid.New()
	if err := s.ScheduleChange(newPV, CycleAnnual, t0); err != nil {
		t.Fatalf("ScheduleChange: %v", err)
	}
	if !s.ApplyScheduledChange(t0.Add(time.Hour)) {
		t.Fatal("apply with a scheduled change must report true")
	}
	if s.PlanVersionID != newPV || s.BillingCycle != CycleAnnual || s.Scheduled != nil {
		t.Fatalf("not re-pinned/cleared: pv=%s cycle=%s scheduled=%v", s.PlanVersionID, s.BillingCycle, s.Scheduled)
	}
	change := s.PendingPlanChange()
	if change == nil || change.Timing != TimingPeriodEnd || change.NewPlanVersionID != newPV {
		t.Fatalf("change = %+v", change)
	}
}

func TestPlanChangeGuards(t *testing.T) {
	s := newActive(t)
	// Same version + same cycle is a validation error.
	if err := s.ChangePlanNow(s.PlanVersionID, s.BillingCycle, t0); apperr.KindOf(err) != apperr.KindValidation {
		t.Fatalf("same pin = %v, want validation", err)
	}
	// Terminal subscriptions cannot change plans.
	s.Status = StateCanceled
	if err := s.ChangePlanNow(uuid.New(), CycleMonthly, t0); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("terminal change = %v, want conflict", err)
	}
	if err := s.ScheduleChange(uuid.New(), CycleMonthly, t0); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("terminal schedule = %v, want conflict", err)
	}
}

func TestAddonQuantityRules(t *testing.T) {
	cases := []struct {
		name            string
		quantityAllowed bool
		qty             int
		wantErr         bool
	}{
		{"zero rejected", true, 0, true},
		{"negative rejected", true, -1, true},
		{"one always ok", false, 1, false},
		{"multiple ok when allowed", true, 3, false},
		{"multiple rejected when not allowed", false, 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAddonQuantity(tc.quantityAllowed, tc.qty)
			if tc.wantErr && apperr.KindOf(err) != apperr.KindValidation {
				t.Fatalf("err = %v, want validation", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}
}
