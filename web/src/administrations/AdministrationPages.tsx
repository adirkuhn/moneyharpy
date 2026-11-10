import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { useAdministration } from './queries'
import { Loading } from '../layout/Loading'

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
    return <Loading />
  }
  return (
    <Page title={query.data.name}>
      <p className="empty">{t('administrations.homeEmpty')}</p>
    </Page>
  )
}
