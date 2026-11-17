import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { errorMessages } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { groupIBAN } from '../invoices/paper'
import { Loading } from '../layout/Loading'
import {
  type BankConnection, type ConnectableBank, type FinancialAccount, useBankConnections, useConnectableBanks, useConnectBank, useConnectWithKey, useDisconnectBank,
  useReconnectBank, useReplaceKey, useSyncBankConnection,
} from './queries'
import './connections.css'

/** Connections made with an API key are reconnected with a new key, not at the bank. */
const withKey = (c: BankConnection) => c.provider === 'revolut'

/** A field to paste an API key in, as Revolut Business makes them. */
function KeyForm({ label, submit, pending, error, onSubmit }: { label: string; submit: string; pending: boolean; error: unknown; onSubmit: (key: string) => void }) {
  const { t } = useTranslation()
  const [key, setKey] = useState('')
  const send = (e: FormEvent) => {
    e.preventDefault()
    if (key.trim()) {
      onSubmit(key.trim())
    }
  }
  return (
    <form className="key-form" onSubmit={send} noValidate>
      <label className="key-form__field">
        <span>{label}</span>
        <input type="password" autoComplete="off" spellCheck={false} value={key} onChange={(e) => setKey(e.target.value)} />
      </label>
      <button className="btn btn--primary" disabled={pending || !key.trim()}>{pending ? t('bank.connect.checking') : submit}</button>
      <Problems messages={errorMessages(error, { api_key: label })} />
    </form>
  )
}

/** A bank the user can pick, with the ways it connects. */
type Choice = { name: string; logo: string; direct?: 'revolut'; viaEnableBanking?: ConnectableBank }

/** Banks MoneyHarpy connects to itself, without a service in between. */
const directBanks: { name: string; provider: 'revolut'; matches: RegExp }[] = [{ name: 'Revolut Business', provider: 'revolut', matches: /revolut/i }]

/** The banks to choose from: the direct ones, joined with the same bank at Enable Banking, and the rest of Enable Banking's. */
function choices(banks: ConnectableBank[]): Choice[] {
  const out: Choice[] = directBanks.map((d) => {
    const via = banks.find((b) => d.matches.test(b.name))
    return { name: d.name, logo: via?.logo ?? '', direct: d.provider, ...(via ? { viaEnableBanking: via } : {}) }
  })
  for (const b of banks) {
    if (!directBanks.some((d) => d.matches.test(b.name))) {
      out.push({ name: b.name, logo: b.logo, viaEnableBanking: b })
    }
  }
  return out.sort((a, b) => a.name.localeCompare(b.name))
}

function BankLogo({ choice }: { choice: Choice }) {
  return choice.logo ? <img src={choice.logo} alt="" loading="lazy" /> : <span className="connect-bank__logo" aria-hidden="true">{choice.name.slice(0, 1)}</span>
}

/**
 * Connect a bank in two steps: choose the bank from one list, then how to
 * connect it: directly with an API key where the bank allows that, or by
 * logging in at the bank through Enable Banking.
 */
export function ConnectBank({ id, administrationId }: { id: string; administrationId: string }) {
  const { t } = useTranslation()
  const banks = useConnectableBanks(administrationId, true)
  const [query, setQuery] = useState('')
  const [chosen, setChosen] = useState<Choice | null>(null)
  const listId = useId()
  // Without Enable Banking the direct banks can still be connected.
  const all = choices(banks.data ?? [])
  const shown = all.filter((c) => c.name.toLowerCase().includes(query.trim().toLowerCase()))
  return (
    <section id={id} className="connect-bank" aria-label={t('bank.connect.title')}>
      {chosen ? (
        <ConnectChoice administrationId={administrationId} choice={chosen} onBack={() => setChosen(null)} />
      ) : (
        <>
          <h2 className="connect-bank__step">{t('bank.connect.chooseBank')}</h2>
          <input
            type="search"
            className="connect-bank__search"
            aria-label={t('bank.connect.search')}
            placeholder={t('bank.connect.search')}
            aria-controls={listId}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          {banks.isPending && <Loading />}
          <ul id={listId} className="connect-bank__banks" aria-label={t('bank.connect.banks')}>
            {shown.map((c) => (
              <li key={c.name}>
                <button type="button" className="connect-bank__bank" onClick={() => setChosen(c)}>
                  <BankLogo choice={c} />
                  <span>{c.name}</span>
                </button>
              </li>
            ))}
            {shown.length === 0 && !banks.isPending && <li className="connect-bank__empty">{t('bank.connect.none')}</li>}
          </ul>
          {banks.isError && <p className="hint">{t('bank.connect.moreBanksOff')}</p>}
        </>
      )}
    </section>
  )
}

