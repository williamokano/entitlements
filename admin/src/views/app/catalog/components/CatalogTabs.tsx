// Sub-navigation shared by the catalog sections (plans / addons / pricing
// preview). The "Catalog" sidebar item points at the plans list; these tabs move
// between the sections without leaving the catalog.
import { NavLink } from 'react-router'
import { cn } from '@/utils/helpers'

const tabs = [
  { to: '/catalog', label: 'Plans', end: true },
  { to: '/catalog/addons', label: 'Add-ons', end: false },
  { to: '/catalog/pricing', label: 'Pricing preview', end: false },
]

const CatalogTabs = () => (
  <div className="mb-5 border-b border-default-200">
    <nav className="flex gap-1" aria-label="Catalog sections">
      {tabs.map((tab) => (
        <NavLink
          key={tab.to}
          to={tab.to}
          end={tab.end}
          className={({ isActive }) =>
            cn(
              '-mb-px border-b-2 px-4 py-2.5 text-sm font-medium',
              isActive ? 'border-primary text-primary' : 'border-transparent text-default-500 hover:text-default-700',
            )
          }
        >
          {tab.label}
        </NavLink>
      ))}
    </nav>
  </div>
)

export default CatalogTabs
