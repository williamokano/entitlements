// Package apperr defines the application error taxonomy shared across every
// layer and module.
//
// Domain and application code returns these typed errors; the transport layer
// (internal/platform/httpx) maps a Kind to an HTTP status and an RFC 7807
// problem+json body. Keeping the taxonomy here — free of net/http — lets the
// domain return meaningful errors without importing the transport layer.
//
// An Error carries a client-safe Message and, optionally, a wrapped internal
// cause. The cause is for logs only and is never exposed to clients.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies an error so the transport layer can map it to a status.
type Kind int

const (
	// KindInternal is an unexpected failure. Its detail is never exposed to
	// clients (maps to HTTP 500).
	KindInternal Kind = iota
	// KindValidation is a malformed or invalid request (HTTP 400).
	KindValidation
	// KindUnauthorized means the caller is unauthenticated (HTTP 401).
	KindUnauthorized
	// KindForbidden means the caller is authenticated but not allowed (HTTP 403).
	KindForbidden
	// KindNotFound means the target resource does not exist (HTTP 404).
	KindNotFound
	// KindConflict means the request conflicts with current state (HTTP 409).
	KindConflict
	// KindQuotaExceeded means a metered limit would be breached by the request
	// (HTTP 422). The request is well-formed; the tenant is simply out of quota.
	KindQuotaExceeded
)

// String returns a stable, lowercase identifier for the Kind.
func (k Kind) String() string {
	switch k {
	case KindValidation:
		return "validation"
	case KindUnauthorized:
		return "unauthorized"
	case KindForbidden:
		return "forbidden"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindQuotaExceeded:
		return "quota_exceeded"
	default:
		return "internal"
	}
}

// Error is a typed application error.
type Error struct {
	Kind    Kind
	Message string // client-safe; may be shown to callers
	cause   error  // internal; for logs only, never exposed
}

// Error implements the error interface. It includes the cause for logs; the
// transport layer must not put Error() into a client response verbatim for
// internal errors.
func (e *Error) Error() string {
	switch {
	case e.Message != "" && e.cause != nil:
		return fmt.Sprintf("%s: %v", e.Message, e.cause)
	case e.Message != "":
		return e.Message
	case e.cause != nil:
		return e.cause.Error()
	default:
		return e.Kind.String()
	}
}

// Unwrap exposes the wrapped cause for errors.Is/As traversal.
func (e *Error) Unwrap() error { return e.cause }

// New builds an Error with a client-safe message.
func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

// Wrap builds an Error of kind that wraps an internal cause. The message is
// client-safe; the cause is retained for logs only.
func Wrap(kind Kind, cause error, message string) *Error {
	return &Error{Kind: kind, Message: message, cause: cause}
}

// Convenience constructors.

// NotFound builds a KindNotFound error.
func NotFound(message string) *Error { return New(KindNotFound, message) }

// Validation builds a KindValidation error.
func Validation(message string) *Error { return New(KindValidation, message) }

// Conflict builds a KindConflict error.
func Conflict(message string) *Error { return New(KindConflict, message) }

// QuotaExceeded builds a KindQuotaExceeded error (a metered limit was reached).
func QuotaExceeded(message string) *Error { return New(KindQuotaExceeded, message) }

// Unauthorized builds a KindUnauthorized error.
func Unauthorized(message string) *Error { return New(KindUnauthorized, message) }

// Forbidden builds a KindForbidden error.
func Forbidden(message string) *Error { return New(KindForbidden, message) }

// Internal builds a KindInternal error wrapping an internal cause.
func Internal(cause error) *Error { return &Error{Kind: KindInternal, cause: cause} }

// Coerce returns err as an *Error, treating any non-Error as KindInternal.
// It never returns nil for a non-nil err.
func Coerce(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Kind: KindInternal, cause: err}
}

// KindOf reports the Kind of err (KindInternal if err is not an *Error).
func KindOf(err error) Kind {
	if e := Coerce(err); e != nil {
		return e.Kind
	}
	return KindInternal
}
