import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { contactName } from '../contacts/queries'
import { ConfirmPanel } from '../detail/Detail'
import { DateInput } from '../forms/DatePicker'
import { formErrors, TextInput } from '../forms/Form'
import { ContactPicker } from '../invoices/ContactPicker'
import { type DetailRow, DetailsEditor } from '../invoices/DetailsEditor'
import { AttachmentDropZone } from '../invoices/InvoiceExtras'
import { detailField, detailOf, type NewEntity, type NewEntityKey } from '../invoices/newEntity'
import { today } from '../invoices/queries'
import { computeTotals, parseCents, parseQuantity } from '../invoices/totals'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { PageIcon } from '../layout/pageIcons'
import { type ExternalChanges, type ExternalInvoice, useDeleteExternalInvoice, useExternalInvoice, useSaveExternalInvoice } from './queries'
import '../invoices/editor.css'
import './external.css'

/** Moneybird's edit page of an external invoice (/external_sales_invoices/:id/edit), or a new one (/external_sales_invoices/new). */
export function ExternalEditPage() {
  const { administrationId = '', invoiceId } = useParams()
  const invoice = useExternalInvoice(administrationId, invoiceId)
  if (invoiceId && invoice.error instanceof ApiError && invoice.error.status < 500) {
    return <NotFoundPage />
  }
  if (invoiceId && !invoice.data) {
    return <Loading />
  }
  // A fresh form for every version, so saved lines carry their ids.
  return <ExternalFormPage key={invoice.data?.updated_at ?? 'new'} administrationId={administrationId} invoice={invoice.data ?? null} />
}

let nextKey = 0
const newKey = () => `new-${++nextKey}`
const blank = (taxRateId: string, ledgerAccountId = ''): DetailRow => ({
  key: newKey(), description: '', amount: '1', price: '', tax_rate_id: taxRateId, ledger_account_id: ledgerAccountId, period: '', product_id: '',
})
/** Shows "100.0" from the server as "100" and keeps "12.5" as it is. */
const priceText = (p: string | undefined) => (p === undefined || p === '' ? '' : String(Number(p)))
const isBlank = (r: DetailRow) => !r.id && !r.description.trim() && !r.price.trim()

/** The day a number of days after a yyyy-mm-dd date, or nothing when the number cannot be read. */
export function daysAfter(date: string, days: string) {
  const n = Number(days.trim())
  if (!/^\d{1,4}$/.test(days.trim()) || !/^\d{4}-\d{2}-\d{2}$/.test(date)) {
    return null
  }
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + n)
  return d.toISOString().slice(0, 10)
}

type Props = { administrationId: string; invoice: ExternalInvoice | null }

/**
 * After Moneybird's "Edit external invoice": the contact as the first step,
 * then the invoice's id, dates and source and its details, with Save and
 * Cancel underneath and the invoice's files beside it.
 */
