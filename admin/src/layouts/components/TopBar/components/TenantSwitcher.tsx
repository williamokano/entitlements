import { useSyncExternalStore } from 'react'
import { Link } from 'react-router'
import Icon from '@/components/wrappers/Icon'
import { appConfig } from '@/lib/config'
import { getTenantState, setCurrentTenant, subscribe } from '@/lib/tenant'

/**
 * Tenant switcher (TopBar, header tenant mode). Lists the locally-known tenants
 * and the current selection; picking one makes it current so subsequent API
 * calls carry its X-Tenant-ID. A "Create tenant" action links to the create
 * screen. In subdomain mode the backend resolves the tenant from the Host, so
 * the switcher renders nothing.
 */
const TenantSwitcher = () => {
  const { tenants, currentId } = useSyncExternalStore(subscribe, getTenantState)

  if (appConfig().tenantMode !== 'header') return null

  const current = tenants.find((t) => t.id === currentId) ?? null

  // Setting the current tenant is enough: the api client reads it per request,
  // and tenant-scoped screens keyed on the current id re-fetch on the change.
  const select = (id: string) => setCurrentTenant(id)

  return (
    <div id="tenant-switcher" className="topbar-item hs-dropdown relative inline-flex">
      <button
        className="hs-dropdown-toggle topbar-link flex cursor-pointer items-center gap-2 px-3!"
        aria-haspopup="menu"
        aria-expanded="false"
        aria-label="Switch tenant"
      >
        <Icon icon="building" className="text-lg" />
        <span className="hidden max-w-40 truncate lg:inline">{current ? current.name : 'Select tenant'}</span>
        <Icon icon="chevron-down" className="align-middle" />
      </button>

      <div className="hs-dropdown-menu min-w-56" role="menu" aria-orientation="vertical">
        <div className="px-3.5 py-2">
          <h6 className="text-xs">Tenants</h6>
        </div>

        {tenants.length === 0 ? (
          <p className="text-default-500 px-3.5 py-2 text-sm">No tenants yet.</p>
        ) : (
          tenants.map((tenant) => (
            <button
              key={tenant.id}
              type="button"
              role="menuitemradio"
              aria-checked={tenant.id === currentId}
              onClick={() => select(tenant.id)}
              className="dropdown-item flex w-full items-center justify-between text-start"
            >
              <span className="min-w-0">
                <span className="block truncate font-medium">{tenant.name}</span>
                <span className="text-default-400 block truncate text-xs">{tenant.slug}</span>
              </span>
              {tenant.id === currentId && <Icon icon="check" className="text-primary text-base" />}
            </button>
          ))
        )}

        <div className="dropdown-divider"></div>

        <Link to="/tenant/new" className="dropdown-item">
          <Icon icon="plus" className="text-base align-middle" />
          <span className="align-middle">Create tenant</span>
        </Link>
      </div>
    </div>
  )
}

export default TenantSwitcher
