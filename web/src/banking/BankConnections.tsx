import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { errorMessages } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { groupIBAN } from '../invoices/paper'
import { Loading } from '../layout/Loading'
import {
  type BankConnection, type ConnectableBank, type FinancialAccount, useBankConnections, useConnectableBanks, useConnectBank, useDisconnectBank,
  useReconnectBank, useSyncBankConnection,
} from './queries'
import './connections.css'

const day = (at: string | null | undefined, language: string) => (at ? new Date(at).toLocaleDateString(language, { dateStyle: 'medium' }) : '')
const moment = (at: string | null | undefined, language: string) => (at ? new Date(at).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' }) : '')

/**
 * Connect a bank: the banks that can be connected, to search and pick one;
 * the browser then goes to the bank, where the user authorises access.
 */
export function ConnectBank({ id, administrationId }: { id: string; administrationId: string }) {
  const { t } = useTranslation()
  const banks = useConnectableBanks(administrationId, true)
  const connect = useConnectBank(administrationId)
  const [query, setQuery] = useState('')
  const listId = useId()
  const shown = (banks.data ?? []).filter((b) => b.name.toLowerCase().includes(query.trim().toLowerCase()))
  return (
    <section id={id} className="connect-bank" aria-label={t('bank.connect.title')}>
      <p className="connect-bank__intro">{t('bank.connect.intro')}</p>
      {banks.isError ? (
        <Problems messages={errorMessages(banks.error, {})} />
      ) : !banks.data ? (
        <Loading />
      ) : (
        <>
          <input
            type="search"
            className="connect-bank__search"
            aria-label={t('bank.connect.search')}
            placeholder={t('bank.connect.search')}
            aria-controls={listId}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <ul id={listId} className="connect-bank__banks" aria-label={t('bank.connect.banks')}>
            {shown.map((b: ConnectableBank) => (
              <li key={`${b.country}-${b.name}`}>
                <button type="button" className="connect-bank__bank" disabled={connect.isPending} onClick={() => connect.mutate(b)}>
                  {b.logo ? <img src={b.logo} alt="" loading="lazy" /> : <span className="connect-bank__logo" aria-hidden="true" />}
                  <span>{b.name}</span>
                </button>
              </li>
            ))}
            {shown.length === 0 && <li className="connect-bank__empty">{t('bank.connect.none')}</li>}
          </ul>
          {connect.isPending && <p role="status" className="hint">{t('bank.connect.going')}</p>}
          <Problems messages={errorMessages(connect.error, {})} />
        </>
      )}
    </section>
  )
}

/**
 * What needs the user about the connections: one click to reconnect a bank
 * whose consent ended or is about to, and fetches that failed.
 */
export function ConnectionNotices({ administrationId }: { administrationId: string }) {
  const { t, i18n } = useTranslation()
  const connections = useBankConnections(administrationId)
  const reconnect = useReconnectBank(administrationId)
  const sync = useSyncBankConnection(administrationId)
  const notices = (connections.data ?? []).filter((c) => c.needs_reconnect || c.last_error)
  if (notices.length === 0) {
    return null
  }
  return (
    <div className="connection-notices">
      {notices.map((c) => (
        <div key={c.id} className={`connection-notice${c.state === 'expired' ? ' is-expired' : ''}`} role="status">
          <p>
            {c.state === 'expired'
              ? t('bank.connections.expired', { bank: c.bank })
              : c.needs_reconnect
                ? t('bank.connections.expiresOn', { bank: c.bank, date: day(c.valid_until, i18n.language) })
                : t('bank.connections.failed', { bank: c.bank, error: c.last_error })}
          </p>
          {c.needs_reconnect ? (
            <button type="button" className="btn btn--primary btn--small" disabled={reconnect.isPending} onClick={() => reconnect.mutate(c.id)}>
              {t('bank.connections.reconnect', { bank: c.bank })}
            </button>
          ) : (
            <button type="button" className="btn btn--secondary btn--small" disabled={sync.isPending} onClick={() => sync.mutate(c.id)}>
              {t('bank.connections.tryAgain')}
            </button>
          )}
        </div>
      ))}
      <Problems messages={[...errorMessages(reconnect.error, {}), ...errorMessages(sync.error, {})]} />
    </div>
  )
}

/** The connection an account's transactions come through, if any. */
export function connectionOf(connections: BankConnection[] | undefined, account: FinancialAccount) {
  return connections?.find((c) => c.financial_account_ids.includes(account.id ?? ''))
}

/** Settings > Bank's connections: each bank with its accounts, consent, last fetch and actions. */
export function ConnectionList({ administrationId, accounts }: { administrationId: string; accounts: FinancialAccount[] }) {
  const { t, i18n } = useTranslation()
  const connections = useBankConnections(administrationId)
  const reconnect = useReconnectBank(administrationId)
  const sync = useSyncBankConnection(administrationId)
  const disconnect = useDisconnectBank(administrationId)
  const [confirming, setConfirming] = useState<string | null>(null)
  const [connecting, setConnecting] = useState(false)
  const connectId = useId()
  if (!connections.data) {
    return <Loading />
  }
  const accountName = (id: string) => {
    const a = accounts.find((x) => x.id === id)
    return a ? (a.identifier ? groupIBAN(a.identifier) : (a.name ?? '')) : ''
  }
  return (
    <div className="connection-list">
      {connections.data.length === 0 && <p className="hint">{t('bank.connections.none')}</p>}
      {connections.data.map((c) => (
        <article key={c.id} className="card connection" aria-label={c.bank}>
          <div className="connection__head">
            <p className="card__title">{c.bank}</p>
            <span className={`badge badge--${c.state === 'active' && !c.needs_reconnect ? 'paid' : 'late'}`}>
              {c.state === 'expired' ? t('bank.connections.stateExpired') : c.needs_reconnect ? t('bank.connections.stateEnding') : t('bank.connections.stateActive')}
            </span>
          </div>
          <dl className="connection__facts">
            <dt>{t('bank.connections.accounts')}</dt>
            <dd>{c.financial_account_ids.map(accountName).filter(Boolean).join(', ')}</dd>
            <dt>{t('bank.connections.validUntil')}</dt>
            <dd>{day(c.valid_until, i18n.language)}</dd>
            <dt>{t('bank.connections.lastSync')}</dt>
            <dd>{c.last_sync_at ? moment(c.last_sync_at, i18n.language) : t('bank.accounts.never')}</dd>
            {c.last_error && (
              <>
                <dt>{t('bank.connections.problem')}</dt>
                <dd className="connection__error">{c.last_error}</dd>
              </>
            )}
          </dl>
          <div className="connection__actions">
            <button type="button" className={`btn ${c.needs_reconnect ? 'btn--primary' : 'btn--secondary'}`} disabled={reconnect.isPending} onClick={() => reconnect.mutate(c.id)}>
              {t('bank.connections.reconnectShort')}
            </button>
            {c.state === 'active' && (
              <button type="button" className="btn btn--secondary" disabled={sync.isPending} onClick={() => sync.mutate(c.id)}>
                {sync.isPending && sync.variables === c.id ? t('bank.connections.fetching') : t('bank.connections.fetchNow')}
              </button>
            )}
            {confirming === c.id ? (
              <>
                <button type="button" className="btn btn--danger" disabled={disconnect.isPending} onClick={() => disconnect.mutate(c.id, { onSuccess: () => setConfirming(null) })}>
                  {t('bank.connections.confirmDisconnect')}
                </button>
                <button type="button" className="btn btn--link" onClick={() => setConfirming(null)}>{t('common.cancel')}</button>
              </>
            ) : (
              <button type="button" className="btn btn--link" onClick={() => setConfirming(c.id)}>{t('bank.connections.disconnect')}</button>
            )}
          </div>
          {confirming === c.id && <p className="hint">{t('bank.connections.disconnectHint')}</p>}
        </article>
      ))}
      <Problems messages={[...errorMessages(reconnect.error, {}), ...errorMessages(sync.error, {}), ...errorMessages(disconnect.error, {})]} />
      <p>
        <button type="button" className="btn btn--secondary" aria-expanded={connecting} aria-controls={connectId} onClick={() => setConnecting(!connecting)}>
          {t('bank.connect.button')}
        </button>
      </p>
      {connecting && <ConnectBank id={connectId} administrationId={administrationId} />}
    </div>
  )
}
