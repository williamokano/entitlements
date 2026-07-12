import logoDark from '@/assets/images/logo-black.png'
import logo from '@/assets/images/logo.png'
import { appConfig } from '@/lib/config'
import { Link } from 'react-router'

const AuthLogo = () => {
  const { appName } = appConfig()
  return (
    <>
      <Link to="/" className="auth-logo">
        <img src={logoDark} alt={`${appName} logo`} className="flex dark:hidden" />
        <img src={logo} alt={`${appName} dark logo`} className="hidden dark:flex" />
      </Link>
    </>
  )
}

export default AuthLogo
