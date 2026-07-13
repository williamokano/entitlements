// Account security area (F-003): change password, manage active sessions, and
// (re)request an email-verification link. Adapted from the theme's
// account-settings page; wired to the real /api/v1/auth self-service endpoints.
import PageBreadcrumb from '@/components/PageBreadcrumb'
import ChangePasswordForm from './components/ChangePasswordForm'
import ResendVerification from './components/ResendVerification'
import SessionsCard from './components/SessionsCard'

const Page = () => {
  return (
    <>
      <PageBreadcrumb title="Security" subtitle="Account" />
      <div className="grid grid-cols-1 gap-5 xl:grid-cols-2">
        <ChangePasswordForm />
        <ResendVerification />
      </div>
      <div className="mt-5">
        <SessionsCard />
      </div>
    </>
  )
}

export default Page
