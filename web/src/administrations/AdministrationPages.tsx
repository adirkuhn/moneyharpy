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
  const { valid, query } = useRouteAdministration()
  if (!valid || (query.error instanceof ApiError && query.error.status < 500)) {
    return <NotFoundPage />
  }
  if (!query.data) {
    return null
  }
  return (
    <Page title={query.data.name}>
      <p className="empty">Invoices, contacts and quotes for this administration will appear here.</p>
    </Page>
  )
}

export function AdministrationSettingsPage() {
  const { valid, query } = useRouteAdministration()
  if (!valid || (query.error instanceof ApiError && query.error.status < 500)) {
    return <NotFoundPage />
  }
  if (!query.data) {
    return null
  }
  const a = query.data
  return (
    <Page title={`${a.name} settings`}>
      <dl className="facts">
        <dt>Language</dt><dd>{a.language}</dd>
        <dt>Currency</dt><dd>{a.currency}</dd>
        <dt>Country</dt><dd>{a.country}</dd>
        <dt>Time zone</dt><dd>{a.time_zone}</dd>
        <dt>Bookkeeping starts</dt><dd>{a.period_start_date}</dd>
      </dl>
    </Page>
  )
}
