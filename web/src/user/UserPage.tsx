import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, unwrap } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formErrors, FormSection, TextInput } from '../forms/Form'
import { useCurrentUser } from '../auth/queries'
import { Page } from '../layout/Page'
import { Block } from '../settings/Block'
import { TwoFactorBlock } from './TwoFactorBlock'

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
  const tokenErrors = formErrors(create.error, { name: t('user.tokenName') })

  // The settings pattern of Moneybird: a titled block per subject.
  return (
    <Page title={t('user.title')} subtitle={t('user.subtitle')}>
      <Block id="account" title={t('user.details')} subtitle={t('user.detailsSubtitle')}>
        <div className="datalist account-details">
          <div className="datalist__header">
            <p className="datalist__title">{me.data?.name}</p>
          </div>
          <dl>
            <div className="datalist__row"><dt>{t('user.name')}</dt><dd>{me.data?.name}</dd></div>
            <div className="datalist__row"><dt>{t('user.email')}</dt><dd>{me.data?.email}</dd></div>
          </dl>
        </div>
      </Block>
      <TwoFactorBlock />
      <Block
        id="api-tokens"
        title={t('user.tokens')}
        subtitle={t('user.tokensSubtitle')}
        action={<a className="btn btn--secondary" href="/docs/api/">{t('user.apiDocs')}</a>}
      >
        {secret && (
          <div className="secret" role="status">
            <p>{t('user.copy')}</p>
            <code>{secret}</code>
          </div>
        )}
        {tokens.data && tokens.data.length > 0 && (
          <table className="data-table" aria-label={t('user.list')}>
            <thead>
              <tr>
                <th scope="col">{t('user.tokenName')}</th>
                <th scope="col">{t('user.created')}</th>
                <th scope="col"><span className="sr-only">{t('user.revoke')}</span></th>
              </tr>
            </thead>
            <tbody>
              {tokens.data.map((token) => (
                <tr key={token.id}>
                  <td>{token.name}</td>
                  <td><time dateTime={token.created_at}>{new Date(token.created_at).toLocaleDateString(i18n.language)}</time></td>
                  <td className="data-table__actions">
                    <button type="button" className="btn btn--secondary btn--small" aria-label={t('user.revokeName', { name: token.name })} onClick={() => revoke.mutate(token.id)}>{t('user.revoke')}</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <form className="form-sheet form-sheet--block" onSubmit={submit} aria-label={t('user.newToken')}>
          <Problems messages={tokenErrors.general} />
          <FormSection>
            <TextInput label={t('user.tokenName')} required maxLength={255} autoComplete="off" value={name} errors={tokenErrors.field('name')} onChange={(e) => setName(e.target.value)} />
          </FormSection>
          <div className="form-sheet__actions">
            <button className="btn btn--primary" disabled={create.isPending}>{t('user.create')}</button>
          </div>
        </form>
      </Block>
    </Page>
  )
}
