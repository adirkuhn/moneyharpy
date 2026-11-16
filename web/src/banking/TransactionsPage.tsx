import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { formErrors } from '../forms/Form'
import { groupIBAN } from '../invoices/paper'
import { CaretIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { AdvancedFilter, FilterPanel, RadioFilter, SelectFilter } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ActionMenu } from '../list/ListActions'
import { allPeriods } from '../list/PeriodFilter'
import { isPeriod, parsePeriod, periodValue } from '../list/period'
import { PeriodPicker } from '../list/PeriodPicker'
import { AddTransactions } from './AddTransactions'
import { LinkPanel } from './LinkPanel'
import { type Booking, formatAmount, type Mutation, useFinancialAccounts, useMutations, useUploadForTransaction } from './queries'
import './transactions.css'

const states = ['all', 'unprocessed', 'processed', 'auto_booked'] as const
type State = (typeof states)[number]
const types = ['all', 'credit', 'debit'] as const
type Kind = (typeof types)[number]
const defaultPeriod = 'this_year'
/** How many transactions show before "Show more", so a year of them stays quick. */
const pageSize = 50

type Filter = { state: State; period: string; account: string; type: Kind }

/**
 * Moneybird's transactions (/financial_mutations): every transaction as a
 * card, newest first, with what it is linked to under it and, while money
 * is still open, Link open amount; filtered by period, status, type and
 * account, everything from this year unless asked.
 */
export function TransactionsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const navigate = useNavigate()
  const [search, setSearch] = useSearchParams()
  const today = useMemo(() => new Date(), [])
  const administration = useAdministration(administrationId)
  const accounts = useFinancialAccounts(administrationId)
  const state: State = (states as readonly string[]).includes(search.get('state') ?? '') ? (search.get('state') as State) : 'all'
  const type: Kind = (types as readonly string[]).includes(search.get('type') ?? '') ? (search.get('type') as Kind) : 'all'
  const asked = search.get('period') ?? ''
  const period = asked === allPeriods || isPeriod(asked, today) ? asked : defaultPeriod
  const account = search.get('account') ?? ''
  const filter = [
    ...(period === allPeriods ? [] : [`period:${period}`]), `state:${state}`, ...(type === 'all' ? [] : [`mutation_type:${type}`]),
    ...(account ? [`financial_account_id:${account}`] : []),
  ].join(',')
  const mutations = useMutations(administrationId, filter)
  const [info, setInfo] = useState(false)
  const [adding, setAdding] = useState(false)
  const [shownCount, setShownCount] = useState(pageSize)
  const introId = useId()
  const addId = useId()

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const base = `/${administrationId}`
  const list = `${base}/financial_mutations`
  const address = (f: Filter) => {
    const next = new URLSearchParams()
    if (f.state !== 'all') next.set('state', f.state)
    if (f.type !== 'all') next.set('type', f.type)
    if (f.period !== defaultPeriod) next.set('period', f.period)
    if (f.account) next.set('account', f.account)
    return next
  }
  const apply = (f: Filter) => setSearch(address(f), { replace: true })
  const linkTo = (f: Filter) => {
    const query = address(f).toString()
    return query ? `${list}?${query}` : list
  }
  const custom = period !== defaultPeriod || account !== '' || type !== 'all'
  const accountName = accounts.data?.find((a) => a.id === account)?.name
  const all = mutations.data ?? []
  const shown = all.slice(0, shownCount)
  const plain = { period: defaultPeriod, account: '', type: 'all' as Kind }

  const filterPanel = (
    <FilterPanel
      label={custom ? (accountName ? t('bank.feed.viewAccount', { name: accountName }) : t('bank.feed.views.custom')) : t(`bank.feed.views.${state}`)}
      title={t('bank.filters')}
      quick={[
        ...states.map((s) => ({ label: t(`bank.feed.views.${s}`), to: linkTo({ ...plain, state: s }), current: !custom && s === state })),
        { label: t('list.allPeriods'), to: linkTo({ ...plain, state, period: allPeriods }), current: period === allPeriods && account === '' && type === 'all' },
      ]}
    >
      <TransactionFilter
        filter={{ state, period, account, type }}
        today={today}
        accounts={(accounts.data ?? []).map((a) => ({ value: a.id ?? '', label: a.name ?? '' }))}
        onApply={apply}
      />
    </FilterPanel>
  )

  const actions = (
    <div className="accounts-actions">
      <button type="button" className={`btn btn--accent btn--with-icon${adding ? ' is-active' : ''}`} aria-expanded={adding} aria-controls={addId} onClick={() => setAdding(!adding)}>
        <ListIcon kind="plus" />
        <span>{t('bank.accounts.addTransactions')}</span>
        <CaretIcon />
      </button>
      <ActionMenu
        label={t('bank.accounts.more')}
        iconOnly
        icon={<ListIcon kind="more" />}
        items={[
          { label: t('bank.feed.about'), onSelect: () => setInfo(!info) },
          { label: t('bank.accounts.title'), to: `${base}/financial_accounts` },
          { label: t('bank.settings'), to: `${base}/settings/bank` },
        ]}
      />
    </div>
  )

  return (
    <Page wide hideTitle title={t('bank.transactions')} lead={filterPanel} actions={actions}>
      {info && (
        <GettingStarted
          id={introId}
          title={t('bank.intro.title')}
          text={t('bank.intro.text')}
          steps={[
            { icon: 'upload', title: t('bank.intro.import'), text: t('bank.intro.importText') },
            { icon: 'flow', title: t('bank.intro.rules'), text: t('bank.intro.rulesText') },
            { icon: 'coins', title: t('bank.intro.link'), text: t('bank.intro.linkText') },
          ]}
        />
      )}
      {adding && (
        <AddTransactions
          id={addId}
          administrationId={administrationId}
          manualTo={`${base}/financial_accounts/${account || (accounts.data?.find((a) => a.active)?.id ?? 'new')}`}
          onImported={(accountId) => {
            setAdding(false)
            navigate(`${list}?account=${accountId}`)
          }}
        />
      )}
      {!mutations.data ? (
        <Loading />
      ) : all.length === 0 ? (
        <p className="empty">{state === 'unprocessed' ? t('bank.nothingToBook') : t('bank.noTransactions')}</p>
      ) : (
        <>
          <ol className="transactions-feed" aria-label={t('bank.transactions')}>
            {shown.map((m) => (
              <li key={m.id}>
                <TransactionCard administrationId={administrationId} mutation={m} currency={administration.data?.currency ?? 'EUR'} />
              </li>
            ))}
          </ol>
          {all.length > shown.length && (
            <div className="transactions-feed__more">
              <button type="button" className="btn btn--secondary" onClick={() => setShownCount(shownCount + pageSize)}>
                {t('bank.feed.showMore', { count: all.length - shown.length })}
              </button>
            </div>
          )}
        </>
      )}
    </Page>
  )
}

