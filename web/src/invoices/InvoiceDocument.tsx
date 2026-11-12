import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { formatAmount } from '../banking/queries'
import { useTaxRates } from '../bookkeeping/queries'
import { contactName } from '../contacts/queries'
import { styleFileURL, useDocumentStyles, useWorkflows } from '../invoicing/queries'
import { ListIcon } from '../layout/icons'
import { useIdentities } from '../settings/queries'
import { quantityLabel } from './details'
import type { Opening } from './InvoiceFormPage'
import { DateView, DetailsView, Paper, RecipientContent, SenderAddress, type ShownLine } from './InvoicePaper'
import { addDays, addressLines, documentDate, percentText, senderEntries, today } from './paper'
import { invoiceTitle, type SalesInvoice } from './queries'
import { computeTotals, parseCents, parsePercentage, parseQuantity } from './totals'
import { type TotalRow, TotalsTable } from './TotalsTable'

type Props = {
  administrationId: string
  invoice: SalesInvoice
  /** Whether its blocks open the editor, as the blue markers on Moneybird's document do. */
  editable: boolean
  /** The category the sidebar points at, whose lines stand out. */
  highlight?: string
  /** Asks before the editor opens; without it the blocks open the editor straight away. */
  onEdit?: (opening: Opening) => void
}

/**
 * The invoice drawn as its document, read-only, after Moneybird's invoice
 * page; each block of an invoice that can still change opens the editor.
 */
