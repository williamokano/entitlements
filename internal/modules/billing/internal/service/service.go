// Package service holds the billing use cases: issuing an invoice from a
// subscription (snapshotting its pinned plan version + attached addons into
// line items, computing totals and tax, assigning the next per-tenant number,
// persisting draft→open) and guarded lifecycle transitions and credit notes.
// Each write runs in one UnitOfWork transaction so its parts commit together.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/billing/internal/domain"
	billingports "github.com/williamokano/entitlements/internal/modules/billing/ports"
	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	subports "github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// CatalogReader is the slice of the catalog the billing service needs to
// snapshot pinned plan/addon versions into invoice line items. The catalog
// module's CatalogReader satisfies it.
type CatalogReader interface {
	GetPlanVersion(ctx context.Context, id uuid.UUID) (catalogports.PlanVersionInfo, error)
	GetAddonVersion(ctx context.Context, id uuid.UUID) (catalogports.AddonVersionInfo, error)
}

// SubscriptionReader is the slice of the subscription module billing needs: the
// tenant's live subscription and its attached addons. The subscription module's
// SubscriptionReader satisfies it.
type SubscriptionReader interface {
	GetLiveForTenant(ctx context.Context, tenantID uuid.UUID) (subports.SubscriptionInfo, error)
	GetAttachedAddons(ctx context.Context, subscriptionID uuid.UUID) ([]subports.AddonAttachment, error)
}

// TaxCalculator computes tax for a set of snapshotted lines, in minor units. It
// is invoked during issuance. The skeleton ships NoopTaxCalculator (zero tax); a
// SaaS plugs in its own. A calculator must return integer minor units (already
// rounded) — the domain never does float or proportional arithmetic.
type TaxCalculator interface {
	Calculate(ctx context.Context, lines []domain.LineItem, currency string) (int64, error)
}

// NoopTaxCalculator is the default: it charges no tax.
type NoopTaxCalculator struct{}

// Calculate always returns zero tax.
func (NoopTaxCalculator) Calculate(context.Context, []domain.LineItem, string) (int64, error) {
	return 0, nil
}

// Service implements the billing use cases and ports.BillingReader.
type Service struct {
	uow       *postgres.UnitOfWork
	outbox    *events.Outbox
	repo      domain.Repository
	catalog   CatalogReader
	subs      SubscriptionReader
	tax       TaxCalculator
	provider  PaymentProvider
	proration ProrationStrategy
	// dunningOffsets is the retry schedule (offsets from the initial failure) for
	// a declined renewal charge. An empty schedule exhausts dunning immediately.
	dunningOffsets []time.Duration
	ids            id.Generator
	clk            clock.Clock
}

// New builds a Service. A nil TaxCalculator defaults to NoopTaxCalculator and a
// nil ProrationStrategy defaults to immediate proration. The PaymentProvider is
// required for the charge flow (renewal charges, dunning, payment methods) but
// may be nil for read-only/issuance-only use; the composition root injects the
// fake provider by default. dunningOffsets is the retry schedule for a failed
// renewal charge.
func New(uow *postgres.UnitOfWork, outbox *events.Outbox, repo domain.Repository, catalog CatalogReader, subs SubscriptionReader, tax TaxCalculator, provider PaymentProvider, proration ProrationStrategy, dunningOffsets []time.Duration, ids id.Generator, clk clock.Clock) *Service {
	if tax == nil {
		tax = NoopTaxCalculator{}
	}
	if proration == nil {
		proration = ImmediateProration{}
	}
	return &Service{
		uow: uow, outbox: outbox, repo: repo, catalog: catalog, subs: subs, tax: tax,
		provider: provider, proration: proration, dunningOffsets: dunningOffsets, ids: ids, clk: clk,
	}
}

// LineView is a read model of a snapshotted line item.
type LineView struct {
	Kind           string
	Description    string
	Key            string
	Version        int
	UnitPriceMinor int64
	Quantity       int
	AmountMinor    int64
	Currency       string
}

