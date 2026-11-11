import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Outlet, useMatch, useNavigate } from 'react-router-dom'
import { useAdministration, useAdministrations } from '../administrations/queries'
import { useCurrentUser, useLogout } from '../auth/queries'
import { useAdministrationId } from './administrationId'
import { MenuToggle, SidenavProvider } from './Sidenav'
import { MenuIcon, SearchIcon } from './icons'
import { NotificationsAside } from './NotificationsAside'
import { LanguageSelect } from './LanguageSelect'
import { Logo } from './Logo'
import { Menu } from './Menu'
import { createPath } from '../onboarding/queries'
import { lastAdministration } from '../overview/queries'
import { saveTheme, storedTheme, type Theme } from './theme'

export function AppShell() {
  const { t } = useTranslation()
  return (
    <SidenavProvider>
      <a className="skip-link" href="#main">{t('common.skip')}</a>
      <nav className="topnav" aria-label={t('nav.main')}>
        <div className="topnav__left">
          <MenuToggle />
          <HomeLink />
          <AdministrationSwitcher />
        </div>
        <div className="topnav__search" role="search">
          <SearchIcon />
          <input type="search" aria-label={t('nav.search')} placeholder={t('nav.search')} disabled aria-describedby="search-soon" />
          <span id="search-soon" className="sr-only">{t('nav.searchSoon')}</span>
        </div>
        <div className="topnav__right">
          <NotificationsAside />
          <UserMenu />
        </div>
      </nav>
      <TwoFactorBanner />
      <main id="main" tabIndex={-1}>
        <Outlet />
      </main>
    </SidenavProvider>
  )
}

/** The logo leads to the open administration's overview, or the one opened last. */
function HomeLink() {
  const { t } = useTranslation()
  const id = useAdministrationId()
  const administrations = useAdministrations()
  const last = lastAdministration()
  const known = last && administrations.data?.some((a) => a.id === last)
  const to = id ? `/${id}` : known ? `/${last}` : '/administrations'
  return (
    <Link className="brand" to={to} aria-label={t('nav.home')}>
      <Logo className="brand__full" />
      <Logo variant="mark" className="brand__mark" decorative />
    </Link>
  )
}

function AdministrationSwitcher() {
  const { t } = useTranslation()
  const administrations = useAdministrations()
  const home = useMatch('/:id')
  const nested = useMatch('/:id/*')
  const currentId = home?.params.id ?? nested?.params.id
  const current = administrations.data?.find((a) => a.id === currentId)

  return (
    <Menu label={current?.name ?? t('nav.select')}>
      <nav aria-label={t('nav.yours')}>
        <ul className="menu__list">
          {administrations.data?.map((a) => (
            <li key={a.id}>
              <Link to={a.uri} aria-current={a.id === currentId ? 'true' : undefined}>
                <MenuIcon kind={a.id === currentId ? 'current' : 'other'} />{a.name}
              </Link>
            </li>
          ))}
        </ul>
        <hr />
        <ul className="menu__list">
          <li><Link to="/administrations"><MenuIcon kind="all" />{t('nav.all')}</Link></li>
          <li><Link to={createPath}><MenuIcon kind="new" />{t('nav.new')}</Link></li>
        </ul>
      </nav>
    </Menu>
  )
}

/** Tells a user without two-step verification that this administration needs it. */
function TwoFactorBanner() {
  const { t } = useTranslation()
  const id = useAdministrationId()
  const administration = useAdministration(id)
  const me = useCurrentUser()
  if (!id || !administration.data?.require_two_factor || me.data?.two_factor_enabled !== false) {
    return null
  }
  return (
    <div className="banner" role="alert">
      <p>{t('management.bannerRequired', { name: administration.data.name })}</p>
      <Link className="btn btn--accent btn--small" to="/user#two-step">{t('management.setUpTwoFactor')}</Link>
    </div>
  )
}

function UserMenu() {
  const { t } = useTranslation()
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
    <Menu label={<><MenuIcon kind="user" /><span className="topnav__name">{me.data?.name ?? ''}</span></>} align="right">
      <ul className="menu__list">
        <li><Link to="/user"><MenuIcon kind="account" />{t('nav.account')}</Link></li>
        {me.data?.instance_admin && <li><Link to="/instance/settings"><MenuIcon kind="instance" />{t('nav.instance')}</Link></li>}
        <li><a href="/docs/api/"><MenuIcon kind="docs" />{t('nav.docs')}</a></li>
      </ul>
      <label className="menu__theme">
        <span>{t('common.theme')}</span>
        <select value={theme} onChange={changeTheme}>
          <option value="system">{t('common.system')}</option>
          <option value="light">{t('common.light')}</option>
          <option value="dark">{t('common.dark')}</option>
        </select>
      </label>
      <LanguageSelect className="menu__theme" />
      <hr />
      <button type="button" className="menu__action" onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/login') })}>
        <MenuIcon kind="signOut" />{t('nav.signOut')}
      </button>
    </Menu>
  )
}
