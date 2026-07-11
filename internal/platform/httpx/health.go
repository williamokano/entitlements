package httpx

import "net/http"

// Health returns a handler that reports service health as 200 with a small
// JSON body. It is used for both liveness (/healthz) and readiness (/readyz)
// until readiness gains real dependency checks.
func Health() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