const documentPaths: Record<string, string> = {
  SalesInvoice: 'sales_invoices', ExternalSalesInvoice: 'external_sales_invoices', PurchaseInvoice: 'documents', Receipt: 'documents',
}

/**
 * A transaction as Moneybird's card: the status circle, who it was with,
 * its date and account number, its payment reference and amount; then the
 * documents and categories it is linked to; and, while money is open, the
 * way to link it.
 */
function TransactionCard({ administrationId, mutation: m, currency }: { administrationId: string; mutation: Mutation; currency: string }) {
  const { t, i18n } = useTranslation()
  const categories = useLedgerAccounts(administrationId)
  const names = new Map(categories.data?.map((c) => [c.id, c.name]))
  const cur = m.currency ?? currency
  // Moneybird signs every amount: €-1.98 out, €+2,778.60 in.
  const signed = (v: string | number | null | undefined) => {
    const n = Number(v ?? 0)
    const text = formatAmount(Math.abs(n), cur, i18n.language)
    return text.replace(/^(\D*)/, `$1${n < 0 ? '-' : '+'}`)
  }
  const plain = (v: string | number | null | undefined) => formatAmount(v, cur, i18n.language)
  const processed = m.state === 'processed'
  const auto = processed && (m.ledger_account_bookings as Booking[]).some((b) => b.booking_rule_id)
  const reference = m.message ? t('bank.feed.paymentReference', { reference: m.message }) : ''
  const title = m.contra_account_name || reference || t('bank.noDescription')
  const base = `/${administrationId}`
  const links = m.links ?? []
  const bookings = m.ledger_account_bookings as Booking[]
  const open = Number(m.amount_open ?? 0) !== 0
  const [linking, setLinking] = useState(false)
  const upload = useUploadForTransaction(administrationId, m.id ?? '')

  return (
    <article className={`transaction${processed ? ' is-processed' : ''}`} aria-label={title}>
      <div className="transaction__row">
        <span className={`transaction__status${processed ? ' is-processed' : ''}`} role="img" aria-label={processed ? t('bank.feed.processed') : t('bank.feed.unprocessed')} title={processed ? t('bank.feed.processed') : t('bank.feed.unprocessed')}>
          <ListIcon kind="repeat" />
        </span>
        <div className="transaction__main">
          <Link className="transaction__title" to={`${base}/financial_mutations/${m.id}`}>{title}</Link>
          <p className="transaction__subtitle">
            <time dateTime={m.date ?? undefined}>{m.date}</time>
            {m.contra_account_number && ` - ${groupIBAN(m.contra_account_number)}`}
          </p>
          {m.contra_account_name && reference && <p className="transaction__text">{reference}</p>}
        </div>
        {auto && <span className="transaction__auto" role="img" aria-label={t('bank.byRule')} title={t('bank.byRule')}>⚡</span>}
        <span className={`transaction__amount${Number(m.amount) < 0 ? ' is-out' : ' is-in'}`}>{signed(m.amount)}</span>
      </div>
      {(links.length > 0 || bookings.length > 0) && (
        <ul className="transaction__links" aria-label={t('bank.feed.linked')}>
          {links.map((l) => (
            <li key={l.payment_id} className="transaction__link">
              <span className="transaction__link-icon" aria-hidden="true"><ListIcon kind="paperclip" /></span>
              <div className="transaction__main">
                <Link className="transaction__link-title" to={`${base}/${documentPaths[l.document_type] ?? 'documents'}/${l.document_id}`}>
                  <span className="transaction__label">{l.label}</span> {[l.reference, l.contact_name].filter(Boolean).join(' · ')}
                </Link>
                <p className="transaction__subtitle">{[l.date, plain(l.total)].filter(Boolean).join(' · ')}</p>
                {l.payment_reference && <p className="transaction__text">{t('bank.feed.paymentReference', { reference: l.payment_reference })}</p>}
                {l.details && <p className="transaction__text transaction__details">{l.details}</p>}
              </div>
              <span className="transaction__price">{signed(Number(m.amount) < 0 ? -Math.abs(Number(l.price)) : Math.abs(Number(l.price)))}</span>
            </li>
          ))}
          {bookings.map((b) => (
            <li key={b.id} className="transaction__link">
              <span className="transaction__link-icon" aria-hidden="true"><ListIcon kind="flow" /></span>
              <div className="transaction__main">
                <span className="transaction__link-title">{names.get(b.ledger_account_id ?? '') ?? t('bank.category')}</span>
                {b.description && <p className="transaction__text">{b.description}</p>}
              </div>
              <span className="transaction__price">{signed(Number(m.amount) < 0 ? -Math.abs(Number(b.price)) : Math.abs(Number(b.price)))}</span>
            </li>
          ))}
        </ul>
      )}
      {open && linking && <LinkPanel administrationId={administrationId} mutation={m} onDone={() => setLinking(false)} />}
      {open && !linking && (
        <div className="transaction__actions">
          <button type="button" className="btn btn--secondary btn--small btn--with-icon" onClick={() => setLinking(true)}>
            <ListIcon kind="plus" />
            <span>{processed ? t('bank.feed.linkRest') : t('bank.feed.linkOpen')}</span>
          </button>
          <label className={`btn btn--dashed btn--small btn--with-icon${upload.isPending ? ' is-busy' : ''}`}>
            <ListIcon kind="upload" />
            <span>{upload.isPending ? t('bank.feed.uploading') : t('bank.feed.upload')}</span>
            <input
              className="sr-only"
              type="file"
              accept="application/pdf,image/*"
              aria-label={t('bank.feed.upload')}
              disabled={upload.isPending}
              onChange={(e) => {
                const file = e.target.files?.[0]
                if (file) upload.mutate(file)
                e.target.value = ''
              }}
            />
          </label>
          <Problems messages={formErrors(upload.error, {}).general} />
        </div>
      )}
    </article>
  )
}