// View is a read model of an invoice.
type View struct {
	ID             uuid.UUID
	Number         int64
	SubscriptionID uuid.UUID
	Status         string
	Currency       string
	SubtotalMinor  int64
	TaxMinor       int64
	TotalMinor     int64
	IssuedAt       string
	Lines          []LineView
}

// CreditNoteView is a read model of a credit note.
type CreditNoteView struct {
	ID          uuid.UUID
	InvoiceID   uuid.UUID
	Number      int64
	AmountMinor int64
	Currency    string
	Reason      string
}

// Issue snapshots the current tenant's live subscription into a new invoice: it
// copies the pinned plan version and each attached addon (key, version, unit
// price for the cycle, quantity, currency) into line items, computes totals and
// tax, allocates the next per-tenant invoice number, and persists the invoice
// as open (draft→open) — all in one transaction.
func (s *Service) Issue(ctx context.Context) (View, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return View{}, apperr.Validation("tenant not specified")
	}
	sub, err := s.subs.GetLiveForTenant(ctx, tenantID)
	if err != nil {
		return View{}, err
	}
	pv, err := s.catalog.GetPlanVersion(ctx, sub.PlanVersionID)
	if err != nil {
		return View{}, err
	}
	currency := pv.Currency

	lines := make([]domain.LineItem, 0, 4)
	pos := 0
	planUnit, _ := priceFor(pv.Prices, sub.BillingCycle)
	lines = append(lines, domain.LineItem{
		ID:             s.ids.New(),
		Kind:           domain.LineKindPlan,
		Description:    fmt.Sprintf("Plan %s v%d (%s)", pv.PlanKey, pv.Version, sub.BillingCycle),
		Key:            pv.PlanKey,
		Version:        pv.Version,
		UnitPriceMinor: planUnit,
		Quantity:       1,
		Currency:       currency,
		Position:       pos,
	})

	addons, err := s.subs.GetAttachedAddons(ctx, sub.ID)
	if err != nil {
		return View{}, err
	}
	for _, att := range addons {
		av, err := s.catalog.GetAddonVersion(ctx, att.AddonVersionID)
		if err != nil {
			return View{}, err
		}
		qty := att.Quantity
		if qty < 1 {
			qty = 1
		}
		unit, _ := priceFor(av.Prices, sub.BillingCycle)
		pos++
		lines = append(lines, domain.LineItem{
			ID:             s.ids.New(),
			Kind:           domain.LineKindAddon,
			Description:    fmt.Sprintf("Addon %s v%d (%s) ×%d", av.AddonKey, av.Version, sub.BillingCycle, qty),
			Key:            av.AddonKey,
			Version:        av.Version,
			UnitPriceMinor: unit,
			Quantity:       qty,
			Currency:       currency,
			Position:       pos,
		})
	}

	now := s.clk.Now().UTC()
	var inv *domain.Invoice
	err = s.uow.Do(ctx, func(ctx context.Context) error {
		// Drain any deferred (credit_next_invoice) prorations for this subscription
		// into proration lines, so a plan change credited to the next invoice lands
		// here exactly once.
		pending, err := s.repo.ListPendingProrations(ctx, sub.ID)
		if err != nil {
			return err
		}
		drained := make([]uuid.UUID, 0, len(pending))
		for _, p := range pending {
			pos++
			lines = append(lines, domain.LineItem{
				ID:             s.ids.New(),
				Kind:           domain.LineKindProration,
				Description:    p.Description,
				Key:            p.Key,
				Version:        p.Version,
				UnitPriceMinor: p.AmountMinor,
				Quantity:       1,
				Currency:       p.Currency,
				Position:       pos,
			})
			drained = append(drained, p.ID)
		}

		taxMinor, err := s.tax.Calculate(ctx, lines, currency)
		if err != nil {
			return err
		}
		number, err := s.repo.NextNumber(ctx, tenantID, "invoice")
		if err != nil {
			return err
		}
		inv, err = domain.NewInvoice(s.ids.New(), tenantID, sub.ID, number, currency, lines, taxMinor, now)
		if err != nil {
			return err
		}
		if err := inv.Apply(domain.EventOpen, now); err != nil {
			return err
		}
		if err := s.repo.CreateInvoice(ctx, inv); err != nil {
			return err
		}
		return s.repo.MarkProrationsApplied(ctx, drained, inv.ID)
	})
	if err != nil {
		return View{}, err
	}
	return toView(inv), nil
}

