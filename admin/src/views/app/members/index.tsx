// Members & invitations (F-005) — wired to the tenant module's membership
// endpoints under /api/v1/tenants/{id}. Those routes take the tenant in the
// path, so the screen reads the active tenant from the tenant store rather than
// relying on the X-Tenant-ID header the api client sends.
import PageBreadcrumb from '@/components/PageBreadcrumb'
import { getCurrentTenantId, subscribe } from '@/lib/tenant'
import { useSyncExternalStore } from 'react'
import InvitationsCard from './components/InvitationsCard'
import MembersCard from './components/MembersCard'

const Page = () => {
  const tenantId = useSyncExternalStore(subscribe, getCurrentTenantId)

  return (
    <>
      <PageBreadcrumb title="Members" subtitle="Organization" />
      {tenantId ? (
        <>
          <MembersCard tenantId={tenantId} />
          <InvitationsCard tenantId={tenantId} />
        </>
      ) : (
        // RequireTenant normally redirects before this renders; this covers the
        // store being cleared underneath us (in another tab, say).
        <div className="card">
          <div className="card-body py-10 text-center">
            <h5 className="font-medium">No organization selected</h5>
            <p className="text-default-500 mt-1 text-sm">Choose an organization to manage its members.</p>
          </div>
        </div>
      )}
    </>
  )
}

export default Page
