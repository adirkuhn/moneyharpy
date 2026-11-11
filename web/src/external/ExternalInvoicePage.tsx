import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { contactName } from '../contacts/queries'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { CheckboxInput, FormSection, formErrors, TextInput } from '../forms/Form'
import { WriteOffPanel } from '../invoices/CollectPanels'
import { ContactPicker } from '../invoices/ContactPicker'
import { InvoiceAttachments, InvoiceNotes } from '../invoices/InvoiceExtras'
import { type LineRow, LinesEditor } from '../invoices/LinesEditor'
import { PaymentsPanel } from '../invoices/PaymentsPanel'
import { attachmentURL, isPayable, today } from '../invoices/queries'
import { StateBadge } from '../invoices/StateBadge'
import { computeTotals, parseCents, parseQuantity } from '../invoices/totals'
import { TotalsTable } from '../invoices/TotalsTable'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import {
  type ExternalChanges, type ExternalInvoice, externalTitle, isEditable, useDeleteExternalInvoice, useExternalInvoice, useSaveExternalInvoice,
} from './queries'

/** One external invoice (/external_sales_invoices/:id), or a new one typed in by hand (/external_sales_invoices/new). */
export function ExternalInvoicePage() {
  const { administrationId = '', invoiceId } = useParams()
  const invoice = useExternalInvoice(administrationId, invoiceId)
  const rates = useTaxRates(administrationId)
  if (invoiceId && invoice.error instanceof ApiError && invoice.error.status < 500) {
    return <NotFoundPage />
  }
  if ((invoiceId && !invoice.data) || !rates.data) {
    return <Loading />
  }
  const inv = invoice.data ?? null
  // Rates the invoice already uses stay available even when they were turned off since.
  const usable = rates.data.filter((r) => r.tax_rate_type === 'sales_invoice' && (r.active || inv?.details?.some((d) => d.tax_rate_id === r.id)))
  if (!inv) {
    return <NewExternalInvoice administrationId={administrationId} rates={usable} />
  }
  return <ExternalInvoiceView key={inv.id} administrationId={administrationId} invoice={inv} rates={usable} />
}

function NewExternalInvoice({ administrationId, rates }: { administrationId: string; rates: TaxRate[] }) {
  const { t } = useTranslation()
  const list = `/${administrationId}/external_sales_invoices`
  return (
    <Page title={t('external.newTitle')} back={{ to: list, label: t('external.backToList') }}>
      <ExternalForm administrationId={administrationId} invoice={null} rates={rates} />
    </Page>
  )
}

function ExternalInvoiceView({ administrationId, invoice: inv, rates }: { administrationId: string; invoice: ExternalInvoice; rates: TaxRate[] }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const remove = useDeleteExternalInvoice(administrationId, inv.id ?? '')
  const [panel, setPanel] = useState<'dubious' | 'uncollectible' | null>(null)
  const list = `/${administrationId}/external_sales_invoices`
  const payable = isPayable(inv.state)
  const file = inv.attachments?.[0]

  const actions = (
    <>
      {payable && !inv.marked_dubious_on && (
        <button type="button" className="btn btn--secondary btn--small" onClick={() => setPanel('dubious')}>{t('invoices.writeOff.dubious')}</button>
      )}
      {payable && (
        <button type="button" className="btn btn--secondary btn--small" onClick={() => setPanel('uncollectible')}>{t('invoices.writeOff.uncollectible')}</button>
      )}
      <ConfirmDelete label={t('invoices.delete')} confirmLabel={t('invoices.confirmDelete')} disabled={remove.isPending} onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })} />
    </>
  )

  return (
    <Page title={externalTitle(inv, t)} subtitle={inv.contact ? contactName(inv.contact) : undefined} back={{ to: list, label: t('external.backToList') }} actions={actions}>
      <Problems messages={formErrors(remove.error, {}).general} />
      {inv.state === 'new' && <p className="notice">{t('external.newNotice')}</p>}
      {file && (
        <p>
          <a className="icon-link" href={attachmentURL(administrationId, inv.id ?? '', file.id ?? '', 'external')} target="_blank" rel="noreferrer">
            {t('external.viewFile', { name: file.filename })}
          </a>
        </p>
      )}
      {panel && <WriteOffPanel key={panel} administrationId={administrationId} invoice={inv} kind={panel} invoiceKind="external" onDone={() => setPanel(null)} />}
      {inv.marked_dubious_on && inv.state !== 'paid' && inv.state !== 'uncollectible' && <p className="notice">{t('invoices.writeOff.dubiousNotice')}</p>}
      {isEditable(inv) ? (
        <>
          <p><StateBadge state={inv.state} /></p>
          {/* A fresh form for every version, so saved lines carry their ids. */}
          <ExternalForm key={inv.updated_at} administrationId={administrationId} invoice={inv} rates={rates} />
        </>
      ) : (
        <>
          <Details administrationId={administrationId} invoice={inv} />
          <Lines invoice={inv} rates={rates} />
        </>
      )}
      {inv.state !== 'new' && <PaymentsPanel administrationId={administrationId} invoice={inv} kind="external" />}
      <InvoiceNotes administrationId={administrationId} invoice={inv} kind="external" />
      <InvoiceAttachments administrationId={administrationId} invoice={inv} kind="external" />
    </Page>
  )
}

