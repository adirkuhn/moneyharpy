import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { errorMessages } from '../api/client'
import { AuthInput, AuthLayout, Problems } from './AuthLayout'
import { useCompleteSignIn } from './queries'

/** The second step of signing in: a code from the authenticator app or a recovery code. */
export function TwoFactorSignInPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const from = (location.state as { from?: string } | null)?.from ?? '/administrations'
  const complete = useCompleteSignIn()
  const [code, setCode] = useState('')
  const submit = (e: FormEvent) => {
    e.preventDefault()
    complete.mutate(code, { onSuccess: () => navigate(from, { replace: true }) })
  }
  return (
    <AuthLayout title={t('twoFactor.signInTitle')}>
      <p className="hint">{t('twoFactor.signInHint')}</p>
      <Problems messages={errorMessages(complete.error, { code: t('twoFactor.code') })} />
      <form onSubmit={submit}>
        <AuthInput label={t('twoFactor.code')} inputMode="text" autoComplete="one-time-code" autoFocus required maxLength={32} value={code} onChange={(e) => setCode(e.target.value)} />
        <button className="btn btn--primary btn--wide" disabled={complete.isPending}>{t('twoFactor.verify')}</button>
      </form>
      <div className="auth__actions">
        <Link className="btn btn--secondary" to="/login">{t('twoFactor.startOver')}</Link>
      </div>
    </AuthLayout>
  )
}