type FilterProps = { filter: Filter; today: Date; accounts: { value: string; label: string }[]; onApply: (filter: Filter) => void }

/** Moneybird's transaction filter: a period, a status, a type of transaction and an account. */
function TransactionFilter({ filter, today, accounts, onApply }: FilterProps) {
  const { t } = useTranslation()
  const [range, setRange] = useState(() => (filter.period === allPeriods ? null : parsePeriod(filter.period, today)))
  const [state, setState] = useState<State>(filter.state)
  const [type, setType] = useState<Kind>(filter.type)
  const [account, setAccount] = useState(filter.account)
  return (
    <AdvancedFilter value={{ range, state, account, type }} onApply={() => onApply({ state, account, type, period: range ? periodValue(range, today) : allPeriods })}>
      <PeriodPicker value={range} today={today} onChange={setRange} />
      <RadioFilter legend={t('bank.feed.status')} value={state} onChange={(v) => setState(v as State)} options={states.map((s) => ({ value: s, label: t(`bank.feed.states.${s}`) }))} />
      <RadioFilter legend={t('bank.feed.type')} value={type} onChange={(v) => setType(v as Kind)} options={types.map((k) => ({ value: k, label: t(`bank.feed.types.${k}`) }))} />
      <SelectFilter label={t('bank.account')} value={account} onChange={setAccount} options={[{ value: '', label: t('bank.allAccounts') }, ...accounts]} />
    </AdvancedFilter>
  )
}
