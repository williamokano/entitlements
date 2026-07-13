// Package rest is the billing module's driving adapter: HTTP handlers for
// invoices and credit notes, mounted under /api/v1/billing (tenant-scoped). Every
// route requires an authenticated principal.
package rest

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/billing/internal/service"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// New returns the billing module's HTTP handler. Routes are prefix-relative; the
// composition root mounts them under /api/v1/billing.
func New(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /invoices", issueInvoice(svc))
	mux.HandleFunc("GET /invoices", listInvoices(svc))
	mux.HandleFunc("GET /invoices/{id}", getInvoice(svc))
	mux.HandleFunc("POST /invoices/{id}/pay", transition(svc.Pay))
	mux.HandleFunc("POST /invoices/{id}/void", transition(svc.Void))
	mux.HandleFunc("POST /invoices/{id}/uncollectible", transition(svc.MarkUncollectible))
	mux.HandleFunc("POST /invoices/{id}/credit-notes", createCreditNote(svc))
	mux.HandleFunc("GET /invoices/{id}/credit-notes", listCreditNotes(svc))
	return mux
}

func issueInvoice(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		view, err := svc.Issue(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, invoiceResponse(view))
	}
}

func listInvoices(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		views, err := svc.List(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(views))
		for _, v := range views {
			out = append(out, invoiceResponse(v))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"invoices": out})
	}
}

func getInvoice(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid invoice id"))
			return
		}
		view, err := svc.Get(r.Context(), id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, invoiceResponse(view))
	}
}

func transition(action func(ctx context.Context, id uuid.UUID) (service.View, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid invoice id"))
			return
		}
		view, err := action(r.Context(), id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, invoiceResponse(view))
	}
}

func createCreditNote(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid invoice id"))
			return
		}
		var body struct {
			AmountMinor int64  `json:"amount_minor"`
			Reason      string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid request body"))
			return
		}
		cn, err := svc.CreateCreditNote(r.Context(), id, body.AmountMinor, body.Reason)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, creditNoteResponse(cn))
	}
}

func listCreditNotes(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r) {
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid invoice id"))
			return
		}
		notes, err := svc.ListCreditNotes(r.Context(), id)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		out := make([]map[string]any, 0, len(notes))
		for _, cn := range notes {
			out = append(out, creditNoteResponse(cn))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"credit_notes": out})
	}
}

func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := authctx.PrincipalFromContext(r.Context()); !ok {
		httpx.WriteProblem(w, r, apperr.Unauthorized("authentication required"))
		return false
	}
	return true
}

func invoiceResponse(v service.View) map[string]any {
	lines := make([]map[string]any, 0, len(v.Lines))
	for _, li := range v.Lines {
		lines = append(lines, map[string]any{
			"kind":             li.Kind,
			"description":      li.Description,
			"key":              li.Key,
			"version":          li.Version,
			"unit_price_minor": li.UnitPriceMinor,
			"quantity":         li.Quantity,
			"amount_minor":     li.AmountMinor,
			"currency":         li.Currency,
		})
	}
	out := map[string]any{
		"id":              v.ID,
		"number":          v.Number,
		"subscription_id": v.SubscriptionID,
		"status":          v.Status,
		"currency":        v.Currency,
		"subtotal_minor":  v.SubtotalMinor,
		"tax_minor":       v.TaxMinor,
		"total_minor":     v.TotalMinor,
		"line_items":      lines,
	}
	if v.IssuedAt != "" {
		out["issued_at"] = v.IssuedAt
	}
	return out
}

func creditNoteResponse(cn service.CreditNoteView) map[string]any {
	return map[string]any{
		"id":           cn.ID,
		"invoice_id":   cn.InvoiceID,
		"number":       cn.Number,
		"amount_minor": cn.AmountMinor,
		"currency":     cn.Currency,
		"reason":       cn.Reason,
	}
}
