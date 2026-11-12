import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { type Contact, contactName, useContact } from '../contacts/queries'
import { formErrors } from '../forms/Form'
import { styleFileURL, useDocumentStyles, useInvoiceSettings, useWorkflows } from '../invoicing/queries'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { useCustomFields, useIdentities } from '../settings/queries'
import { AddInvoiceDialog, type ChosenContact, type InvoiceSetup } from './AddInvoiceDialog'
import { applyBelow, moveRow, quantityLabel } from './details'
import { type DetailRow, DetailsEditor } from './DetailsEditor'
import { detailField, detailOf, type NewEntity, type NewEntityKey } from './newEntity'
import { conditionTagValues } from './conditions'
import { PaymentConditions } from './PaymentConditions'
import { DateView, DetailsView, Paper, RecipientContent, SenderAddress, type ShownLine } from './InvoicePaper'
import { ChangeLayoutPanel, LayoutPicker } from './LayoutPicker'
import { addDays, addressLines, documentDate, percentText, senderEntries, today, withWebsite } from './paper'
import { invoiceTitle, type SalesInvoice, type SalesInvoiceChanges, useNextDraftId, useSaveSalesInvoice } from './queries'
import { type RecurringChanges, type RecurringInvoice, useSaveRecurringInvoice } from '../recurring/queries'
import { computeTotals, parseCents, parsePercentage, parseQuantity } from './totals'
import { type TotalRow, TotalsTable } from './TotalsTable'
import './editor.css'

/**
 * What a page hands to the editor: open the details or the popup, as the
 * invoice's page does from the blocks of its document, and keep a number
 * filled in by hand after a save.
 */
/**
 * What the editor opens with: its details or popup, a sequence number chosen,
 * and a category or VAT rate added for one of its rows.
 */
export type Opening = { details?: boolean; dialog?: boolean; conditions?: boolean; sequenceId?: string; newEntity?: NewEntity } | null

/**
 * Creates an invoice (/sales_invoices/new, ?contact_id= to start with that
 * contact, as a contact's Add menu and the Add contact page do). A saved
 * invoice is edited on its own page, as on Moneybird.
 */
export function InvoiceFormPage() {
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const preset = useContact(administrationId, params.get('contact_id') ?? undefined)
  if (preset.fetchStatus === 'fetching' && !preset.data) {
    return <Loading />
  }
  return (
    <InvoiceEditor
      mode="new"
      administrationId={administrationId}
      invoice={null}
      contact={preset.data}
      opening={location.state as Opening}
      // The new draft opens on its own page, with what the popup asked for.
      onSaved={(saved, opening) => navigate(`/${administrationId}/sales_invoices/${saved.id}`, { state: opening })}
    />
  )
}

/** The old address of the editor (/sales_invoices/:id/edit), which is now the invoice's own page. */
export function InvoiceEditRedirect() {
  const location = useLocation()
  return <Navigate to={location.pathname.replace(/\/edit\/?$/, '')} replace state={location.state} />
}

type ViewMode = {
  /** In the invoice's own page: the document is edited in place, and saved when something changed. */
  mode: 'view'
  /** Whether the layouts show, as Extra's Change layout asks. */
  layoutsOpen: boolean
  onLayoutsClose: () => void
  /** Called when a part of the document closes without saving, so a sent invoice can show its document again. */
  onCancel: () => void
  /** The category the sidebar points at, whose lines stand out. */
  highlight?: string
}

/** What the editor and a recurring invoice's document have in common. */
type EditedFields = 'id' | 'version' | 'contact' | 'contact_id' | 'contact_person' | 'reference' | 'workflow_id' | 'first_due_interval' | 'document_style_id'
  | 'prices_are_incl_tax' | 'currency' | 'custom_fields' | 'discount' | 'payment_conditions' | 'details' | 'language'
export type EditedInvoice = Pick<SalesInvoice, EditedFields>

/**
 * A recurring invoice is edited with the same editor, as Moneybird draws it
 * as the invoice it makes: its schedule is set above the document, and the
 * document has no date or number of its own to set.
 */
export type RecurringEditing = {
  /** The saved recurring invoice; null while it is being added. */
  invoice: RecurringInvoice | null
  /** For a new one: the schedule its Continue chose, sent along when it is made. */
  schedule?: RecurringChanges
}

