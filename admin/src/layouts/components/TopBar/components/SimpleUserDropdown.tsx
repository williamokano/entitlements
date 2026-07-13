import user1 from '@/assets/images/users/user-1.jpg'
import Icon from '@/components/wrappers/Icon'
import { META_DATA } from '@/config/constants'
import { useAuth } from '@/hooks/useAuth'
import { Link } from 'react-router'

const UserDropdown = () => {
  const { logout } = useAuth()

  return (
    <div id="simple-user-dropdown" className="topbar-item hs-dropdown before:bg-default-700/35 relative inline-flex before:h-4.5 before:w-px before:content-['']">
      <button className="hs-dropdown-toggle topbar-link ms-2.5 cursor-pointer items-center px-3! flex" aria-haspopup="menu" aria-expanded="false" aria-label="Dropdown">
        <img src={user1} alt="user-image" className="size-8 rounded-full lg:me-3" />
        <div className="hidden lg:flex items-center gap-1.5">
          <h5 className="pro-username">{META_DATA.username}</h5>
          <Icon icon="chevron-down" className="align-middle" />
        </div>
      </button>

      <div className="hs-dropdown-menu min-w-48" role="menu" aria-orientation="vertical" aria-labelledby="hs-dropdown-with-icons">
        <div className="py-2 px-3.5">
          <h6 className="text-xs">Welcome back 👋!</h6>
        </div>

        <Link to="/account/security" className="dropdown-item">
          <Icon icon="settings-2" className="text-base align-middle" />
          <span className="align-middle">Account Security</span>
        </Link>

        <div className="dropdown-divider"></div>

        <button type="button" onClick={() => void logout()} className="dropdown-item font-semibold text-danger w-full text-start">
          <Icon icon="logout" className="text-base align-middle" />
          <span className="align-middle">Log Out</span>
        </button>
      </div>
    </div>
  )
}

export default UserDropdown
