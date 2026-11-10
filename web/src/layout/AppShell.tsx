import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Outlet, useMatch, useNavigate } from 'react-router-dom'
import { useAdministration, useAdministrations } from '../administrations/queries'
import { can } from '../management/queries'
import { useCurrentUser, useLogout } from '../auth/queries'
import { BellIcon, SearchIcon } from './icons'
import { LanguageSelect } from './LanguageSelect'
import { Logo } from './Logo'
import { Menu } from './Menu'
import { createPath } from '../onboarding/queries'
import { saveTheme, storedTheme, type Theme } from './theme'

export function AppShell() {
  const { t } = useTranslation()
  return (
    <>
      <a className="skip-link" href="#main">{t('common.skip')}</a>
      <nav className="topnav" aria-label={t('nav.main')}>
        <div className="topnav__left">
          <Link className="brand" to="/administrations" aria-label={t('nav.home')}>
            <Logo className="brand__full" />
            <Logo variant="mark" className="brand__mark" decorative />
          </Link>
          <AdministrationSwitcher />
          <AdministrationSections />
        </div>
        <div className="topnav__search" role="search">
          <SearchIcon />
          <input type="search" aria-label={t('nav.search')} placeholder={t('nav.search')} disabled aria-describedby="search-soon" />
          <span id="search-soon" className="sr-only">{t('nav.searchSoon')}</span>
        </div>
        <div className="topnav__right">
          <Menu label={<BellIcon />} ariaLabel={t('nav.notifications')} caret={false} align="right">
            <p className="menu__note">{t('nav.noNotifications')}</p>
          </Menu>
          <UserMenu />
        </div>
      </nav>
      <TwoFactorBanner />
      <main id="main" tabIndex={-1}>
        <Outlet />
      </main>
    </>
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
            <li key={a.id}><Link to={a.uri}>{a.name}</Link></li>
          ))}
        </ul>
        <hr />
        <ul className="menu__list">
          <li><Link to="/administrations">{t('nav.all')}</Link></li>
          <li><Link to={createPath}>{t('nav.new')}</Link></li>
        </ul>
      </nav>
    </Menu>
  )
}

/** Links to the parts of the open administration. */
function AdministrationSections() {
  const { t } = useTranslation()
  const id = useAdministrationId()
  const administration = useAdministration(id)
  const nested = useMatch('/:id/*')
  if (!id) {
    return null
  }
  const section = nested?.params['*']?.split('/')[0] ?? ''
  const bank = can(administration.data, 'bank')
  const link = (to: string, label: string, active: boolean) => (
    <Link className="topnav__btn topnav__section" to={`/${id}/${to}`} aria-current={active ? 'page' : undefined}>{label}</Link>
  )
  return (
    <div className="topnav__sections">
      {bank && link('financial_mutations', t('nav.bank'), section === 'financial_mutations')}
      {link('settings', t('nav.settings'), section === 'settings')}
    </div>
  )
}

/** The open administration's id, if the address is inside one. */
function useAdministrationId() {
  const nested = useMatch('/:id/*')
  const home = useMatch('/:id')
  const id = home?.params.id ?? nested?.params.id ?? ''
  return /^\d{1,19}$/.test(id) ? id : ''
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
    <Menu label={me.data?.name ?? ''} align="right">
      <ul className="menu__list">
        <li><Link to="/user">{t('nav.account')}</Link></li>
        {me.data?.instance_admin && <li><Link to="/instance/settings">{t('nav.instance')}</Link></li>}
        <li><a href="/docs/api/">{t('nav.docs')}</a></li>
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
        {t('nav.signOut')}
      </button>
    </Menu>
  )
}
