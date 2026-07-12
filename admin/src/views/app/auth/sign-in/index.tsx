// Adapted from the theme's views/auth/basic/sign-in — wired to the real
// auth store. The full auth suite (sign-up, recovery, …) arrives with F-003.
import AuthLogo from '@/components/AuthLogo'
import { currentYear, META_DATA } from '@/config/constants'
import { isAuthenticated, subscribe } from '@/lib/auth'
import { useSyncExternalStore } from 'react'
import { Navigate } from 'react-router'
import Form from './components/Form'

const Page = () => {
  const authenticated = useSyncExternalStore(subscribe, isAuthenticated)

  if (authenticated) {
    return <Navigate to="/" replace />
  }

  return (
    <>
      <div className="flex min-h-screen items-center p-12.5">
        <div className="container">
          <div className="flex justify-center px-2.5">
            <div className="2xl:w-4/10 md:w-1/2 sm:w-2/3 w-full">
              <div className="mb-3 flex flex-col items-center justify-center text-center">
                <AuthLogo />
                <h4 className="font-bold text-base text-dark mt-5 mb-2">Welcome</h4>
                <p className="text-default-400 mx-auto w-full lg:w-3/4 mb-4">Let’s get you signed in. Enter your email and password to continue.</p>
              </div>
              <div className="card p-7.5 rounded-2xl">
                <Form />
              </div>

              <p className="text-default-400 mt-7.5 text-center">© {currentYear}&nbsp;{META_DATA.name}</p>
            </div>
          </div>
        </div>
      </div>
    </>
  )
}

export default Page
