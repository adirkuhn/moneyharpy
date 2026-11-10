import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { errorMessages } from '../api/client'
import { AuthInput, AuthLayout, Problems } from './AuthLayout'
import { useProviders, useRegister } from './queries'

export function RegisterPage() {
  const { t } = useTranslation()
  const labels = { email: t('login.email'), name: t('register.name'), password: t('login.password') }
  const [search] = useSearchParams()
  const invitation = search.get('invitation') ?? ''
  const [form, setForm] = useState({ name: '', email: search.get('email') ?? '', password: '' })
  const navigate = useNavigate()
  const providers = useProviders()
  const register = useRegister()

  const submit = (e: FormEvent) => {
    e.preventDefault()
    register.mutate(invitation ? { ...form, invitation_token: invitation } : form, {
      onSuccess: () => navigate(invitation ? `/invitations/${invitation}` : '/administrations', { replace: true }),
    })
  }
  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm({ ...form, [key]: e.target.value })
  const external = providers.data?.google || providers.data?.lm2

  return (
    <AuthLayout title={t('register.title')}>
      <Problems messages={errorMessages(register.error, labels)} />
      <form onSubmit={submit}>
        <AuthInput label={t('register.name')} autoComplete="name" autoFocus required value={form.name} onChange={set('name')} />
        <AuthInput label={t('login.email')} type="email" autoComplete="email" required value={form.email} onChange={set('email')} />
        <AuthInput label={t('login.password')} type="password" autoComplete="new-password" required minLength={12} value={form.password} onChange={set('password')} />
        <p className="hint">{t('register.hint')}</p>
        <button className="btn btn--primary btn--wide" disabled={register.isPending}>{t('register.submit')}</button>
      </form>
      {external && <p className="auth__or">{t('login.or')}</p>}
      {providers.data?.google && <a className="btn btn--secondary btn--wide" href="/api/auth/google/start">{t('login.google')}</a>}
      {providers.data?.lm2 && <a className="btn btn--secondary btn--wide" href="/api/auth/lm2/start">{t('login.lm2')}</a>}
      <div className="auth__actions">
        <Link className="btn btn--secondary" to="/login">{t('register.already')}</Link>
      </div>
    </AuthLayout>
  )
}
