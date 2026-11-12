import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { containsPaymentReference } from './paymentReference'
import { formatAmount, useFinancialAccounts, useLinkInvoice, useMutations, type Mutation } from '../banking/queries'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { formErrors, SelectInput, TextInput } from '../forms/Form'
import { ChevronIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { useExternalInvoice } from '../external/queries'
import { type InvoiceKind, type PaymentAction, type PaymentChanges, type SalesInvoice, today, useRegisterPayment, useSalesInvoice, useSettleCredit } from './queries'
import './view.css'

/** The ways Moneybird asks an invoice was paid, in its order. */
const ways = ['bank_transfer', 'cash_payment', 'rounding_error', 'balance_settlement', 'payment_without_proof'] as const
type Way = (typeof ways)[number]
const withText: Way[] = ['bank_transfer', 'cash_payment', 'payment_without_proof']
const balanceTypes = ['non_current_assets', 'current_assets', 'equity', 'provisions', 'non_current_liabilities', 'current_liabilities']

/** What the payment pages need of an invoice of either kind. */
type Payable = Pick<SalesInvoice, 'id' | 'total_unpaid' | 'total_price_incl_tax' | 'currency' | 'reference'> &
  Partial<Pick<SalesInvoice, 'invoice_id' | 'payment_reference' | 'original_sales_invoice_id'>>

/**
 * Moneybird's "Register payment" of a sales invoice: first how it was paid
 * (/payments/new), then that way's own page (/payments/:way), such as linking
 * the bank transaction or booking a cash payment.
 */
export function RegisterPaymentPage() {
  const { administrationId = '', invoiceId = '' } = useParams()
  const invoice = useSalesInvoice(administrationId, invoiceId)
  return <PaymentPages invoice={invoice} kind="sales" />
}

/** The same pages for an external invoice (/external_sales_invoices/:id/payments/:way). */
export function ExternalRegisterPaymentPage() {
  const { administrationId = '', invoiceId = '' } = useParams()
  const invoice = useExternalInvoice(administrationId, invoiceId)
  return <PaymentPages invoice={invoice} kind="external" />
}

function PaymentPages({ invoice, kind }: { invoice: { data?: Payable; error: Error | null }; kind: InvoiceKind }) {
  const { administrationId = '', way = 'new' } = useParams()
  const { t } = useTranslation()
  if (invoice.error instanceof ApiError && invoice.error.status < 500) {
    return <NotFoundPage />
  }
  if (way !== 'new' && !(ways as readonly string[]).includes(way)) {
    return <NotFoundPage />
  }
  if (!invoice.data) {
    return <Loading />
  }
  const inv = invoice.data
  return (
    <Page title={t('invoices.registerPage.title')}>
      {way === 'new'
        ? <Ways administrationId={administrationId} invoice={inv} kind={kind} />
        : way === 'bank_transfer'
          ? <BankTransfer administrationId={administrationId} invoice={inv} kind={kind} />
          : <PaymentForm key={way} administrationId={administrationId} invoice={inv} kind={kind} way={way as Exclude<Way, 'bank_transfer'>} />}
    </Page>
  )
}

const invoicePath = (administrationId: string, inv: Payable, kind: InvoiceKind) =>
  `/${administrationId}/${kind === 'external' ? 'external_sales_invoices' : 'sales_invoices'}/${inv.id}`
/** How the pages name the invoice: a sales invoice by its number, an external one by its reference. */
const number = (inv: Payable, kind: InvoiceKind) => (kind === 'external' ? inv.reference : inv.invoice_id) ?? ''
type PageProps = { administrationId: string; invoice: Payable; kind: InvoiceKind }

/** A page's introduction, its content and the buttons along its bottom, after Moneybird's content pages. */
function ContentPage({ title, intro, children, actions }: { title: string; intro?: ReactNode; children?: ReactNode; actions: ReactNode }) {
  return (
    <div className="content-page">
      <div className="content-page__introduction">
        <h2 className="content-page__title">{title}</h2>
        {intro && <p className="content-page__subtitle">{intro}</p>}
      </div>
      {children && <div className="content-page__item">{children}</div>}
      <div className="content-page__actions">{actions}</div>
    </div>
  )
}

function Ways({ administrationId, invoice: inv, kind }: PageProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const heading = useId()
  const settle = useSettleCredit(administrationId, inv.id ?? '')
  const back = invoicePath(administrationId, inv, kind)
  const credit = !!inv.original_sales_invoice_id && Number(inv.total_price_incl_tax ?? 0) < 0
  return (
    <div className="content-page">
      <div className="content-page__introduction">
        <h2 id={heading} className="content-page__title">{t('invoices.registerPage.question')}</h2>
        <p className="content-page__subtitle">{t(kind === 'external' ? 'invoices.registerPage.externalIntro' : 'invoices.registerPage.intro', { number: number(inv, kind) })}</p>
      </div>
      <div className="content-page__item">
        <Problems messages={formErrors(settle.error, {}).general} />
        {credit && (
          <div className="link-list__credit">
            <button type="button" className="btn btn--primary" disabled={settle.isPending} onClick={() => settle.mutate(undefined, { onSuccess: () => navigate(back) })}>
              {t('invoices.payments.settleCredit')}
            </button>
            <p className="hint">{t('invoices.payments.settleCreditHelp')}</p>
          </div>
        )}
        <ul className="link-list" aria-labelledby={heading}>
          {ways.map((w) => (
            <li key={w}>
              <Link className="link-list__item" to={`${back}/payments/${w}`} aria-label={t(`invoices.registerPage.ways.${w}`)}>
                <span className="link-list__body">
                  <span className="link-list__title">{t(`invoices.registerPage.ways.${w}`)}</span>
                  {withText.includes(w) && <span className="link-list__subtitle">{t(`invoices.registerPage.waysText.${w}`)}</span>}
                </span>
                <ChevronIcon className="link-list__chevron" />
              </Link>
            </li>
          ))}
        </ul>
      </div>
      <div className="content-page__actions">
        <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
      </div>
    </div>
  )
}

/** The buttons along the bottom of a way's page: Cancel to the invoice, Back to the ways. */
function Bottom({ back, children }: { back: string; children?: ReactNode }) {
  const { t } = useTranslation()
  return (
    <>
      <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
      <Link className="btn btn--secondary" to={`${back}/payments/new`}>{t('invoices.registerPage.back')}</Link>
      {children}
    </>
  )
}

/**
 * How well a transaction fits the invoice: 2 when it quotes the invoice's
 * payment reference, 1 when it has the open amount or names the number, 0
 * when it does not fit.
 */
function fit(m: Mutation, inv: Payable, kind: InvoiceKind) {
  if (m.state === 'processed') {
    return 0
  }
  if (containsPaymentReference(m.message, inv.payment_reference)) {
    return 2
  }
  const open = Number(inv.total_unpaid ?? 0)
  const amount = Number(m.amount_open ?? m.amount ?? 0)
  const number = (kind === 'external' ? inv.reference : inv.invoice_id)?.toLowerCase()
  return (open !== 0 && amount === open) || (!!number && (m.message ?? '').toLowerCase().includes(number)) ? 1 : 0
}

function BankTransfer({ administrationId, invoice: inv, kind }: PageProps) {
  const { t, i18n } = useTranslation()
  const mutations = useMutations(administrationId, 'state:unprocessed')
  const back = invoicePath(administrationId, inv, kind)
  const list = useId()
  if (!mutations.data && !mutations.error) {
    return <Loading />
  }
  // A transaction that quotes the payment reference comes first.
  const found = (mutations.data ?? []).map((m) => ({ m, fit: fit(m, inv, kind) })).filter((f) => f.fit > 0).sort((a, b) => b.fit - a.fit).map((f) => f.m)
  if (found.length === 0) {
    return (
      <ContentPage
        title={t('invoices.registerPage.noMatches')}
        intro={<>{t('invoices.registerPage.noMatchesText')} <Link to={`/${administrationId}/financial_mutations`}>{t('invoices.registerPage.bank')}</Link>.</>}
        actions={<Bottom back={back} />}
      />
    )
  }
  return (
    <ContentPage title={t('invoices.registerPage.ways.bank_transfer')} intro={t('invoices.registerPage.matchesText', { number: number(inv, kind) })} actions={<Bottom back={back} />}>
      <ul id={list} className="link-list" aria-label={t('invoices.registerPage.matches')}>
        {found.map((m) => (
          <li key={m.id} className="link-list__item link-list__item--static">
            <span className="link-list__body">
              <span className="link-list__title">{m.contra_account_name || m.message}</span>
              <span className="link-list__subtitle">
                {[m.message, m.date && new Date(`${m.date}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })].filter(Boolean).join(' · ')}
              </span>
            </span>
            <span className="link-list__amount">{formatAmount(m.amount, m.currency ?? inv.currency ?? 'EUR', i18n.language)}</span>
            <LinkButton administrationId={administrationId} mutation={m} invoice={inv} kind={kind} />
          </li>
        ))}
      </ul>
    </ContentPage>
  )
}

function LinkButton({ administrationId, mutation, invoice: inv, kind }: PageProps & { mutation: Mutation }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const link = useLinkInvoice(administrationId, mutation.id ?? '')
  return (
    <>
      <button
        type="button"
        className="btn btn--primary btn--small"
        disabled={link.isPending}
        onClick={() => link.mutate(
          { booking_type: kind === 'external' ? 'ExternalSalesInvoice' : 'SalesInvoice', booking_id: inv.id ?? '' },
          { onSuccess: () => navigate(invoicePath(administrationId, inv, kind)) },
        )}
      >
        {t('invoices.registerPage.link')}
      </button>
      <Problems messages={formErrors(link.error, {}).general} />
    </>
  )
}

/** The form of a way that books the payment itself: cash, a rounding difference, a balance settlement or without proof. */
function PaymentForm({ administrationId, invoice: inv, kind, way }: PageProps & { way: Exclude<Way, 'bank_transfer'> }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const register = useRegisterPayment(administrationId, inv.id ?? '', kind)
  const ledger = useLedgerAccounts(administrationId)
  const accounts = useFinancialAccounts(administrationId)
  const back = invoicePath(administrationId, inv, kind)
  // A balance settlement goes against a balance sheet category, but not the debtors or creditors themselves.
  const balance = (ledger.data ?? []).filter((a) =>
    a.active !== false && balanceTypes.includes(a.account_type ?? '') && a.system_role !== 'accounts_receivable' && a.system_role !== 'accounts_payable')
  const [price, setPrice] = useState(Number(inv.total_unpaid ?? 0).toFixed(2))
  const [date, setDate] = useState(today)
  const [category, setCategory] = useState('')
  const [account, setAccount] = useState('')
  const labels = { price: t('invoices.registerPage.amount'), payment_date: t('invoices.registerPage.date'), ledger_account_id: t('invoices.registerPage.category'), financial_account_id: t('invoices.registerPage.method') }
  const errors = formErrors(register.error, labels)
  const chosenCategory = category || balance[0]?.id || ''
  // An external invoice is named by its reference already.
  const reference = inv.reference && kind === 'sales' ? t('invoices.registerPage.withReference', { reference: inv.reference }) : ''

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const payment: PaymentChanges = {
      payment_date: date,
      price: way === 'rounding_error' ? Number(inv.total_unpaid ?? 0).toFixed(2) : price.trim().replace(',', '.'),
      manual_payment_action: way as PaymentAction,
      ...(way === 'balance_settlement' && chosenCategory ? { ledger_account_id: chosenCategory } : {}),
      ...(way === 'payment_without_proof' && account ? { financial_account_id: account } : {}),
    }
    register.mutate(payment, { onSuccess: () => navigate(back) })
  }

  return (
    <form onSubmit={submit} noValidate aria-label={t('invoices.registerPage.title')}>
      <ContentPage
        title={t(`invoices.registerPage.headings.${way}`)}
        intro={t(`invoices.registerPage.intros.${way}`, { number: number(inv, kind), reference })}
        actions={<Bottom back={back}><button className="btn btn--primary" disabled={register.isPending}>{t('invoices.registerPage.save')}</button></Bottom>}
      >
        <div className="form-sheet">
          <Problems messages={errors.general} />
          {way === 'balance_settlement' && (
            <SelectInput label={labels.ledger_account_id} width="long" value={chosenCategory} errors={errors.field('ledger_account_id')} onChange={(e) => setCategory(e.target.value)}>
              {balance.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
            </SelectInput>
          )}
          {way === 'payment_without_proof' && (
            <SelectInput label={labels.financial_account_id} width="long" value={account} errors={errors.field('financial_account_id')} onChange={(e) => setAccount(e.target.value)}>
              <option value="">{t('invoices.registerPage.withoutAccount')}</option>
              {(accounts.data ?? []).filter((a) => a.active !== false).map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
            </SelectInput>
          )}
          {way !== 'rounding_error' && (
            <TextInput label={labels.price} inputMode="decimal" width="medium" maxLength={16} required value={price} errors={errors.field('price')} onChange={(e) => setPrice(e.target.value)} />
          )}
          <TextInput label={labels.payment_date} type="date" width="long" required value={date} errors={errors.field('payment_date')} onChange={(e) => setDate(e.target.value)} />
        </div>
      </ContentPage>
    </form>
  )
}
