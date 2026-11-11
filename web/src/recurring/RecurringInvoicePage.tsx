import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { contactName } from '../contacts/queries'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { ContactPicker } from '../invoices/ContactPicker'
import { type LineRow, LinesEditor } from '../invoices/LinesEditor'
import { invoiceTitle, type SalesInvoice, today, useSalesInvoice } from '../invoices/queries'
import { StateBadge } from '../invoices/StateBadge'
import { computeTotals, parseCents, parsePercentage, parseQuantity } from '../invoices/totals'
import { TotalsTable } from '../invoices/TotalsTable'
import { useDocumentStyles, useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import {
  frequencyText, frequencyTypes, type FrequencyType, type RecurringChanges, type RecurringInvoice, useDeactivateRecurringInvoice,
  useDeleteRecurringInvoice, useInvoicesOf, useRecurringInvoice, useSaveRecurringInvoice,
} from './queries'

/** What a new recurring invoice can be filled from: a sales invoice ("Make recurring"). */
type Template = Pick<SalesInvoice, 'contact_id' | 'contact' | 'reference' | 'workflow_id' | 'document_style_id' | 'prices_are_incl_tax' | 'discount'
  | 'payment_conditions' | 'first_due_interval' | 'details' | 'currency'>

/** One recurring invoice (/recurring_sales_invoices/:id), or a new one (/recurring_sales_invoices/new, ?from=a sales invoice to copy). */
export function RecurringInvoicePage() {
  const { administrationId = '', recurringId } = useParams()
  const [params] = useSearchParams()
  const from = recurringId ? undefined : params.get('from') ?? undefined
  const recurring = useRecurringInvoice(administrationId, recurringId)
  const source = useSalesInvoice(administrationId, from)
  const rates = useTaxRates(administrationId)
  if (recurringId && recurring.error instanceof ApiError && recurring.error.status < 500) {
    return <NotFoundPage />
  }
  if ((recurringId && !recurring.data) || (from && !source.data && !source.error) || !rates.data) {
    return <Loading />
  }
  const rec = recurring.data ?? null
  const usable = rates.data.filter((r) => r.tax_rate_type === 'sales_invoice' && (r.active || rec?.details?.some((d) => d.tax_rate_id === r.id)))
  if (!rec) {
    return <NewRecurringInvoice administrationId={administrationId} template={source.data ?? null} rates={usable} />
  }
  return <RecurringView key={rec.id} administrationId={administrationId} recurring={rec} rates={usable} />
}

function NewRecurringInvoice({ administrationId, template, rates }: { administrationId: string; template: Template | null; rates: TaxRate[] }) {
  const { t } = useTranslation()
  return (
    <Page title={t('recurring.newTitle')} back={{ to: `/${administrationId}/recurring_sales_invoices`, label: t('recurring.backToList') }}>
      <RecurringForm administrationId={administrationId} recurring={null} template={template} rates={rates} />
    </Page>
  )
}

function RecurringView({ administrationId, recurring: rec, rates }: { administrationId: string; recurring: RecurringInvoice; rates: TaxRate[] }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const deactivate = useDeactivateRecurringInvoice(administrationId, rec.id ?? '')
  const remove = useDeleteRecurringInvoice(administrationId, rec.id ?? '')
  const list = `/${administrationId}/recurring_sales_invoices`
  const made = rec.invoices_created ?? 0
  const date = (d: string | null | undefined) => d && new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })

  const actions = (
    <>
      {rec.active && (
        <button type="button" className="btn btn--secondary btn--small" disabled={deactivate.isPending} onClick={() => deactivate.mutate()}>
          {t('recurring.deactivate')}
        </button>
      )}
      {(made === 0 || rec.active) && (
        <ConfirmDelete
          label={t('invoices.delete')}
          confirmLabel={made > 0 ? t('recurring.confirmDeleteKept') : t('invoices.confirmDelete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => (made > 0 ? undefined : navigate(list)) })}
        />
      )}
    </>
  )
  const rows: [string, React.ReactNode][] = [
    [t('recurring.status'), rec.active ? t('recurring.states.active') : t('recurring.states.inactive')],
    [t('recurring.frequency'), frequencyText(rec, t)],
    [t('recurring.startDate'), date(rec.start_date)],
    [t('recurring.nextDate'), rec.active && date(rec.invoice_date)],
    [t('recurring.lastDate'), date(rec.last_date)],
    [t('recurring.autoSend'), rec.auto_send ? t('recurring.yes') : t('recurring.no')],
    [t('recurring.invoicesMade'), String(made)],
  ]

  return (
    <Page title={rec.contact ? contactName(rec.contact) : t('recurring.untitled')} subtitle={rec.reference ?? undefined} back={{ to: list, label: t('recurring.backToList') }} actions={actions}>
      <Problems messages={formErrors(deactivate.error ?? remove.error, {}).general} />
      {!rec.active && <p className="notice">{t('recurring.inactiveNotice')}</p>}
      <dl className="details">
        {rows.filter(([, v]) => v).map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      {/* A fresh form for every version, so saved lines carry their ids. */}
      <RecurringForm key={rec.updated_at} administrationId={administrationId} recurring={rec} template={rec} rates={rates} />
      <InvoicesMade administrationId={administrationId} recurring={rec} />
    </Page>
  )
}

function InvoicesMade({ administrationId, recurring: rec }: { administrationId: string; recurring: RecurringInvoice }) {
  const { t, i18n } = useTranslation()
  const invoices = useInvoicesOf(administrationId, rec.id)
  const date = (d: string | null | undefined) => (d ? new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' }) : '')
  return (
    <section className="invoice-view" aria-labelledby="recurring-made">
      <h2 id="recurring-made">{t('recurring.madeTitle')}</h2>
      {!invoices.data ? (
        <Loading />
      ) : invoices.data.length === 0 ? (
        <p className="empty">{t('recurring.noneMade')}</p>
      ) : (
        <table className="data-table invoices" aria-label={t('recurring.madeTitle')}>
          <thead>
            <tr>
              <th scope="col" className="invoices__date">{t('invoices.invoiceDate')}</th>
              <th scope="col" className="invoices__number">{t('invoices.number')}</th>
              <th scope="col" className="invoices__amount">{t('invoices.amount')}</th>
              <th scope="col" className="invoices__state">{t('invoices.status')}</th>
            </tr>
          </thead>
          <tbody>
            {invoices.data.map((inv) => (
              <tr key={inv.id}>
                <td className="invoices__date">{date(inv.invoice_date)}</td>
                <td className="invoices__number"><Link to={`/${administrationId}/sales_invoices/${inv.id}`}>{invoiceTitle(inv, t)}</Link></td>
                <td className="invoices__amount">{formatAmount(inv.total_price_incl_tax, inv.currency ?? 'EUR', i18n.language)}</td>
                <td className="invoices__state"><StateBadge state={inv.state} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  )
}

let nextKey = 0
const newKey = () => `new-${++nextKey}`
const blank = (taxRateId: string): LineRow => ({ key: newKey(), description: '', amount: '1', price: '', tax_rate_id: taxRateId, ledger_account_id: '' })
/** Shows "100.0" from the server as "100" and keeps "12.5" as it is. */
const priceText = (p: string | undefined) => (p === undefined || p === '' ? '' : String(Number(p)))

type FormProps = { administrationId: string; recurring: RecurringInvoice | null; template: Template | null; rates: TaxRate[] }

/** The customer, schedule and lines of a recurring invoice; a deactivated one shows them without changing. */
function RecurringForm({ administrationId, recurring, template, rates }: FormProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveRecurringInvoice(administrationId, recurring?.id)
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const workflows = useWorkflows(administrationId)
  const styles = useDocumentStyles(administrationId)
  const defaultRate = rates.find((r) => r.active)?.id ?? ''
  const locked = recurring !== null && !recurring.active

  const [contact, setContact] = useState({ id: template?.contact_id ?? '', name: template?.contact ? contactName(template.contact) : '' })
  const [reference, setReference] = useState(template?.reference ?? '')
  const [invoiceDate, setInvoiceDate] = useState(recurring?.invoice_date ?? today())
  const [frequency, setFrequency] = useState(String(recurring?.frequency ?? 1))
  const [frequencyType, setFrequencyType] = useState<FrequencyType>(recurring?.frequency_type ?? 'month')
  const [lastDate, setLastDate] = useState(recurring?.last_date ?? '')
  const [autoSend, setAutoSend] = useState(recurring?.auto_send ?? false)
  const [term, setTerm] = useState(template?.first_due_interval === undefined ? '' : String(template.first_due_interval))
  const [workflowId, setWorkflowId] = useState(template?.workflow_id ?? '')
  const [styleId, setStyleId] = useState(template?.document_style_id ?? '')
  const [inclTax, setInclTax] = useState<boolean | null>(template?.prices_are_incl_tax ?? null)
  const [discount, setDiscount] = useState(template?.discount && Number(template.discount) !== 0 ? String(Number(template.discount)) : '')
  const [conditions, setConditions] = useState(template?.payment_conditions ?? '')
  // Lines copied from a sales invoice are new lines of the recurring invoice.
  const [rows, setRows] = useState<LineRow[]>(() =>
    template?.details?.length
      ? template.details.map((d) => ({
          key: d.id ?? newKey(), id: recurring ? d.id : undefined, description: d.description ?? '', amount: d.amount ?? '1', price: priceText(d.price),
          tax_rate_id: d.tax_rate_id ?? defaultRate, ledger_account_id: d.ledger_account_id ?? '',
        }))
      : [blank(defaultRate)],
  )
  const [removed, setRemoved] = useState<string[]>([])

  const invoiceFlows = (workflows.data ?? []).filter((w) => w.type === 'InvoiceWorkflow' && (w.active || w.id === workflowId))
  const flow = invoiceFlows.find((w) => w.id === workflowId) ?? invoiceFlows.find((w) => w.default)
  const pricesInclTax = inclTax ?? flow?.prices_are_incl_tax ?? false
  const currency = template?.currency ?? flow?.currency ?? administration.data?.currency ?? 'EUR'
  const money = (cents: number) => formatAmount(cents / 100, currency, i18n.language)
  const parsed = rows.map((r) => ({ price: parseCents(r.price), quantity: parseQuantity(r.amount) }))
  const percentage = (id: string) => Math.round(Number(rates.find((r) => r.id === id)?.percentage ?? 0) * 100)
  const discountValue = parsePercentage(discount)
  const totals = computeTotals(
    rows.map((r, i) => ({ price: parsed[i]?.price ?? 0, quantity: parsed[i]?.quantity ?? 0, taxRateId: r.tax_rate_id, rate: percentage(r.tax_rate_id) })),
    discountValue ?? 0,
    pricesInclTax,
  )
  const rateName = (id: string) => rates.find((r) => r.id === id)?.name ?? ''

  const labels: Record<string, string> = {
    contact_id: t('invoices.contact'), reference: t('invoices.reference'), invoice_date: recurring ? t('recurring.nextDate') : t('recurring.firstDate'),
    frequency: t('recurring.interval'), frequency_type: t('recurring.unit'), last_date: t('recurring.lastDate'), auto_send: t('recurring.autoSendLabel'),
    first_due_interval: t('invoices.term'), workflow_id: t('invoices.workflow'), document_style_id: t('invoices.documentStyle'), discount: t('invoices.discount'),
    payment_conditions: t('invoices.paymentConditions'), prices_are_incl_tax: t('invoices.pricesInclTax'), details_attributes: t('invoices.lines'),
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
    const body: RecurringChanges = {
      contact_id: contact.id,
      reference: reference.trim(),
      // The schedule restarts from a new date only.
      ...(invoiceDate !== recurring?.invoice_date ? { invoice_date: invoiceDate } : {}),
      frequency: Number(frequency) || frequency,
      frequency_type: frequencyType,
      last_date: lastDate,
      auto_send: autoSend,
      prices_are_incl_tax: pricesInclTax,
      discount: discount.trim(),
      payment_conditions: conditions.trim(),
      ...(term.trim() !== '' ? { first_due_interval: Number(term) } : {}),
      ...(workflowId ? { workflow_id: workflowId } : {}),
      ...(styleId ? { document_style_id: styleId } : {}),
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
        if (!recurring) {
          navigate(`/${administrationId}/recurring_sales_invoices/${saved.id}`)
        }
      },
    })
  }

  const totalRows = [
    { label: t('invoices.subtotal'), amount: money(totals.subtotal) },
    ...(totals.discount !== 0 ? [{ label: t('invoices.discountAmount', { percentage: discount.trim() }), amount: money(-totals.discount) }] : []),
    ...totals.byRate.map((r) => ({ label: pricesInclTax ? t('invoices.includesVat', { name: rateName(r.taxRateId) }) : rateName(r.taxRateId), amount: money(r.tax) })),
    { label: t('invoices.total'), amount: money(totals.inclTax), strong: true },
  ]

  return (
    <form className="form-sheet invoice-form" onSubmit={submit} noValidate aria-label={t('recurring.form')}>
      <fieldset className="form-sheet__fieldset" disabled={locked}>
        <Problems messages={[...errors.general, ...(errors.field('details_attributes') ?? [])]} />
        <FormSection>
          <ContactPicker
            administrationId={administrationId}
            label={labels.contact_id ?? ''}
            value={contact.name}
            errors={errors.field('contact_id')}
            onChange={(c) => {
              setContact({ id: String(c.id ?? ''), name: contactName(c) })
              if (!recurring && !workflowId && c.invoice_workflow_id) {
                setWorkflowId(String(c.invoice_workflow_id))
              }
            }}
          />
          <TextInput label={labels.reference ?? ''} maxLength={1000} help={t('recurring.referenceHelp')} value={reference} errors={errors.field('reference')} onChange={(e) => setReference(e.target.value)} />
        </FormSection>
        <FormSection title={t('recurring.schedule')}>
          <TextInput label={labels.invoice_date ?? ''} type="date" width="medium" required min={today()} help={recurring ? t('recurring.nextDateHelp') : undefined} value={invoiceDate} errors={errors.field('invoice_date')} onChange={(e) => setInvoiceDate(e.target.value)} />
          <TextInput label={labels.frequency ?? ''} type="number" inputMode="numeric" min={1} max={1000} width="small" value={frequency} errors={errors.field('frequency')} onChange={(e) => setFrequency(e.target.value)} />
          <SelectInput label={labels.frequency_type ?? ''} width="medium" value={frequencyType} errors={errors.field('frequency_type')} onChange={(e) => setFrequencyType(e.target.value as FrequencyType)}>
            {frequencyTypes.map((f) => <option key={f} value={f}>{t(`recurring.units.${f}`)}</option>)}
          </SelectInput>
          <TextInput label={labels.last_date ?? ''} type="date" width="medium" help={t('recurring.lastDateHelp')} value={lastDate} errors={errors.field('last_date')} onChange={(e) => setLastDate(e.target.value)} />
          <CheckboxInput label={labels.auto_send ?? ''} help={t('recurring.autoSendHelp')} checked={autoSend} onChange={(e) => setAutoSend(e.target.checked)} />
        </FormSection>
        <FormSection title={t('recurring.invoiceSettings')}>
          <SelectInput label={labels.workflow_id ?? ''} width="medium" value={workflowId} errors={errors.field('workflow_id')} onChange={(e) => setWorkflowId(e.target.value)}>
            {!recurring && <option value="">{t('invoices.defaultWorkflow')}</option>}
            {invoiceFlows.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
          </SelectInput>
          <SelectInput label={labels.document_style_id ?? ''} width="medium" value={styleId} errors={errors.field('document_style_id')} onChange={(e) => setStyleId(e.target.value)}>
            {!recurring && <option value="">{t('invoices.defaultStyle')}</option>}
            {(styles.data ?? []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </SelectInput>
          <TextInput
            label={labels.first_due_interval ?? ''}
            type="number"
            inputMode="numeric"
            min={0}
            max={365}
            width="small"
            placeholder={String(flow?.steps?.[0]?.due_interval ?? '')}
            help={t('invoices.termHelp')}
            value={term}
            errors={errors.field('first_due_interval')}
            onChange={(e) => setTerm(e.target.value)}
          />
          <CheckboxInput label={labels.prices_are_incl_tax ?? ''} checked={pricesInclTax} onChange={(e) => setInclTax(e.target.checked)} />
          <TextInput
            label={labels.discount ?? ''}
            width="small"
            inputMode="decimal"
            maxLength={10}
            help={t('invoices.discountHelp')}
            value={discount}
            aria-invalid={discountValue === null || undefined}
            errors={errors.field('discount')}
            onChange={(e) => setDiscount(e.target.value)}
          />
          <TextArea label={labels.payment_conditions ?? ''} maxLength={1000} value={conditions} errors={errors.field('payment_conditions')} onChange={(e) => setConditions(e.target.value)} />
        </FormSection>
        <p className="hint">{t('recurring.periodHelp')}</p>
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
        {!locked && (
          <div className="form-sheet__actions">
            <button className="btn btn--primary" disabled={save.isPending}>{t('recurring.save')}</button>
            {!recurring && <Link className="btn btn--secondary" to={`/${administrationId}/recurring_sales_invoices`}>{t('common.cancel')}</Link>}
          </div>
        )}
      </fieldset>
    </form>
  )
}
