import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { type LedgerAccount, type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { ContactHoverCard } from '../contacts/ContactHoverCard'
import { contactName } from '../contacts/queries'
import { formErrors } from '../forms/Form'
import { type Confidence, leastSure, Remark } from '../forms/Remark'
import { applyBelow, moveRow, periodText } from '../invoices/details'
import { type DetailRow, DetailsEditor } from '../invoices/DetailsEditor'
import { StateBadge } from '../invoices/StateBadge'
import { chargedRate, computeTotals, parseCents, parseQuantity, vatRows } from '../invoices/totals'
import { type TotalRow } from '../invoices/TotalsTable'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { FactRows, NextFooter } from './DocumentCard'
import { type FieldSaving, fieldErrors, InlineContact, InlineDate, InlineDueDate, InlineText } from './InlineFields'
import { type IncomingDocument, type IncomingKind, type PurchaseChanges, useSavePurchase } from './queries'
import { type Remarks, remarksOf } from './recognition'
import '../invoices/editor.css'

type Props = {
  administrationId: string
  kind: IncomingKind
  doc: IncomingDocument
  /** The type row on top of the card. */
  typeRow: ReactNode
  /** Whether the document can still change: not once it is paid or has payments, as the server refuses it then. */
  editable: boolean
  /** How many other documents are still to process. */
  left: number
  /** Opens the next document to process, for a document that is still new. */
  onNext?: () => void
  /** What Scan and Recognise is doing, on top of the card. */
  notice?: ReactNode
}

/** The rates a document can use: the active purchase rates, and those it already has. */
const purchaseRates = (rates: TaxRate[], doc: IncomingDocument) =>
  rates.filter((r) => r.tax_rate_type === 'purchase_invoice' && (r.active || doc.details?.some((d) => d.tax_rate_id === r.id)))

/**
 * A purchase invoice or receipt as Moneybird shows it, without an Edit
 * button: each field saves itself, and a click on the details opens the
 * lines editor in their place. A paid document shows the same, but nothing
 * can change.
 */
export function PurchaseDocument(props: Props) {
  const rates = useTaxRates(props.administrationId)
  const accounts = useLedgerAccounts(props.administrationId)
  if (!rates.data || !accounts.data) {
    return <Loading />
  }
  return <PurchaseCard {...props} rates={purchaseRates(rates.data, props.doc)} accounts={accounts.data} />
}

type Loaded = Props & { rates: TaxRate[]; accounts: LedgerAccount[] }

const words = (kind: IncomingKind) => ({
  label: kind === 'receipts' ? 'incoming.types.receipt' : 'incoming.types.purchase_invoice',
  lines: kind === 'receipts' ? 'incoming.view.receiptLines' : 'incoming.view.purchaseLines',
  details: kind === 'receipts' ? 'incoming.view.receiptDetails' : 'incoming.view.invoiceDetails',
  reference: kind === 'receipts' ? 'incoming.view.reference' : 'incoming.view.invoiceId',
})

function PurchaseCard(props: Loaded) {
  const { administrationId, kind, doc, typeRow, editable, left, onNext, notice } = props
  const { t } = useTranslation()
  const w = words(kind)
  const remarks = remarksOf(doc.recognition, doc.details)
  const isNew = doc.state === 'new'
  // What was read shows on the details until the document or its lines are saved; the lines editor still shows it.
  const [linesSaved, setLinesSaved] = useState(false)
  const shownRemarks = isNew && !linesSaved ? remarks : remarksOf(undefined)
  // A document without lines opens with the lines editor, as Moneybird shows a fresh one.
  const [linesOpen, setLinesOpen] = useState(editable && (doc.details ?? []).length === 0)
  const open = editable && linesOpen
  return (
    <section className="incoming-card" aria-label={t(w.label)}>
      {notice}
      <div className="incoming-card__rows">
        {typeRow}
        {editable
          ? <PurchaseFields administrationId={administrationId} kind={kind} doc={doc} remarks={remarks} />
          : (
            <FactRows
              rows={[
                [t('invoices.contact'), doc.contact && <ContactHoverCard key="contact" administrationId={administrationId} contact={{ ...doc.contact, id: doc.contact_id }} />],
                [t(w.reference), doc.reference],
                [t('incoming.view.date'), doc.date],
                ...(kind === 'purchase_invoices' ? [[t('incoming.view.dueDate'), doc.due_date] as [string, ReactNode]] : []),
              ]}
            />
          )}
        {!isNew && doc.state && <FactRows rows={[[t('invoices.status'), <StateBadge key="state" state={doc.state} />]]} />}
      </div>
      {open
        ? (
          <PurchaseLines
            // What Scan and Recognise reads fills in the lines while they are open.
            key={doc.recognition?.state ?? 'none'}
            {...props}
            remarks={remarks}
            onSaved={() => setLinesSaved(true)}
            onClose={() => setLinesOpen(false)}
            onNext={isNew ? onNext : undefined}
          />
        )
        : <DetailsSummary {...props} remarks={shownRemarks} onOpen={editable ? () => setLinesOpen(true) : undefined} />}
      {isNew && editable && <NextFooter left={left} onNext={open ? undefined : onNext} />}
    </section>
  )
}

/** Saves one field of a purchase invoice or receipt with a request of its own, so its problems show under it. */
function usePurchaseField(administrationId: string, kind: IncomingKind, id: string, name: 'contact_id' | 'reference' | 'date' | 'due_date', label: string): FieldSaving {
  const save = useSavePurchase(administrationId, kind, id)
  return { save: (value) => save.mutate({ [name]: value }), pending: save.isPending, errors: fieldErrors(save.error, name, label) }
}

type FieldsProps = { administrationId: string; kind: IncomingKind; doc: IncomingDocument; remarks: Remarks }

/** The contact, reference and dates, each saved on its own as Moneybird's document page does. */
function PurchaseFields({ administrationId, kind, doc, remarks }: FieldsProps) {
  const { t } = useTranslation()
  const w = words(kind)
  const id = doc.id ?? ''
  const contact = usePurchaseField(administrationId, kind, id, 'contact_id', t('invoices.contact'))
  const reference = usePurchaseField(administrationId, kind, id, 'reference', t(w.reference))
  const date = usePurchaseField(administrationId, kind, id, 'date', t('incoming.view.date'))
  const due = usePurchaseField(administrationId, kind, id, 'due_date', t('incoming.view.dueDate'))
  return (
    <>
      <InlineContact administrationId={administrationId} contactId={doc.contact_id} name={doc.contact ? contactName(doc.contact) : ''} saving={contact} remark={remarks.field('contact_id')} />
      <InlineText label={t(w.reference)} saved={doc.reference ?? ''} saving={reference} remark={remarks.field('reference')} />
      <InlineDate label={t('incoming.view.date')} saved={doc.date ?? ''} saving={date} remark={remarks.field('date')} />
      {kind === 'purchase_invoices' && (
        <InlineDueDate label={t('incoming.view.dueDate')} saved={doc.due_date ?? ''} saving={due} remark={remarks.field('due_date')} from={doc.date ?? ''} />
      )}
    </>
  )
}

type SummaryProps = Loaded & { remarks: Remarks; onOpen?: () => void }

/** A line's mark: the least sure of what Scan and Recognise filled in on it. */
const lineRemark = (remarks: Remarks, id: string | undefined) => leastSure(Object.values(remarks.lines[id ?? ''] ?? {}))

/**
 * The details as Moneybird shows them on a document: the categories used, a
 * short table of the lines and the totals. The whole block opens the lines
 * editor, by a click anywhere on it or by its button from the keyboard.
 */
function DetailsSummary({ kind, doc, rates, accounts, remarks, onOpen }: SummaryProps) {
  const { t, i18n } = useTranslation()
  const w = words(kind)
  const [hovered, setHovered] = useState<string | null>(null)
  const currency = doc.currency ?? 'EUR'
  const money = (amount: string | number | null | undefined) => formatAmount(amount ?? 0, currency, i18n.language)
  const rateName = (id: string | null | undefined) => rates.find((r) => r.id === id)?.name ?? ''
  const account = (id: string | null | undefined) => accounts.find((a) => a.id === id)
  const accountName = (id: string | null | undefined) => account(id)?.name ?? t('invoices.uncategorized')
  const details = [...(doc.details ?? [])].sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
  const categories = [...new Set(details.map((d) => d.ledger_account_id ?? ''))]
  const totalsRemark = leastSure([...details.map((d) => lineRemark(remarks, d.id)), remarks.field('currency'), remarks.field('prices_are_incl_tax')])
  const totals: TotalRow[] = [
    { label: t('invoices.subtotal'), amount: money(doc.total_price_excl_tax) },
    ...vatRows(
      (doc.tax_totals ?? []).map((r) => ({ taxRateId: r.tax_rate_id ?? '', taxable: parseCents(r.taxable_amount ?? '0') ?? 0, tax: parseCents(r.tax_amount ?? '0') ?? 0 })),
      rates,
      (cents) => money(cents / 100),
    ),
    { label: t('invoices.total'), amount: money(doc.total_price_incl_tax), strong: true },
  ]
  const tip = (id: string) => {
    const a = account(id)
    return a ? t('incoming.view.categoryTip', { type: t(`bookkeeping.accountTypes.${a.account_type}`), name: a.name }) : t('invoices.uncategorized')
  }
  return (
    <div className={`document-details${onOpen ? ' is-clickable' : ''}`} onClick={onOpen}>
      {onOpen && (
        <button
          type="button"
          className="document-details__open"
          aria-label={t(w.details)}
          onClick={(e) => {
            e.stopPropagation()
            onOpen()
          }}
        />
      )}
      {categories.length > 0 && (
        <dl className="document-details__categories">
          <dt>{t('incoming.view.category')}</dt>
          <dd>
            {categories.map((id) => (
              <span
                key={id || 'none'}
                className="category-tag"
                title={tip(id)}
                onMouseEnter={() => setHovered(id)}
                onMouseLeave={() => setHovered(null)}
              >
                <TagIcon />
                <span className="category-tag__name">{accountName(id)}</span>
              </span>
            ))}
          </dd>
        </dl>
      )}
      <table className="incoming-lines" aria-label={t(w.lines)}>
        <thead>
          <tr>
            <th scope="col">{t('invoices.details.description')}</th>
            <th scope="col">{t('invoices.category')}</th>
            <th scope="col">{t('invoices.vatRate')}</th>
            <th scope="col" className="incoming-lines__money">{t('invoices.details.total')}</th>
            <th scope="col" className="incoming-lines__icons" />
          </tr>
        </thead>
        <tbody>
          {details.length === 0 && (
            <tr><td colSpan={5} className="empty">{t('purchases.noLines')}</td></tr>
          )}
          {details.map((d) => (
            <tr key={d.id} className={hovered !== null && (d.ledger_account_id ?? '') === hovered ? 'is-highlighted' : undefined}>
              <td>
                <span className="incoming-lines__description">{d.description}</span>
                {d.period && <span className="incoming-lines__period">{t('invoices.details.period')}: {periodText(d.period, i18n.language)}</span>}
              </td>
              <td className="incoming-lines__muted">{accountName(d.ledger_account_id)}</td>
              <td className="incoming-lines__muted">{rateName(d.tax_rate_id)}</td>
              <td className="incoming-lines__money">{money(d.total_price_excl_tax_with_discount)}</td>
              <td className="incoming-lines__icons"><Remark confidence={lineRemark(remarks, d.id)} /></td>
            </tr>
          ))}
        </tbody>
      </table>
      <DocumentTotals rows={totals} remark={details.length > 0 ? totalsRemark : undefined} />
    </div>
  )
}

/** The totals of a document as a list at the right: subtotal, VAT and total, each with what Scan and Recognise read. */
function DocumentTotals({ rows, remark }: { rows: TotalRow[]; remark?: Confidence }) {
  const { t } = useTranslation()
  return (
    <ul className="document-totals" aria-label={t('invoices.totals')}>
      {rows.map((r, i) => (
        // Labels can repeat, as reverse-charged VAT's two rows, so rows are keyed by position.
        <li key={i} className={r.strong ? 'document-totals__strong' : undefined}>
          <span className="document-totals__label">{r.label}</span>
          <span className="document-totals__amount">{r.amount}</span>
          <span className="document-totals__remark"><Remark confidence={remark} /></span>
        </li>
      ))}
    </ul>
  )
}

function TagIcon() {
  return (
    <svg className="category-tag__icon" viewBox="0 0 24 24" width="0.875em" height="0.875em" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3.5 12.2V4.5a1 1 0 0 1 1-1h7.7l8.3 8.3a1.5 1.5 0 0 1 0 2.1l-6.6 6.6a1.5 1.5 0 0 1-2.1 0Z" />
      <circle cx="8" cy="8" r="1.25" />
    </svg>
  )
}

let nextKey = 0
const newKey = () => `line-${++nextKey}`
const blank = (taxRateId: string, over: Partial<DetailRow> = {}): DetailRow => ({
  key: newKey(), description: '', amount: '1', price: '', tax_rate_id: taxRateId, ledger_account_id: '', period: '', product_id: '', ...over,
})
const priceText = (p: string | number | undefined | null) => (p === undefined || p === null || p === '' ? '' : String(Number(p)))
const isBlank = (r: DetailRow) => !r.id && !r.description.trim() && !r.price.trim()
/** A line's amount times its price, in cents, as the Total column shows it. */
const grossCents = (price: number, quantity: number) => Math.sign(price * quantity) * Math.round(Math.abs(price * quantity) / 1000)

type LinesProps = Loaded & {
  remarks: Remarks
  /** The lines were saved: what was read no longer shows on the details. */
  onSaved: () => void
  /** Back to the details as they read, after Save or Cancel. */
  onClose: () => void
  /** Saves and opens the next document to process, for a new document. */
  onNext?: () => void
}

/**
 * The lines editor in place of the details: the lines with whether prices
 * include VAT and their currency, saved together by Save, or put back by
 * Cancel.
 */
function PurchaseLines({ administrationId, kind, doc, rates, accounts, remarks, onSaved, onClose, onNext }: LinesProps) {
  const { t, i18n } = useTranslation()
  const administration = useAdministration(administrationId)
  const save = useSavePurchase(administrationId, kind, doc.id ?? '')
  const w = words(kind)
  const defaultRate = rates.find((r) => r.active)?.id ?? ''
  const [currency, setCurrency] = useState(doc.currency ?? administration.data?.currency ?? 'EUR')
  const [inclTax, setInclTax] = useState(doc.prices_are_incl_tax ?? false)
  const [rows, setRows] = useState<DetailRow[]>(() => {
    const loaded = [...(doc.details ?? [])]
      .sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
      .map((d) => ({
        key: d.id ?? newKey(), id: d.id, description: d.description ?? '', amount: d.amount ?? '1', price: priceText(d.price),
        tax_rate_id: d.tax_rate_id ?? defaultRate, ledger_account_id: d.ledger_account_id ?? '', period: d.period ?? '', savedPeriod: d.period ?? '',
        product_id: d.product_id ? String(d.product_id) : '',
      }))
    return loaded.length ? loaded : [blank(defaultRate)]
  })
  const [removed, setRemoved] = useState<string[]>([])

  const parsed = rows.map((r) => ({ price: parseCents(r.price), quantity: parseQuantity(r.amount) }))
  const rateOf = (id: string) => rates.find((r) => r.id === id)
  const totals = computeTotals(
    rows.map((r, i) => ({ price: parsed[i]?.price ?? 0, quantity: parsed[i]?.quantity ?? 0, taxRateId: r.tax_rate_id, rate: chargedRate(rateOf(r.tax_rate_id)) })),
    0,
    inclTax,
  )
  const money = (cents: number) => formatAmount(cents / 100, currency, i18n.language)
  const totalRows: TotalRow[] = [
    { label: t('invoices.subtotal'), amount: money(totals.exclTax) },
    ...vatRows(totals.byRate, rates, money),
    { label: t('invoices.total'), amount: money(totals.inclTax), strong: true },
  ]

  const labels: Record<string, string> = { currency: t('invoices.details.currency'), prices_are_incl_tax: t('invoices.details.pricesAre') }
  rows.forEach((_, i) => {
    const names = { description: t('invoices.description'), amount: t('invoices.quantity'), price: t('invoices.price'), tax_rate_id: t('invoices.vatRate'), ledger_account_id: t('invoices.category'), period: t('invoices.details.period') }
    for (const [field, name] of Object.entries(names)) {
      labels[`details_attributes.${i}.${field}`] = t('invoices.lineField', { field: name, n: i + 1 })
    }
  })
  const errors = formErrors(save.error, labels)

  const changeRow = (i: number, c: Partial<DetailRow>) => setRows((all) => all.map((r, j) => (j === i ? { ...r, ...c } : r)))
  const removeRow = (i: number) => {
    const row = rows[i]
    if (row?.id) {
      setRemoved([...removed, row.id])
    }
    const rest = rows.filter((_, j) => j !== i)
    setRows(rest.length ? rest : [blank(defaultRate)])
  }
  const addRow = (over?: Partial<DetailRow>) => {
    const above = rows.at(-1)
    setRows([...rows, blank(above?.tax_rate_id || defaultRate, { ledger_account_id: above?.ledger_account_id ?? '', ...over })])
  }

  // Only what the editor shows: the lines, whether their prices include VAT, and their currency.
  const body = (): PurchaseChanges => ({
    currency,
    prices_are_incl_tax: inclTax,
    details_attributes: [
      ...rows.filter((r) => !isBlank(r)).map((r, i) => ({
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
  })
  const submit = (next: boolean) =>
    save.mutate(body(), {
      onSuccess: () => {
        onSaved()
        onClose()
        if (next) {
          onNext?.()
        }
      },
    })

  return (
    <div className="incoming-card__editor">
      <Problems messages={errors.general} />
      <DetailsEditor
        administrationId={administrationId}
        label={t(w.lines)}
        documentType="purchase_invoice"
        rows={rows}
        rates={rates}
        accounts={accounts}
        currency={currency}
        pricesInclTax={inclTax}
        onPricesInclTax={setInclTax}
        onCurrency={setCurrency}
        lineTotals={parsed.map((p) => grossCents(p.price ?? 0, p.quantity ?? 0))}
        invalid={parsed.map((p) => ({ price: p.price === null, amount: p.quantity === null }))}
        totals={totalRows}
        discount=""
        discountInvalid={false}
        discounts={false}
        errors={errors}
        onChange={changeRow}
        onRemove={removeRow}
        onAdd={addRow}
        onMove={(from, to) => setRows(moveRow(rows, from, to))}
        onApplyBelow={(i, c) => setRows(applyBelow(rows, i, c))}
        onDiscount={() => undefined}
        onSave={() => submit(false)}
        onCancel={onClose}
        saving={save.isPending}
        buttons={onNext ? (
          // A new document saves its lines and goes on to the next one to process, as Moneybird's Save and Next.
          <div className="editable-document-form__buttons">
            <div className="btn-group">
              <button type="button" className="btn btn--primary" disabled={save.isPending} onClick={() => submit(false)}>{t('incoming.view.save')}</button>
              <button type="button" className="btn btn--primary btn--with-icon" disabled={save.isPending} onClick={() => submit(true)}>
                <ListIcon kind="plus" />
                <span>{t('incoming.view.next')}</span>
              </button>
            </div>
            <button type="button" className="btn btn--secondary" onClick={onClose}>{t('common.cancel')}</button>
          </div>
        ) : undefined}
        remarks={remarks.lines}
        headerRemark={leastSure([remarks.field('currency'), remarks.field('prices_are_incl_tax')])}
      />
    </div>
  )
}
