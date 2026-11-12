import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { formatAmount } from '../banking/queries'
import { DateView, DetailsView, Paper } from '../invoices/InvoicePaper'
import { addressLines, documentDate } from '../invoices/paper'
import { TotalsTable } from '../invoices/TotalsTable'
import type { Identity } from '../settings/queries'
import type { PreviewData } from './previewData'
import { type DocumentStyle, type Line, styleFileURL } from './queries'
import { customFieldId } from './styleLayout'

type Lines = 'invoice_sender_address' | 'invoice_metadata_left' | 'invoice_metadata_right' | 'estimate_sender_address' | 'estimate_metadata_left'
  | 'estimate_metadata_right' | 'invoice_recipient_address' | 'estimate_recipient_address'

export type PreviewLayout = {
  logo_position: 'left' | 'center' | 'right'
  logo_display_width: number
  logo_container_full_width: boolean
  address_position: 'left' | 'right'
  font_family: string
  font_size: number
  hide_sender_address: boolean
  sender_address_width: number
  show_epc_code: boolean
} & Record<Lines, Line[]>

export const fontStacks: Record<string, string> = {
  lucida_grande: "'Lucida Grande', 'Lucida Sans Unicode', sans-serif", helvetica: 'Helvetica, Arial, sans-serif', arial: 'Arial, Helvetica, sans-serif',
  verdana: 'Verdana, Geneva, sans-serif', georgia: 'Georgia, serif', times_new_roman: "'Times New Roman', Times, serif", courier: "'Courier New', Courier, monospace",
}

/** The width, in the pixels the sender address width is set in, the preview sheet stands for. */
const pageWidth = 560

type Props = {
  administrationId: string
  /** The saved style, for its logo. */
  style: DocumentStyle
  layout: PreviewLayout
  sender: Identity | undefined
  /** The language the preview prints in. */
  language: string
  data: PreviewData
  /** The names of custom fields by their layout field, such as custom_field_5. */
  names: Record<string, string>
}

/**
 * The style applied to an invoice and a quote, as Moneybird shows next to
 * its editor, with the administration's own data. It follows every change
 * before it is saved.
 */
export function StylePreview(props: Props) {
  const { t, i18n } = useTranslation()
  const dt = i18n.getFixedT(props.language)
  return (
    <div className="style-preview" role="region" aria-label={t('documentStyle.previewTitle')}>
      <PreviewDocument {...props} dt={dt} kind="invoice" label={t('documentStyle.invoicePreview')} />
      <PreviewDocument {...props} dt={dt} kind="estimate" label={t('documentStyle.estimatePreview')} />
    </div>
  )
}

/** What a sender field prints: the identity's value, its address over several lines, or a custom field. */
export function senderText(field: string, sender: Identity | undefined): string[] {
  if (!sender) {
    return []
  }
  if (field === 'address') {
    return addressLines(sender)
  }
  const custom = customFieldId(field)
  if (custom) {
    const value = sender.custom_fields?.find((f) => String(f.id) === custom)?.value
    return value ? [value] : []
  }
  const v = (sender as Record<string, unknown>)[field]
  return typeof v === 'string' && v !== '' ? [v] : []
}

/** The name a field prints with as a label: a custom field's own name, or the field's. */
export function fieldLabel(field: string, dt: TFunction, names: Record<string, string>) {
  return names[field] ?? dt(`documentStyle.fields.${field}`)
}

type Entry = { key: string; label?: string; text: string[]; bold: boolean; italic: boolean } | { key: string; blank: true }

/** A block of the style's lines: blank lines kept, fields without a value left out, as on the printed document. */
function entries(lines: Line[], value: (field: string) => string[], label: (field: string) => string): Entry[] {
  return lines.flatMap((l, i): Entry[] => {
    if (l.field === 'newline') {
      return [{ key: `${i}`, blank: true }]
    }
    const text = value(l.field)
    return text.length ? [{ key: `${i}`, label: l.label ? label(l.field) : undefined, text, bold: l.bold === true, italic: l.italic === true }] : []
  })
}

function Block({ items, className, label }: { items: Entry[]; className: string; label?: string }) {
  return (
    <ul className={className} aria-label={label}>
      {items.map((e) =>
        'blank' in e ? (
          <li key={e.key} className="invoice-paper__newline" />
        ) : (
          <li key={e.key} className={[e.bold && 'invoice-paper__bold', e.italic && 'style-preview__italic'].filter(Boolean).join(' ') || undefined}>
            {e.label && <span>{e.label}: </span>}
            {e.text.map((line, j) => <span key={j} className={j > 0 ? 'invoice-paper__line' : undefined}>{line}</span>)}
          </li>
        ),
      )}
    </ul>
  )
}