type EditorProps = {
  administrationId: string
  invoice: SalesInvoice | null
  /** A recurring invoice instead of a sales invoice. */
  recurring?: RecurringEditing
  /** What a new invoice starts from, such as the invoice "Duplicate to recurring invoice" copies. */
  template?: EditedInvoice | null
  contact?: Contact
  opening: Opening
  /** After a save: the saved invoice, and what the editor should open again. */
  onSaved: (saved: { id?: string }, opening: Opening) => void
} & ({ mode: 'new' } | ViewMode)

/** The invoice the editor changes: a sales invoice or a recurring one. */
const editedOf = (props: Pick<EditorProps, 'invoice' | 'recurring'>): EditedInvoice | null => (props.recurring ? props.recurring.invoice : props.invoice)

/**
 * The invoice editor once what it draws with is loaded: the rates, the
 * workflows, the layouts and the senders.
 */
export function InvoiceEditor(props: EditorProps) {
  const { administrationId } = props
  const inv = editedOf(props)
  const rates = useTaxRates(administrationId)
  // The document's language, layout and sender come from these, so it waits for them.
  const workflows = useWorkflows(administrationId)
  const styles = useDocumentStyles(administrationId)
  const identities = useIdentities(administrationId)
  if (!rates.data || [workflows, styles, identities].some((q) => q.isPending)) {
    return <Loading />
  }
  // Rates the invoice already uses stay available even when they were turned off since.
  const usable = rates.data.filter((r) => r.tax_rate_type === 'sales_invoice' && (r.active || inv?.details?.some((d) => d.tax_rate_id === r.id)))
  return <KeyedForm {...props} rates={usable} />
}

/**
 * A saved invoice starts the editor again from what the server keeps, with
 * the ids of new lines. What to open (details, the popup…) belongs to the
 * editor it came with: a new version of the same invoice starts closed,
 * unless something new is asked to open.
 */
function KeyedForm(props: EditorProps & { rates: TaxRate[] }) {
  const { opening } = props
  const inv = editedOf(props)
  const key = inv ? `${inv.id}-${inv.version ?? ''}` : 'new'
  const opened = useRef({ opening, key })
  if (opened.current.opening !== opening) {
    opened.current = { opening, key }
  }
  return <InvoiceForm key={key} {...props} opening={opened.current.key === key ? opening : null} />
}

let nextKey = 0
const newKey = () => `new-${++nextKey}`
const blank = (taxRateId: string, over: Partial<DetailRow> = {}): DetailRow => ({
  key: newKey(), description: '', amount: '1 x', price: '', tax_rate_id: taxRateId, ledger_account_id: '', period: '', product_id: '', ...over,
})
/** Shows "100.0" from the server as "100" and keeps "12.5" as it is. */
const priceText = (p: string | undefined) => (p === undefined || p === '' ? '' : String(Number(p)))
/** A detail nobody typed in yet is not shown on the document, nor sent. */
const isBlank = (r: DetailRow) => !r.id && !r.description.trim() && !r.price.trim()
/** A detail's amount times its price, in cents, before any discount over the total, as the editor's Total column shows it. */
const grossCents = (price: number, quantity: number) => Math.sign(price * quantity) * Math.round(Math.abs(price * quantity) / 1000)

const chosen = (c: Pick<Contact, 'id' | 'company_name' | 'firstname' | 'lastname' | 'address1' | 'address2' | 'zipcode' | 'city'>): ChosenContact =>
  ({ id: String(c.id ?? ''), name: contactName(c), address: addressLines(c) })

type FormProps = EditorProps & { rates: TaxRate[] }

/**
 * The invoice editor, after Moneybird's "Add invoice": the invoice drawn as
 * its document, edited in place, with the "Add invoice" popup for the
 * contact and settings, and the details editor for its lines.
 */
