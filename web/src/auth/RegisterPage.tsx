import { type FormEvent, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { errorMessages } from '../api/client'
import { AuthLayout, Field, Problems } from './AuthLayout'
import { useProviders, useRegister } from './queries'

const labels = { email: 'Email', name: 'Name', password: 'Password' }

export function RegisterPage() {
  const [form, setForm] = useState({ name: '', email: '', password: '' })
  const navigate = useNavigate()
  const providers = useProviders()
  const register = useRegister()

  const submit = (e: FormEvent) => {
    e.preventDefault()
    register.mutate(form, { onSuccess: () => navigate('/administrations', { replace: true }) })
  }
  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm({ ...form, [key]: e.target.value })

  return (
    <AuthLayout title="Create your MoneyHarpy account">
      <Problems messages={errorMessages(register.error, labels)} />
      <form onSubmit={submit}>
        <Field label="Name" autoComplete="name" required value={form.name} onChange={set('name')} />
        <Field label="Email" type="email" autoComplete="email" required value={form.email} onChange={set('email')} />
        <Field label="Password" type="password" autoComplete="new-password" required minLength={12} value={form.password} onChange={set('password')} />
        <p className="hint">Use at least 12 characters.</p>
        <button className="btn btn--primary btn--wide" disabled={register.isPending}>Create account</button>
      </form>
      {providers.data?.google && <a className="btn btn--secondary btn--wide" href="/api/auth/google/start">Continue with Google</a>}
      {providers.data?.lm2 && <a className="btn btn--secondary btn--wide" href="/api/auth/lm2/start">Continue with LM2</a>}
      <p className="auth__alt">Already registered? <Link to="/login">Sign in</Link></p>
    </AuthLayout>
  )
}
