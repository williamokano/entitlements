// Package entitlements answers "what can this tenant do right now, and how
// much of it".
//
// It holds a dynamic feature registry, a resolution pipeline (plan grants →
// addon deltas → tenant overrides ⇒ effective entitlements, materialized per
// tenant and invalidated by events), tenant overrides with expiry and audit,
// and usage tracking with atomic quota enforcement (soft/hard limits).
// Hexagonal layout. Implemented across T-022, T-023, and T-024.
package entitlements
