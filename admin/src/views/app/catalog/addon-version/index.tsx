// Add-on version editor page: chrome around AddonVersionEditor, which loads the
// version and enforces the draft-only rule.
import { Link, useParams } from 'react-router'
import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'
import CatalogTabs from '../components/CatalogTabs'
import AddonVersionEditor from './components/AddonVersionEditor'

const Page = () => {
  const { addonId = '', vid = '' } = useParams()

  return (
    <>
      <PageBreadcrumb title="Add-on version" subtitle="Catalog" />
      <CatalogTabs />

      <Link to={`/catalog/addons/${addonId}`} className="text-default-500 hover:text-primary mb-4 inline-flex items-center gap-1 text-sm">
        <Icon icon="arrow-left" className="text-base" />
        Back to add-on
      </Link>

      <AddonVersionEditor addonId={addonId} vid={vid} />
    </>
  )
}

export default Page
