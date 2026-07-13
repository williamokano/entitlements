// No-tenant onboarding empty state. Shown (via RequireTenant) when the user is
// authenticated in header tenant mode but has no current tenant. Creating one
// stores + selects it, then drops the user on the dashboard — from where every
// API call carries the new tenant's X-Tenant-ID.
import { useNavigate } from 'react-router'
import Icon from '@/components/wrappers/Icon'
import { appConfig } from '@/lib/config'
import CreateTenantForm from '../components/CreateTenantForm'

const Onboarding = () => {
  const navigate = useNavigate()

  return (
    <div className="flex min-h-screen items-center justify-center bg-default-50 p-4">
      <div className="w-full max-w-md">
        <div className="mb-6 text-center">
          <span className="bg-primary/10 text-primary mx-auto flex size-14 items-center justify-center rounded-full">
            <Icon icon="building" className="text-2xl" />
          </span>
          <h3 className="mt-4 text-xl font-semibold">Create your first tenant</h3>
          <p className="text-default-500 mt-1 text-sm">
            {appConfig().appName} organizes everything under a tenant. Create one to get started.
          </p>
        </div>
        <div className="card">
          <div className="card-body">
            <CreateTenantForm onCreated={() => navigate('/dashboard', { replace: true })} submitLabel="Create tenant & continue" />
          </div>
        </div>
      </div>
    </div>
  )
}

export default Onboarding
