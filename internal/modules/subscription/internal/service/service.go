// Package service holds the subscription use cases. Each state change persists
// the subscription, appends a history row, and publishes an event — all in one
// transaction, so they commit or roll back together.
package service

import (
	"context"

	"github.com/google/uuid"

	catalogports "github.com/williamokano/entitlements/internal/modules/catalog/ports"
	"github.com/williamokano/entitlements/internal/modules/subscription/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/subscription/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/authctx"
	"github.com/williamokano/entitlements/internal/platform/clock"
	"github.com/williamokano/entitlements/internal/platform/events"
	"github.com/williamokano/entitlements/internal/platform/id"
	"github.com/williamokano/entitlements/internal/platform/postgres"
)

// CatalogReader is the slice of the catalog the subscription service needs:
// reading pinned plan versions and addon versions. The catalog module's
// CatalogReader satisfies it.
type CatalogReader interface {
	GetPlanVersion(ctx context.Context, id uuid.UUID) (catalogports.PlanVersionInfo, error)
	GetAddonVersion(ctx context.Context, id uuid.UUID) (catalogports.AddonVersionInfo, error)
}

// Service implements the subscription use cases and ports.SubscriptionReader.
type Service struct {
	uow     *postgres.UnitOfWork
	outbox  *events.Outbox
	repo    domain.Repository
	catalog CatalogReader
	ids     id.Generator
	clk     clock.Clock
}

// New builds a Service.
func New(uow *postgres.UnitOfWork, outbox *events.Outbox, repo domain.Repository, catalog CatalogReader, ids id.Generator, clk clock.Clock) *Service {
	return &Service{uow: uow, outbox: outbox, repo: repo, catalog: catalog, ids: ids, clk: clk}
}

// ScheduledChangeView is the read model of a pending plan change.
type ScheduledChangeView struct {
	PlanVersionID uuid.UUID
	BillingCycle  string
}

// AddonView is the read model of an attached addon.
type AddonView struct {
	AddonVersionID uuid.UUID
	Quantity       int
}

// View is a read model of a subscription.
type View struct {
	ID                 uuid.UUID
	PlanVersionID      uuid.UUID
	BillingCycle       string
	Status             string
	CurrentPeriodStart string
	CurrentPeriodEnd   string
	TrialEndsAt        string
	CancelAtPeriodEnd  bool
	ScheduledChange    *ScheduledChangeView
	Addons             []AddonView
}

// Create starts a subscription for the current tenant against a pinned plan
// version, trialing or active per the plan's trial config. A tenant may hold at
// most one live subscription (a second create is a 409).
func (s *Service) Create(ctx context.Context, planVersionID uuid.UUID, cycle string) (View, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return View{}, apperr.Validation("tenant not specified")
	}
	pv, err := s.catalog.GetPlanVersion(ctx, planVersionID)
	if err != nil {
		return View{}, err
	}
	if pv.Status != "published" {
		return View{}, apperr.Validation("plan version is not published")
	}
	if !cycleOffered(pv, cycle) {
		return View{}, apperr.Validation("plan version has no price for this billing cycle")
	}

	trialDays := 0
	if pv.TrialEnabled {
		trialDays = pv.TrialDays
	}
	now := s.clk.Now().UTC()
	sub, err := domain.Create(s.ids.New(), tenantID, planVersionID, domain.BillingCycle(cycle), trialDays, now, actorOf(ctx))
	if err != nil {
		return View{}, err
	}

	err = s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, sub); err != nil {
			return err
		}
		return s.recordAndPublish(ctx, sub)
	})
	if err != nil {
		return View{}, err
	}
	return toView(sub), nil
}

// GetForTenant returns the current tenant's live subscription, including its
// attached addons.
func (s *Service) GetForTenant(ctx context.Context) (View, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return View{}, apperr.Validation("tenant not specified")
	}
	sub, err := s.repo.GetLiveForTenant(ctx, tenantID)
	if err != nil {
		return View{}, err
	}
	addons, err := s.repo.ListAddons(ctx, sub.ID)
	if err != nil {
		return View{}, err
	}
	view := toView(sub)
	for _, a := range addons {
		view.Addons = append(view.Addons, AddonView{AddonVersionID: a.AddonVersionID, Quantity: a.Quantity})
	}
	return view, nil
}

// Cancel cancels the current tenant's live subscription, immediately or at the
// period boundary.
func (s *Service) Cancel(ctx context.Context, immediate bool, reason string) (View, error) {
	sub, err := s.liveForTenant(ctx)
	if err != nil {
		return View{}, err
	}
	if immediate {
		return s.apply(ctx, sub, domain.EventCancel, reason)
	}
	// Schedule at period end: no state change, no transition event.
	if err := sub.ScheduleCancelAtPeriodEnd(); err != nil {
		return View{}, err
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		return s.repo.Update(ctx, sub)
	}); err != nil {
		return View{}, err
	}
	return toView(sub), nil
}

