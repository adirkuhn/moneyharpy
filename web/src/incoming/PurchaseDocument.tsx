import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { contactName } from '../contacts/queries'
import { DateInput } from '../forms/DatePicker'
import { formErrors, TextInput } from '../forms/Form'
import { ContactSelect } from '../invoices/ContactSelect'
import { applyBelow, moveRow, periodText } from '../invoices/details'
import { type DetailRow, DetailsEditor } from '../invoices/DetailsEditor'
import { addDays } from '../invoices/paper'
import { computeTotals, parseCents, parseQuantity } from '../invoices/totals'
import { type TotalRow, TotalsTable } from '../invoices/TotalsTable'
import { StateBadge } from '../invoices/StateBadge'
import { Loading } from '../layout/Loading'
import { DocumentFacts, DocumentForm, FactRows, type Saving } from './DocumentCard'
import { type IncomingDocument, type IncomingKind, type PurchaseChanges, useSavePurchase } from './queries'
import '../invoices/editor.css'

type Props = {
  administrationId: string
  kind: IncomingKind
  doc: IncomingDocument
  /** The type row on top of the card. */
  typeRow: ReactNode
  editing: boolean
  /** Shows the form, for a document that can still change. */
  onEdit?: () => void
  /** After saving: whether the document is still new, and whether Next was asked. */
  onSaved: (stillNew: boolean, next: boolean) => void
  left: number
  cancelTo: string
}

/** The rates a document can use: the active purchase rates, and those it already has. */
const purchaseRates = (rates: TaxRate[], doc: IncomingDocument) =>
  rates.filter((r) => r.tax_rate_type === 'purchase_invoice' && (r.active || doc.details?.some((d) => d.tax_rate_id === r.id)))

/** A purchase invoice or receipt: its values, or Moneybird's form with its lines. */
export function PurchaseDocument(props: Props) {
  const rates = useTaxRates(props.administrationId)
  const accounts = useLedgerAccounts(props.administrationId)
  if (!rates.data || !accounts.data) {
    return <Loading />
  }
  const usable = purchaseRates(rates.data, props.doc)
  return props.editing
    ? <PurchaseForm key={props.doc.version} {...props} rates={usable} accounts={accounts.data} />
    : <PurchaseFacts {...props} rates={usable} accounts={accounts.data} />
}

type Loaded = Props & { rates: TaxRate[]; accounts: NonNullable<ReturnType<typeof useLedgerAccounts>['data']> }

const words = (kind: IncomingKind) => ({
  label: kind === 'receipts' ? 'incoming.types.receipt' : 'incoming.types.purchase_invoice',
  lines: kind === 'receipts' ? 'incoming.view.receiptLines' : 'incoming.view.purchaseLines',
  reference: kind === 'receipts' ? 'incoming.view.reference' : 'incoming.view.invoiceId',
})