function InvoiceForm(props: FormProps) {
  const { administrationId, invoice, recurring, contact: preset, rates, opening, onSaved } = props
  const view = props.mode === 'view' ? props : null
  // The invoice being edited, sales or recurring; a new one starts from its template.
  const doc = editedOf(props)
  const init = doc ?? props.template ?? null
  const nextDraftId = useNextDraftId(administrationId, !doc)
  const { t, i18n } = useTranslation()
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const [ownLayoutsOpen, setOwnLayoutsOpen] = useState(false)
  const layoutsOpen = view ? view.layoutsOpen : ownLayoutsOpen
  const closeLayouts = view ? view.onLayoutsClose : () => setOwnLayoutsOpen(false)
  const saveSales = useSaveSalesInvoice(administrationId, recurring ? undefined : invoice?.id)
  const saveRecurring = useSaveRecurringInvoice(administrationId, recurring?.invoice?.id)
  const mutation = recurring ? saveRecurring : saveSales
  /** Saves the sales invoice, or the recurring one without what only an invoice has: its date and number belong to the invoices it makes. */
  const save = {
    isPending: mutation.isPending,
    error: mutation.error,
    reset: mutation.reset,
    mutate: (b: SalesInvoiceChanges, o: { onSuccess: (s: { id?: string }) => void }) => {
      if (!recurring) {
        saveSales.mutate(b, o)
        return
      }
      const { invoice_date: _date, invoice_sequence_id: _number, ...rest } = b
      saveRecurring.mutate({ ...rest, ...(recurring.invoice ? {} : recurring.schedule) } as RecurringChanges, o)
    },
  }
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const workflows = useWorkflows(administrationId)
  const styles = useDocumentStyles(administrationId)
  const identities = useIdentities(administrationId)
  const numbering = useInvoiceSettings(administrationId)
  const fields = useCustomFields(administrationId)
  const defaultRate = rates.find((r) => r.active)?.id ?? ''
  const layoutsId = useId()
  const paperRef = useRef<HTMLDivElement>(null)

  const [setup, setSetup] = useState<InvoiceSetup>(() => {
    const known = preset ?? init?.contact
    return {
      contact: known ? chosen({ ...known, id: known.id ?? init?.contact_id }) : { id: init?.contact_id ?? '', name: '', address: [] },
      reference: init?.reference ?? '',
      invoiceDate: invoice?.invoice_date ?? '',
      dateUnlocked: !!invoice?.invoice_date,
      sequenceId: opening?.sequenceId ?? '',
      numberUnlocked: !!opening?.sequenceId,
      workflowId: init?.workflow_id ?? '',
      term: init?.first_due_interval === undefined ? '' : String(init.first_due_interval),
      styleId: init?.document_style_id ?? '',
      inclTax: init?.prices_are_incl_tax ?? null,
      currency: init?.currency ?? null,
      custom: Object.fromEntries((init?.custom_fields ?? []).map((f) => [String(f.id ?? ''), f.value ?? ''])),
    }
  })
  const [discount, setDiscount] = useState(Number(init?.discount ?? 0) ? String(Number(init?.discount)) : '')
  const [conditions, setConditions] = useState(init?.payment_conditions ?? '')
  // A new invoice takes its workflow's payment conditions until they are edited, as the server does; a copy keeps its own.
  const [conditionsTouched, setConditionsTouched] = useState(!doc && !!props.template?.payment_conditions)
  const [editingConditions, setEditingConditions] = useState(!!opening?.conditions)
  const [conditionsBefore, setConditionsBefore] = useState(conditions)
  const [rows, setRows] = useState<DetailRow[]>(() => {
    const loaded = init?.details?.length
      ? [...init.details]
          .sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
          .map((d) => ({
            // The lines of a template are new lines of this invoice.
            key: (doc && d.id) || newKey(), id: doc ? d.id : undefined, description: d.description ?? '', amount: d.amount ?? '1', price: priceText(d.price),
            tax_rate_id: d.tax_rate_id ?? defaultRate, ledger_account_id: d.ledger_account_id ?? '', period: d.period ?? '', savedPeriod: d.period ?? '',
            product_id: d.product_id ? String(d.product_id) : '',
          }))
      : [blank(defaultRate)]
    // Back from adding a category or VAT rate for a row, it is chosen there, as Moneybird's new_entity_field does.
    const entity = opening?.newEntity
    const added = entity ? detailOf(entity.field) : null
    return entity && added ? loaded.map((r, i) => (i === added.index ? { ...r, [added.key]: entity.id } : r)) : loaded
  })
  const [removed, setRemoved] = useState<string[]>([])
  const [editingLines, setEditingLines] = useState(!!opening?.details)
  // The details as they were when the editor opened, for its Cancel, with the prices and currency set above them.
  const [beforeEditing, setBeforeEditing] = useState(() => ({ rows, discount, removed, inclTax: setup.inclTax, currency: setup.currency }))
  const editLines = () => {
    setBeforeEditing({ rows, discount, removed, inclTax: setup.inclTax, currency: setup.currency })
    setEditingLines(true)
  }
  const cancelLines = () => {
    setRows(beforeEditing.rows)
    setDiscount(beforeEditing.discount)
    setRemoved(beforeEditing.removed)
    change({ inclTax: beforeEditing.inclTax, currency: beforeEditing.currency })
    setEditingLines(false)
    view?.onCancel()
  }
  // A new detail takes the VAT rate and category of the one above it.
  const addRow = (over?: Partial<DetailRow>) => {
    const above = rows.at(-1)
    setRows([...rows, blank(above?.tax_rate_id || defaultRate, { ledger_account_id: above?.ledger_account_id ?? '', ...over })])
  }
  // A new invoice starts in the popup, as on Moneybird; a new recurring one after its schedule's Continue.
  const [dialogOpen, setDialogOpen] = useState(opening?.dialog ?? (!doc && !recurring))
  // The dates open the popup with its settings shown, as the heading of Moneybird's document does.
  const [dialogAdvanced, setDialogAdvanced] = useState(false)
  const before = useRef<InvoiceSetup>(setup)
  // Another workflow brings its own currency and price mode, as the server takes them from it.
  const change = (c: Partial<InvoiceSetup>) =>
    setSetup((s) => ({ ...s, ...(c.workflowId !== undefined && c.workflowId !== s.workflowId ? { inclTax: null, currency: null } : {}), ...c }))

  const invoiceFlows = (workflows.data ?? []).filter((w) => w.type === 'InvoiceWorkflow' && (w.active || w.id === setup.workflowId))
  const flow = invoiceFlows.find((w) => w.id === setup.workflowId) ?? invoiceFlows.find((w) => w.default)
  const allStyles = styles.data ?? []
  const style = allStyles.find((s) => s.id === setup.styleId) ?? allStyles.find((s) => s.default) ?? allStyles[0]
  const allIdentities = identities.data ?? []
  const sender = withWebsite(allIdentities.find((i) => i.id === style?.identity_id) ?? allIdentities.find((i) => i.default) ?? allIdentities[0], administration.data)
  const shownConditions = doc || conditionsTouched ? conditions : (flow?.payment_conditions ?? '')
  const pricesInclTax = setup.inclTax ?? flow?.prices_are_incl_tax ?? false
  const currency = setup.currency ?? flow?.currency ?? administration.data?.currency ?? 'EUR'
  const customFields = (fields.data ?? []).filter((f) => f.source === 'sales_invoice')
  // The document is written in the workflow's language, as Moneybird prints it.
  const language = flow?.language ?? doc?.language ?? i18n.language
  const dt = i18n.getFixedT(language)
  const money = (cents: number) => formatAmount(cents / 100, currency, language)
  const uiMoney = (cents: number) => formatAmount(cents / 100, currency, i18n.language)

  const parsed = rows.map((r) => ({ price: parseCents(r.price), quantity: parseQuantity(r.amount) }))
  const rateOf = (id: string) => rates.find((r) => r.id === id)
  const percentage = (id: string) => Math.round(Number(rateOf(id)?.percentage ?? 0) * 100)
  const discountValue = parsePercentage(discount)
  const totals = computeTotals(
    rows.map((r, i) => ({ price: parsed[i]?.price ?? 0, quantity: parsed[i]?.quantity ?? 0, taxRateId: r.tax_rate_id, rate: percentage(r.tax_rate_id) })),
    discountValue ?? 0,
    pricesInclTax,
  )

  // A recurring invoice is dated as the next invoice it makes.
  const scheduled = recurring && (recurring.invoice?.invoice_date ?? recurring.schedule?.invoice_date)
  const shownDate = scheduled || setup.invoiceDate || today()
  const flowTerm = flow?.steps?.[0]?.due_interval ?? 0
  const dueTerm = setup.term.trim() !== '' && Number.isFinite(Number(setup.term)) ? Number(setup.term) : flowTerm
  const dueDate = addDays(shownDate, dueTerm)

  const labels: Record<string, string> = {
    contact_id: t('invoices.contact'), reference: t('invoices.reference'), invoice_date: t('invoices.invoiceDate'), first_due_interval: t('invoices.popup.dueAfter'),
    workflow_id: t('invoices.workflow'), document_style_id: t('invoices.popup.layout'), discount: t('invoices.discount'), payment_conditions: t('invoices.paymentConditions'),
    prices_are_incl_tax: t('invoices.details.pricesAre'), invoice_sequence_id: t('invoices.popup.invoiceId'), currency: t('invoices.details.currency'),
  }
  customFields.forEach((c, i) => {
    labels[`custom_fields_attributes.${i}`] = c.name
  })
  rows.forEach((_, i) => {
    const names = { description: t('invoices.description'), amount: t('invoices.quantity'), price: t('invoices.price'), tax_rate_id: t('invoices.vatRate'), ledger_account_id: t('invoices.category'), period: t('invoices.details.period') }
    for (const [field, name] of Object.entries(names)) {
      labels[`details_attributes.${i}.${field}`] = t('invoices.lineField', { field: name, n: i + 1 })
    }
  })
  const errors = formErrors(save.error, labels)
  const linesInvalid = rows.some((_, i) => Object.keys(labels).some((k) => k.startsWith(`details_attributes.${i}.`) && errors.field(k)))
  const contactErrors = errors.field('contact_id')
  const dateProblems = [...(errors.field('invoice_date') ?? []), ...(errors.field('first_due_interval') ?? [])]
  const dateErrors = !dialogOpen && dateProblems.length > 0 ? dateProblems : undefined


  const current = setup
  const changeRow = (i: number, c: Partial<DetailRow>) => setRows((all) => all.map((r, j) => (j === i ? { ...r, ...c } : r)))
  const remove = (i: number) => {
    const row = rows[i]
    if (row?.id) {
      setRemoved([...removed, row.id])
    }
    const left = rows.filter((_, j) => j !== i)
    // The editor keeps one row to type in, as Moneybird's does.
    setRows(left.length ? left : [blank(defaultRate)])
  }
  const pickContact = (c: Contact) => {
    change({ contact: chosen(c), ...(!doc && !setup.workflowId && c.invoice_workflow_id ? { workflowId: String(c.invoice_workflow_id) } : {}) })
  }

  const body = (setup: InvoiceSetup = current): SalesInvoiceChanges => {
    const kept = rows.filter((r) => !isBlank(r))
    return {
      contact_id: setup.contact.id,
      reference: setup.reference.trim(),
      invoice_date: setup.dateUnlocked ? setup.invoiceDate : '',
      ...(setup.numberUnlocked && setup.sequenceId.trim() ? { invoice_sequence_id: setup.sequenceId.trim() } : {}),
      ...(setup.workflowId ? { workflow_id: setup.workflowId } : {}),
      ...(setup.term.trim() !== '' ? { first_due_interval: Number(setup.term) } : {}),
      ...(setup.styleId ? { document_style_id: setup.styleId } : {}),
      prices_are_incl_tax: pricesInclTax,
      currency,
      // A new invoice leaves out what is empty; a saved one sends it to clear it.
      ...(doc || discount.trim() ? { discount: discount.trim() } : {}),
      ...(doc || conditionsTouched ? { payment_conditions: conditions.trim() } : {}),
      ...(customFields.length > 0 ? { custom_fields_attributes: customFields.map((c) => ({ id: c.id, value: setup.custom[c.id] ?? '' })) } : {}),
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
        ...removed.map((id) => ({ id, _destroy: true })),
      ],
    }
  }

  const list = `/${administrationId}/${recurring ? 'recurring_sales_invoices' : 'sales_invoices'}`
  // What the invoice was when the editor opened, so Add category knows whether to save it first.
  const sent = JSON.stringify(body())
  const saved = useRef(sent)
  const dirty = sent !== saved.current
  /** The blue Save of the details editor and of the payment conditions: the invoice is saved and the editor closes. */
  const saveAndClose = (close: () => void) => {
    save.mutate(body(), {
      onSuccess: (s) => {
        saved.current = sent
        close()
        onSaved(s, null)
      },
    })
  }
  const editConditions = () => {
    setConditionsBefore(shownConditions)
    setConditions(shownConditions)
    setConditionsTouched(true)
    setEditingConditions(true)
  }
  const cancelConditions = () => {
    setConditions(conditionsBefore)
    setEditingConditions(false)
    view?.onCancel()
  }
  /**
   * A layout picked under Extra's Change layout: the invoice is saved with it
   * straight away, as nothing else on the page would save it.
   */
  const pickLayout = (styleId: string) => {
    const next = { ...setup, styleId }
    setSetup(next)
    closeLayouts()
    if (doc) {
      save.mutate(body(next), {
        onSuccess: (s) => {
          saved.current = JSON.stringify(body(next))
          onSaved(s, null)
        },
      })
    }
  }
  /** The popup's Save and Invoice details: the draft is saved and its document shown, with its details open for the latter. */
  const saveSetup = (then: 'save' | 'details') => {
    const sequenceId = setup.numberUnlocked ? setup.sequenceId.trim() : ''
    save.mutate(body(), {
      onSuccess: (s) => {
        saved.current = sent
        setDialogOpen(false)
        const details = then === 'details' || editingLines
        // A new invoice opens its details in the editor of the saved draft, not in this one that goes away.
        if (then === 'details' && doc) {
          editLines()
        }
        onSaved(s, { details, ...(sequenceId ? { sequenceId } : {}) })
      },
    })
  }
  /**
   * "Add Category..." and "Add VAT rate..." of a row: Moneybird's Add category
   * or Add VAT rate in this tab, with new_entity_field naming the row's field,
   * which comes back to the invoice with its details open and the new one on
   * that row. Moneybird keeps a draft as it is typed; here the invoice is saved
   * first, so nothing typed is lost, and stays put when it can't be saved.
   */
  const addEntity = (index: number, key: NewEntityKey) => {
    const go = (id: string) => {
      const query = new URLSearchParams({ return_to: `${list}/${id}`, ...(key === 'ledger_account_id' ? { source: 'sales_invoice' } : {}), new_entity_field: detailField(index, key) })
      navigate(`/${administrationId}/${key === 'ledger_account_id' ? 'ledger_accounts/blueprints' : 'tax_rates/new'}?${query}`)
    }
    if (doc?.id && !dirty) {
      go(doc.id)
      return
    }
    save.mutate(body(), {
      onSuccess: (s) => {
        saved.current = sent
        go(String(s.id))
      },
    })
  }
  const openDialog = (advanced = false) => {
    before.current = setup
    save.reset()
    setDialogAdvanced(advanced)
    setDialogOpen(true)
  }
  const cancelDialog = () => {
    setSetup(before.current)
    setDialogOpen(false)
    view?.onCancel()
  }

  const title = invoice ? t('invoices.editTitle', { name: invoiceTitle(invoice, t) }) : t('invoices.newTitle')
  const paperLabel = recurring?.invoice
    ? t('recurring.document.label')
    : invoice ? t('invoices.view.document', { name: invoiceTitle(invoice, t) }) : t('invoices.editor.preview')
  const draftId = invoice ? invoice.draft_id : nextDraftId.data
  // A recurring invoice shows the number of the invoice it makes next, as Moneybird's "Periodieke factuur #1".
  const draftLabel = recurring?.invoice
    ? dt('recurring.document.number', { n: (recurring.invoice.invoices_created ?? 0) + 1 })
    : invoice?.invoice_id || (draftId ? dt('invoices.document.draftNumber', { id: draftId }) : dt('invoices.document.draft'))
  const rateName = (id: string) => rateOf(id)?.name ?? ''
  const totalRows = (tt: typeof t, format: (cents: number) => string, words: 'document' | 'editor'): TotalRow[] => {
    const key = (k: string) => (words === 'document' ? `invoices.document.${k}` : `invoices.${k === 'discount' ? 'discountAmount' : k}`)
    return [
      { label: tt(key('subtotal')), amount: format(totals.subtotal) },
      ...(totals.discount !== 0 ? [{ label: tt(key('discount'), { percentage: discount.trim() }), amount: format(-totals.discount) }] : []),
      ...totals.byRate
        .filter((r) => r.tax !== 0 || r.taxable !== 0)
        .map((r) => ({ label: pricesInclTax ? tt(key('includesVat'), { name: rateName(r.taxRateId) }) : rateName(r.taxRateId), amount: format(r.tax) })),
      { label: tt(key('total')), amount: format(totals.inclTax), strong: true },
    ]
  }
  const shownLines: ShownLine[] = rows.flatMap((r, i) =>
    isBlank(r)
      ? []
      : [{
          quantity: quantityLabel(r.amount), description: r.description, price: money(parsed[i]?.price ?? 0),
          total: money(totals.lines[i] ?? 0), vat: percentText(rateOf(r.tax_rate_id)?.percentage),
        }],
  )
  const zero = money(0)
  const emptyLine: ShownLine = { quantity: '1 x', description: t('invoices.editor.noDetails'), price: zero, total: zero, vat: percentText(rateOf(defaultRate)?.percentage) }
  const recipient = preset ?? init?.contact
  const tagValues = conditionTagValues({
    language, currency, total: totals.inclTax, unpaid: totals.inclTax - Math.round(Number(invoice?.total_paid ?? 0) * 100),
    invoiceDate: shownDate, dueDate, invoiceId: invoice?.invoice_id, draftId, reference: setup.reference, paymentReference: invoice?.payment_reference,
    contact: (recipient?.id ?? init?.contact_id) === setup.contact.id ? recipient : null, contactPerson: doc?.contact_person, sender,
  })
  const senderTags = (sender?.custom_fields ?? []).map((f) => ({ group: 'company' as const, name: `custom_field_${f.id}`, label: f.name ?? '' }))

  const content = (
    <>
      {/* A saved invoice asks to confirm the layout, as Moneybird's Extra › Change layout; a new one picks it straight away. */}
      {layoutsOpen && doc && (
        <ChangeLayoutPanel
          id={layoutsId}
          administrationId={administrationId}
          styles={allStyles}
          current={style?.id ?? ''}
          text={recurring ? t('recurring.layoutText') : t('invoices.editor.layoutText')}
          saving={save.isPending}
          onPick={pickLayout}
          onCancel={closeLayouts}
        />
      )}
      {layoutsOpen && !doc && (
        <LayoutPicker
          id={layoutsId}
          administrationId={administrationId}
          styles={allStyles}
          current={style?.id ?? ''}
          onPick={pickLayout}
        />
      )}
      <div className={`invoice-editor${view ? ' invoice-editor--in-view' : ''}`}>
        {!dialogOpen && <Problems messages={errors.general} />}
        <Paper ref={paperRef} label={paperLabel} fontSize={style?.font_size}>
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
              <button
                type="button"
                className={`invoice-paper__recipient-button${setup.contact.id ? '' : ' is-empty'}${contactErrors ? ' is-invalid' : ''}`}
                aria-label={t('invoices.contact')}
                aria-haspopup="dialog"
                {...(contactErrors && !dialogOpen ? { 'aria-invalid': true as const, 'aria-describedby': `${layoutsId}-contact-message` } : {})}
                onClick={() => openDialog()}
              >
                <RecipientContent chosen={!!setup.contact.id} lines={setup.contact.id ? [setup.contact.name, ...setup.contact.address] : []} />
              </button>
              {contactErrors && !dialogOpen && <p id={`${layoutsId}-contact-message`} className="form-input__message form-input__message--error">{contactErrors.join(' ')}</p>}
            </div>
            <SenderAddress dt={dt} entries={senderEntries(style?.invoice_sender_address, sender)} />
          </div>

          <div className="invoice-paper__heading">
            <div className="invoice-paper__heading-text">
              <h2 className="invoice-paper__title">
                {dt('invoices.document.invoice')} <em className="invoice-paper__draft">{draftLabel}</em>
              </h2>
              <p className="invoice-paper__contact">
                <span>{dt('invoices.document.contact')}:</span> <span>{setup.contact.name || t('invoices.editor.placeholderName')}</span>
              </p>
              <button type="button" className="invoice-paper__settings" aria-label={t('invoices.settingsTitle')} title={t('invoices.settingsTitle')} aria-haspopup="dialog" onClick={() => openDialog()}>
                <ListIcon kind="edit" />
              </button>
            </div>
            <div className="invoice-paper__dates">
              {[
                { label: dt('invoices.document.invoiceDate'), value: documentDate(shownDate, language), tentative: !scheduled && !setup.invoiceDate },
                { label: dt('invoices.document.dueDate'), value: documentDate(dueDate, language), tentative: false },
              ].map((d) => (
                <button key={d.label} type="button" className={`in-place__button${dateErrors ? ' is-invalid' : ''}`} aria-haspopup="dialog" onClick={() => openDialog(true)}>
                  <DateView label={d.label} value={d.value} tentative={d.tentative} />
                  <span className="in-place__indicator"><ListIcon kind="edit" /></span>
                </button>
              ))}
              {dateErrors && <p className="form-input__message form-input__message--error">{dateErrors.join(' ')}</p>}
            </div>
          </div>

          <div className="invoice-paper__details">
            {/* As on Moneybird, the editor is a card laid over the document, which stays underneath; it comes first so it starts where the details do. */}
            {(editingLines || linesInvalid) && (
              <DetailsEditor
                administrationId={administrationId}
                rows={rows}
                rates={rates}
                accounts={accounts.data ?? []}
                currency={currency}
                pricesInclTax={pricesInclTax}
                onPricesInclTax={(inclTax) => change({ inclTax })}
                onCurrency={(c) => change({ currency: c })}
                currencyLocked={!!invoice && invoice.state !== 'draft'}
                lineTotals={parsed.map((p) => grossCents(p.price ?? 0, p.quantity ?? 0))}
                invalid={parsed.map((p) => ({ price: p.price === null, amount: p.quantity === null }))}
                totals={totalRows(t, uiMoney, 'editor')}
                discount={discount}
                discountInvalid={discountValue === null}
                errors={errors}
                onChange={changeRow}
                onRemove={remove}
                onAdd={addRow}
                onMove={(from, to) => setRows(moveRow(rows, from, to))}
                onApplyBelow={(i, c) => setRows(applyBelow(rows, i, c))}
                onDiscount={setDiscount}
                onSave={() => saveAndClose(() => setEditingLines(false))}
                saving={save.isPending}
                onCancel={cancelLines}
                onAddEntity={addEntity}
              />
            )}
            <div className={editingLines || linesInvalid ? 'invoice-paper__under-edit' : undefined} aria-hidden={editingLines || linesInvalid || undefined} inert={editingLines || linesInvalid}>
              <DetailsView
                dt={dt}
                lines={shownLines}
                empty={emptyLine}
                onEdit={editLines}
                rowClass={(i) => (view?.highlight && rows.filter((r) => !isBlank(r))[i]?.ledger_account_id === view.highlight ? 'is-highlighted' : undefined)}
              />
              <TotalsTable rows={totalRows(dt, money, 'document')} />
            </div>
          </div>

          <div className="invoice-paper__footer">
            <PaymentConditions
              value={shownConditions}
              values={tagValues}
              editing={editingConditions}
              onEdit={editConditions}
              onChange={setConditions}
              onSave={() => saveAndClose(() => setEditingConditions(false))}
              onCancel={cancelConditions}
              saving={save.isPending}
              errors={errors.field('payment_conditions')}
              extraTags={senderTags}
              workflow={flow?.id ? { name: flow.name ?? '', to: `/${administrationId}/workflows/${flow.id}?${new URLSearchParams({ return_to: pathname })}` } : undefined}
            />
          </div>
        </Paper>
      </div>

      {dialogOpen && (
        <AddInvoiceDialog
          administrationId={administrationId}
          value={setup}
          onChange={change}
          onPickContact={pickContact}
          flows={invoiceFlows}
          flow={flow}
          styles={allStyles}
          style={style}
          existing={!!doc}
          recurring={!!recurring}
          advancedOpen={dialogAdvanced}
          anchor={paperRef}
          dueTerm={flowTerm}
          sentNumber={invoice?.invoice_id}
          numbering={numbering.data}
          customFields={customFields}
          homeCountry={administration.data?.country ?? 'NL'}
          here={pathname}
          errors={errors}
          pending={save.isPending}
          cancel={doc || recurring ? { onClick: cancelDialog } : { to: list }}
          onSave={saveSetup}
          // On a new invoice the × does what Cancel does: back to the invoices, as nothing was made yet; a new recurring one goes back to its schedule.
          onClose={doc || recurring ? cancelDialog : () => navigate(list)}
        />
      )}
    </>
  )

  if (view) {
    return content
  }
  const actions = (
    <>
      <button type="button" className="btn btn--secondary" aria-expanded={layoutsOpen} aria-controls={layoutsId} onClick={() => setOwnLayoutsOpen(!layoutsOpen)}>
        {t('invoices.editor.changeLayout')}
      </button>
      <MoreMenu items={[{ label: t('invoices.settingsTitle'), onSelect: () => openDialog() }, { label: t('common.cancel'), to: list }]} />
    </>
  )
  return <Page title={title} actions={actions}>{content}</Page>
}
