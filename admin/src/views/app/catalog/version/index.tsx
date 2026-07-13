// Plan version editor page: chrome (breadcrumb, tabs, back link) around the
// VersionEditor, which loads the version and enforces the draft-only rule.
import { Link, useParams } from 'react-router'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'
import CatalogTabs from '../components/CatalogTabs'
import VersionEditor from './components/VersionEditor'

const Page = () => {
  const { planId = '', vid = '' } = useParams()

  return (
    <>
      <PageBreadcrumb title="Plan version" subtitle="Catalog" />
      <CatalogTabs />

      <Link to={`/catalog/plans/${planId}`} className="text-default-500 hover:text-primary mb-4 inline-flex items-center gap-1 text-sm">
        <Icon icon="arrow-left" className="text-base" />
        Back to plan
      </Link>

      <VersionEditor planId={planId} vid={vid} />
    </>
  )
}

export default Page
