package authorization

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/app"
	pgadapter "github.com/williamokano/entitlements/internal/modules/authorization/internal/adapters/postgres"
	"github.com/williamokano/entitlements/internal/modules/authorization/internal/adapters/rest"
	"github.com/williamokano/entitlements/internal/modules/authorization/internal/service"
	"github.com/williamokano/entitlements/internal/modules/authorization/ports"
	tenantports "github.com/williamokano/entitlements/internal/modules/tenant/ports"
	"github.com/williamokano/entitlements/internal/platform/httpx"
)

// Module wires the authorization module from platform dependencies.
type Module struct {
	deps    app.Deps
	svc     *service.Service
	handler http.Handler
}

// New constructs the authorization module.
func New(deps app.Deps) *Module {
	svc := service.New(
		deps.UnitOfWork,
		pgadapter.NewRoles(deps.Pool),
		pgadapter.NewAssignments(deps.Pool),
		deps.IDs,
		deps.Clock,
	)
	m := &Module{deps: deps, svc: svc}
	guard := func(permission string) httpx.Middleware {
		return RequirePermission(svc, permission)
	}
	m.handler = rest.New(svc, guard)
	return m
}

// Name is the module's route prefix segment.
func (m *Module) Name() string { return "roles" }

// Handler is the module's HTTP handler, mounted under /api/v1/roles.
func (m *Module) Handler() http.Handler { return m.handler }

// Subscriptions is empty: authorization is queried synchronously, not via events.
func (m *Module) Subscriptions() []app.Subscription { return nil }

// Authorizer returns the module's permission-check facade for other modules and
// the RequirePermission middleware.
func (m *Module) Authorizer() ports.Authorizer { return m.svc }

// SeedRolesHook returns a tenant provisioning hook that seeds the system roles
// (owner/admin/member) when a tenant is created. Register it at the composition
// root alongside the tenant module's other hooks.
func (m *Module) SeedRolesHook() tenantports.ProvisioningHook {
	return seedRolesHook{svc: m.svc}
}

type seedRolesHook struct{ svc *service.Service }

func (h seedRolesHook) Name() string { return "authorization-seed-roles" }

func (h seedRolesHook) Provision(ctx context.Context, tenantID uuid.UUID) error {
	return h.svc.SeedSystemRoles(ctx, tenantID)
}
