import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { can } from '../management/queries'
import { type CreatedConnection, type Scope, scopes, useCreateConnection } from './queries'

/** Creates an API token (?kind=api) or AI connection (?kind=ai), showing its token once. */
export function ConnectionFormPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search] = useSearchParams()
  const kind = search.get('kind') === 'ai' ? 'ai' : 'api'
  const administration = useAdministration(administrationId)
  const create = useCreateConnection(administrationId)
  const [form, setForm] = useState({ name: '', scopes: [] as Scope[], read_only: false })
  const [created, setCreated] = useState<CreatedConnection | null>(null)
  const back = `/${administrationId}/settings/connections#${kind === 'api' ? 'external-applications' : 'ai-connections'}`
  if (!administration.data) {
    return <Loading />
  }
  const a = administration.data
  const mine = scopes.filter((s) => can(a, s))
  const labels = { name: t('integrations.application'), scopes: t('integrations.scopesLabel') }
  const errors = formErrors(create.error, labels)
  const title = kind === 'api' ? t('integrations.newToken') : t('integrations.newAI')

  if (created) {
    const config = JSON.stringify({ mcpServers: { moneyharpy: { type: 'http', url: created.mcp_url, headers: { Authorization: `Bearer ${created.token}` } } } }, null, 2)
    return (
      <Page title={title} back={{ to: back, label: t('integrations.back') }}>
        <div className="secret" role="status">
          <p>{t('integrations.copyToken')}</p>
          <code>{created.token}</code>
        </div>
        {created.mcp_url && (
          <div className="card card-stack">
            <p>{t('integrations.mcpHow')}</p>
            <p className="hint">{t('integrations.mcpUrl')} <code className="secret-inline">{created.mcp_url}</code></p>
            <pre className="code-block">{config}</pre>
          </div>
        )}
        <Link className="btn btn--primary" to={back}>{t('management.done')}</Link>
      </Page>
    )
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    create.mutate({ kind, ...form }, { onSuccess: setCreated })
  }
  return (
    <Page title={title} subtitle={kind === 'api' ? t('integrations.tokenSubtitle') : t('integrations.aiFormSubtitle')} back={{ to: back, label: t('integrations.back') }}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" help={kind === 'api' ? t('integrations.nameHelp') : t('integrations.aiNameHelp')} value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <SelectInput label={t('integrations.access')} width="medium" value={form.read_only ? 'read' : 'write'} onChange={(e) => setForm({ ...form, read_only: e.target.value === 'read' })}>
            <option value="write">{t('integrations.readWrite')}</option>
            <option value="read">{t('integrations.readOnly')}</option>
          </SelectInput>
        </FormSection>
        <FormSection title={labels.scopes}>
          {errors.field('scopes') && <Problems messages={errors.field('scopes') ?? []} />}
          {mine.map((s) => (
            <CheckboxInput key={s} label={t(`integrations.scopes.${s}`)} help={t(`management.permissionHelp.${s}`)} checked={form.scopes.includes(s)} onChange={(e) => setForm({ ...form, scopes: e.target.checked ? [...form.scopes, s] : form.scopes.filter((x) => x !== s) })} />
          ))}
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={create.isPending}>{t('integrations.create')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