function PurchaseFacts({ kind, doc, typeRow, onEdit, rates, accounts }: Loaded) {
  const { t, i18n } = useTranslation()
  const w = words(kind)
  const currency = doc.currency ?? 'EUR'
  const money = (amount: string | number | null | undefined) => formatAmount(amount ?? 0, currency, i18n.language)
  const rateName = (id: string | null | undefined) => rates.find((r) => r.id === id)?.name ?? ''
  const accountName = (id: string | null | undefined) => accounts.find((a) => a.id === id)?.name ?? t('invoices.uncategorized')
  const details = [...(doc.details ?? [])].sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
  const totals: TotalRow[] = [
    { label: t('invoices.subtotal'), amount: money(doc.total_price_excl_tax) },
    ...(doc.tax_totals ?? []).map((r) => ({ label: rateName(r.tax_rate_id), amount: money(r.tax_amount) })),
    { label: t('invoices.total'), amount: money(doc.total_price_incl_tax), strong: true },
  ]
  return (
    <DocumentFacts label={t(w.label)} onEdit={onEdit}>
      <div className="incoming-card__rows">
        {typeRow}
        <FactRows
          rows={[
            [t('invoices.contact'), doc.contact ? contactName(doc.contact) : ''],
            [t(w.reference), doc.reference],
            [t('incoming.view.date'), doc.date],
            ...(kind === 'purchase_invoices' ? [[t('incoming.view.dueDate'), doc.due_date] as [string, ReactNode]] : []),
            [t('invoices.status'), <StateBadge key="state" state={doc.state} />],
          ]}
        />
      </div>
      <div className="incoming-card__lines details-editor">
        <h3 className="details-editor__label">{t(w.lines)}</h3>
        <table className="incoming-lines" aria-label={t(w.lines)}>
          <thead>
            <tr>
              <th scope="col" className="incoming-lines__amount">{t('invoices.details.amount')}</th>
              <th scope="col">{t('invoices.details.description')}</th>
              <th scope="col" className="incoming-lines__money">{t('invoices.details.priceHeader')}</th>
              <th scope="col" className="incoming-lines__money">{t('invoices.details.total')}</th>
              <th scope="col">{t('invoices.details.rateAndCategory')}</th>
            </tr>
          </thead>
          <tbody>
            {details.length === 0 && (
              <tr><td colSpan={5} className="empty">{t('purchases.noLines')}</td></tr>
            )}
            {details.map((d) => (
              <tr key={d.id}>
                <td className="incoming-lines__amount">{d.amount}</td>
                <td>
                  <span className="incoming-lines__description">{d.description}</span>
                  {d.period && <span className="incoming-lines__period">{t('invoices.details.period')}: {periodText(d.period, i18n.language)}</span>}
                </td>
                <td className="incoming-lines__money">{money(d.price)}</td>
                <td className="incoming-lines__money">{money(d.total_price_excl_tax_with_discount)}</td>
                <td>
                  <span className="incoming-lines__rate">{rateName(d.tax_rate_id)}</span>
                  <span className="incoming-lines__category">{accountName(d.ledger_account_id)}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <TotalsTable rows={totals} />
      </div>
    </DocumentFacts>
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

function PurchaseForm({ administrationId, kind, doc, typeRow, rates, accounts, onSaved, left, cancelTo }: Loaded) {
  const { t, i18n } = useTranslation()
  const { pathname } = useLocation()
  const administration = useAdministration(administrationId)
  const save = useSavePurchase(administrationId, kind, doc.id ?? '')
  const w = words(kind)
  const defaultRate = rates.find((r) => r.active)?.id ?? ''
  const [contact, setContact] = useState({ id: doc.contact_id ?? '', name: doc.contact ? contactName(doc.contact) : '' })
  const [reference, setReference] = useState(doc.reference ?? '')
  const [date, setDate] = useState(doc.date ?? '')
  const [dueDate, setDueDate] = useState(doc.due_date ?? '')
  const [days, setDays] = useState('')
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
    rows.map((r, i) => ({ price: parsed[i]?.price ?? 0, quantity: parsed[i]?.quantity ?? 0, taxRateId: r.tax_rate_id, rate: Math.round(Number(rateOf(r.tax_rate_id)?.percentage ?? 0) * 100) })),
    0,
    inclTax,
  )
  const money = (cents: number) => formatAmount(cents / 100, currency, i18n.language)
  const totalRows: TotalRow[] = [
    { label: t('invoices.subtotal'), amount: money(totals.exclTax) },
    ...totals.byRate.filter((r) => r.tax !== 0 || r.taxable !== 0).map((r) => ({ label: rateOf(r.taxRateId)?.name ?? '', amount: money(r.tax) })),
    { label: t('invoices.total'), amount: money(totals.inclTax), strong: true },
  ]

  const labels: Record<string, string> = {
    contact_id: t('invoices.contact'), reference: t(w.reference), date: t('incoming.view.date'), due_date: t('incoming.view.dueDate'),
    currency: t('invoices.details.currency'), prices_are_incl_tax: t('invoices.details.pricesAre'),
  }
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
  const changeDays = (value: string) => {
    setDays(value)
    if (/^\d{1,4}$/.test(value.trim()) && date) {
      setDueDate(addDays(date, Number(value)))
    }
  }

  const body = (): PurchaseChanges => ({
    contact_id: contact.id,
    reference: reference.trim(),
    date,
    ...(kind === 'purchase_invoices' ? { due_date: dueDate } : {}),
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
  const saving: Saving = {
    pending: save.isPending,
    save: (next) => save.mutate(body(), { onSuccess: (saved) => onSaved(saved.state === 'new', next) }),
    left,
    cancelTo,
  }

  return (
    <DocumentForm label={t(w.label)} saving={saving}>
      <Problems messages={errors.general} />
      <div className="incoming-card__rows">
        {typeRow}
        <ContactSelect
          administrationId={administrationId}
          value={contact.name}
          errors={errors.field('contact_id')}
          homeCountry={administration.data?.country ?? 'NL'}
          returnTo={pathname}
          onChange={(c) => setContact({ id: String(c.id ?? ''), name: contactName(c) })}
        />
        <TextInput label={t(w.reference)} maxLength={255} value={reference} errors={errors.field('reference')} onChange={(e) => setReference(e.target.value)} />
        <DateInput label={t('incoming.view.date')} value={date} errors={errors.field('date')} onChange={setDate} />
        {kind === 'purchase_invoices' && (
          <DateInput
            label={t('incoming.view.dueDate')}
            value={dueDate}
            errors={errors.field('due_date')}
            onChange={(value) => {
              setDueDate(value)
              setDays('')
            }}
            action={
              <span className="incoming-card__days">
                <input type="text" inputMode="numeric" aria-label={t('incoming.view.days')} maxLength={4} value={days} onChange={(e) => changeDays(e.target.value)} />
                <span aria-hidden="true">{t('incoming.view.days')}</span>
              </span>
            }
          />
        )}
      </div>
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
        buttons={null}
      />
    </DocumentForm>
  )
}
