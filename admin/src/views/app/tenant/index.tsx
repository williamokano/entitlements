// Tenant settings screen: general settings (name/slug/settings) plus a danger
// zone (suspend/reactivate/delete). Operates on the current tenant from the
// tenant store; RequireTenant guarantees one is selected in header mode.
import { useState, useSyncExternalStore } from 'react'
import { useNavigate } from 'react-router'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import { getCurrentTenantId, subscribe } from '@/lib/tenant'
import type { Tenant } from './api'
import DangerZone from './components/DangerZone'
import TenantSettingsForm from './components/TenantSettingsForm'

const statusClass = (status: string): string => {
  switch (status) {
    case 'active':
      return 'bg-success/15 text-success'
    case 'suspended':
      return 'bg-warning/15 text-warning'
    default:
      return 'bg-default-200 text-default-600'
  }
}

const Page = () => {
  const navigate = useNavigate()
  const currentId = useSyncExternalStore(subscribe, getCurrentTenantId)
  const [tenant, setTenant] = useState<Tenant | null>(null)
  // Bump to force the settings form to reload after a lifecycle change.
  const [reloadKey, setReloadKey] = useState(0)

  if (!currentId) {
    // RequireTenant normally prevents this; render nothing while it redirects.
    return null
  }

  return (
    <>
      <PageBreadcrumb title="Tenant" subtitle="Organization" />

      {tenant && (
        <div className="mb-4 flex items-center gap-3">
          <h4 className="text-lg font-semibold">{tenant.name}</h4>
          <span className={`badge badge-label ${statusClass(tenant.status)}`}>{tenant.status}</span>
        </div>
      )}

      <div className="grid grid-cols-1 gap-6">
        <TenantSettingsForm key={`${currentId}:${reloadKey}`} tenantId={currentId} onTenantChange={setTenant} />
        {tenant && (
          <DangerZone
            tenant={tenant}
            onStatusChange={() => setReloadKey((k) => k + 1)}
            onDeleted={() => navigate('/dashboard', { replace: true })}
          />
        )}
      </div>
    </>
  )
}

export default Page
