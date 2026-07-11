// Package catalog is the product definition: plans with immutable published
// versions, per-cycle pricing, trial and grace configuration, feature
// grants, and addons carrying entitlement deltas.
//
// Subscriptions pin a plan version so existing customers are grandfathered
// when a plan changes. Hexagonal layout. Implemented across T-017 and T-018.
package catalog
