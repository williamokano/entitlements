import logoBlack from '@/assets/images/logo-black.png'
import logoSm from '@/assets/images/logo-sm.png'
import logo from '@/assets/images/logo.png'
import { appConfig } from '@/lib/config'

const AppLogo = () => {
  const { appName } = appConfig()
  return (
    <>
      <div className="logo-light">
        <img src={logo} className="logo-lg h-6" alt={`${appName} logo`} />
        <img src={logoSm} className="logo-sm h-6" alt={`${appName} small logo`} />
      </div>
      <div className="logo-dark">
        <img src={logoBlack} className="logo-lg h-6" alt={`${appName} logo`} />
        <img src={logoSm} className="logo-sm h-6" alt={`${appName} small logo`} />
      </div>
    </>
  )
}

export default AppLogo
