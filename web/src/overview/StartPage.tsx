import { Navigate } from 'react-router-dom'
import { useAdministrations } from '../administrations/queries'
import { Loading } from '../layout/Loading'
import { lastAdministration } from './queries'

/** The start page opens the administration opened last, or the list of administrations. */
export function StartPage() {
  const last = lastAdministration()
  const administrations = useAdministrations()
  if (!last) {
    return <Navigate to="/administrations" replace />
  }
  if (administrations.isPending) {
    return <Loading />
  }
  const found = administrations.data?.some((a) => a.id === last)
  return <Navigate to={found ? `/${last}/feed` : '/administrations'} replace />
}
