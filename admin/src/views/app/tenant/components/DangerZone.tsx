// Tenant lifecycle actions (suspend / reactivate / delete). Each destructive
// action is gated behind a sweetalert2 confirm; the request only fires after
// the user confirms. Deleting the current tenant removes it from the store and
// falls back to onboarding or another known tenant.
import { useState } from 'react'
import { toast } from 'react-toastify'
import Icon from '@/components/wrappers/Icon'
import { ApiError } from '@/lib/api'
import { confirmAction } from '@/lib/confirm'
import { forgetTenant } from '@/lib/tenant'
import { deleteTenant, reactivateTenant, suspendTenant, type Tenant } from '../api'

const errorMessage = (err: unknown): string => {
  if (err instanceof ApiError) return err.detail || err.title || `Request failed (${err.status})`
  return 'Something went wrong. Please try again.'
}

type Props = {
  tenant: Tenant
  /** Called after a suspend/reactivate so the page can refresh the status. */
  onStatusChange?: () => void
  /** Called after a successful delete so the page can route away. */
  onDeleted?: () => void
}

const DangerZone = ({ tenant, onStatusChange, onDeleted }: Props) => {
  const [busy, setBusy] = useState(false)
  const isSuspended = tenant.status === 'suspended'

  const run = async (action: () => Promise<void>, success: string) => {
    if (busy) return
    setBusy(true)
    try {
      await action()
      toast.success(success)
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const onSuspend = async () => {
    const ok = await confirmAction({
      title: 'Suspend tenant?',
      text: `"${tenant.name}" will be suspended and its users will lose access until reactivated.`,
      confirmText: 'Suspend',
    })
    if (!ok) return
    await run(async () => {
      await suspendTenant(tenant.id)
      onStatusChange?.()
    }, 'Tenant suspended.')
  }

  const onReactivate = async () => {
    const ok = await confirmAction({
      title: 'Reactivate tenant?',
      text: `"${tenant.name}" will be reactivated and its users will regain access.`,
      confirmText: 'Reactivate',
      danger: false,
    })
    if (!ok) return
    await run(async () => {
      await reactivateTenant(tenant.id)
      onStatusChange?.()
    }, 'Tenant reactivated.')
  }

  const onDelete = async () => {
    const ok = await confirmAction({
      title: 'Delete tenant?',
      text: `"${tenant.name}" will be permanently deleted. This cannot be undone.`,
      confirmText: 'Delete',
    })
    if (!ok) return
    await run(async () => {
      await deleteTenant(tenant.id)
      forgetTenant(tenant.id)
      onDeleted?.()
    }, 'Tenant deleted.')
  }

  return (
    <div className="card border border-danger/30">
      <div className="card-header">
        <h4 className="card-title text-danger flex items-center gap-1.5">
          <Icon icon="alert-triangle" className="text-base" />
          Danger zone
        </h4>
      </div>
      <div className="card-body space-y-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <h5 className="font-medium">{isSuspended ? 'Reactivate tenant' : 'Suspend tenant'}</h5>
            <p className="text-default-500 text-sm">
              {isSuspended ? 'Restore access for this tenant and its members.' : 'Temporarily block access without deleting data.'}
            </p>
          </div>
          {isSuspended ? (
            <button type="button" className="btn bg-success text-white hover:bg-success-hover" disabled={busy} onClick={() => void onReactivate()}>
              Reactivate
            </button>
          ) : (
            <button type="button" className="btn bg-warning text-white hover:bg-warning-hover" disabled={busy} onClick={() => void onSuspend()}>
              Suspend
            </button>
          )}
        </div>

        <div className="border-default-200 border-t" />

        <div className="flex items-center justify-between gap-4">
          <div>
            <h5 className="font-medium">Delete tenant</h5>
            <p className="text-default-500 text-sm">Permanently remove this tenant and all of its data.</p>
          </div>
          <button type="button" className="btn bg-danger text-white hover:bg-danger-hover" disabled={busy} onClick={() => void onDelete()}>
            Delete
          </button>
        </div>
      </div>
    </div>
  )
}

export default DangerZone
