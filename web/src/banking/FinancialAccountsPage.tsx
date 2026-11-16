import { type DragEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { CaretIcon, InfoIcon, ListIcon, type ListIconKind } from '../layout/icons'
import { groupIBAN } from '../invoices/paper'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { GettingStarted } from '../list/GettingStarted'
import { ActionMenu } from '../list/ListActions'
import { useOverview } from '../overview/queries'
import { type FinancialAccount, formatAmount, useFinancialAccounts, useImportStatementFile } from './queries'
import './accounts.css'

/** The icon of an account by its kind, as Moneybird pictures banks, cash and the others. */
function accountIcon(type: string | undefined): ListIconKind {
  switch (type) {
    case 'BankAccount':
      return 'bank'
    case 'CashAccount':
      return 'coins'
    default:
      return 'card'
  }
}

/** How long ago, in words, as Moneybird's "Last update: about 2 months ago". */
function ago(at: string | null | undefined, language: string, never: string): string {
  if (!at) {
    return never
  }
  const seconds = (new Date(at).getTime() - Date.now()) / 1000
  const units: [Intl.RelativeTimeFormatUnit, number][] = [['year', 31536000], ['month', 2592000], ['week', 604800], ['day', 86400], ['hour', 3600], ['minute', 60]]
  const format = new Intl.RelativeTimeFormat(language, { numeric: 'auto' })
  for (const [unit, size] of units) {
    if (Math.abs(seconds) >= size) {
      return format.format(Math.round(seconds / size), unit)
    }
  }
  return format.format(0, 'minute')
}

/**
 * Moneybird's financial accounts page (/financial_accounts): each account
 * with its balance and last update, opening its transactions; the
 * deactivated ones folded away; a link to the transactions still to link;
 * and Add transactions, a field to drop a statement file on.
 */
export function FinancialAccountsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const navigate = useNavigate()
  const administration = useAdministration(administrationId)
  const accounts = useFinancialAccounts(administrationId)
  const overview = useOverview(administrationId)
  const [info, setInfo] = useState(false)
  const [adding, setAdding] = useState(false)
  const [showInactive, setShowInactive] = useState(false)
  const infoId = useId()
  const addId = useId()

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data || !accounts.data) {
    return <Loading />
  }
  const base = `/${administrationId}`
  const all = accounts.data
  const active = all.filter((a) => a.active)
  const inactive = all.filter((a) => !a.active)
  const shown = showInactive ? all : active
  const unprocessed = overview.data?.bank?.unprocessed_count ?? 0

  const actions = (
    <div className="accounts-actions">
      <button
        type="button"
        className={`btn btn--secondary btn--icon${info ? ' is-active' : ''}`}
        aria-label={t('bank.accounts.moreInfo')}
        title={t('bank.accounts.moreInfo')}
        aria-expanded={info}
        aria-controls={infoId}
        onClick={() => setInfo(!info)}
      >
        <InfoIcon />
      </button>
      <Link className="btn btn--secondary" to={`${base}/financial_mutations`}>{t('bank.accounts.linkUnprocessed', { count: unprocessed })}</Link>
      <button type="button" className={`btn btn--accent btn--with-icon${adding ? ' is-active' : ''}`} aria-expanded={adding} aria-controls={addId} onClick={() => setAdding(!adding)}>
        <span>{t('bank.accounts.addTransactions')}</span>
        <CaretIcon />
      </button>
      <ActionMenu
        label={t('bank.accounts.more')}
        iconOnly
        icon={<ListIcon kind="more" />}
        items={[
          { label: t('bank.accounts.addAccount'), to: `${base}/financial_accounts/new` },
          { label: t('bank.accounts.settings'), to: `${base}/settings/bank` },
        ]}
      />
    </div>
  )

  return (
    <Page wide title={t('bank.accounts.title')} actions={actions}>
      {info && (
        <GettingStarted
          id={infoId}
          title={t('bank.accounts.intro.title')}
          text={t('bank.accounts.intro.text')}
          steps={[
            { icon: 'bank', title: t('bank.accounts.intro.bank'), text: t('bank.accounts.intro.bankText') },
            { icon: 'coins', title: t('bank.accounts.intro.cash'), text: t('bank.accounts.intro.cashText') },
            { icon: 'person', title: t('bank.accounts.intro.private'), text: t('bank.accounts.intro.privateText') },
          ]}
        />
      )}
      {adding && (
        <AddTransactions
          id={addId}
          administrationId={administrationId}
          manualTo={active[0]?.id ? `${base}/financial_accounts/${active[0].id}` : `${base}/financial_accounts/new`}
          onImported={(accountId) => navigate(`${base}/financial_mutations?account=${accountId}`)}
        />
      )}
      {all.length === 0 ? (
        <p className="empty">{t('bank.accounts.none')}</p>
      ) : (
        <section className="accounts-card" aria-label={t('bank.accounts.title')}>
          <ul className="accounts">
            {shown.map((a) => (
              <AccountRow key={a.id} account={a} to={`${base}/financial_mutations?account=${a.id}&state=all`} language={i18n.language} />
            ))}
          </ul>
          {inactive.length > 0 && (
            <p className="accounts__inactive">
              {t('bank.accounts.inactive', { count: inactive.length })}{' '}
              <button type="button" className="btn btn--link" onClick={() => setShowInactive(!showInactive)}>
                {showInactive ? t('bank.accounts.hideInactive', { count: inactive.length }) : t('bank.accounts.showInactive', { count: inactive.length })}
              </button>
            </p>
          )}
        </section>
      )}
    </Page>
  )
}

