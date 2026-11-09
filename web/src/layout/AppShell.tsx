import { useState } from 'react'
import { Link, Outlet, useMatch, useNavigate } from 'react-router-dom'
import { useAdministrations } from '../administrations/queries'
import { useCurrentUser, useLogout } from '../auth/queries'
import { BellIcon, Mark } from './icons'
import { Menu } from './Menu'
import { saveTheme, storedTheme, type Theme } from './theme'

export function AppShell() {
  return (
    <>
      <a className="skip-link" href="#main">Skip to content</a>
      <nav className="topnav" aria-label="Main">
        <div className="topnav__left">
          <Link className="brand" to="/administrations" aria-label="MoneyHarpy home">
            <Mark className="brand__mark" />
            <span className="brand__name">MoneyHarpy</span>
          </Link>
          <AdministrationSwitcher />
        </div>
        <div className="topnav__right">
          <Menu label={<BellIcon />} ariaLabel="Notifications" caret={false} align="right">
            <p className="menu__note">You have no notifications.</p>
          </Menu>
          <UserMenu />
        </div>
      </nav>
      <main id="main" tabIndex={-1}>
        <Outlet />
      </main>
    </>
  )
}

function AdministrationSwitcher() {
  const administrations = useAdministrations()
  const home = useMatch('/:id')
  const nested = useMatch('/:id/*')
  const currentId = home?.params.id ?? nested?.params.id
  const current = administrations.data?.find((a) => a.id === currentId)

  return (
    <Menu label={current?.name ?? 'Select administration'}>
      <nav aria-label="Your administrations">
        <ul className="menu__list">
          {administrations.data?.map((a) => (
            <li key={a.id}><Link to={a.uri}>{a.name}</Link></li>
          ))}
        </ul>
        <hr />
        <ul className="menu__list">
          <li><Link to="/administrations">All administrations</Link></li>
          <li><Link to="/administrations/new">New administration</Link></li>
        </ul>
      </nav>
    </Menu>
  )
}

function UserMenu() {
  const me = useCurrentUser()
  const logout = useLogout()
  const navigate = useNavigate()
  const [theme, setTheme] = useState<Theme>(storedTheme)

  const changeTheme = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const next = e.target.value as Theme
    setTheme(next)
    saveTheme(next)
  }

  return (
    <Menu label={me.data?.name ?? ''} align="right">
      <ul className="menu__list">
        <li><Link to="/user">User account</Link></li>
        {me.data?.instance_admin && <li><Link to="/instance/settings">Instance settings</Link></li>}
        <li><a href="/docs/api/">API documentation</a></li>
      </ul>
      <label className="menu__theme">
        <span>Theme</span>
        <select value={theme} onChange={changeTheme}>
          <option value="system">System</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </label>
      <hr />
      <button type="button" className="menu__action" onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/login') })}>
        Sign out
      </button>
    </Menu>
  )
}
