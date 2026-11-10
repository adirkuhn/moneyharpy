import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { errorMessages } from '../api/client'
import { AuthInput, AuthLayout, Problems } from './AuthLayout'
import { useCurrentUser, useLogin, useProviders } from './queries'

const providerErrors = ['google', 'lm2', 'registration_closed', 'email_taken']

export function LoginPage() {
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [params] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const me = useCurrentUser()
  const providers = useProviders()
  const login = useLogin()
  const state = location.state as { from?: string; passwordChanged?: boolean } | null
  const from = state?.from ?? '/administrations'

  if (me.data) {
    return <Navigate to={from} replace />
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    login.mutate({ email, password }, { onSuccess: () => navigate(from, { replace: true }) })
  }

  const code = params.get('error') ?? ''
  const providerError = providerErrors.includes(code) ? t(`login.errors.${code}`) : ''
  const messages = providerError && !login.isError ? [providerError] : errorMessages(login.error, {})
  const external = providers.data?.google || providers.data?.lm2

  return (
    <AuthLayout title={t('login.title')}>
      <Problems messages={messages} />
      {state?.passwordChanged && !login.isError && <p role="status" className="notice">{t('login.passwordChanged')}</p>}
      <form onSubmit={submit}>
        <AuthInput label={t('login.email')} type="email" autoComplete="username" autoFocus required value={email} onChange={(e) => setEmail(e.target.value)} />
        <AuthInput label={t('login.password')} type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        <button className="btn btn--primary btn--wide" disabled={login.isPending}>{t('login.submit')}</button>
      </form>
      {external && <p className="auth__or">{t('login.or')}</p>}
      {providers.data?.google && <a className="btn btn--secondary btn--wide" href="/api/auth/google/start">{t('login.google')}</a>}
      {providers.data?.lm2 && <a className="btn btn--secondary btn--wide" href="/api/auth/lm2/start">{t('login.lm2')}</a>}
      <div className="auth__actions">
        {providers.data?.password_reset && <Link className="btn btn--secondary" to="/passwords/new">{t('login.forgot')}</Link>}
        {providers.data?.registration_open && <Link className="btn btn--secondary" to="/register">{t('login.noAccount')}</Link>}
      </div>
    </AuthLayout>
  )
}
