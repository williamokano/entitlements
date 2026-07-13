package domain

import (
	"testing"
	"time"
)

func TestPeriodKey(t *testing.T) {
	now := time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)
	start := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)

	if got := PeriodKey(ResetMonthly, now, nil); got != "2026-01" {
		t.Errorf("monthly = %q, want 2026-01", got)
	}
	if got := PeriodKey(ResetNever, now, nil); got != "never" {
		t.Errorf("never = %q, want never", got)
	}
	if got := PeriodKey("", now, nil); got != "never" {
		t.Errorf("empty = %q, want never", got)
	}
	if got := PeriodKey(ResetBillingCycle, now, &start); got != "cycle:"+start.Format(time.RFC3339) {
		t.Errorf("billing_cycle = %q, want cycle:<start>", got)
	}
	if got := PeriodKey(ResetBillingCycle, now, nil); got != "cycle:none" {
		t.Errorf("billing_cycle without sub = %q, want cycle:none", got)
	}

	// A new calendar month yields a distinct key (lazy reset).
	next := time.Date(2026, 2, 1, 0, 0, 1, 0, time.UTC)
	if PeriodKey(ResetMonthly, next, nil) == PeriodKey(ResetMonthly, now, nil) {
		t.Error("monthly key did not change across a month boundary")
	}
}

func TestEffectiveBehaviorDefaultsToHard(t *testing.T) {
	if (&Feature{LimitBehavior: ""}).EffectiveBehavior() != LimitHard {
		t.Error("unset behavior should default to hard")
	}
	if (&Feature{LimitBehavior: LimitSoft}).EffectiveBehavior() != LimitSoft {
		t.Error("soft behavior should be preserved")
	}
	if (&Feature{LimitBehavior: LimitHard}).EffectiveBehavior() != LimitHard {
		t.Error("hard behavior should be preserved")
	}
}
