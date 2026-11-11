import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { contactName } from '../contacts/queries'
import { DetailBlock, DetailList } from '../detail/Detail'
import { FormSection, formErrors, TextInput } from '../forms/Form'
import { externalTitle, unpaidExternalStates, useExternalInvoice, useExternalInvoices } from '../external/queries'
import { invoiceTitle, unpaidStates, useSalesInvoice, useSalesInvoices } from '../invoices/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { CategorySelect } from './CategorySelect'
import {
  type Booking, formatAmount, type Mutation, useFinancialAccounts, useLinkBooking, useLinkInvoice, useMutationOne, useUnlinkBooking, useUnlinkPayment,
} from './queries'

/** One transaction: its details, its bookings and a form to book what is open. */
export function TransactionPage() {
  const { administrationId = '', mutationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const mutation = useMutationOne(administrationId, mutationId)
  if (mutation.error instanceof ApiError && mutation.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data || !mutation.data) {
    return <Loading />
  }
  return <Transaction administrationId={administrationId} language={administration.data.language} mutation={mutation.data} />
}

function Transaction({ administrationId, mutation: m }: { administrationId: string; language: string; mutation: Mutation }) {
  const { t, i18n } = useTranslation()
  const accounts = useFinancialAccounts(administrationId)
  const categories = useLedgerAccounts(administrationId)
  const unlink = useUnlinkBooking(administrationId, m.id ?? '')
  const money = (v: string | null | undefined) => formatAmount(v, m.currency ?? 'EUR', i18n.language)
  const names = new Map(categories.data?.map((c) => [c.id, c.name]))
  const account = accounts.data?.find((a) => a.id === m.financial_account_id)
  const open = Number(m.amount_open ?? 0)
  const back = `/${administrationId}/financial_mutations`

  return (
    <Page
      title={m.contra_account_name || m.message || t('bank.transaction')}
      subtitle={money(m.amount)}
      back={{ to: back, label: t('bank.toTransactions') }}
      actions={
        <MoreMenu
          items={[
            ...(m.financial_account_id ? [{ to: `/${administrationId}/financial_accounts/${m.financial_account_id}`, label: t('bank.accountSettings') }] : []),
            { to: `/${administrationId}/settings/bank`, label: t('bank.settings') },
          ]}
        />
      }
    >
      <DetailBlock title={t('bank.detailsTitle')}>
        <DetailList
          rows={[
            [t('bank.date'), new Date(`${m.date}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'long' })],
            [t('bank.account'), account?.name ?? '-'],
            [t('bank.amount'), <span key="amount" className={Number(m.amount) < 0 ? 'is-out' : 'is-in'}>{money(m.amount)}</span>],
            [t('bank.contraName'), m.contra_account_name],
            [t('bank.contraAccount'), m.contra_account_number],
            [t('bank.description'), m.message],
            [t('bank.open'), m.state === 'processed' ? <span key="state" className="badge badge--paid">{t('bank.booked')}</span> : money(m.amount_open)],
          ]}
        />
      </DetailBlock>

      <DetailBlock id="bookings" title={t('bank.bookings')} subtitle={t('bank.bookingsSubtitle')}>
        <Problems messages={formErrors(unlink.error, {}).general} />
        {m.ledger_account_bookings.length === 0 ? (
          <p className="empty">{t('bank.noBookings')}</p>
        ) : (
          <table className="data-table" aria-label={t('bank.bookings')}>
            <thead>
              <tr>
                <th scope="col">{t('bank.category')}</th>
                <th scope="col" className="transactions__amount">{t('bank.amount')}</th>
                <th scope="col"><span className="sr-only">{t('bookkeeping.actions')}</span></th>
              </tr>
            </thead>
            <tbody>
              {(m.ledger_account_bookings as Booking[]).map((b) => (
                <tr key={b.id}>
                  <td>
                    {names.get(b.ledger_account_id ?? '') ?? b.ledger_account_id}
                    {b.description && <p className="hint">{b.description}</p>}
                    {b.booking_rule_id && <p className="hint">{t('bank.byRule')}</p>}
                  </td>
                  <td className="transactions__amount">{money(b.price)}</td>
                  <td className="data-table__actions">
                    <button type="button" className="btn btn--secondary btn--small" disabled={unlink.isPending} aria-label={t('bank.removeBookingOn', { name: names.get(b.ledger_account_id ?? '') ?? '' })} onClick={() => unlink.mutate(b.id ?? '')}>
                      {t('bank.removeBooking')}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </DetailBlock>

      <InvoicePayments administrationId={administrationId} mutation={m} />
      {open !== 0 && <LinkInvoiceForm key={`invoice-${m.amount_open}`} administrationId={administrationId} mutation={m} />}
      {open !== 0 && <BookForm key={m.amount_open} administrationId={administrationId} mutation={m} />}
    </Page>
  )
}

function BookForm({ administrationId, mutation: m }: { administrationId: string; mutation: Mutation }) {
  const { t } = useTranslation()
  const link = useLinkBooking(administrationId, m.id ?? '')
  // Amounts are entered in the transaction's direction: positive books it.
  const rest = String(Math.abs(Number(m.amount_open ?? 0)))
  const [form, setForm] = useState({ booking_id: '', price_base: rest, description: '' })
  const labels = { booking_id: t('bank.category'), price_base: t('bank.amount'), description: t('bank.description'), date: t('bank.date') }
  const errors = formErrors(link.error, labels)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const price = Number(form.price_base.replace(',', '.'))
    const sign = Number(m.amount) < 0 === Number(m.amount_open) < 0 ? 1 : -1
    link.mutate({ ...form, price_base: Number.isFinite(price) ? String(price * sign) : form.price_base })
  }
  return (
    <DetailBlock id="book" title={t('bank.book')} subtitle={t('bank.bookSubtitle')}>
      <form className="form-sheet form-sheet--block" onSubmit={submit} noValidate aria-label={t('bank.book')}>
        <Problems messages={errors.general} />
        <FormSection>
          <CategorySelect administrationId={administrationId} label={labels.booking_id} value={form.booking_id} errors={errors.field('booking_id')} onChange={(booking_id) => setForm({ ...form, booking_id })} />
          <TextInput label={labels.price_base} inputMode="decimal" width="small" maxLength={16} help={t('bank.splitHelp')} value={form.price_base} errors={errors.field('price_base') ?? errors.field('date')} onChange={(e) => setForm({ ...form, price_base: e.target.value })} />
          <TextInput label={labels.description} maxLength={1000} value={form.description} errors={errors.field('description')} onChange={(e) => setForm({ ...form, description: e.target.value })} />
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={link.isPending || !form.booking_id}>{t('bank.bookButton')}</button>
        </div>
      </form>
    </DetailBlock>
  )
}

/** The sales invoices the transaction pays, which can be unlinked again. */
function InvoicePayments({ administrationId, mutation: m }: { administrationId: string; mutation: Mutation }) {
  const { t } = useTranslation()
  const unlink = useUnlinkPayment(administrationId, m.id ?? '')
  const payments = m.payments ?? []
  if (payments.length === 0) {
    return null
  }
  return (
    <DetailBlock id="invoice-payments" title={t('bank.invoicePayments')} subtitle={t('bank.invoicePaymentsSubtitle')}>
      <Problems messages={formErrors(unlink.error, {}).general} />
      <table className="data-table" aria-label={t('bank.invoicePayments')}>
        <thead>
          <tr>
            <th scope="col">{t('bank.invoice')}</th>
            <th scope="col" className="transactions__amount">{t('bank.amount')}</th>
            <th scope="col"><span className="sr-only">{t('bookkeeping.actions')}</span></th>
          </tr>
        </thead>
        <tbody>
          {payments.map((p) => (
            <PaymentRow key={p.id} administrationId={administrationId} payment={p} currency={m.currency ?? 'EUR'} disabled={unlink.isPending} onUnlink={() => unlink.mutate(p.id ?? '')} />
          ))}
        </tbody>
      </table>
    </DetailBlock>
  )
}

type PaymentRowProps = { administrationId: string; payment: NonNullable<Mutation['payments']>[number]; currency: string; disabled: boolean; onUnlink: () => void }

function PaymentRow({ administrationId, payment: p, currency, disabled, onUnlink }: PaymentRowProps) {
  const { t, i18n } = useTranslation()
  const external = p.invoice_type === 'ExternalSalesInvoice'
  const sales = useSalesInvoice(administrationId, external ? undefined : p.invoice_id ?? '')
  const other = useExternalInvoice(administrationId, external ? p.invoice_id ?? '' : undefined)
  const name = external
    ? (other.data ? externalTitle(other.data, t) : t('external.externalInvoice'))
    : (sales.data ? invoiceTitle(sales.data, t) : t('bank.salesInvoice'))
  const path = external ? 'external_sales_invoices' : 'sales_invoices'
  return (
    <tr>
      <td><Link to={`/${administrationId}/${path}/${p.invoice_id}`}>{name}</Link></td>
      <td className="transactions__amount">{formatAmount(p.price, currency, i18n.language)}</td>
      <td className="data-table__actions">
        <button type="button" className="btn btn--secondary btn--small" disabled={disabled} aria-label={t('bank.unlinkInvoice', { id: name })} onClick={onUnlink}>
          {t('bank.removeBooking')}
        </button>
      </td>
    </tr>
  )
}

/** An open invoice of either kind the transaction can pay. */
type Payable = { key: string; type: 'SalesInvoice' | 'ExternalSalesInvoice'; id: string; title: string; contact: string; reference: string; unpaid: string | undefined; currency: string }

/** Pays an open sales or external invoice with the transaction, found by its number, reference or contact. */
function LinkInvoiceForm({ administrationId, mutation: m }: { administrationId: string; mutation: Mutation }) {
  const { t, i18n } = useTranslation()
  const id = useId()
  const link = useLinkInvoice(administrationId, m.id ?? '')
  const invoices = useSalesInvoices(administrationId, `state:${unpaidStates}`, 1)
  const externals = useExternalInvoices(administrationId, `state:${unpaidExternalStates}`, 1)
  const [search, setSearch] = useState('')
  const [chosen, setChosen] = useState('')
  const errors = formErrors(link.error, { booking_id: t('bank.invoice'), price_base: t('bank.amount'), date: t('bank.date') })
  const term = search.trim().toLowerCase()
  const all: Payable[] = [
    ...(invoices.data?.invoices ?? []).map((inv): Payable => ({
      key: `SalesInvoice:${inv.id}`, type: 'SalesInvoice', id: inv.id ?? '', title: invoiceTitle(inv, t), contact: inv.contact ? contactName(inv.contact) : '',
      reference: inv.reference ?? '', unpaid: inv.total_unpaid, currency: inv.currency ?? 'EUR',
    })),
    ...(externals.data?.invoices ?? []).map((inv): Payable => ({
      key: `ExternalSalesInvoice:${inv.id}`, type: 'ExternalSalesInvoice', id: inv.id ?? '', title: externalTitle(inv, t), contact: inv.contact ? contactName(inv.contact) : '',
      reference: inv.reference ?? '', unpaid: inv.total_unpaid, currency: inv.currency ?? 'EUR',
    })),
  ]
  const matches = all.filter((inv) => !term || [inv.title, inv.reference, inv.contact].some((v) => v.toLowerCase().includes(term)))
  const loaded = !!invoices.data || !!externals.data
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const inv = all.find((i) => i.key === chosen)
    if (inv) {
      link.mutate({ booking_type: inv.type, booking_id: inv.id })
    }
  }
  return (
    <DetailBlock id="link-invoice" title={t('bank.linkInvoice')} subtitle={t('bank.linkInvoiceSubtitle')}>
      <form className="form-sheet form-sheet--block" onSubmit={submit} noValidate aria-label={t('bank.linkInvoice')}>
        <Problems messages={[...errors.general, ...(errors.field('booking_id') ?? []), ...(errors.field('price_base') ?? []), ...(errors.field('date') ?? [])]} />
        <TextInput label={t('bank.findInvoice')} type="search" maxLength={100} help={t('bank.findInvoiceHelp')} value={search} onChange={(e) => setSearch(e.target.value)} />
        {loaded && matches.length === 0 ? (
          <p className="empty">{t('bank.noOpenInvoices')}</p>
        ) : (
          <fieldset className="invoice-choice">
            <legend className="sr-only">{t('bank.invoice')}</legend>
            {matches.slice(0, 10).map((inv) => (
              <label key={inv.key} className="invoice-choice__row" htmlFor={`${id}-${inv.key}`}>
                <input id={`${id}-${inv.key}`} type="radio" name={`${id}-invoice`} value={inv.key} checked={chosen === inv.key} onChange={() => setChosen(inv.key)} />
                <span>{inv.title}{inv.type === 'ExternalSalesInvoice' && <span className="hint"> · {t('external.externalInvoice')}</span>}</span>
                <span>{inv.contact}</span>
                <span className="transactions__amount">{formatAmount(inv.unpaid, inv.currency, i18n.language)}</span>
              </label>
            ))}
          </fieldset>
        )}
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={link.isPending || !chosen}>{t('bank.linkInvoiceButton')}</button>
        </div>
      </form>
    </DetailBlock>
  )
}
