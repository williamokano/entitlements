import { useRoutes } from 'react-router'
import { ToastContainer } from 'react-toastify'
import 'react-toastify/dist/ReactToastify.css'
import { routes } from '@/routes'

const App = () => {
  return (
    <>
      {useRoutes(routes)}
      <ToastContainer position="top-right" autoClose={4000} newestOnTop theme="colored" />
    </>
  )
}

export default App
