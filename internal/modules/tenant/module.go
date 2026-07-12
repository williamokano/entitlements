// Package tenant is the tenancy module: the root of isolation that every other
// module is scoped by. It owns tenant lifecycle, extensible JSONB settings, and
// a hookable provisioning pipeline driven by the tenant.created event.
package tenant

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	pgadapter "github.com/williamokano/entitlements/internal/modules/tenant/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/tenant/internal/adapters/rest"
	"github.com/williamokano/entitlements/internal/modules/tenant/internal/service"
	"github.com/williamokano/entitlements/internal/modules/tenant/ports"
	"github.com/williamokano/entitlements/internal/platform/events"
)

const consumerName = "tenant-provisioning"

// defaultInvitationTTL bounds how long a tenant invitation stays acceptable.
const defaultInvitationTTL = 7 * 24 * time.Hour

// Module wires the tenant module from platform dependencies.
type Module struct {
	deps    app.Deps
	svc     *service.Service
	members *service.MembershipService
	handler http.Handler
	hooks   []ports.ProvisioningHook
}

// Option customizes the module.
type Option func(*Module)

// WithProvisioningHooks registers onboarding steps that run, in order, after a
// tenant is created (delivered via the outbox).
func WithProvisioningHooks(hooks ...ports.ProvisioningHook) Option {
	return func(m *Module) { m.hooks = append(m.hooks, hooks...) }
}

// New constructs the tenant module.
func New(deps app.Deps, opts ...Option) *Module {
	repo := pgadapter.New(deps.Pool)
	svc := service.New(deps.UnitOfWork, deps.Outbox, repo, deps.IDs, deps.Clock)
	members := service.NewMembershipService(
		deps.UnitOfWork, deps.Outbox,
		pgadapter.NewMemberships(deps.Pool), pgadapter.NewInvitations(deps.Pool),
		deps.IDs, deps.Clock, defaultInvitationTTL,
	)
	m := &Module{deps: deps, svc: svc, members: members, handler: rest.New(svc, members)}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Name is the module's route prefix segment.
func (m *Module) Name() string { return "tenants" }

// Handler is the module's HTTP handler, mounted under /api/v1/tenants.
func (m *Module) Handler() http.Handler { return m.handler }

// Subscriptions runs the provisioning pipeline when a tenant is created. The
// whole pipeline is idempotent and runs in the delivery transaction, so its
// steps and the processed-event marker commit together (exactly once).
func (m *Module) Subscriptions() []app.Subscription {
	handler := events.Idempotent(consumerName, m.deps.Pool, func(ctx context.Context, e events.Event) error {
		var payload ports.TenantCreated
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			return err
		}
		for _, hook := range m.hooks {
			if err := hook.Provision(ctx, payload.TenantID); err != nil {
				return err
			}
		}
		return nil
	})
	return []app.Subscription{{EventType: ports.EventTenantCreated, Handler: handler}}
}

// Port returns the module's public facade for other modules.
func (m *Module) Port() ports.TenantReader { return m.svc }

// MembershipPort returns the membership facade other modules use (e.g.
// authorization resolves a user's role in a tenant through it).
func (m *Module) MembershipPort() ports.MembershipReader { return m.members }

// loggingHook is the default provisioning hook: it logs each provisioned
// tenant. Real SaaS steps (seed roles, create a trial subscription) are
// registered alongside it at the composition root.
type loggingHook struct{ logger *slog.Logger }

// NewLoggingHook returns a provisioning hook that logs each provisioned tenant.
func NewLoggingHook(logger *slog.Logger) ports.ProvisioningHook { return &loggingHook{logger: logger} }

func (h *loggingHook) Name() string { return "logging" }

func (h *loggingHook) Provision(ctx context.Context, tenantID uuid.UUID) error {
	h.logger.InfoContext(ctx, "provisioning tenant", "tenant_id", tenantID)
	return nil
}
