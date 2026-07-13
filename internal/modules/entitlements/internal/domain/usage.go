package domain

import "time"

// PeriodKey derives the usage-counter partition key for a feature's reset_period.
// A new period is simply a new key, so the absent counter row reads as zero —
// period reset is lazy and needs no background job.
//
//   - monthly       → the UTC calendar month, "2006-01".
//   - billing_cycle → the subscription's current-period start, or "cycle:none"
//     when the tenant has no live subscription to anchor the cycle.
//   - never / ""    → the single fixed key "never".
func PeriodKey(resetPeriod string, now time.Time, periodStart *time.Time) string {
	switch resetPeriod {
	case ResetMonthly:
		return now.UTC().Format("2006-01")
	case ResetBillingCycle:
		if periodStart == nil {
			return "cycle:none"
		}
		return "cycle:" + periodStart.UTC().Format(time.RFC3339)
	default: // ResetNever and the empty (unset) default
		return "never"
	}
}

// EffectiveBehavior returns a limit feature's enforcement behavior, defaulting an
// unset behavior to hard so quotas are enforced unless explicitly opted soft.
func (f *Feature) EffectiveBehavior() string {
	if f.LimitBehavior == LimitSoft {
		return LimitSoft
	}
	return LimitHard
}

// ToInt64 coerces a JSON-decoded numeric limit value to int64, reporting whether
// it is a whole number. Limits round-trip through jsonb as float64, so both int
// and float forms are accepted; a non-integral or non-numeric value is rejected.
func ToInt64(v any) (int64, bool) { return toInt64(v) }