export function ExternalFormPage({ administrationId, invoice }: Props) {
  const rates = useTaxRates(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const administration = useAdministration(administrationId)
  if (!rates.data || !accounts.data || (!invoice && !administration.data)) {
    return <Loading />
  }
  // Rates the invoice already uses stay available even when they were turned off since.
  const usable = rates.data.filter((r) => r.tax_rate_type === 'sales_invoice' && (r.active || invoice?.details?.some((d) => d.tax_rate_id === r.id)))
  return <Form administrationId={administrationId} invoice={invoice} rates={usable} currency={invoice?.currency ?? administration.data?.currency ?? 'EUR'} />
}

function Form({ administrationId, invoice, rates, currency: startCurrency }: Props & { rates: TaxRate[]; currency: string }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const id = useId()
  const save = useSaveExternalInvoice(administrationId, invoice?.id)
  const remove = useDeleteExternalInvoice(administrationId, invoice?.id ?? '')
  const accounts = useLedgerAccounts(administrationId)
  const defaultRate = rates.find((r) => r.active)?.id ?? ''
  const list = `/${administrationId}/external_sales_invoices`

  const [contact, setContact] = useState({ id: invoice?.contact_id ?? '', name: invoice?.contact ? contactName(invoice.contact) : '' })
  const [choosingContact, setChoosingContact] = useState(!invoice?.contact_id)
  const [reference, setReference] = useState(invoice?.reference ?? '')
  const [date, setDate] = useState(invoice?.date ?? today())
  const [dueDate, setDueDate] = useState(invoice?.due_date ?? '')
  const [dueMode, setDueMode] = useState<'date' | 'days'>('date')
  const [days, setDays] = useState('')
  const [source, setSource] = useState(invoice?.source ?? '')
  const [sourceUrl, setSourceUrl] = useState(invoice?.source_url ?? '')
  const [currency, setCurrency] = useState(startCurrency)
  const [inclTax, setInclTax] = useState(invoice?.prices_are_incl_tax ?? false)
  const [rows, setRows] = useState<DetailRow[]>(() => {
    const loaded = invoice?.details?.length
      ? [...invoice.details]
          .sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
          .map((d) => ({
            key: d.id ?? newKey(), id: d.id, description: d.description ?? '', amount: d.amount ?? '1', price: priceText(d.price),
            tax_rate_id: d.tax_rate_id ?? defaultRate, ledger_account_id: d.ledger_account_id ?? '', period: d.period ?? '', savedPeriod: d.period ?? '',
            product_id: d.product_id ? String(d.product_id) : '',
          }))
      : [blank(defaultRate)]
    // Back from adding a category or VAT rate for a row, it is chosen there, as Moneybird's new_entity_field does.
    const entity = (location.state as { newEntity?: NewEntity } | null)?.newEntity
    const added = entity ? detailOf(entity.field) : null
    return entity && added ? loaded.map((r, i) => (i === added.index ? { ...r, [added.key]: entity.id } : r)) : loaded
  })
  const [removed, setRemoved] = useState<string[]>([])
  const [confirming, setConfirming] = useState(false)

  const due = dueMode === 'days' ? (daysAfter(date, days) ?? dueDate) : dueDate
  const percentage = (rateId: string) => Math.round(Number(rates.find((r) => r.id === rateId)?.percentage ?? 0) * 100)
  const parsed = rows.map((r) => ({ price: parseCents(r.price), quantity: parseQuantity(r.amount) }))
  const totals = computeTotals(
    rows.map((r, i) => ({ price: parsed[i]?.price ?? 0, quantity: parsed[i]?.quantity ?? 0, taxRateId: r.tax_rate_id, rate: percentage(r.tax_rate_id) })),
    0,
    inclTax,
  )
  const money = (cents: number) => formatAmount(cents / 100, currency, i18n.language)
  const rateName = (rateId: string) => rates.find((r) => r.id === rateId)?.name ?? ''
  const hasPayments = (invoice?.payments ?? []).length > 0

  const f = (key: string) => t(`external.edit.${key}`)
  const labels: Record<string, string> = {
    contact_id: f('contact'), reference: f('invoiceId'), date: f('date'), due_date: f('dueDate'), source: f('source'), source_url: f('sourceUrl'),
    currency: t('invoices.details.currency'), prices_are_incl_tax: t('invoices.details.pricesAre'), details_attributes: t('invoices.details.title'),
  }
  rows.forEach((_, i) => {
    for (const [field, name] of Object.entries({
      description: t('invoices.description'), amount: t('invoices.quantity'), price: t('invoices.price'), tax_rate_id: t('invoices.vatRate'),
      ledger_account_id: t('invoices.category'), period: t('invoices.details.period'),
    })) {
      labels[`details_attributes.${i}.${field}`] = t('invoices.lineField', { field: name, n: i + 1 })
    }
  })
  const errors = formErrors(save.error ?? remove.error, labels)

  const body = (): ExternalChanges => {
    const kept = rows.filter((r) => !isBlank(r))
    return {
      ...(contact.id ? { contact_id: contact.id } : {}),
      reference: reference.trim(),
      date,
      due_date: due,
      source: source.trim(),
      source_url: sourceUrl.trim(),
      currency,
      prices_are_incl_tax: inclTax,
      details_attributes: [
        ...kept.map((r, i) => ({
          ...(r.id ? { id: r.id } : {}),
          description: r.description.trim(),
          amount: r.amount.trim(),
          price: r.price.trim(),
          tax_rate_id: r.tax_rate_id,
          ...(r.ledger_account_id ? { ledger_account_id: r.ledger_account_id } : {}),
          ...(r.product_id ? { product_id: r.product_id } : {}),
          ...(r.period || r.savedPeriod ? { period: r.period } : {}),
          row_order: i + 1,
        })),
        ...removed.map((removedId) => ({ id: removedId, _destroy: true })),
      ],
    }
  }
  const submit = (then?: (saved: ExternalInvoice) => void) => save.mutate(body(), {
    onSuccess: (saved) => {
      setRemoved([])
      if (then) {
        then(saved)
      } else {
        navigate(`${list}/${saved.id}`)
      }
    },
  })
  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    submit()
  }

  const change = (i: number, c: Partial<DetailRow>) => setRows(rows.map((r, j) => (j === i ? { ...r, ...c } : r)))
  const removeRow = (i: number) => {
    const row = rows[i]
    if (row?.id) {
      setRemoved([...removed, row.id])
    }
    const left = rows.filter((_, j) => j !== i)
    // The editor keeps one row to type in, as Moneybird's does.
    setRows(left.length ? left : [blank(defaultRate)])
  }
  // A new detail takes the VAT rate and category of the one above it.
  const addRow = (over?: Partial<DetailRow>) => {
    const above = rows.at(-1)
    setRows([...rows, { ...blank(above?.tax_rate_id || defaultRate, above?.ledger_account_id ?? ''), ...over }])
  }
  const move = (from: number, to: number) => {
    const next = [...rows]
    const [row] = next.splice(from, 1)
    if (row) {
      next.splice(to, 0, row)
      setRows(next)
    }
  }
  const applyBelow = (i: number, c: Partial<DetailRow>) => setRows(rows.map((r, j) => (j > i ? { ...r, ...c } : r)))
  /** "Add Category..." and "Add VAT rate..." of a row: the invoice is saved first, then Moneybird's page comes back to it with the new one on that row. */
  const addEntity = (index: number, key: NewEntityKey) => submit((saved) => {
    const query = new URLSearchParams({
      return_to: `${list}/${saved.id}/edit`, ...(key === 'ledger_account_id' ? { source: 'sales_invoice' } : {}), new_entity_field: detailField(index, key),
    })
    navigate(`/${administrationId}/${key === 'ledger_account_id' ? 'ledger_accounts/blueprints' : 'tax_rates/new'}?${query}`)
  })
  const cancel = () => navigate(invoice ? `${list}/${invoice.id}` : list)

  const title = invoice ? f('editTitle') : f('newTitle')
  const contactStep = `${id}-contact`
  const detailsStep = `${id}-details`

  return (
    <div className="external-edit">
      <div className="external-edit__main">
        <header className="external-edit__bar">
          <h1>{title}</h1>
          {invoice && (
            <button type="button" className="btn btn--secondary btn--icon" aria-label={t('external.view.delete')} title={t('external.view.delete')} onClick={() => setConfirming(true)}>
              <PageIcon kind="trash" />
            </button>
          )}
        </header>
        {confirming && (
          <ConfirmPanel
            message={t('external.confirmDeleteText')}
            confirmLabel={t('invoices.confirmDelete')}
            disabled={remove.isPending}
            onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
            onCancel={() => setConfirming(false)}
          />
        )}
        <form className="external-edit__form" onSubmit={onSubmit} noValidate aria-label={title}>
          <Problems messages={[...errors.general, ...(errors.field('details_attributes') ?? [])]} />
          <ol className="vertical-stepper">
            <li className={`vertical-stepper__step${contact.id && !choosingContact ? ' vertical-stepper__step--checked' : ' vertical-stepper__step--active'}`}>
              <div role="group" aria-labelledby={contactStep}>
                <div className="vertical-stepper__header">
                  <span className="vertical-stepper__circle" aria-hidden="true">{contact.id && !choosingContact && <CheckIcon />}</span>
                  <h3 id={contactStep} className="vertical-stepper__title">{f('contact')}</h3>
                  {contact.id && !choosingContact && (
                    <button type="button" className="vertical-stepper__action" aria-label={f('changeContact')} onClick={() => setChoosingContact(true)}>
                      <ListIcon kind="edit" />
                      <span>{contact.name}</span>
                    </button>
                  )}
                </div>
                {choosingContact && (
                  <div className="vertical-stepper__body external-edit__fields">
                    <ContactPicker
                      administrationId={administrationId}
                      label={f('contact')}
                      value={contact.name}
                      errors={errors.field('contact_id')}
                      onChange={(c) => {
                        setContact({ id: String(c.id ?? ''), name: contactName(c) })
                        setChoosingContact(false)
                      }}
                    />
                  </div>
                )}
              </div>
            </li>
            <li className="vertical-stepper__step vertical-stepper__step--active">
              <div role="group" aria-labelledby={detailsStep}>
                <div className="vertical-stepper__header">
                  <span className="vertical-stepper__circle" aria-hidden="true" />
                  <h3 id={detailsStep} className="vertical-stepper__title">{f('addDetails')}</h3>
                </div>
                <div className="vertical-stepper__body">
                  <div className="external-edit__fields">
                    <TextInput label={labels.reference ?? ''} width="medium" maxLength={1000} value={reference} errors={errors.field('reference')} onChange={(e) => setReference(e.target.value)} />
                    <DateInput label={labels.date ?? ''} width="medium" required value={date} errors={errors.field('date')} onChange={setDate} />
                    <DueDate
                      label={labels.due_date ?? ''}
                      mode={dueMode}
                      date={dueDate}
                      days={days}
                      errors={errors.field('due_date')}
                      onMode={(m) => {
                        setDueMode(m)
                        // Back to a date, the date the days gave stays.
                        if (m === 'date') {
                          setDueDate(due)
                        }
                      }}
                      onDate={setDueDate}
                      onDays={setDays}
                    />
                  </div>
                  <hr />
                  <div className="external-edit__fields">
                    <TextInput label={labels.source ?? ''} width="medium" maxLength={1000} value={source} errors={errors.field('source')} onChange={(e) => setSource(e.target.value)} />
                    <TextInput
                      label={labels.source_url ?? ''}
                      width="medium"
                      maxLength={1000}
                      placeholder="https://..."
                      inputMode="url"
                      value={sourceUrl}
                      errors={errors.field('source_url')}
                      onChange={(e) => setSourceUrl(e.target.value)}
                    />
                  </div>
                  <hr />
                  <DetailsEditor
                    administrationId={administrationId}
                    rows={rows}
                    rates={rates}
                    accounts={accounts.data ?? []}
                    currency={currency}
                    pricesInclTax={inclTax}
                    onPricesInclTax={setInclTax}
                    onCurrency={setCurrency}
                    currencyLocked={hasPayments}
                    currencyTip={hasPayments ? f('currencyLocked') : undefined}
                    lineTotals={totals.lines}
                    invalid={parsed.map((p) => ({ price: p.price === null, amount: p.quantity === null }))}
                    totals={[
                      { label: t('invoices.subtotal'), amount: money(totals.exclTax) },
                      ...totals.byRate.map((r) => ({ label: rateName(r.taxRateId), amount: money(r.tax) })),
                      { label: t('invoices.total'), amount: money(totals.inclTax), strong: true },
                    ]}
                    discount=""
                    discountInvalid={false}
                    discounts={false}
                    errors={errors}
                    onChange={change}
                    onRemove={removeRow}
                    onAdd={addRow}
                    onCancel={cancel}
                    onMove={move}
                    onApplyBelow={applyBelow}
                    onDiscount={() => undefined}
                    onSave={() => submit()}
                    saving={save.isPending}
                    onAddEntity={addEntity}
                  />
                </div>
              </div>
            </li>
          </ol>
        </form>
      </div>
      <aside className="external-edit__files">
        {invoice
          ? <AttachmentDropZone administrationId={administrationId} invoice={invoice} kind="external" />
          : <p className="external-edit__later">{f('filesLater')}</p>}
      </aside>
    </div>
  )
}

