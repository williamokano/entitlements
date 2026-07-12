import { LayoutProvider } from '@/context/useLayoutContext'
import { preline } from '@/utils/preline'
import React from 'react'

// Route protection lives in the RequireAuth wrapper (see routes/index.tsx),
// so this wrapper only wires up global providers.
const AppProvidersWrapper = ({ children }: { children: React.ReactNode }) => {
  preline.init()
  return <LayoutProvider>{children}</LayoutProvider>
}

export default AppProvidersWrapper