function Details({ administrationId, invoice: inv }: { administrationId: string; invoice: ExternalInvoice }) {
  const { t, i18n } = useTranslation()
  const date = (d: string | null | undefined) => d && new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })
  const source = inv.source_url ? <a href={inv.source_url} target="_blank" rel="noreferrer">{inv.source || inv.source_url}</a> : inv.source
  const rows: [string, React.ReactNode][] = [
    [t('invoices.status'), <StateBadge key="state" state={inv.state} />],
    [t('invoices.contact'), inv.contact && <Link to={`/${administrationId}/contacts/${inv.contact_id}`}>{contactName(inv.contact)}</Link>],
    [t('invoices.invoiceDate'), date(inv.date)],
    [t('invoices.dueDate'), date(inv.due_date)],
    [t('invoices.paidAt'), date(inv.paid_at)],
    [t('invoices.markedDubiousOn'), date(inv.marked_dubious_on)],
    [t('invoices.markedUncollectibleOn'), date(inv.marked_uncollectible_on)],
    [t('invoices.reference'), inv.reference],
    [t('external.source'), source],
  ]
  return (
    <dl className="details">
      {rows.filter(([, v]) => v).map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  )
}

function Lines({ invoice: inv, rates }: { invoice: ExternalInvoice; rates: TaxRate[] }) {
  const { t, i18n } = useTranslation()
  const money = (amount: string | number | null | undefined) => formatAmount(amount, inv.currency ?? 'EUR', i18n.language)
  const rateName = (id: string | number | null | undefined) => rates.find((r) => r.id === String(id))?.name ?? ''
  const totals = [
    { label: t('invoices.subtotalExcl'), amount: money(inv.total_price_excl_tax) },
    ...(inv.tax_totals ?? []).map((r) => ({ label: rateName(r.tax_rate_id), amount: money(r.tax_amount) })),
    { label: t('invoices.total'), amount: money(inv.total_price_incl_tax), strong: true },
    { label: t('invoices.unpaid'), amount: money(inv.total_unpaid) },
  ]
  return (
    <section className="invoice-view" aria-labelledby="external-lines">
      <h2 id="external-lines">{inv.prices_are_incl_tax ? t('invoices.linesInclTax') : t('invoices.linesExclTax')}</h2>
      <table className="data-table invoice-view__lines" aria-label={t('invoices.lines')}>
        <thead>
          <tr>
            <th scope="col">{t('invoices.description')}</th>
            <th scope="col" className="invoice-view__quantity data-table__extra">{t('invoices.quantity')}</th>
            <th scope="col" className="invoice-view__money data-table__extra">{t('invoices.price')}</th>
            <th scope="col" className="invoice-view__rate data-table__extra">{t('invoices.vatRate')}</th>
            <th scope="col" className="invoice-view__money">{t('invoices.amount')}</th>
          </tr>
        </thead>
        <tbody>
          {(inv.details ?? []).map((d) => (
            <tr key={d.id}>
              <td className="invoice-view__description">{d.description}</td>
              <td className="invoice-view__quantity data-table__extra">{d.amount}</td>
              <td className="invoice-view__money data-table__extra">{money(d.price)}</td>
              <td className="invoice-view__rate data-table__extra">{rateName(d.tax_rate_id)}</td>
              <td className="invoice-view__money">{money(d.total_price_excl_tax_with_discount)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <TotalsTable rows={totals} />
    </section>
  )
}

let nextKey = 0
const newKey = () => `new-${++nextKey}`
const blank = (taxRateId: string): LineRow => ({ key: newKey(), description: '', amount: '1', price: '', tax_rate_id: taxRateId, ledger_account_id: '' })
/** Shows "100.0" from the server as "100" and keeps "12.5" as it is. */
const priceText = (p: string | undefined) => (p === undefined || p === '' ? '' : String(Number(p)))

/** The contact, dates, reference and lines of an external invoice; with a contact and a line it is booked. */
function ExternalForm({ administrationId, invoice, rates }: { administrationId: string; invoice: ExternalInvoice | null; rates: TaxRate[] }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveExternalInvoice(administrationId, invoice?.id)
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const defaultRate = rates.find((r) => r.active)?.id ?? ''

  const [contact, setContact] = useState({ id: invoice?.contact_id ?? '', name: invoice?.contact ? contactName(invoice.contact) : '' })
  const [reference, setReference] = useState(invoice?.reference ?? '')
  const [date, setDate] = useState(invoice?.date ?? today())
  const [dueDate, setDueDate] = useState(invoice?.due_date ?? '')
  const [inclTax, setInclTax] = useState(invoice?.prices_are_incl_tax ?? false)
  const [rows, setRows] = useState<LineRow[]>(() =>
    invoice?.details?.length
      ? invoice.details.map((d) => ({
          key: d.id ?? newKey(), id: d.id, description: d.description ?? '', amount: d.amount ?? '1', price: priceText(d.price),
          tax_rate_id: d.tax_rate_id ?? defaultRate, ledger_account_id: d.ledger_account_id ?? '',
        }))
      : [blank(defaultRate)],
  )
  const [removed, setRemoved] = useState<string[]>([])

  const currency = invoice?.currency ?? administration.data?.currency ?? 'EUR'
  const money = (cents: number) => formatAmount(cents / 100, currency, i18n.language)
  const parsed = rows.map((r) => ({ price: parseCents(r.price), quantity: parseQuantity(r.amount) }))
  const percentage = (id: string) => Math.round(Number(rates.find((r) => r.id === id)?.percentage ?? 0) * 100)
  const totals = computeTotals(
    rows.map((r, i) => ({ price: parsed[i]?.price ?? 0, quantity: parsed[i]?.quantity ?? 0, taxRateId: r.tax_rate_id, rate: percentage(r.tax_rate_id) })),
    0,
    inclTax,
  )
  const rateName = (id: string) => rates.find((r) => r.id === id)?.name ?? ''

  const labels: Record<string, string> = {
    contact_id: t('invoices.contact'), reference: t('invoices.reference'), date: t('invoices.invoiceDate'), due_date: t('invoices.dueDate'),
    prices_are_incl_tax: t('invoices.pricesInclTax'), details_attributes: t('invoices.lines'), currency: t('external.currency'),
  }
  rows.forEach((_, i) => {
    for (const [field, name] of Object.entries({ description: t('invoices.description'), amount: t('invoices.quantity'), price: t('invoices.price'), tax_rate_id: t('invoices.vatRate'), ledger_account_id: t('invoices.category') })) {
      labels[`details_attributes.${i}.${field}`] = t('invoices.lineField', { field: name, n: i + 1 })
    }
  })
  const errors = formErrors(save.error, labels)

  const change = (i: number, c: Partial<LineRow>) => setRows(rows.map((r, j) => (j === i ? { ...r, ...c } : r)))
  const remove = (i: number) => {
    const row = rows[i]
    if (row?.id) {
      setRemoved([...removed, row.id])
    }
    setRows(rows.filter((_, j) => j !== i))
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    // A new line left empty is not meant as a line.
    const kept = rows.filter((r) => r.id || r.description.trim() || r.price.trim())
    const body: ExternalChanges = {
      ...(contact.id ? { contact_id: contact.id } : {}),
      reference: reference.trim(),
      date,
      due_date: dueDate,
      prices_are_incl_tax: inclTax,
      details_attributes: [
        ...kept.map((r) => ({
          ...(r.id ? { id: r.id } : {}),
          description: r.description.trim(),
          amount: r.amount.trim(),
          price: r.price.trim(),
          tax_rate_id: r.tax_rate_id,
          ...(r.ledger_account_id ? { ledger_account_id: r.ledger_account_id } : {}),
        })),
        ...removed.map((id) => ({ id, _destroy: true })),
      ],
    }
    save.mutate(body, {
      onSuccess: (saved) => {
        setRemoved([])
        if (!invoice) {
          navigate(`/${administrationId}/external_sales_invoices/${saved.id}`)
        }
      },
    })
  }

  const totalRows = [
    { label: t('invoices.subtotal'), amount: money(totals.subtotal) },
    ...totals.byRate.map((r) => ({ label: inclTax ? t('invoices.includesVat', { name: rateName(r.taxRateId) }) : rateName(r.taxRateId), amount: money(r.tax) })),
    { label: t('invoices.total'), amount: money(totals.inclTax), strong: true },
  ]

  return (
    <form className="form-sheet invoice-form" onSubmit={submit} noValidate aria-label={t('external.form')}>
      <Problems messages={[...errors.general, ...(errors.field('details_attributes') ?? [])]} />
      {invoice?.state === 'new' && <p className="hint">{t('external.bookHint')}</p>}
      <FormSection>
        <ContactPicker
          administrationId={administrationId}
          label={labels.contact_id ?? ''}
          value={contact.name}
          errors={errors.field('contact_id')}
          onChange={(c) => setContact({ id: String(c.id ?? ''), name: contactName(c) })}
        />
        <TextInput label={labels.reference ?? ''} maxLength={1000} help={t('external.referenceHelp')} value={reference} errors={errors.field('reference')} onChange={(e) => setReference(e.target.value)} />
        <TextInput label={labels.date ?? ''} type="date" width="medium" required value={date} errors={errors.field('date')} onChange={(e) => setDate(e.target.value)} />
        <TextInput label={labels.due_date ?? ''} type="date" width="medium" value={dueDate} errors={errors.field('due_date')} onChange={(e) => setDueDate(e.target.value)} />
        <CheckboxInput label={labels.prices_are_incl_tax ?? ''} checked={inclTax} onChange={(e) => setInclTax(e.target.checked)} />
      </FormSection>
      <LinesEditor
        rows={rows}
        rates={rates}
        accounts={accounts.data ?? []}
        amounts={totals.lines.map(money)}
        invalid={parsed.map((p) => ({ price: p.price === null, amount: p.quantity === null }))}
        errors={errors}
        onChange={change}
        onRemove={remove}
        onAdd={() => setRows([...rows, blank(defaultRate)])}
      />
      <TotalsTable rows={totalRows} />
      <div className="form-sheet__actions">
        <button className="btn btn--primary" disabled={save.isPending}>{t('external.save')}</button>
        {!invoice && <Link className="btn btn--secondary" to={`/${administrationId}/external_sales_invoices`}>{t('common.cancel')}</Link>}
      </div>
    </form>
  )
}