export function InvoiceDocument({ administrationId, invoice: inv, editable, highlight, onEdit }: Props) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const rates = useTaxRates(administrationId)
  const workflows = useWorkflows(administrationId)
  const styles = useDocumentStyles(administrationId)
  const identities = useIdentities(administrationId)

  const flow = (workflows.data ?? []).find((w) => w.id === inv.workflow_id)
  const allStyles = styles.data ?? []
  const style = allStyles.find((s) => s.id === inv.document_style_id) ?? allStyles.find((s) => s.default) ?? allStyles[0]
  const allIdentities = identities.data ?? []
  const sender = allIdentities.find((i) => i.id === style?.identity_id) ?? allIdentities.find((i) => i.default) ?? allIdentities[0]
  // The document is written in the invoice's language, as Moneybird prints it.
  const language = inv.language || flow?.language || i18n.language
  const dt = i18n.getFixedT(language)
  const currency = inv.currency ?? 'EUR'
  const money = (v: string | number | null | undefined) => formatAmount(v, currency, language)
  const cents = (c: number) => money(c / 100)
  const rateOf = (id: string | null | undefined) => rates.data?.find((r) => r.id === String(id ?? ''))
  const percentage = (id: string | null | undefined) => Math.round(Number(rateOf(id)?.percentage ?? 0) * 100)

  const details = [...(inv.details ?? [])].sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
  const totals = computeTotals(
    details.map((d) => ({ price: parseCents(String(Number(d.price ?? 0))) ?? 0, quantity: parseQuantity(d.amount ?? '1') ?? 0, taxRateId: d.tax_rate_id ?? '', rate: percentage(d.tax_rate_id) })),
    parsePercentage(Number(inv.discount ?? 0) ? String(Number(inv.discount)) : '') ?? 0,
    inv.prices_are_incl_tax ?? false,
  )
  const lines: ShownLine[] = details.map((d, i) => ({
    quantity: quantityLabel(d.amount ?? ''), description: d.description ?? '', price: money(d.price), total: cents(totals.lines[i] ?? 0), vat: percentText(rateOf(d.tax_rate_id)?.percentage),
  }))
  const inclTax = inv.prices_are_incl_tax ?? false
  const totalRows: TotalRow[] = [
    { label: inclTax ? dt('invoices.document.subtotal') : dt('invoices.document.subtotalExcl'), amount: cents(totals.subtotal) },
    ...(totals.discount !== 0 ? [{ label: dt('invoices.document.discount', { percentage: Number(inv.discount ?? 0) }), amount: cents(-totals.discount) }] : []),
    ...(inv.tax_totals ?? []).map((r) => {
      const name = rateOf(r.tax_rate_id)?.name ?? ''
      return { label: inclTax ? dt('invoices.document.includesVat', { name }) : dt('invoices.document.vatOver', { name, amount: money(r.taxable_amount) }), amount: money(r.tax_amount) }
    }),
    { label: dt('invoices.document.total'), amount: money(inv.total_price_incl_tax), strong: true },
  ]

  const invoiceDate = inv.invoice_date || today()
  const dueDate = inv.due_date || addDays(invoiceDate, inv.first_due_interval ?? flow?.steps?.[0]?.due_interval ?? 0)
  const number = inv.invoice_id || dt('invoices.document.draftNumber', { id: inv.draft_id ?? '' })
  const bank = sender?.bank_account_number
  const payBefore = bank && dt('invoices.document.payBefore', {
    amount: money(inv.total_price_incl_tax), date: documentDate(dueDate, language), account: bank,
    holder: sender.bank_account_name || sender.company_name || '', reference: inv.payment_reference || dt('invoices.document.numberOnSend'),
  })
  const contact = inv.contact
  const recipient = contact ? [contactName(contact), ...addressLines(contact)] : []
  const edit = `/${administrationId}/sales_invoices/${inv.id}/edit`
  // A sent invoice asks first, as Moneybird warns before it is edited.
  const open = (opening: Opening) => ({
    to: edit,
    state: opening,
    onClick: (e: React.MouseEvent) => {
      if (onEdit) {
        e.preventDefault()
        onEdit(opening)
      }
    },
  })
  const ledger = (i: number) => details[i]?.ledger_account_id ?? ''

  return (
    <Paper label={t('invoices.view.document', { name: invoiceTitle(inv, t) })} fontSize={style?.font_size}>
      <div className={`invoice-document${editable ? ' is-editable' : ''}`}>
        <div className={`invoice-paper__header invoice-paper__header--${style?.address_position ?? 'left'}`}>
          <div className={`invoice-paper__logo invoice-paper__logo--${style?.logo_position ?? 'left'}`}>
            {style?.logo_hash && (
              <img src={styleFileURL(administrationId, style.id ?? '', 'logo', style.logo_hash)} alt={t('invoices.editor.logo')} style={{ width: `${style.logo_display_width ?? 50}%` }} />
            )}
          </div>
          <div className="invoice-paper__recipient">
            {editable ? (
              <Link className="invoice-paper__recipient-button invoice-document__recipient" aria-label={t('invoices.view.editContact')} {...open({ dialog: true })}>
                <RecipientContent chosen lines={recipient} />
              </Link>
            ) : (
              <div className="invoice-paper__recipient-button invoice-document__recipient">
                <RecipientContent chosen lines={recipient} editable={false} />
              </div>
            )}
          </div>
          <SenderAddress dt={dt} entries={senderEntries(style?.invoice_sender_address, sender)} />
        </div>

        <Block editable={editable} marker={<Link className="invoice-document__marker" aria-label={t('invoices.settingsTitle')} title={t('invoices.settingsTitle')} {...open({ dialog: true })}><ListIcon kind="edit" /></Link>}>
          <div className="invoice-paper__heading">
            <div className="invoice-paper__heading-text">
              <h2 className="invoice-paper__title">{dt('invoices.document.invoice')} {inv.invoice_id ? number : <em className="invoice-paper__draft">{number}</em>}</h2>
              {inv.reference && (
                <p className="invoice-document__reference">
                  <span>{dt('invoices.document.reference')}:</span> <span>{inv.reference}</span>
                </p>
              )}
            </div>
            <div className="invoice-paper__dates">
              <DateView label={dt('invoices.document.invoiceDate')} value={documentDate(invoiceDate, language)} tentative={!inv.invoice_date} />
              <DateView label={dt('invoices.document.dueDate')} value={documentDate(dueDate, language)} />
            </div>
          </div>
        </Block>

        <div className="invoice-paper__details invoice-document__details">
          <DetailsView
            dt={dt}
            lines={lines}
            empty={{ quantity: '1 x', description: t('invoices.editor.noDetails'), price: cents(0), total: cents(0), vat: '' }}
            onEdit={editable ? () => (onEdit ? onEdit({ details: true }) : navigate(edit, { state: { details: true } satisfies Opening })) : undefined}
            rowClass={(i) => (highlight && ledger(i) === highlight ? 'is-highlighted' : undefined)}
          />
          <TotalsTable rows={totalRows} />
        </div>

        <Block
          editable={editable}
          className="invoice-paper__footer"
          marker={<Link className="invoice-document__marker" aria-label={t('invoices.view.editConditions')} title={t('invoices.view.editConditions')} {...open(null)}><ListIcon kind="edit" /></Link>}
        >
          {inv.payment_conditions && <p className="invoice-paper__conditions">{inv.payment_conditions}</p>}
          {payBefore && <p className="invoice-paper__pay">{payBefore}</p>}
        </Block>
      </div>
    </Paper>
  )
}

/** A block of the document, with Moneybird's blue marker on its left edge when it can be edited. */
function Block({ editable, marker, className = '', children }: { editable: boolean; marker: ReactNode; className?: string; children: ReactNode }) {
  return (
    <div className={`invoice-document__block ${className}`}>
      {editable && marker}
      {children}
    </div>
  )
}
