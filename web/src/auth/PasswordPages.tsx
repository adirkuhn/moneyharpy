import { useMutation } from '@tanstack/react-query'
import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, ApiError, errorMessages, unwrap } from '../api/client'
import { currentLanguage } from '../i18n'
import { AuthInput, AuthLayout, Problems } from './AuthLayout'

export function ForgotPasswordPage() {
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const request = useMutation({
    mutationFn: () => unwrap(api.POST('/api/auth/password_reset/request', { body: { email, language: currentLanguage() } })),
  })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    request.mutate()
  }

  return (
    <AuthLayout title={t('passwords.forgotTitle')}>
      <Problems messages={errorMessages(request.error, {})} />
      {request.isSuccess ? (
        <p role="status" className="notice">{t('passwords.sent')}</p>
      ) : (
        <form onSubmit={submit}>
          <p className="hint auth__intro">{t('passwords.forgotBody')}</p>
          <AuthInput label={t('login.email')} type="email" autoComplete="username" autoFocus required value={email} onChange={(e) => setEmail(e.target.value)} />
          <button className="btn btn--primary btn--wide" disabled={request.isPending}>{t('passwords.send')}</button>
        </form>
      )}
      <div className="auth__actions">
        <Link className="btn btn--secondary" to="/login">{t('passwords.back')}</Link>
      </div>
    </AuthLayout>
  )
}

export function EditPasswordPage() {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const confirm = useMutation({
    mutationFn: () => unwrap(api.POST('/api/auth/password_reset/confirm', { body: { token, password } })),
    onSuccess: () => navigate('/login', { replace: true, state: { passwordChanged: true } }),
  })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    confirm.mutate()
  }

  const badLink = !token || (confirm.error instanceof ApiError && 'token' in confirm.error.fields)
  const messages = badLink
    ? [t('passwords.badLink')]
    : errorMessages(confirm.error, { password: t('passwords.new') })

  return (
    <AuthLayout title={t('passwords.editTitle')}>
      <Problems messages={messages} />
      {token && !badLink && (
        <form onSubmit={submit}>
          <AuthInput label={t('passwords.new')} type="password" autoComplete="new-password" autoFocus required minLength={12} value={password} onChange={(e) => setPassword(e.target.value)} />
          <p className="hint">{t('register.hint')}</p>
          <button className="btn btn--primary btn--wide" disabled={confirm.isPending}>{t('passwords.save')}</button>
        </form>
      )}
      <div className="auth__actions">
        {badLink && <Link className="btn btn--secondary" to="/passwords/new">{t('passwords.newLink')}</Link>}
        <Link className="btn btn--secondary" to="/login">{t('passwords.back')}</Link>
      </div>
    </AuthLayout>
  )
}