/** One account: its icon, name and kind, and its balance with when it last changed. */
function AccountRow({ account: a, to, language }: { account: FinancialAccount; to: string; language: string }) {
  const { t } = useTranslation()
  // A bank account is named by its number, as Moneybird lists it; a name of its own shows under it.
  const title = a.type === 'BankAccount' && a.identifier ? groupIBAN(a.identifier) : a.name
  const subtitle = title === a.name ? (a.provider ?? '') : a.name && groupIBAN(a.name) !== title ? a.name : ''
  return (
    <li className={a.active ? undefined : 'is-inactive'}>
      <Link className="accounts__row" to={to} aria-label={title ?? ''}>
        <span className="accounts__icon" aria-hidden="true"><ListIcon kind={accountIcon(a.type)} /></span>
        <span className="accounts__name">
          <span className="accounts__title">{title}</span>
          {subtitle && <span className="accounts__subtitle">{subtitle}</span>}
        </span>
        <span className="accounts__balance">
          {a.active ? (
            <>
              <span className="accounts__amount">{formatAmount(a.balance, a.currency ?? 'EUR', language)}</span>
              <span className="accounts__subtitle">{t('bank.accounts.lastUpdate', { when: ago(a.last_import_at, language, t('bank.accounts.never')) })}</span>
            </>
          ) : (
            <span className="accounts__title">{t('bank.accounts.disabled')}</span>
          )}
        </span>
        <span className="accounts__chevron" aria-hidden="true">›</span>
      </Link>
    </li>
  )
}

type AddProps = { id: string; administrationId: string; manualTo: string; onImported: (accountId: string) => void }

/** Moneybird's Add transactions: a field to drop a CAMT.053 or MT940 file on, or a way to add them by hand. */
function AddTransactions({ id, administrationId, manualTo, onImported }: AddProps) {
  const { t } = useTranslation()
  const upload = useImportStatementFile(administrationId)
  const [over, setOver] = useState(false)
  const take = (files: FileList | null) => {
    const file = files?.[0]
    if (file) {
      upload.mutate(file, { onSuccess: (res) => onImported(res.financial_account_id) })
    }
  }
  const drop = (e: DragEvent) => {
    e.preventDefault()
    setOver(false)
    take(e.dataTransfer.files)
  }
  const errors = formErrors(upload.error, {})
  return (
    <section id={id} className="add-transactions" aria-label={t('bank.accounts.addTransactions')}>
      <label
        className={`add-transactions__drop${over ? ' is-over' : ''}`}
        onDragOver={(e) => {
          e.preventDefault()
          setOver(true)
        }}
        onDragLeave={() => setOver(false)}
        onDrop={drop}
      >
        <span className="add-transactions__icon" aria-hidden="true"><ListIcon kind="upload" /></span>
        <span className="add-transactions__title">{upload.isPending ? t('bank.accounts.importing') : t('bank.accounts.dropTitle')}</span>
        <span className="add-transactions__subtitle">{t('bank.accounts.dropSubtitle')}</span>
        <input
          className="sr-only"
          type="file"
          accept=".xml,.sta,.940,.mt940,.txt,.camt,application/xml,text/xml,text/plain"
          aria-label={t('bank.accounts.dropTitle')}
          disabled={upload.isPending}
          onChange={(e) => {
            take(e.target.files)
            e.target.value = ''
          }}
        />
      </label>
      <Problems messages={[...errors.general, ...(errors.field('file') ?? [])]} />
      <p className="add-transactions__manual">
        {t('bank.accounts.manualText')} <Link to={manualTo}>{t('bank.accounts.manualLink')}</Link>
      </p>
    </section>
  )
}
