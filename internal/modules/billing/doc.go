// Package billing is the money side: invoice generation with line-item
// snapshots, an abstract PaymentProvider port (with a fake provider for the
// skeleton), dunning with a configurable retry schedule, and proration
// strategies.
//
// It reports outcomes as events (InvoicePaid / PaymentFailed / ...) and never
// decides subscription policy. Hexagonal layout. Implemented across T-025,
// T-026, and T-027.
package billing