// Reactivate returns a suspended subscription to active.
func (s *Service) Reactivate(ctx context.Context, reason string) (View, error) {
	return s.applyForTenant(ctx, domain.EventReactivate, reason)
}

// Pause voluntarily holds an active subscription.
func (s *Service) Pause(ctx context.Context, reason string) (View, error) {
	return s.applyForTenant(ctx, domain.EventPause, reason)
}

// Resume returns a paused subscription to active.
func (s *Service) Resume(ctx context.Context, reason string) (View, error) {
	return s.applyForTenant(ctx, domain.EventResume, reason)
}

// GetLiveForTenant implements ports.SubscriptionReader.
func (s *Service) GetLiveForTenant(ctx context.Context, tenantID uuid.UUID) (ports.SubscriptionInfo, error) {
	sub, err := s.repo.GetLiveForTenant(ctx, tenantID)
	if err != nil {
		return ports.SubscriptionInfo{}, err
	}
	return ports.SubscriptionInfo{
		ID:                 sub.ID,
		TenantID:           sub.TenantID,
		PlanVersionID:      sub.PlanVersionID,
		BillingCycle:       string(sub.BillingCycle),
		Status:             string(sub.Status),
		CurrentPeriodStart: sub.CurrentPeriodStart,
		CurrentPeriodEnd:   sub.CurrentPeriodEnd,
		CancelAtPeriodEnd:  sub.CancelAtPeriodEnd,
	}, nil
}

func (s *Service) applyForTenant(ctx context.Context, event domain.Event, reason string) (View, error) {
	sub, err := s.liveForTenant(ctx)
	if err != nil {
		return View{}, err
	}
	return s.apply(ctx, sub, event, reason)
}

// apply runs a state transition and persists + publishes it atomically.
func (s *Service) apply(ctx context.Context, sub *domain.Subscription, event domain.Event, reason string) (View, error) {
	now := s.clk.Now().UTC()
	if err := sub.Apply(event, reason, actorOf(ctx), s.ids.New(), now); err != nil {
		return View{}, err
	}
	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, sub); err != nil {
			return err
		}
		return s.recordAndPublish(ctx, sub)
	}); err != nil {
		return View{}, err
	}
	return toView(sub), nil
}

// recordAndPublish appends the pending transition and publishes the event within
// the ambient transaction.
func (s *Service) recordAndPublish(ctx context.Context, sub *domain.Subscription) error {
	tr := sub.PendingTransition()
	if tr == nil {
		return nil
	}
	if err := s.repo.AppendTransition(ctx, tr); err != nil {
		return err
	}
	_, err := s.outbox.Publish(ctx, events.EventInput{
		TenantID: sub.TenantID,
		Module:   "subscription",
		Type:     ports.EventSubscriptionTransitioned,
		Payload: ports.SubscriptionTransitioned{
			SubscriptionID: sub.ID,
			TenantID:       sub.TenantID,
			PlanVersionID:  sub.PlanVersionID,
			From:           string(tr.From),
			To:             string(tr.To),
			Event:          string(tr.Event),
		},
	})
	return err
}

func (s *Service) liveForTenant(ctx context.Context) (*domain.Subscription, error) {
	tenantID, err := authctx.MustTenant(ctx)
	if err != nil {
		return nil, apperr.Validation("tenant not specified")
	}
	return s.repo.GetLiveForTenant(ctx, tenantID)
}

func cycleOffered(pv catalogports.PlanVersionInfo, cycle string) bool {
	for _, p := range pv.Prices {
		if p.Cycle == cycle {
			return true
		}
	}
	return false
}

func actorOf(ctx context.Context) string {
	if p, ok := authctx.PrincipalFromContext(ctx); ok {
		return p.Subject
	}
	return "system"
}

func toView(sub *domain.Subscription) View {
	trial := ""
	if sub.TrialEndsAt != nil {
		trial = sub.TrialEndsAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	var scheduled *ScheduledChangeView
	if sub.Scheduled != nil {
		scheduled = &ScheduledChangeView{PlanVersionID: sub.Scheduled.PlanVersionID, BillingCycle: string(sub.Scheduled.BillingCycle)}
	}
	return View{
		ID:                 sub.ID,
		PlanVersionID:      sub.PlanVersionID,
		BillingCycle:       string(sub.BillingCycle),
		Status:             string(sub.Status),
		CurrentPeriodStart: sub.CurrentPeriodStart.UTC().Format("2006-01-02T15:04:05Z07:00"),
		CurrentPeriodEnd:   sub.CurrentPeriodEnd.UTC().Format("2006-01-02T15:04:05Z07:00"),
		TrialEndsAt:        trial,
		CancelAtPeriodEnd:  sub.CancelAtPeriodEnd,
		ScheduledChange:    scheduled,
	}
}
