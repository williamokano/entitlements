// Package httpx contains the shared HTTP server, middleware chain, router
// composition helper, and RFC 7807 problem+json error mapping used by every
// module's driving adapter.
//
// It holds no business logic. Modules build a Router, mount their handlers,
// and return errors from the apperr taxonomy; httpx maps those to problem+json
// responses and attaches a request ID, structured logging, and panic recovery.
package httpx
