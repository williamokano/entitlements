// Package clock abstracts the current time behind a Clock interface with a
// real implementation and a frozen implementation for deterministic tests.
//
// Domain and application code must read time through this package (UTC),
// never time.Now directly. Implemented in T-002.
package clock
