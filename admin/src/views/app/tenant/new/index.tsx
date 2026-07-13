// "Add another tenant" screen, reached from the TopBar switcher's Create action
// (when the user already has a tenant, so the onboarding gate does not apply).
// On success the new tenant is stored + selected and we route to its settings.
import { useNavigate } from 'react-router'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import CreateTenantForm from '../components/CreateTenantForm'

const Page = () => {
  const navigate = useNavigate()

  return (
    <>
      <PageBreadcrumb title="Create tenant" subtitle="Organization" />
      <div className="grid grid-cols-1">
        <div className="card">
          <div className="card-header">
            <h4 className="card-title">New tenant</h4>
            <p className="text-default-500 mt-1 text-sm">Create a new tenant and switch to it.</p>
          </div>
          <div className="card-body max-w-md">
            <CreateTenantForm onCreated={() => navigate('/tenant', { replace: true })} />
          </div>
        </div>
      </div>
    </>
  )
}

export default Page
