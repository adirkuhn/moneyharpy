import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount, useFinancialAccounts, useLinkInvoice, useMutations, type Mutation } from '../banking/queries'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { formErrors, SelectInput, TextInput } from '../forms/Form'
import { ChevronIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type PaymentAction, type PaymentChanges, type SalesInvoice, today, useRegisterPayment, useSalesInvoice, useSettleCredit } from './queries'
import './view.css'

/** The ways Moneybird asks an invoice was paid, in its order. */
const ways = ['bank_transfer', 'cash_payment', 'rounding_error', 'balance_settlement', 'payment_without_proof'] as const
type Way = (typeof ways)[number]
const withText: Way[] = ['bank_transfer', 'cash_payment', 'payment_without_proof']
const balanceTypes = ['non_current_assets', 'current_assets', 'equity', 'provisions', 'non_current_liabilities', 'current_liabilities']

/**
 * Moneybird's "Register payment" of a sales invoice: first how it was paid
 * (/payments/new), then that way's own page (/payments/:way), such as linking
 * the bank transaction or booking a cash payment.
 */
export function RegisterPaymentPage() {
  const { administrationId = '', invoiceId = '', way = 'new' } = useParams()
  const { t } = useTranslation()
  const invoice = useSalesInvoice(administrationId, invoiceId)
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
        ? <Ways administrationId={administrationId} invoice={inv} />
        : way === 'bank_transfer'
          ? <BankTransfer administrationId={administrationId} invoice={inv} />
          : <PaymentForm key={way} administrationId={administrationId} invoice={inv} way={way as Exclude<Way, 'bank_transfer'>} />}
    </Page>
  )
}

const invoicePath = (administrationId: string, inv: SalesInvoice) => `/${administrationId}/sales_invoices/${inv.id}`
const number = (inv: SalesInvoice) => inv.invoice_id ?? ''

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

function Ways({ administrationId, invoice: inv }: { administrationId: string; invoice: SalesInvoice }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const heading = useId()
  const settle = useSettleCredit(administrationId, inv.id ?? '')
  const back = invoicePath(administrationId, inv)
  const credit = !!inv.original_sales_invoice_id && Number(inv.total_price_incl_tax ?? 0) < 0
  return (
    <div className="content-page">
      <div className="content-page__introduction">
        <h2 id={heading} className="content-page__title">{t('invoices.registerPage.question')}</h2>
        <p className="content-page__subtitle">{t('invoices.registerPage.intro', { number: number(inv) })}</p>
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

/** A transaction pays an invoice when it has the invoice's open amount, or names its number or payment reference. */
function matches(m: Mutation, inv: SalesInvoice) {
  if (m.state === 'processed') {
    return false
  }
  const open = Number(inv.total_unpaid ?? 0)
  const amount = Number(m.amount_open ?? m.amount ?? 0)
  const message = (m.message ?? '').toLowerCase()
  const names = [inv.invoice_id, inv.payment_reference].filter((n): n is string => !!n).map((n) => n.toLowerCase())
  return (open !== 0 && amount === open) || names.some((n) => message.includes(n))
}

function BankTransfer({ administrationId, invoice: inv }: { administrationId: string; invoice: SalesInvoice }) {
  const { t, i18n } = useTranslation()
  const mutations = useMutations(administrationId, 'state:unprocessed')
  const back = invoicePath(administrationId, inv)
  const list = useId()
  if (!mutations.data && !mutations.error) {
    return <Loading />
  }
  const found = (mutations.data ?? []).filter((m) => matches(m, inv))
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
    <ContentPage title={t('invoices.registerPage.ways.bank_transfer')} intro={t('invoices.registerPage.matchesText', { number: number(inv) })} actions={<Bottom back={back} />}>
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
            <LinkButton administrationId={administrationId} mutation={m} invoice={inv} />
          </li>
        ))}
      </ul>
    </ContentPage>
  )
}

function LinkButton({ administrationId, mutation, invoice: inv }: { administrationId: string; mutation: Mutation; invoice: SalesInvoice }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const link = useLinkInvoice(administrationId, mutation.id ?? '')
  return (
    <>
      <button
        type="button"
        className="btn btn--primary btn--small"
        disabled={link.isPending}
        onClick={() => link.mutate({ booking_type: 'SalesInvoice', booking_id: inv.id ?? '' }, { onSuccess: () => navigate(invoicePath(administrationId, inv)) })}
      >
        {t('invoices.registerPage.link')}
      </button>
      <Problems messages={formErrors(link.error, {}).general} />
    </>
  )
}

/** The form of a way that books the payment itself: cash, a rounding difference, a balance settlement or without proof. */
function PaymentForm({ administrationId, invoice: inv, way }: { administrationId: string; invoice: SalesInvoice; way: Exclude<Way, 'bank_transfer'> }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const register = useRegisterPayment(administrationId, inv.id ?? '')
  const ledger = useLedgerAccounts(administrationId)
  const accounts = useFinancialAccounts(administrationId)
  const back = invoicePath(administrationId, inv)
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
  const reference = inv.reference ? t('invoices.registerPage.withReference', { reference: inv.reference }) : ''

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
        intro={t(`invoices.registerPage.intros.${way}`, { number: number(inv), reference })}
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