/** Step two: the ways the chosen bank connects, each explained. */
function ConnectChoice({ administrationId, choice, onBack }: { administrationId: string; choice: Choice; onBack: () => void }) {
  const { t } = useTranslation()
  const connectKey = useConnectWithKey(administrationId)
  const connectBank = useConnectBank(administrationId)
  return (
    <div className="connect-choice">
      <button type="button" className="btn btn--link connect-choice__back" onClick={onBack}>‹ {t('bank.connect.otherBank')}</button>
      <h2 className="connect-bank__step connect-choice__bank">
        <BankLogo choice={choice} />
        <span>{t('bank.connect.how', { bank: choice.name })}</span>
      </h2>
      <div className="connect-choice__ways">
        {choice.direct && (
          <article className="connect-way" aria-label={t('bank.connect.directTitle')}>
            <p className="connect-way__title">
              {t('bank.connect.directTitle')}
              {choice.viaEnableBanking && <span className="badge badge--paid">{t('bank.connect.recommended')}</span>}
            </p>
            <p className="hint">{t('bank.connect.directText')}</p>
            <p className="hint">{t('bank.connect.revolutHelp')}</p>
            {connectKey.isSuccess ? (
              <p className="notice" role="status">{t('bank.connections.connected', { bank: connectKey.data.bank })}</p>
            ) : (
              <KeyForm label={t('bank.connect.apiKey')} submit={t('bank.connect.connect')} pending={connectKey.isPending} error={connectKey.error} onSubmit={(key) => connectKey.mutate(key)} />
            )}
          </article>
        )}
        {choice.viaEnableBanking && (
          <article className="connect-way" aria-label={t('bank.connect.viaTitle')}>
            <p className="connect-way__title">{t('bank.connect.viaTitle')}</p>
            <p className="hint">{t('bank.connect.viaText')}</p>
            <div>
              <button type="button" className="btn btn--primary" disabled={connectBank.isPending} onClick={() => connectBank.mutate(choice.viaEnableBanking!)}>
                {connectBank.isPending ? t('bank.connect.going') : t('bank.connect.logIn', { bank: choice.name })}
              </button>
            </div>
            <Problems messages={errorMessages(connectBank.error, {})} />
          </article>
        )}
      </div>
    </div>
  )
}

const day = (at: string | null | undefined, language: string) => (at ? new Date(at).toLocaleDateString(language, { dateStyle: 'medium' }) : '')
const moment = (at: string | null | undefined, language: string) => (at ? new Date(at).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' }) : '')

/**
 * What needs the user about the connections: one click to reconnect a bank
 * whose consent ended or is about to, and fetches that failed.
 */
export function ConnectionNotices({ administrationId }: { administrationId: string }) {
  const { t, i18n } = useTranslation()
  const connections = useBankConnections(administrationId)
  const reconnect = useReconnectBank(administrationId)
  const sync = useSyncBankConnection(administrationId)
  const replace = useReplaceKey(administrationId)
  const [replacing, setReplacing] = useState<string | null>(null)
  const notices = (connections.data ?? []).filter((c) => c.needs_reconnect || c.last_error)
  if (notices.length === 0) {
    return null
  }
  return (
    <div className="connection-notices">
      {notices.map((c) => (
        <div key={c.id} className={`connection-notice${c.state === 'expired' ? ' is-expired' : ''}`} role="status">
          <p>
            {c.state === 'expired' && withKey(c)
              ? t('bank.connections.keyRejected', { bank: c.bank })
              : c.state === 'expired'
              ? t('bank.connections.expired', { bank: c.bank })
              : c.needs_reconnect
                ? t('bank.connections.expiresOn', { bank: c.bank, date: day(c.valid_until, i18n.language) })
                : t('bank.connections.failed', { bank: c.bank, error: c.last_error })}
          </p>
          {c.needs_reconnect && withKey(c) ? (
            replacing === c.id ? (
              <KeyForm
                label={t('bank.connect.apiKey')}
                submit={t('bank.connections.replaceKey')}
                pending={replace.isPending}
                error={replace.error}
                onSubmit={(apiKey) => replace.mutate({ id: c.id, apiKey }, { onSuccess: () => setReplacing(null) })}
              />
            ) : (
              <button type="button" className="btn btn--primary btn--small" onClick={() => setReplacing(c.id)}>{t('bank.connections.newKey', { bank: c.bank })}</button>
            )
          ) : c.needs_reconnect ? (
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
  const replace = useReplaceKey(administrationId)
  const [replacing, setReplacing] = useState<string | null>(null)
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
            {c.valid_until && (
              <>
                <dt>{t('bank.connections.validUntil')}</dt>
                <dd>{day(c.valid_until, i18n.language)}</dd>
              </>
            )}
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
            {withKey(c) ? (
              <button type="button" className={`btn ${c.needs_reconnect ? 'btn--primary' : 'btn--secondary'}`} aria-expanded={replacing === c.id} onClick={() => setReplacing(replacing === c.id ? null : c.id)}>
                {t('bank.connections.replaceKey')}
              </button>
            ) : (
              <button type="button" className={`btn ${c.needs_reconnect ? 'btn--primary' : 'btn--secondary'}`} disabled={reconnect.isPending} onClick={() => reconnect.mutate(c.id)}>
                {t('bank.connections.reconnectShort')}
              </button>
            )}
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
          {replacing === c.id && (
            <KeyForm
              label={t('bank.connect.apiKey')}
              submit={t('bank.connections.replaceKey')}
              pending={replace.isPending}
              error={replace.error}
              onSubmit={(apiKey) => replace.mutate({ id: c.id, apiKey }, { onSuccess: () => setReplacing(null) })}
            />
          )}
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
