// Package httpx contains the shared HTTP server, middleware chain, and
// RFC 7807 problem+json error mapping used by every module's driving adapter.
//
// The server, middleware (request-id, recovery, logging, auth, tenant,
// idempotency), and error mapper are implemented across T-004, T-007, and
// T-008. This package holds no business logic.
package httpx
