import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import type { Administration } from '../api/client'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { SwitchRow } from '../invoicing/Switch'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { can } from '../management/queries'
import { Block } from '../settings/Block'
import { type Connection, useConnections, useConnectionSettings, useRevokeConnection, useUpdateConnectionSettings, useWebhooks } from './queries'

const payments = ['mollie', 'paypal', 'sepa', 'incasso'] as const

/** Settings > Integrations, block by block like Moneybird. */
export function IntegrationsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const { hash } = useLocation()
  const ready = Boolean(administration.data)

  useEffect(() => {
    if (hash && ready) {
      document.getElementById(hash.slice(1))?.scrollIntoView?.({ block: 'start' })
    }
  }, [hash, ready])

  if (!administration.data) {
    return <Loading />
  }
  const a = administration.data
  return (
    <Page title={t('settings.sections.connections')} subtitle={t('integrations.subtitle')}>
      <Block id="available-connections" title={t('integrations.parts.available')} subtitle={t('integrations.availableSubtitle')}>
        <ul className="ledger">
          {payments.map((p) => (
            <li className="ledger__row" key={p}>
              <div className="ledger__text">
                <span className="ledger__name">{t(`integrations.payments.${p}.name`)}</span>
                <p className="hint">{t(`integrations.payments.${p}.help`)}</p>
              </div>
              <span className="hint">{t('integrations.later')}</span>
            </li>
          ))}
        </ul>
        <p className="hint">{t('integrations.paymentsLater')}</p>
      </Block>
      <WhoBlock administration={a} />
      <ConnectionsBlock administration={a} kind="api" />
      <ConnectionsBlock administration={a} kind="ai" />
      {can(a, 'settings') && <WebhooksBlock administration={a} />}
    </Page>
  )
}

function WhoBlock({ administration: a }: { administration: Administration }) {
  const { t } = useTranslation()
  const settings = useConnectionSettings(a.id)
  const update = useUpdateConnectionSettings(a.id)
  return (
    <Block id="who-can-connect" title={t('integrations.parts.who')} subtitle={t('integrations.whoSubtitle')}>
      <div className="card">
        <SwitchRow
          label={t('integrations.usersMayConnect')}
          help={t('integrations.usersMayConnectHelp')}
          checked={settings.data?.users_may_connect ?? false}
          disabled={!can(a, 'ownership') || update.isPending || !settings.data}
          onChange={(on) => update.mutate(on)}
        />
      </div>
    </Block>
  )
}

function ConnectionsBlock({ administration: a, kind }: { administration: Administration; kind: 'api' | 'ai' }) {
  const { t, i18n } = useTranslation()
  const connections = useConnections(a.id)
  const settings = useConnectionSettings(a.id)
  const revoke = useRevokeConnection(a.id)
  const list = connections.data?.filter((c) => c.kind === kind) ?? []
  const mayCreate = a.access !== 'support' && (can(a, 'ownership') || (settings.data?.users_may_connect ?? false))
  const owner = can(a, 'ownership')
  const date = (s: string) => new Date(s).toLocaleDateString(i18n.language, { dateStyle: 'medium' })
  const access = (c: Connection) => `${c.read_only ? t('integrations.readOnly') : t('integrations.readWrite')} · ${c.scopes.map((s) => t(`integrations.scopes.${s}`)).join(', ')}`
  const title = kind === 'api' ? t('integrations.parts.apps') : t('integrations.parts.ai')
  return (
    <Block
      id={kind === 'api' ? 'external-applications' : 'ai-connections'}
      title={title}
      subtitle={kind === 'api' ? t('integrations.appsSubtitle') : t('integrations.aiSubtitle')}
      action={mayCreate && <Link className="btn btn--accent" to={`/${a.id}/connections/new?kind=${kind}`}>{kind === 'api' ? t('integrations.newToken') : t('integrations.newAI')}</Link>}
    >
      {kind === 'api' && list.length > 0 && <p className="hint">{t('integrations.revokeWebhooksNote')}</p>}
      {list.length === 0 ? (
        <p className="empty">{kind === 'api' ? t('integrations.noApps') : t('integrations.noAI')}</p>
      ) : (
        <table className="data-table" aria-label={title}>
          <thead>
            <tr>
              <th scope="col">{t('integrations.application')}</th>
              <th scope="col" className="data-table__extra">{t('integrations.access')}</th>
              <th scope="col" className="data-table__extra">{t('integrations.grantedBy')}</th>
              <th scope="col" className="data-table__extra">{t('integrations.grantedOn')}</th>
              <th scope="col"><span className="sr-only">{t('bookkeeping.actions')}</span></th>
            </tr>
          </thead>
          <tbody>
            {list.map((c) => (
              <tr key={c.id}>
                <td>
                  {c.name}
                  <p className="hint">{c.last_used_at ? t('integrations.lastUsed', { date: date(c.last_used_at) }) : t('integrations.neverUsed')}</p>
                </td>
                <td className="data-table__extra">{access(c)}</td>
                <td className="data-table__extra">{c.granted_by}</td>
                <td className="data-table__extra">{date(c.created_at)}</td>
                <td className="data-table__actions">
                  {(c.mine || owner) && (
                    <ConfirmDelete
                      label={t('integrations.revokeName', { name: c.name })}
                      confirmLabel={t('integrations.confirmRevokeName', { name: c.name })}
                      text={t('integrations.revoke')}
                      confirmText={t('integrations.confirmRevoke')}
                      disabled={revoke.isPending}
                      onConfirm={() => revoke.mutate(c.id)}
                    />
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Block>
  )
}

function WebhooksBlock({ administration: a }: { administration: Administration }) {
  const { t, i18n } = useTranslation()
  const hooks = useWebhooks(a.id)
  return (
    <Block
      id="webhooks"
      title={t('integrations.parts.webhooks')}
      subtitle={t('integrations.webhooksSubtitle')}
      action={<Link className="btn btn--accent" to={`/${a.id}/webhooks/new`}>{t('settings.add')}</Link>}
    >
      {hooks.data?.length ? (
        <table className="data-table data-table--rows" aria-label={t('integrations.parts.webhooks')}>
          <thead>
            <tr>
              <th scope="col">URL</th>
              <th scope="col" className="data-table__extra">{t('integrations.events')}</th>
              <th scope="col">{t('integrations.status')}</th>
            </tr>
          </thead>
          <tbody>
            {hooks.data.map((h) => (
              <tr key={h.id} className={h.deactivated_at ? 'is-inactive' : undefined}>
                <td><Link className="row-link" to={`/${a.id}/webhooks/${h.id}`}>{h.url}</Link></td>
                <td className="data-table__extra" title={(h.enabled_events ?? []).join(', ')}>{t('integrations.eventCount', { count: h.enabled_events?.length ?? 0 })}</td>
                <td>
                  {h.deactivated_at ? t('integrations.deactivatedOn', { date: new Date(h.deactivated_at).toLocaleDateString(i18n.language) }) : t('integrations.active')}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="empty">{t('integrations.noWebhooks')}</p>
      )}
    </Block>
  )
}