// Pay transitions an open invoice to paid and publishes billing.invoice_paid so
// the subscription module can advance its period.
func (s *Service) Pay(ctx context.Context, invoiceID uuid.UUID) (View, error) {
	return s.transition(ctx, invoiceID, domain.EventPay)
}

// Void transitions a draft or open invoice to void.
func (s *Service) Void(ctx context.Context, invoiceID uuid.UUID) (View, error) {
	return s.transition(ctx, invoiceID, domain.EventVoid)
}

// MarkUncollectible transitions an open invoice to uncollectible.
func (s *Service) MarkUncollectible(ctx context.Context, invoiceID uuid.UUID) (View, error) {
	return s.transition(ctx, invoiceID, domain.EventMarkUncollectible)
}

func (s *Service) transition(ctx context.Context, invoiceID uuid.UUID, event domain.Event) (View, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return View{}, apperr.Validation("tenant not specified")
	}
	inv, err := s.repo.GetInvoice(ctx, tenantID, invoiceID)
	if err != nil {
		return View{}, err
	}
	now := s.clk.Now().UTC()
	if err := inv.Apply(event, now); err != nil {
		return View{}, err
	}
	err = s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.UpdateInvoice(ctx, inv); err != nil {
			return err
		}
		if event == domain.EventPay {
			_, err := s.outbox.Publish(ctx, events.EventInput{
				TenantID: inv.TenantID,
				Module:   "billing",
				Type:     billingports.EventInvoicePaid,
				Payload: billingports.InvoicePaid{
					InvoiceID:      inv.ID,
					TenantID:       inv.TenantID,
					SubscriptionID: inv.SubscriptionID,
				},
			})
			return err
		}
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return toView(inv), nil
}

// CreateCreditNote issues a credit note that negates creditMinor against an
// invoice, allocating the next per-tenant credit-note number in the same
// transaction. A draft or void invoice cannot be credited.
func (s *Service) CreateCreditNote(ctx context.Context, invoiceID uuid.UUID, creditMinor int64, reason string) (CreditNoteView, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return CreditNoteView{}, apperr.Validation("tenant not specified")
	}
	inv, err := s.repo.GetInvoice(ctx, tenantID, invoiceID)
	if err != nil {
		return CreditNoteView{}, err
	}
	if inv.Status == domain.StatusDraft || inv.Status == domain.StatusVoid {
		return CreditNoteView{}, apperr.Conflict(fmt.Sprintf("cannot credit a %s invoice", inv.Status))
	}
	now := s.clk.Now().UTC()
	var cn *domain.CreditNote
	err = s.uow.Do(ctx, func(ctx context.Context) error {
		number, err := s.repo.NextNumber(ctx, tenantID, "credit_note")
		if err != nil {
			return err
		}
		cn, err = domain.NewCreditNote(s.ids.New(), inv.ID, tenantID, number, creditMinor, inv.Currency, reason, now)
		if err != nil {
			return err
		}
		return s.repo.CreateCreditNote(ctx, cn)
	})
	if err != nil {
		return CreditNoteView{}, err
	}
	return CreditNoteView{
		ID: cn.ID, InvoiceID: cn.InvoiceID, Number: cn.Number,
		AmountMinor: cn.AmountMinor, Currency: cn.Currency, Reason: cn.Reason,
	}, nil
}

// Get returns one invoice scoped to the current tenant (another tenant's invoice
// is a NotFound).
func (s *Service) Get(ctx context.Context, invoiceID uuid.UUID) (View, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return View{}, apperr.Validation("tenant not specified")
	}
	inv, err := s.repo.GetInvoice(ctx, tenantID, invoiceID)
	if err != nil {
		return View{}, err
	}
	return toView(inv), nil
}

