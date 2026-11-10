import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Outlet, useMatch, useNavigate } from 'react-router-dom'
import { useAdministrations } from '../administrations/queries'
import { useCurrentUser, useLogout } from '../auth/queries'
import { BellIcon, Mark } from './icons'
import { LanguageSelect } from './LanguageSelect'
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
            <Mark className="brand__mark" />
            <span className="brand__name">MoneyHarpy</span>
          </Link>
          <AdministrationSwitcher />
        </div>
        <div className="topnav__right">
          <Menu label={<BellIcon />} ariaLabel={t('nav.notifications')} caret={false} align="right">
            <p className="menu__note">{t('nav.noNotifications')}</p>
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
