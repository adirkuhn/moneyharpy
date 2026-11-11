import { type FormEvent, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { type Contact, contactName, useContact } from '../contacts/queries'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { styleFileURL, useDocumentStyles, useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { useIdentities } from '../settings/queries'
import { ContactCombobox } from './ContactPicker'
import { closeOnEnter, InPlace } from './InPlace'
import { DateView, DetailsView, Paper, RecipientContent, SenderAddress, type ShownLine } from './InvoicePaper'
import { LayoutPicker } from './LayoutPicker'
import { type LineRow, LinesEditor } from './LinesEditor'
import { addDays, addressLines, daysBetween, documentDate, percentText, senderEntries, today } from './paper'
import { invoiceTitle, type SalesInvoice, type SalesInvoiceChanges, useNextDraftId, useSalesInvoice, useSaveSalesInvoice } from './queries'
import { computeTotals, parseCents, parsePercentage, parseQuantity } from './totals'
import { TotalsTable } from './TotalsTable'

/**
 * Creates an invoice (/sales_invoices/new, ?contact_id= to start with that
 * contact, as a contact's Add menu does) or edits one (/sales_invoices/:id/edit).
 */
export function InvoiceFormPage() {
  const { administrationId = '', invoiceId } = useParams()
  const [params] = useSearchParams()
  const invoice = useSalesInvoice(administrationId, invoiceId)
  const preset = useContact(administrationId, invoiceId ? undefined : params.get('contact_id') ?? undefined)
  const rates = useTaxRates(administrationId)
  // The document's language, layout and sender come from these, so it waits for them.
  const workflows = useWorkflows(administrationId)
  const styles = useDocumentStyles(administrationId)
  const identities = useIdentities(administrationId)
  if (invoiceId && invoice.error instanceof ApiError && invoice.error.status < 500) {
    return <NotFoundPage />
  }
  const settled = [workflows, styles, identities].every((q) => !q.isPending) && !(preset.fetchStatus === 'fetching' && !preset.data)
  if ((invoiceId && !invoice.data) || !rates.data || !settled) {
    return <Loading />
  }
  const inv = invoice.data ?? null
  // Rates the invoice already uses stay available even when they were turned off since.
  const usable = rates.data.filter((r) => r.tax_rate_type === 'sales_invoice' && (r.active || inv?.details?.some((d) => d.tax_rate_id === r.id)))
  return <InvoiceForm key={inv?.id ?? 'new'} administrationId={administrationId} invoice={inv} contact={preset.data} rates={usable} />
}

let nextKey = 0
const newKey = () => `new-${++nextKey}`
const blank = (taxRateId: string): LineRow => ({ key: newKey(), description: '', amount: '1', price: '', tax_rate_id: taxRateId, ledger_account_id: '' })
/** Shows "100.0" from the server as "100" and keeps "12.5" as it is. */
const priceText = (p: string | undefined) => (p === undefined || p === '' ? '' : String(Number(p)))
/** A line nobody typed in yet is not shown on the document, nor sent. */
const isBlank = (r: LineRow) => !r.id && !r.description.trim() && !r.price.trim()
const validDate = (v: string) => /^\d{4}-\d{2}-\d{2}$/.test(v)

type ChosenContact = { id: string; name: string; address: string[] }
const chosen = (c: Pick<Contact, 'id' | 'company_name' | 'firstname' | 'lastname' | 'address1' | 'address2' | 'zipcode' | 'city'>): ChosenContact =>
  ({ id: String(c.id ?? ''), name: contactName(c), address: addressLines(c) })

/**
 * The invoice editor, after Moneybird's "Add invoice": the invoice drawn as
 * its document, edited in place, with what the document does not show in the
 * settings underneath.
 */
type FormProps = { administrationId: string; invoice: SalesInvoice | null; contact?: Contact; rates: TaxRate[] }

function InvoiceForm({ administrationId, invoice, contact: preset, rates }: FormProps) {
  const nextDraftId = useNextDraftId(administrationId, !invoice)
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveSalesInvoice(administrationId, invoice?.id)
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const workflows = useWorkflows(administrationId)
  const styles = useDocumentStyles(administrationId)
  const identities = useIdentities(administrationId)
  const defaultRate = rates.find((r) => r.active)?.id ?? ''
  const layoutsId = useId()
  const settingsRef = useRef<HTMLDivElement>(null)

  const [contact, setContact] = useState<ChosenContact>(() => {
    const known = invoice?.contact ?? preset
    return known ? chosen(known) : { id: invoice?.contact_id ?? '', name: '', address: [] }
  })
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
  const [editingLines, setEditingLines] = useState(false)
  const [layoutsOpen, setLayoutsOpen] = useState(false)

  const invoiceFlows = (workflows.data ?? []).filter((w) => w.type === 'InvoiceWorkflow' && (w.active || w.id === workflowId))
  const flow = invoiceFlows.find((w) => w.id === workflowId) ?? invoiceFlows.find((w) => w.default)
  const allStyles = styles.data ?? []
  const style = allStyles.find((s) => s.id === styleId) ?? allStyles.find((s) => s.default) ?? allStyles[0]
  const allIdentities = identities.data ?? []
  const sender = allIdentities.find((i) => i.id === style?.identity_id) ?? allIdentities.find((i) => i.default) ?? allIdentities[0]
  const pricesInclTax = inclTax ?? flow?.prices_are_incl_tax ?? false
  const currency = invoice?.currency ?? flow?.currency ?? administration.data?.currency ?? 'EUR'
  // The document is written in the workflow's language, as Moneybird prints it.
  const language = flow?.language ?? invoice?.language ?? i18n.language
  const dt = i18n.getFixedT(language)
  const money = (cents: number) => formatAmount(cents / 100, currency, language)

  const parsed = rows.map((r) => ({ price: parseCents(r.price), quantity: parseQuantity(r.amount) }))
  const rateOf = (id: string) => rates.find((r) => r.id === id)
  const percentage = (id: string) => Math.round(Number(rateOf(id)?.percentage ?? 0) * 100)
  const discountValue = parsePercentage(discount)
  const totals = computeTotals(
    rows.map((r, i) => ({ price: parsed[i]?.price ?? 0, quantity: parsed[i]?.quantity ?? 0, taxRateId: r.tax_rate_id, rate: percentage(r.tax_rate_id) })),
    discountValue ?? 0,
    pricesInclTax,
  )

  const shownDate = invoiceDate || today()
  const dueTerm = term.trim() !== '' && Number.isFinite(Number(term)) ? Number(term) : (flow?.steps?.[0]?.due_interval ?? 0)
  const dueDate = addDays(shownDate, dueTerm)

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
  const linesInvalid = rows.some((_, i) => Object.keys(labels).some((k) => k.startsWith(`details_attributes.${i}.`) && errors.field(k)))
  const contactErrors = errors.field('contact_id')

  const change = (i: number, c: Partial<LineRow>) => setRows(rows.map((r, j) => (j === i ? { ...r, ...c } : r)))
  const remove = (i: number) => {
    const row = rows[i]
    if (row?.id) {
      setRemoved([...removed, row.id])
    }
    setRows(rows.filter((_, j) => j !== i))
  }
  const pickContact = (c: Contact) => {
    setContact(chosen(c))
    if (!invoice && !workflowId && c.invoice_workflow_id) {
      setWorkflowId(String(c.invoice_workflow_id))
    }
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const kept = rows.filter((r) => !isBlank(r))
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
  const title = invoice ? t('invoices.editTitle', { name: invoiceTitle(invoice, t) }) : t('invoices.newTitle')
  const draftId = invoice ? invoice.draft_id : nextDraftId.data
  const draftLabel = invoice?.invoice_id || (draftId ? dt('invoices.document.draftNumber', { id: draftId }) : dt('invoices.document.draft'))
  const rateName = (id: string) => rateOf(id)?.name ?? ''
  const totalRows = [
    { label: dt('invoices.document.subtotal'), amount: money(totals.subtotal) },
    ...(totals.discount !== 0 ? [{ label: dt('invoices.document.discount', { percentage: discount.trim() }), amount: money(-totals.discount) }] : []),
    ...totals.byRate
      .filter((r) => r.tax !== 0 || r.taxable !== 0)
      .map((r) => ({ label: pricesInclTax ? dt('invoices.document.includesVat', { name: rateName(r.taxRateId) }) : rateName(r.taxRateId), amount: money(r.tax) })),
    { label: dt('invoices.document.total'), amount: money(totals.inclTax), strong: true },
  ]
  const shownLines: ShownLine[] = rows.flatMap((r, i) =>
    isBlank(r)
      ? []
      : [{
          quantity: `${r.amount.trim() || '1'} x`, description: r.description, price: money(parsed[i]?.price ?? 0),
          total: money(totals.lines[i] ?? 0), vat: percentText(rateOf(r.tax_rate_id)?.percentage),
        }],
  )
  const zero = money(0)
  const emptyLine: ShownLine = { quantity: '1 x', description: t('invoices.editor.noDetails'), price: zero, total: zero, vat: percentText(rateOf(defaultRate)?.percentage) }
  const bank = sender?.bank_account_number
  const payBefore = bank && dt('invoices.document.payBefore', {
    amount: money(totals.inclTax), date: documentDate(dueDate, language), account: bank,
    holder: sender.bank_account_name || sender.company_name || '', reference: invoice?.payment_reference || dt('invoices.document.numberOnSend'),
  })
  const focusSettings = () => {
    settingsRef.current?.scrollIntoView({ block: 'start' })
    settingsRef.current?.querySelector<HTMLElement>('input, select')?.focus()
  }

  const actions = (
    <>
      <button type="button" className="btn btn--secondary" aria-expanded={layoutsOpen} aria-controls={layoutsId} onClick={() => setLayoutsOpen(!layoutsOpen)}>
        {t('invoices.editor.changeLayout')}
      </button>
      <MoreMenu items={[{ label: t('invoices.settingsTitle'), onSelect: focusSettings }, { label: t('common.cancel'), to: back }]} />
    </>
  )

  return (
    <Page title={title} back={invoice ? { to: back, label: t('invoices.back') } : undefined} actions={actions}>
      {layoutsOpen && (
        <LayoutPicker
          id={layoutsId}
          administrationId={administrationId}
          styles={allStyles}
          current={style?.id ?? ''}
          onPick={(id) => {
            setStyleId(id)
            setLayoutsOpen(false)
          }}
        />
      )}
      <form className="invoice-editor" onSubmit={submit} noValidate aria-label={title}>
        <Problems messages={errors.general} />
        <Paper label={t('invoices.editor.preview')} fontSize={style?.font_size}>
          <div className={`invoice-paper__header invoice-paper__header--${style?.address_position ?? 'left'}`}>
            <div className={`invoice-paper__logo invoice-paper__logo--${style?.logo_position ?? 'left'}`}>
              {style?.logo_hash && (
                <img
                  src={styleFileURL(administrationId, style.id ?? '', 'logo', style.logo_hash)}
                  alt={t('invoices.editor.logo')}
                  style={{ width: `${style.logo_display_width ?? 50}%` }}
                />
              )}
            </div>
            <div className="invoice-paper__recipient">
              <ContactCombobox
                administrationId={administrationId}
                label={t('invoices.contact')}
                className={`invoice-paper__recipient-button${contact.id ? '' : ' is-empty'}${contactErrors ? ' is-invalid' : ''}`}
                control={{
                  id: `${layoutsId}-contact`,
                  ...(contactErrors ? { 'aria-invalid': true as const, 'aria-describedby': `${layoutsId}-contact-message` } : {}),
                }}
                onChange={pickContact}
              >
                <RecipientContent chosen={!!contact.id} lines={contact.id ? [contact.name, ...contact.address] : []} />
              </ContactCombobox>
              {contactErrors && <p id={`${layoutsId}-contact-message`} className="form-input__message form-input__message--error">{contactErrors.join(' ')}</p>}
            </div>
            <SenderAddress dt={dt} entries={senderEntries(style?.invoice_sender_address, sender)} />
          </div>

          <div className="invoice-paper__heading">
            <div>
              <h2 className="invoice-paper__title">
                {dt('invoices.document.invoice')} <em className="invoice-paper__draft">{draftLabel}</em>
              </h2>
              <p className="invoice-paper__contact">
                <span>{dt('invoices.document.contact')}:</span> <span>{contact.name || t('invoices.editor.placeholderName')}</span>
              </p>
            </div>
            <div className="invoice-paper__dates">
              <InPlace
                errors={errors.field('invoice_date')}
                view={<DateView label={dt('invoices.document.invoiceDate')} value={documentDate(shownDate, language)} tentative={!invoiceDate} />}
                editor={(close) => (
                  <>
                    <input
                      type="date"
                      autoFocus
                      aria-label={t('invoices.invoiceDate')}
                      aria-describedby={invoice?.invoice_id ? undefined : `${layoutsId}-date-help`}
                      value={invoiceDate}
                      onChange={(e) => setInvoiceDate(e.target.value)}
                      onKeyDown={closeOnEnter(close)}
                    />
                    {!invoice?.invoice_id && <p id={`${layoutsId}-date-help`} className="form-input__message form-input__message--help">{t('invoices.dateHelp')}</p>}
                  </>
                )}
              />
              <InPlace
                errors={errors.field('first_due_interval')}
                view={<DateView label={dt('invoices.document.dueDate')} value={documentDate(dueDate, language)} />}
                editor={(close) => <DueDateInput label={t('invoices.dueDate')} value={dueDate} onChange={(v) => setTerm(String(Math.max(0, daysBetween(shownDate, v))))} onEnter={closeOnEnter(close)} />}
              />
            </div>
          </div>

          <div className="invoice-paper__details">
            {editingLines || linesInvalid ? (
              <div className="invoice-paper__lines-editor">
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
                {!linesInvalid && (
                  <button type="button" className="btn btn--secondary btn--small invoice-paper__done" onClick={() => setEditingLines(false)}>{t('invoices.editor.done')}</button>
                )}
              </div>
            ) : (
              <DetailsView dt={dt} lines={shownLines} empty={emptyLine} onEdit={() => setEditingLines(true)} />
            )}
            <TotalsTable rows={totalRows} />
          </div>

          <div className="invoice-paper__footer">
            <InPlace
              label={conditions ? t('invoices.editor.editConditions') : t('invoices.editor.addConditions')}
              errors={errors.field('payment_conditions')}
              view={conditions ? <span className="invoice-paper__conditions">{conditions}</span> : <span className="invoice-paper__placeholder">{t('invoices.editor.addConditions')}</span>}
              editor={() => <textarea rows={3} autoFocus maxLength={1000} aria-label={t('invoices.paymentConditions')} value={conditions} onChange={(e) => setConditions(e.target.value)} />}
            />
            {payBefore && (
              <p className="invoice-paper__pay">{payBefore}</p>
            )}
          </div>
        </Paper>

        <div className="form-sheet invoice-editor__settings" ref={settingsRef}>
          <FormSection title={t('invoices.settingsTitle')}>
            <TextInput label={labels.reference ?? ''} maxLength={1000} value={reference} errors={errors.field('reference')} onChange={(e) => setReference(e.target.value)} />
            <SelectInput label={labels.workflow_id ?? ''} width="medium" value={workflowId} errors={errors.field('workflow_id')} onChange={(e) => setWorkflowId(e.target.value)}>
              {!invoice && <option value="">{t('invoices.defaultWorkflow')}</option>}
              {invoiceFlows.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
            </SelectInput>
            <SelectInput label={labels.document_style_id ?? ''} width="medium" value={styleId} errors={errors.field('document_style_id')} onChange={(e) => setStyleId(e.target.value)}>
              {!invoice && <option value="">{t('invoices.defaultStyle')}</option>}
              {allStyles.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
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
          <div className="form-sheet__actions">
            <button className="btn btn--primary" disabled={save.isPending}>{t('invoices.save')}</button>
            <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
          </div>
        </div>
      </form>
    </Page>
  )
}

/** The due date, typed in place; it keeps what is typed until it is a whole date. */
function DueDateInput({ label, value, onChange, onEnter }: { label: string; value: string; onChange: (date: string) => void; onEnter: (e: React.KeyboardEvent) => void }) {
  const [text, setText] = useState(value)
  return (
    <input
      type="date"
      autoFocus
      aria-label={label}
      value={text}
      onChange={(e) => {
        setText(e.target.value)
        if (validDate(e.target.value)) {
          onChange(e.target.value)
        }
      }}
      onKeyDown={onEnter}
    />
  )
}
