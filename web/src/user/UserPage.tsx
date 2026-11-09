import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useState } from 'react'
import { api, errorMessages, unwrap } from '../api/client'
import { Field, Problems } from '../auth/AuthLayout'
import { useCurrentUser } from '../auth/queries'
import { Page } from '../layout/Page'

export function UserPage() {
  const me = useCurrentUser()
  const client = useQueryClient()
  const [name, setName] = useState('')
  const [secret, setSecret] = useState('')

  const tokens = useQuery({ queryKey: ['tokens'], queryFn: () => unwrap(api.GET('/api/user/tokens')) })
  const refresh = () => client.invalidateQueries({ queryKey: ['tokens'] })
  const create = useMutation({
    mutationFn: () => unwrap(api.POST('/api/user/tokens', { body: { name } })),
    onSuccess: (t) => {
      setSecret(t.token)
      setName('')
      void refresh()
    },
  })
  const revoke = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/user/tokens/{id}', { params: { path: { id } } })),
    onSuccess: refresh,
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    create.mutate()
  }

  return (
    <Page title="User account">
      <dl className="facts">
        <dt>Name</dt><dd>{me.data?.name}</dd>
        <dt>Email</dt><dd>{me.data?.email}</dd>
      </dl>
      <h2>API tokens</h2>
      <p className="hint">Send a token as a bearer token to use the <a href="/docs/api/">API</a>.</p>
      {secret && (
        <div className="secret">
          <p>Copy this token now. It will not be shown again.</p>
          <code>{secret}</code>
        </div>
      )}
      <form className="form form--inline" onSubmit={submit}>
        <Problems messages={errorMessages(create.error, { name: 'Token name' })} />
        <Field label="Token name" required maxLength={255} value={name} onChange={(e) => setName(e.target.value)} />
        <button className="btn btn--primary" disabled={create.isPending}>Create token</button>
      </form>
      {tokens.data && tokens.data.length > 0 && (
        <ul className="ledger" aria-label="API tokens">
          {tokens.data.map((t) => (
            <li className="ledger__row" key={t.id}>
              <span className="ledger__name">{t.name}</span>
              <time className="hint" dateTime={t.created_at}>{new Date(t.created_at).toLocaleDateString()}</time>
              <button type="button" className="btn btn--secondary" aria-label={`Revoke ${t.name}`} onClick={() => revoke.mutate(t.id)}>Revoke</button>
            </li>
          ))}
        </ul>
      )}
    </Page>
  )
}
