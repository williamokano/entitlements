import PageBreadcrumb from '@/components/PageBreadcrumb'
import Icon from '@/components/wrappers/Icon'

type PlaceholderPageProps = {
  title: string
  subtitle?: string
  icon?: string
  description: string
}

/**
 * Empty-state card for product screens whose real implementation ships in a
 * later F-task.
 */
const PlaceholderPage = ({ title, subtitle, icon = 'layout-dashboard', description }: PlaceholderPageProps) => {
  return (
    <>
      <PageBreadcrumb title={title} subtitle={subtitle} />

      <div className="grid grid-cols-1">
        <div className="card">
          <div className="card-body">
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <Icon icon={icon} className="text-default-300 mb-4 size-12" />
              <h4 className="text-dark mb-2 text-lg font-semibold">{title}</h4>
              <p className="text-default-400 mx-auto w-full md:w-1/2">{description}</p>
            </div>
          </div>
        </div>
      </div>
    </>
  )
}

export default PlaceholderPage
