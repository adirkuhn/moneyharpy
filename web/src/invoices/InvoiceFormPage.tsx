import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { contactName } from '../contacts/queries'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { useDocumentStyles, useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { ContactPicker } from './ContactPicker'
import { type LineRow, LinesEditor } from './LinesEditor'
import { invoiceTitle, type SalesInvoice, type SalesInvoiceChanges, useSalesInvoice, useSaveSalesInvoice } from './queries'
import { computeTotals, parseCents, parsePercentage, parseQuantity } from './totals'
import { TotalsTable } from './TotalsTable'

/** Creates an invoice (/sales_invoices/new) or edits one (/sales_invoices/:id/edit). */
export function InvoiceFormPage() {
  const { administrationId = '', invoiceId } = useParams()
  const invoice = useSalesInvoice(administrationId, invoiceId)
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
  return <InvoiceForm key={inv?.id ?? 'new'} administrationId={administrationId} invoice={inv} rates={usable} />
}

let nextKey = 0
const newKey = () => `new-${++nextKey}`
const blank = (taxRateId: string): LineRow => ({ key: newKey(), description: '', amount: '1', price: '', tax_rate_id: taxRateId, ledger_account_id: '' })
/** Shows "100.0" from the server as "100" and keeps "12.5" as it is. */
const priceText = (p: string | undefined) => (p === undefined || p === '' ? '' : String(Number(p)))

function InvoiceForm({ administrationId, invoice, rates }: { administrationId: string; invoice: SalesInvoice | null; rates: TaxRate[] }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveSalesInvoice(administrationId, invoice?.id)
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const workflows = useWorkflows(administrationId)
  const styles = useDocumentStyles(administrationId)
  const defaultRate = rates.find((r) => r.active)?.id ?? ''

  const [contact, setContact] = useState({ id: invoice?.contact_id ?? '', name: invoice?.contact ? contactName(invoice.contact) : '' })
  const [reference, setReference] = useState(invoice?.reference ?? '')
  const [invoiceDate, setInvoiceDate] = useState(invoice?.invoice_date ?? '')
  const [term, setTerm] = useState(invoice?.first_due_interval === undefined ? '' : String(invoice.first_due_interval))
  const [workflowId, setWorkflowId] = useState(invoice?.workflow_id ?? '')
  const [styleId, setStyleId] = useState(invoice?.document_style_id ?? '')
  const [inclTax, setInclTax] = useState<boolean | null>(invoice?.prices_are_incl_tax ?? null)
  const [discount, setDiscount] = useState(invoice?.discount ? String(Number(invoice.discount)) : '')
  const [conditions, setConditions] = useState(invoice?.payment_conditions ?? '')
  const [rows, setRows] = useState<LineRow[]>(() =>
    invoice?.details?.length
      ? invoice.details.map((d) => ({
          key: d.id ?? newKey(), id: d.id, description: d.description ?? '', amount: d.amount ?? '1', price: priceText(d.price),
          tax_rate_id: d.tax_rate_id ?? defaultRate, ledger_account_id: d.ledger_account_id ?? '',
        }))
      : [blank(defaultRate)],
  )
  const [removed, setRemoved] = useState<string[]>([])

  const invoiceFlows = (workflows.data ?? []).filter((w) => w.type === 'InvoiceWorkflow' && (w.active || w.id === workflowId))
  const flow = invoiceFlows.find((w) => w.id === workflowId) ?? invoiceFlows.find((w) => w.default)
  const pricesInclTax = inclTax ?? flow?.prices_are_incl_tax ?? false
  const currency = invoice?.currency ?? flow?.currency ?? administration.data?.currency ?? 'EUR'
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
    contact_id: t('invoices.contact'), reference: t('invoices.reference'), invoice_date: t('invoices.invoiceDate'), first_due_interval: t('invoices.term'),
    workflow_id: t('invoices.workflow'), document_style_id: t('invoices.documentStyle'), discount: t('invoices.discount'), payment_conditions: t('invoices.paymentConditions'),
    prices_are_incl_tax: t('invoices.pricesInclTax'),
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
    const body: SalesInvoiceChanges = {
      contact_id: contact.id,
      reference: reference.trim(),
      invoice_date: invoiceDate,
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
    save.mutate(body, { onSuccess: (saved) => navigate(`/${administrationId}/sales_invoices/${saved.id}`) })
  }

  const list = `/${administrationId}/sales_invoices`
  const back = invoice ? `${list}/${invoice.id}` : list
  const totalRows = [
    { label: t('invoices.subtotal'), amount: money(totals.subtotal) },
    ...(totals.discount !== 0 ? [{ label: t('invoices.discountAmount', { percentage: discount.trim() }), amount: money(-totals.discount) }] : []),
    ...totals.byRate.map((r) => ({ label: pricesInclTax ? t('invoices.includesVat', { name: rateName(r.taxRateId) }) : rateName(r.taxRateId), amount: money(r.tax) })),
    { label: t('invoices.total'), amount: money(totals.inclTax), strong: true },
  ]

  return (
    <Page title={invoice ? t('invoices.editTitle', { name: invoiceTitle(invoice, t) }) : t('invoices.newTitle')} back={{ to: back, label: t('invoices.back') }}>
      <form className="form-sheet invoice-form" onSubmit={submit} noValidate aria-label={invoice ? t('invoices.editTitle', { name: invoiceTitle(invoice, t) }) : t('invoices.newTitle')}>
        <Problems messages={errors.general} />
        <FormSection>
          <ContactPicker
            administrationId={administrationId}
            label={labels.contact_id ?? ''}
            value={contact.name}
            errors={errors.field('contact_id')}
            onChange={(c) => {
              setContact({ id: String(c.id ?? ''), name: contactName(c) })
              if (!invoice && !workflowId && c.invoice_workflow_id) {
                setWorkflowId(String(c.invoice_workflow_id))
              }
            }}
          />
          <TextInput label={labels.reference ?? ''} maxLength={1000} value={reference} errors={errors.field('reference')} onChange={(e) => setReference(e.target.value)} />
          <TextInput label={labels.invoice_date ?? ''} type="date" width="medium" value={invoiceDate} help={invoice?.invoice_id ? undefined : t('invoices.dateHelp')} errors={errors.field('invoice_date')} onChange={(e) => setInvoiceDate(e.target.value)} />
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
        </FormSection>
        <FormSection title={t('invoices.settingsTitle')}>
          <SelectInput label={labels.workflow_id ?? ''} width="medium" value={workflowId} errors={errors.field('workflow_id')} onChange={(e) => setWorkflowId(e.target.value)}>
            {!invoice && <option value="">{t('invoices.defaultWorkflow')}</option>}
            {invoiceFlows.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
          </SelectInput>
          <SelectInput label={labels.document_style_id ?? ''} width="medium" value={styleId} errors={errors.field('document_style_id')} onChange={(e) => setStyleId(e.target.value)}>
            {!invoice && <option value="">{t('invoices.defaultStyle')}</option>}
            {(styles.data ?? []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </SelectInput>
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
        <FormSection title={t('invoices.lines')}>
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
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('invoices.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
