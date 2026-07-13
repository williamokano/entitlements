// Package billing is the money side: invoice generation with line-item
// snapshots, an abstract PaymentProvider port (with a fake adapter for the
// skeleton), tokenized payment-method storage, and an idempotent renewal charge
// flow (issue → charge → invoice_paid | payment_failed). Dunning with a
// configurable retry schedule and proration strategies are T-027.
//
// It reports outcomes as events (InvoicePaid / PaymentFailed / ...) and never
// decides subscription policy. Hexagonal layout. Implemented across T-025 and
// T-026.
package billing
