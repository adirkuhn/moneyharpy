import { Navigate, useParams } from 'react-router-dom'
import { NotFoundPage } from '../layout/NotFoundPage'

/** An administration's address opens its overview, as on Moneybird. */
export function AdministrationHomePage() {
  const { administrationId = '' } = useParams()
  // Only numeric segments are IDs. Anything else is an unknown page.
  if (!/^\d{1,19}$/.test(administrationId)) {
    return <NotFoundPage />
  }
  return <Navigate to={`/${administrationId}/feed`} replace />
}