function PreviewDocument({ administrationId, style, layout, sender, language, data, names, dt, kind, label }: Props & { dt: TFunction; kind: 'invoice' | 'estimate'; label: string }) {
  const { t } = useTranslation()
  const money = (v: number) => formatAmount(v, data.currency, language)
  const number = kind === 'invoice' ? data.invoiceNumber : data.estimateNumber
  const due = kind === 'invoice' ? data.dueDate : data.estimateDueDate
  const values: Record<string, string> = {
    ...data.customer.values, ...data.documentValues,
    invoice_id: data.invoiceNumber, estimate_id: data.estimateNumber, invoice_date: documentDate(data.invoiceDate, language),
    estimate_date: documentDate(data.invoiceDate, language), due_date: documentDate(due, language), reference: data.reference,
  }
  const value = (field: string) => (values[field] ? [values[field]] : [])
  const labelOf = (field: string) => fieldLabel(field, dt, names)
  const meta = (lines: Line[]) =>
    lines.filter((l) => l.field !== 'newline' && values[l.field]).map((l) => ({ field: l.field, label: labelOf(l.field), value: values[l.field] ?? '', bold: l.bold, italic: l.italic }))
  const left = meta(layout[`${kind}_metadata_left`])
  const right = meta(layout[`${kind}_metadata_right`])
  const title = kind === 'invoice' ? `${dt('invoices.document.invoice')} ${number}` : `${dt('documentStyle.sample.quote')} ${number}`
  const lines = data.lines.map((l) => ({
    quantity: `${l.quantity} x`, description: l.description, price: money(l.price), total: money(l.quantity * l.price), vat: `${l.percentage}%`,
  }))
  const iban = sender?.bank_account_number
  const width = Math.min(Math.max(layout.sender_address_width, 100), 400)

  return (
    <div className="style-preview__sheet" style={{ fontFamily: fontStacks[layout.font_family] }}>
      <Paper label={label} fontSize={layout.font_size}>
        <div
          className={`invoice-paper__header invoice-paper__header--${layout.address_position}${layout.logo_container_full_width ? ' style-preview__header--full-logo' : ''}`}
          style={{ gridTemplateColumns: `minmax(0, 1fr) ${((width / pageWidth) * 100).toFixed(1)}%` }}
        >
          <div className={`invoice-paper__logo invoice-paper__logo--${layout.logo_position}`}>
            {style.logo_hash && (
              <img src={styleFileURL(administrationId, style.id ?? '', 'logo', style.logo_hash)} alt="" style={{ width: `${layout.logo_display_width}%` }} />
            )}
          </div>
          <div className="invoice-paper__recipient">
            <span className="invoice-paper__recipient-text">
              <span className="invoice-paper__line invoice-paper__bold">{data.customer.name}</span>
              {data.customer.address.map((l, i) => <span key={i} className="invoice-paper__line">{l}</span>)}
            </span>
            <Block
              className="style-preview__recipient-info"
              label={t(`documentStyle.sections.${kind}Recipient`)}
              items={entries(layout[`${kind}_recipient_address`], value, (f) => fieldLabel(f, dt, names))}
            />
          </div>
          {!layout.hide_sender_address && (
            <Block
              className="invoice-paper__sender"
              label={dt('invoices.document.senderAddress')}
              items={entries(layout[`${kind}_sender_address`], (f) => senderText(f, sender), (f) => fieldLabel(f, dt, names))}
            />
          )}
        </div>
        <div className="invoice-paper__heading">
          <div className="invoice-paper__heading-text">
            <h2 className="invoice-paper__title">{title}</h2>
            {left.map((m) => (
              <p key={m.field} className={['style-preview__meta', m.bold && 'invoice-paper__bold', m.italic && 'style-preview__italic'].filter(Boolean).join(' ')}>
                <span>{m.label}:</span> <span>{m.value}</span>
              </p>
            ))}
          </div>
          <div className="invoice-paper__dates">
            {right.map((m) => <DateView key={m.field} label={m.label} value={m.value} />)}
          </div>
        </div>
        <div className="invoice-paper__details">
          <DetailsView dt={dt} lines={lines} empty={lines[0] ?? { quantity: '', description: '', price: '', total: '', vat: '' }} />
          <TotalsTable
            rows={[
              { label: dt('invoices.document.subtotal'), amount: money(data.totals.subtotal) },
              ...data.totals.vat.map((v) => ({ label: `${v.percentage}% ${dt('invoices.document.vat')}`, amount: money(v.amount) })),
              { label: dt('invoices.document.total'), amount: money(data.totals.total), strong: true },
            ]}
          />
        </div>
        {kind === 'invoice' && (
          <div className="style-preview__payment">
            <div>
              {data.paymentConditions && <p>{data.paymentConditions}</p>}
              {iban && (
                <p>
                  {dt('documentStyle.payBefore', {
                    amount: money(data.totals.total), date: documentDate(data.dueDate, language), iban,
                    name: sender?.bank_account_name || sender?.company_name || '', reference: data.paymentReference,
                  })}
                </p>
              )}
            </div>
            {layout.show_epc_code && iban && data.currency === 'EUR' && <QrMark label={dt('documentStyle.qrCode')} />}
          </div>
        )}
      </Paper>
    </div>
  )
}

/** Stands in for the payment QR code, which the PDF draws from the invoice's real amount. */
function QrMark({ label }: { label: string }) {
  const cells = [0, 1, 2, 4, 6, 8, 9, 12, 13, 15, 17, 19, 20, 22, 24]
  return (
    <svg className="style-preview__qr" viewBox="0 0 25 25" role="img" aria-label={label}>
      <rect width="25" height="25" fill="#fff" />
      {[[0, 0], [18, 0], [0, 18]].map(([x, y]) => (
        <g key={`${x}-${y}`}>
          <rect x={x} y={y} width="7" height="7" fill="#000" />
          <rect x={(x ?? 0) + 1} y={(y ?? 0) + 1} width="5" height="5" fill="#fff" />
          <rect x={(x ?? 0) + 2} y={(y ?? 0) + 2} width="3" height="3" fill="#000" />
        </g>
      ))}
      {cells.map((c) => <rect key={c} x={8 + (c % 9)} y={8 + Math.floor(c / 5) * 2} width="1" height="1" fill="#000" />)}
    </svg>
  )
}
