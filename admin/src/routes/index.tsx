import { lazy } from 'react'
import { Navigate, type RouteObject } from 'react-router'
import RequireAuth from '@/components/RequireAuth'
import MainLayout from '@/layouts/MainLayout'

// Real product routes live at "/". Screens are adapted copies of theme pages
// under views/app/ (see docs/FRONTEND.md); placeholders are replaced by their
// F-task implementations.
const appRoutes: RouteObject[] = [
  {
    element: (
      <RequireAuth>
        <MainLayout />
      </RequireAuth>
    ),
    children: [
      { path: '/', element: <Navigate to="/dashboard" replace /> },
      { path: '/dashboard', Component: lazy(() => import('@/views/app/dashboard')) },
      { path: '/tenant', Component: lazy(() => import('@/views/app/tenant')) },
      { path: '/members', Component: lazy(() => import('@/views/app/members')) },
      { path: '/api-keys', Component: lazy(() => import('@/views/app/api-keys')) },
      { path: '/roles', Component: lazy(() => import('@/views/app/roles')) },
      { path: '/catalog', Component: lazy(() => import('@/views/app/catalog')) },
      { path: '/subscription', Component: lazy(() => import('@/views/app/subscription')) },
    ],
  },
  { path: '/auth/sign-in', Component: lazy(() => import('@/views/app/auth/sign-in')) },
]

// The vendored Inspinia demo, mounted as a lazy splat route. The build-time
// VITE_ENABLE_DEMO flag statically drops this branch (and with it the whole
// demo bundle) from production builds; at runtime window.__APP_CONFIG__ can
// additionally disable it without a rebuild (checked inside routes/demo.tsx).
const demoRoutes: RouteObject[] =
  import.meta.env.VITE_ENABLE_DEMO === 'false'
    ? []
    : [{ path: '/demo/*', Component: lazy(() => import('@/routes/demo')) }]

export const routes: RouteObject[] = [...appRoutes, ...demoRoutes]
