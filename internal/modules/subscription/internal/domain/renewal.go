package domain

import "time"

// RenewalDue reports whether the subscription's current period has ended and a
// SubscriptionRenewalDue has not yet been emitted for this period boundary.
// Only a live, non-trial subscription renews (a trial resolves via the trial
// job, not renewal).
func (s *Subscription) RenewalDue(now time.Time) bool {
	switch s.Status {
	case StateActive, StatePastDue, StateGrace:
	default:
		return false
	}
	if now.Before(s.CurrentPeriodEnd) {
		return false
	}
	return s.RenewalEmittedPeriodEnd == nil || s.RenewalEmittedPeriodEnd.Before(s.CurrentPeriodEnd)
}

// MarkRenewalEmitted records that a renewal was emitted for the current period
// boundary, so it is not emitted again until the period advances.
func (s *Subscription) MarkRenewalEmitted(now time.Time) {
	end := s.CurrentPeriodEnd
	s.RenewalEmittedPeriodEnd = &end
	s.UpdatedAt = now
}

// AdvancePeriod rolls the subscription into its next billing period: it applies
// any scheduled plan change at the boundary (re-pinning, recorded as a pending
// plan change for the service to publish), moves the period window forward from
// the old period end, and clears the renewal marker so the next period can emit
// again. It is idempotent against being called when the period has not yet
// ended (returns false without changing anything).
func (s *Subscription) AdvancePeriod(now time.Time) bool {
	if now.Before(s.CurrentPeriodEnd) {
		return false
	}
	s.ApplyScheduledChange(now) // re-pins + records a pending plan change if scheduled
	s.CurrentPeriodStart = s.CurrentPeriodEnd
	s.CurrentPeriodEnd = periodEnd(s.CurrentPeriodStart, s.BillingCycle)
	s.RenewalEmittedPeriodEnd = nil
	s.UpdatedAt = now
	return true
}

// TrialEndingDue reports whether the subscription is trialing and within
// daysBefore of its trial end, with the TrialEnding event not yet emitted.
func (s *Subscription) TrialEndingDue(now time.Time, daysBefore int) bool {
	if s.Status != StateTrialing || s.TrialEndsAt == nil || s.TrialEndingEmitted {
		return false
	}
	if !now.Before(*s.TrialEndsAt) {
		return false // already ended — that's TrialEnded's job
	}
	threshold := s.TrialEndsAt.AddDate(0, 0, -daysBefore)
	return !now.Before(threshold)
}

// MarkTrialEndingEmitted records that the pre-expiry TrialEnding event fired.
func (s *Subscription) MarkTrialEndingEmitted(now time.Time) {
	s.TrialEndingEmitted = true
	s.UpdatedAt = now
}

// TrialEnded reports whether a trialing subscription has reached its trial end.
func (s *Subscription) TrialEnded(now time.Time) bool {
	return s.Status == StateTrialing && s.TrialEndsAt != nil && !now.Before(*s.TrialEndsAt)
}

// BeginPeriodAfterTrial starts the first paid billing period when a trial
// converts to active: the period runs from the trial end for one billing cycle,
// the trial marker is dropped, and the renewal marker is cleared. Call it after
// applying the activation transition.
func (s *Subscription) BeginPeriodAfterTrial(now time.Time) {
	start := s.CurrentPeriodEnd // equals the trial end while trialing
	s.CurrentPeriodStart = start
	s.CurrentPeriodEnd = periodEnd(start, s.BillingCycle)
	s.TrialEndsAt = nil
	s.RenewalEmittedPeriodEnd = nil
	s.UpdatedAt = now
}
