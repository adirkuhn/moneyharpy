import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, errorMessages, unwrap } from '../api/client'
import { Field, Problems } from '../auth/AuthLayout'
import { useCurrentUser } from '../auth/queries'
import { Page } from '../layout/Page'

export function UserPage() {
  const { t, i18n } = useTranslation()
  const me = useCurrentUser()
  const client = useQueryClient()
  const [name, setName] = useState('')
  const [secret, setSecret] = useState('')

  const tokens = useQuery({ queryKey: ['tokens'], queryFn: () => unwrap(api.GET('/api/user/tokens')) })
  const refresh = () => client.invalidateQueries({ queryKey: ['tokens'] })
  const create = useMutation({
    mutationFn: () => unwrap(api.POST('/api/user/tokens', { body: { name } })),
    onSuccess: (created) => {
      setSecret(created.token)
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
    <Page title={t('user.title')}>
      <dl className="facts">
        <dt>{t('user.name')}</dt><dd>{me.data?.name}</dd>
        <dt>{t('user.email')}</dt><dd>{me.data?.email}</dd>
      </dl>
      <h2>{t('user.tokens')}</h2>
      <p className="hint">{t('user.tokensHint')} <a href="/docs/api/">{t('user.api')}</a>.</p>
      {secret && (
        <div className="secret">
          <p>{t('user.copy')}</p>
          <code>{secret}</code>
        </div>
      )}
      <form className="form form--inline" onSubmit={submit}>
        <Problems messages={errorMessages(create.error, { name: t('user.tokenName') })} />
        <Field label={t('user.tokenName')} required maxLength={255} value={name} onChange={(e) => setName(e.target.value)} />
        <button className="btn btn--primary" disabled={create.isPending}>{t('user.create')}</button>
      </form>
      {tokens.data && tokens.data.length > 0 && (
        <ul className="ledger" aria-label={t('user.list')}>
          {tokens.data.map((token) => (
            <li className="ledger__row" key={token.id}>
              <span className="ledger__name">{token.name}</span>
              <time className="hint" dateTime={token.created_at}>{new Date(token.created_at).toLocaleDateString(i18n.language)}</time>
              <button type="button" className="btn btn--secondary" aria-label={t('user.revokeName', { name: token.name })} onClick={() => revoke.mutate(token.id)}>{t('user.revoke')}</button>
            </li>
          ))}
        </ul>
      )}
    </Page>
  )
}
