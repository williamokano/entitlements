import { type ReactNode, useSyncExternalStore } from 'react'
import { appConfig } from '@/lib/config'
import { getCurrentTenantId, subscribe } from '@/lib/tenant'
import Onboarding from '@/views/app/tenant/onboarding'

/**
 * Tenant gate (sits inside RequireAuth). In header tenant mode, when the user
 * has no current tenant, render the onboarding create-tenant screen instead of
 * the app — so every screen behind it can assume an X-Tenant-ID is being sent.
 *
 * In subdomain mode the backend resolves the tenant from the Host header, so
 * there is nothing to select client-side and the gate is a no-op.
 */
const RequireTenant = ({ children }: { children: ReactNode }) => {
  const currentId = useSyncExternalStore(subscribe, getCurrentTenantId)

  if (appConfig().tenantMode === 'header' && !currentId) {
    return <Onboarding />
  }

  return <>{children}</>
}

export default RequireTenant
