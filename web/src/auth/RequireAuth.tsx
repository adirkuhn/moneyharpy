import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, useLocation } from 'react-router-dom'
import { useCurrentUser } from './queries'

export function RequireAuth({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const me = useCurrentUser()
  const location = useLocation()
  if (me.isPending) {
    return null
  }
  if (me.isError) {
    return <p role="alert" className="auth-failure">{t('login.unreachable')}</p>
  }
  if (me.data === null) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  return children
}