// List returns the current tenant's invoices, newest number first.
func (s *Service) List(ctx context.Context) ([]View, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return nil, apperr.Validation("tenant not specified")
	}
	invs, err := s.repo.ListInvoices(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(invs))
	for _, inv := range invs {
		out = append(out, toView(inv))
	}
	return out, nil
}

// ListCreditNotes returns an invoice's credit notes, scoped to the tenant.
func (s *Service) ListCreditNotes(ctx context.Context, invoiceID uuid.UUID) ([]CreditNoteView, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return nil, apperr.Validation("tenant not specified")
	}
	// Ensure the invoice belongs to the tenant before listing.
	if _, err := s.repo.GetInvoice(ctx, tenantID, invoiceID); err != nil {
		return nil, err
	}
	notes, err := s.repo.ListCreditNotes(ctx, tenantID, invoiceID)
	if err != nil {
		return nil, err
	}
	out := make([]CreditNoteView, 0, len(notes))
	for _, cn := range notes {
		out = append(out, CreditNoteView{
			ID: cn.ID, InvoiceID: cn.InvoiceID, Number: cn.Number,
			AmountMinor: cn.AmountMinor, Currency: cn.Currency, Reason: cn.Reason,
		})
	}
	return out, nil
}

// GetInvoice implements ports.BillingReader.
func (s *Service) GetInvoice(ctx context.Context, id uuid.UUID) (billingports.InvoiceInfo, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return billingports.InvoiceInfo{}, apperr.Validation("tenant not specified")
	}
	inv, err := s.repo.GetInvoice(ctx, tenantID, id)
	if err != nil {
		return billingports.InvoiceInfo{}, err
	}
	return toInfo(inv), nil
}

// ListInvoices implements ports.BillingReader.
func (s *Service) ListInvoices(ctx context.Context) ([]billingports.InvoiceInfo, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return nil, apperr.Validation("tenant not specified")
	}
	invs, err := s.repo.ListInvoices(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]billingports.InvoiceInfo, 0, len(invs))
	for _, inv := range invs {
		out = append(out, toInfo(inv))
	}
	return out, nil
}

// priceFor returns the unit price for a billing cycle from a snapshot's prices,
// and whether one was found. A missing price yields 0 (the line still snapshots
// the cycle at zero rather than dropping it).
func priceFor(prices []catalogports.PriceInfo, cycle string) (int64, bool) {
	for _, p := range prices {
		if p.Cycle == cycle {
			return p.AmountMinor, true
		}
	}
	return 0, false
}

func toInfo(inv *domain.Invoice) billingports.InvoiceInfo {
	return billingports.InvoiceInfo{
		ID: inv.ID, TenantID: inv.TenantID, SubscriptionID: inv.SubscriptionID,
		Number: inv.Number, Status: string(inv.Status), Currency: inv.Currency,
		SubtotalMinor: inv.SubtotalMinor, TaxMinor: inv.TaxMinor, TotalMinor: inv.TotalMinor,
	}
}

func toView(inv *domain.Invoice) View {
	issued := ""
	if inv.IssuedAt != nil {
		issued = inv.IssuedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	v := View{
		ID: inv.ID, Number: inv.Number, SubscriptionID: inv.SubscriptionID,
		Status: string(inv.Status), Currency: inv.Currency,
		SubtotalMinor: inv.SubtotalMinor, TaxMinor: inv.TaxMinor, TotalMinor: inv.TotalMinor,
		IssuedAt: issued,
	}
	for _, li := range inv.Lines {
		v.Lines = append(v.Lines, LineView{
			Kind: string(li.Kind), Description: li.Description, Key: li.Key, Version: li.Version,
			UnitPriceMinor: li.UnitPriceMinor, Quantity: li.Quantity, AmountMinor: li.Amount(), Currency: li.Currency,
		})
	}
	return v
}
