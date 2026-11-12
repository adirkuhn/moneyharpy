import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { formatAmount } from '../banking/queries'
import { DateView, DetailsView, Paper, RecipientContent, SenderAddress } from '../invoices/InvoicePaper'
import { addDays, documentDate, senderEntries, today } from '../invoices/paper'
import { TotalsTable } from '../invoices/TotalsTable'
import type { Identity } from '../settings/queries'
import { type DocumentStyle, type Line, styleFileURL } from './queries'

export type PreviewLayout = {
  logo_position: 'left' | 'center' | 'right'
  logo_display_width: number
  logo_container_full_width: boolean
  address_position: 'left' | 'right'
  font_family: string
  font_size: number
} & Record<'invoice_sender_address' | 'invoice_metadata_left' | 'invoice_metadata_right' | 'estimate_sender_address' | 'estimate_metadata_left' | 'estimate_metadata_right', Line[]>

const fontStacks: Record<string, string> = {
  lucida_grande: "'Lucida Grande', 'Lucida Sans Unicode', sans-serif", helvetica: 'Helvetica, Arial, sans-serif', arial: 'Arial, Helvetica, sans-serif',
  verdana: 'Verdana, Geneva, sans-serif', georgia: 'Georgia, serif', times_new_roman: "'Times New Roman', Times, serif", courier: "'Courier New', Courier, monospace",
}

type Props = {
  administrationId: string
  /** The saved style, for its logo. */
  style: DocumentStyle
  layout: PreviewLayout
  sender: Identity | undefined
  /** The language the preview prints in. */
  language: string
  invoiceNumber: string
  estimateNumber: string
}

/**
 * The style applied to an example invoice and quote, as Moneybird shows next
 * to its editor. It follows every change before it is saved.
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

function PreviewDocument({ administrationId, style, layout, sender, language, invoiceNumber, estimateNumber, dt, kind, label }: Props & { dt: TFunction; kind: 'invoice' | 'estimate'; label: string }) {
  const money = (v: number) => formatAmount(v, 'EUR', language)
  const date = today()
  const values: Record<string, string> = {
    invoice_id: invoiceNumber, estimate_id: estimateNumber, invoice_date: documentDate(date, language), estimate_date: documentDate(date, language),
    due_date: documentDate(addDays(date, 14), language), reference: dt('documentStyle.sample.reference'), customer_id: dt('documentStyle.sample.customerId'),
  }
  const meta = (lines: Line[]) => lines.filter((l) => l.field !== 'newline').map((l) => ({ label: dt(`documentStyle.fields.${l.field}`), value: values[l.field] ?? '', bold: l.bold }))
  const left = meta(layout[`${kind}_metadata_left`])
  const right = meta(layout[`${kind}_metadata_right`])
  const title = kind === 'invoice' ? `${dt('invoices.document.invoice')} ${invoiceNumber}` : `${dt('documentStyle.sample.quote')} ${estimateNumber}`
  const lines = [
    { quantity: '2 x', description: dt('documentStyle.sample.consultancy'), price: money(475.5), total: money(951), vat: '21%' },
    { quantity: '1 x', description: dt('documentStyle.sample.design'), price: money(1250), total: money(1250), vat: '21%' },
  ]

  return (
    <div className="style-preview__sheet" style={{ fontFamily: fontStacks[layout.font_family] }}>
      <Paper label={label} fontSize={layout.font_size}>
        <div className={`invoice-paper__header invoice-paper__header--${layout.address_position}${layout.logo_container_full_width ? ' style-preview__header--full-logo' : ''}`}>
          <div className={`invoice-paper__logo invoice-paper__logo--${layout.logo_position}`}>
            {style.logo_hash && (
              <img src={styleFileURL(administrationId, style.id ?? '', 'logo', style.logo_hash)} alt="" style={{ width: `${layout.logo_display_width}%` }} />
            )}
          </div>
          <div className="invoice-paper__recipient">
            <div className="invoice-paper__recipient-button is-empty">
              <RecipientContent chosen={false} lines={[]} editable={false} />
            </div>
          </div>
          <SenderAddress dt={dt} entries={senderEntries(layout[`${kind}_sender_address`], sender)} />
        </div>
        <div className="invoice-paper__heading">
          <div className="invoice-paper__heading-text">
            <h2 className="invoice-paper__title">{title}</h2>
            {left.map((m) => (
              <p key={m.label} className={`style-preview__meta${m.bold ? ' invoice-paper__bold' : ''}`}><span>{m.label}:</span> <span>{m.value}</span></p>
            ))}
          </div>
          <div className="invoice-paper__dates">
            {right.map((m) => <DateView key={m.label} label={m.label} value={m.value} />)}
          </div>
        </div>
        <div className="invoice-paper__details">
          <DetailsView dt={dt} lines={lines} empty={lines[0] as (typeof lines)[number]} />
          <TotalsTable
            rows={[
              { label: dt('invoices.document.subtotal'), amount: money(2201) },
              { label: '21% ' + dt('invoices.document.vat'), amount: money(462.21) },
              { label: dt('invoices.document.total'), amount: money(2663.21), strong: true },
            ]}
          />
        </div>
      </Paper>
    </div>
  )
}
