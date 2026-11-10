import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { useAdministration } from './queries'

function useRouteAdministration() {
  const { administrationId = '' } = useParams()
  // Only numeric segments are IDs. Anything else is an unknown page.
  return { valid: /^\d{1,19}$/.test(administrationId), query: useAdministration(administrationId) }
}

export function AdministrationHomePage() {
  const { t } = useTranslation()
  const { valid, query } = useRouteAdministration()
  if (!valid || (query.error instanceof ApiError && query.error.status < 500)) {
    return <NotFoundPage />
  }
  if (!query.data) {
    return null
  }
  return (
    <Page title={query.data.name}>
      <p className="empty">{t('administrations.homeEmpty')}</p>
    </Page>
  )
}

export function AdministrationSettingsPage() {
  const { t } = useTranslation()
  const { valid, query } = useRouteAdministration()
  if (!valid || (query.error instanceof ApiError && query.error.status < 500)) {
    return <NotFoundPage />
  }
  if (!query.data) {
    return null
  }
  const a = query.data
  return (
    <Page title={t('administrations.settings', { name: a.name })}>
      <dl className="facts">
        <dt>{t('administrations.language')}</dt><dd>{a.language}</dd>
        <dt>{t('administrations.currency')}</dt><dd>{a.currency}</dd>
        <dt>{t('administrations.country')}</dt><dd>{a.country}</dd>
        <dt>{t('administrations.timeZone')}</dt><dd>{a.time_zone}</dd>
        <dt>{t('administrations.starts')}</dt><dd>{a.period_start_date}</dd>
      </dl>
    </Page>
  )
}
