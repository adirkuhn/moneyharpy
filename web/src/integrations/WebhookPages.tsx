import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { errorMessages } from '../api/client'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { CheckboxInput, FormNotice, FormSection, formErrors, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type Webhook, useCreateWebhook, useDeleteWebhook, useSetWebhookActive, useWebhookEvents, useWebhooks } from './queries'

/** Subscribes a URL to events (/webhooks/new), showing the signing secret once. */
export function WebhookFormPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const events = useWebhookEvents(administrationId)
  const create = useCreateWebhook(administrationId)
  const [url, setUrl] = useState('')
  const [chosen, setChosen] = useState<string[]>([])
  const [created, setCreated] = useState<Webhook | null>(null)
  const back = `/${administrationId}/settings/connections#webhooks`
  const errors = formErrors(create.error, { url: 'URL', enabled_events: t('integrations.events') })
  if (created) {
    return (
      <Page title={t('integrations.newWebhook')} back={{ to: back, label: t('integrations.back') }}>
        <div className="secret" role="status">
          <p>{t('integrations.copySecret')}</p>
          <code>{created.secret}</code>
        </div>
        <Link className="btn btn--primary" to={back}>{t('management.done')}</Link>
      </Page>
    )
  }
  if (!events.data) {
    return <Loading />
  }
  const { groups, events: names } = events.data
  const toggle = (name: string, on: boolean) => setChosen(on ? [...chosen, name] : chosen.filter((x) => x !== name))
  const submit = (e: FormEvent) => {
    e.preventDefault()
    create.mutate({ url, enabled_events: chosen }, { onSuccess: setCreated })
  }
  return (
    <Page title={t('integrations.newWebhook')} subtitle={t('integrations.webhookFormSubtitle')} back={{ to: back, label: t('integrations.back') }}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label="URL" type="url" required placeholder="https://" maxLength={2000} help={t('integrations.urlHelp')} value={url} errors={errors.field('url')} onChange={(e) => setUrl(e.target.value)} />
        </FormSection>
        {errors.field('enabled_events') && <Problems messages={errors.field('enabled_events') ?? []} />}
        {groups.map((g) => {
          const members = names.filter((n) => n.startsWith(`${g}_`) && !groups.some((other) => other !== g && other.startsWith(`${g}_`) && n.startsWith(`${other}_`)))
          return (
            <FormSection key={g} title={t(`integrations.groups.${g}`, { defaultValue: g })}>
              <CheckboxInput label={t('integrations.allOf', { group: t(`integrations.groups.${g}`, { defaultValue: g }) })} checked={chosen.includes(g)} onChange={(e) => toggle(g, e.target.checked)} />
              {!chosen.includes(g) && members.map((n) => <CheckboxInput key={n} label={n} checked={chosen.includes(n)} onChange={(e) => toggle(n, e.target.checked)} />)}
            </FormSection>
          )
        })}
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={create.isPending}>{t('integrations.createWebhook')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}

/** One webhook: its events, last answer, and switching or deleting it. */
export function WebhookPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '', webhookId = '' } = useParams()
  const hooks = useWebhooks(administrationId)
  const setActive = useSetWebhookActive(administrationId, webhookId)
  const remove = useDeleteWebhook(administrationId)
  const back = `/${administrationId}/settings/connections#webhooks`
  if (!hooks.data) {
    return <Loading />
  }
  const h = hooks.data.find((x) => x.id === webhookId)
  if (!h) {
    return <NotFoundPage />
  }
  const active = !h.deactivated_at
  const deleteAction = <ConfirmDelete label={t('settings.delete')} confirmLabel={t('settings.confirmDelete')} disabled={remove.isPending} onConfirm={() => remove.mutate(webhookId, { onSuccess: () => navigate(back) })} />
  return (
    <Page title={t('integrations.webhook')} subtitle={h.url} back={{ to: back, label: t('integrations.back') }} actions={deleteAction}>
      {!active && <FormNotice>{t('integrations.deactivatedOn', { date: new Date(h.deactivated_at ?? '').toLocaleString(i18n.language) })}</FormNotice>}
      <dl className="details card">
        <div><dt>{t('integrations.status')}</dt><dd>{active ? t('integrations.active') : t('integrations.inactive')}</dd></div>
        <div><dt>{t('integrations.lastStatus')}</dt><dd>{h.last_http_status ?? '-'}</dd></div>
        <div><dt>{t('integrations.token')}</dt><dd><code className="secret-inline">{h.token}</code></dd></div>
      </dl>
      <h2>{t('integrations.events')}</h2>
      {h.enabled_events?.length ? (
        <ul className="tag-list">{h.enabled_events.map((e) => <li key={e}><code>{e}</code></li>)}</ul>
      ) : (
        <p className="empty">{t('integrations.legacyEvents')}</p>
      )}
      {h.last_http_body && (
        <>
          <h2>{t('integrations.lastBody')}</h2>
          <pre className="code-block">{h.last_http_body}</pre>
        </>
      )}
      <Problems messages={errorMessages(setActive.error, {})} />
      <div className="form-sheet__actions">
        <button type="button" className="btn btn--secondary" disabled={setActive.isPending} onClick={() => setActive.mutate(!active)}>
          {active ? t('integrations.deactivate') : t('integrations.activate')}
        </button>
      </div>
    </Page>
  )
}