type DueDateProps = {
  label: string
  mode: 'date' | 'days'
  date: string
  days: string
  errors?: string[]
  onMode: (mode: 'date' | 'days') => void
  onDate: (date: string) => void
  onDays: (days: string) => void
}

/** Moneybird's due date: a date, or a number of days after the invoice date, chosen at its right. */
function DueDate({ label, mode, date, days, errors, onMode, onDate, onDays }: DueDateProps) {
  const { t } = useTranslation()
  const name = useId()
  const f = (key: string) => t(`external.edit.${key}`)
  return (
    <div className="external-edit__due">
      {mode === 'date'
        ? <DateInput label={label} width="medium" value={date} errors={errors} onChange={onDate} />
        : <TextInput label={label} aria-label={f('daysUntilDue')} width="small" inputMode="numeric" maxLength={4} value={days} errors={errors} onChange={(e) => onDays(e.target.value)} />}
      <fieldset className="segmented-control" role="radiogroup" aria-label={f('dueAs')}>
        {(['date', 'days'] as const).map((m) => (
          <label key={m} className={`segmented-control__option${mode === m ? ' is-checked' : ''}`}>
            <input type="radio" name={name} value={m} checked={mode === m} onChange={() => onMode(m)} />
            {f(m)}
          </label>
        ))}
      </fieldset>
    </div>
  )
}

function CheckIcon() {
  return (
    <svg viewBox="0 0 24 24" width="0.75em" height="0.75em" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth={3} strokeLinecap="round" strokeLinejoin="round">
      <path d="m5 12.500 4.500 4.500L19 7.500" />
    </svg>
  )
}
