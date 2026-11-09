import { type FormEvent, useState } from 'react'
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { errorMessages } from '../api/client'
import { AuthLayout, Field, Problems } from './AuthLayout'
import { useCurrentUser, useLogin, useProviders } from './queries'

const providerErrors: Record<string, string> = {
  google: 'Google sign-in did not complete. Try again or use your password.',
  registration_closed: 'Registration is closed. Ask an administrator for access.',
  email_taken: 'This email already has an account. Sign in with your password.',
}

export function LoginPage() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [params] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const me = useCurrentUser()
  const providers = useProviders()
  const login = useLogin()
  const from = (location.state as { from?: string } | null)?.from ?? '/administrations'

  if (me.data) {
    return <Navigate to={from} replace />
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    login.mutate({ email, password }, { onSuccess: () => navigate(from, { replace: true }) })
  }

  const providerError = providerErrors[params.get('error') ?? '']
  const messages = providerError && !login.isError ? [providerError] : errorMessages(login.error, {})

  return (
    <AuthLayout title="Sign in to MoneyHarpy">
      <Problems messages={messages} />
      <form onSubmit={submit}>
        <Field label="Email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        <Field label="Password" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        <button className="btn btn--primary btn--wide" disabled={login.isPending}>Sign in</button>
      </form>
      {providers.data?.google && <a className="btn btn--secondary btn--wide" href="/api/auth/google/start">Continue with Google</a>}
      {providers.data?.registration_open && <p className="auth__alt">New here? <Link to="/register">Create an account</Link></p>}
    </AuthLayout>
  )
}
