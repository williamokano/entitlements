import { type ReactNode } from 'react'
import AuthLogo from '@/components/AuthLogo'
import { currentYear, META_DATA } from '@/config/constants'

type AuthShellProps = {
  title: string
  subtitle?: ReactNode
  children: ReactNode
  /** Rendered below the card (e.g. cross-links between auth screens). */
  footer?: ReactNode
}

/**
 * Shared chrome for the product auth screens — adapted from the theme's
 * views/auth/basic layout (logo + centered card + copyright). Every F-003 auth
 * page renders its form inside this shell so they stay visually consistent.
 */
const AuthShell = ({ title, subtitle, children, footer }: AuthShellProps) => {
  return (
    <div className="flex min-h-screen items-center p-12.5">
      <div className="container">
        <div className="flex justify-center px-2.5">
          <div className="2xl:w-4/10 md:w-1/2 sm:w-2/3 w-full">
            <div className="mb-3 flex flex-col items-center justify-center text-center">
              <AuthLogo />
              <h4 className="font-bold text-base text-dark mt-5 mb-2">{title}</h4>
              {subtitle && <p className="text-default-400 mx-auto w-full lg:w-3/4 mb-4">{subtitle}</p>}
            </div>
            <div className="card p-7.5 rounded-2xl">
              {children}
              {footer}
            </div>
            <p className="text-default-400 mt-7.5 text-center">
              © {currentYear}&nbsp;{META_DATA.name}
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

export default AuthShell
