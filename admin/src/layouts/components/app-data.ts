import { appConfig } from '@/lib/config'
import { type MenuItemType } from '@/types'

/**
 * Sidebar menu for the real product (the vendored demo keeps its own menu in
 * data.ts, shown only under /demo).
 */
export const appMenuItems: MenuItemType[] = [
  {
    slug: 'main',
    label: 'Main',
    isTitle: true,
    children: [
      {
        url: '/dashboard',
        icon: 'dashboard',
        slug: 'app:dashboard',
        label: 'Dashboard',
      },
    ],
  },
  {
    slug: 'organization',
    label: 'Organization',
    isTitle: true,
    children: [
      {
        url: '/tenant',
        icon: 'building',
        slug: 'app:tenant',
        label: 'Tenant',
      },
      {
        url: '/members',
        icon: 'users',
        slug: 'app:members',
        label: 'Members',
      },
      {
        url: '/api-keys',
        icon: 'key',
        slug: 'app:api-keys',
        label: 'API Keys',
      },
      {
        url: '/roles',
        icon: 'users-group',
        slug: 'app:roles',
        label: 'Roles',
      },
    ],
  },
  {
    slug: 'billing',
    label: 'Billing',
    isTitle: true,
    children: [
      {
        url: '/catalog',
        icon: 'category',
        slug: 'app:catalog',
        label: 'Catalog',
      },
      {
        url: '/subscription',
        icon: 'invoice',
        slug: 'app:subscription',
        label: 'Subscription',
      },
    ],
  },
  ...(appConfig().enableDemo
    ? [
        {
          slug: 'reference',
          label: 'Reference',
          isTitle: true,
          children: [
            {
              url: '/demo/dashboard/projects',
              icon: 'components',
              slug: 'app:demo',
              label: 'Theme Demo',
            },
          ],
        } satisfies MenuItemType,
      ]
    : []),
]
