package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/williamokano/entitlements/internal/platform/apperr"
)

// ProblemContentType is the media type for RFC 7807 problem details.
const ProblemContentType = "application/problem+json"

// Problem is an RFC 7807 problem-details document. RequestID is a non-standard
// extension member carrying the request's correlation ID.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// statusForKind maps an application error Kind to an HTTP status code.
func statusForKind(k apperr.Kind) int {
	switch k {
	case apperr.KindValidation:
		return http.StatusBadRequest
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindQuotaExceeded:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// WriteProblem writes err as an RFC 7807 problem+json response. Internal
// errors are rendered with an opaque detail so implementation specifics never
// leak to clients; the caller is responsible for logging the real cause.
func WriteProblem(w http.ResponseWriter, r *http.Request, err error) {
	e := apperr.Coerce(err)
	status := statusForKind(e.Kind)

	detail := e.Message
	if e.Kind == apperr.KindInternal {
		detail = "An internal error occurred."
	}

	p := Problem{
		Type:      "about:blank",
		Title:     http.StatusText(status),
		Status:    status,
		Detail:    detail,
		Instance:  r.URL.Path,
		RequestID: RequestIDFromContext(r.Context()),
	}

	w.Header().Set("Content-Type", ProblemContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// WriteJSON writes v as a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
