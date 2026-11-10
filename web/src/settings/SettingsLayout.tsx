import { useTranslation } from 'react-i18next'
import { Link, NavLink, Outlet, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { NotFoundPage } from '../layout/NotFoundPage'

export const sections = ['company', 'invoices_and_estimates', 'bookkeeping', 'bank', 'administration', 'connections'] as const
export type Section = (typeof sections)[number]

/**
 * The settings area of one administration: a section menu beside the page.
 * Edit pages leave the menu out and lead back with their own button.
 */
export function SettingsLayout({ nav = true }: { nav?: boolean }) {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const valid = /^\d{1,19}$/.test(administrationId)
  const administration = useAdministration(administrationId)

  if (!valid || (administration.error instanceof ApiError && administration.error.status < 500)) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return null
  }
  if (!nav) {
    return (
      <div className="settings settings--page">
        <div className="settings__page">
          <Outlet />
        </div>
      </div>
    )
  }
  const base = `/${administrationId}`

  return (
    <div className="settings">
      <nav className="settings__nav" aria-label={t('settings.title')}>
        <Link className="settings__back" to={base}>{t('settings.back')}</Link>
        <p className="settings__heading">{t('settings.title')}</p>
        <ul>
          {sections.map((section) => (
            <li key={section}>
              <NavLink to={`${base}/settings/${section}`} className={({ isActive }) => (isActive ? 'is-active' : undefined)}>
                {t(`settings.sections.${section}`)}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
      <div className="settings__page">
        <Outlet />
      </div>
    </div>
  )
}
