// API keys management — adapted from the theme's apps/api-keys page and wired
// to the real /api/v1/api-keys endpoints (list, create with one-time secret
// reveal, revoke). See docs/FRONTEND.md.
import PageBreadcrumb from '@/components/PageBreadcrumb'
import ApiKeysCard from './components/ApiKeysCard'

const Page = () => {
  return (
    <>
      <PageBreadcrumb title="API Keys" subtitle="Organization" />
      <ApiKeysCard />
    </>
  )
}

export default Page
