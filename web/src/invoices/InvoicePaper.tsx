import type { TFunction } from 'i18next'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ListIcon } from '../layout/icons'
import { type SenderEntry } from './paper'

/**
 * The parts of an invoice drawn as its document, after Moneybird's editor.
 * Printed words come from `dt`, the document's language; what only the
 * editor shows (steps, placeholders, control names) uses the user's language.
 */

/** The sheet itself. */
export function Paper({ label, fontSize, children }: { label: string; fontSize?: number; children: ReactNode }) {
  return (
    <div className="invoice-paper" role="document" aria-label={label} style={fontSize ? { fontSize: `${Math.min(Math.max(fontSize, 8), 14) / 10}em` } : undefined}>
      {children}
    </div>
  )
}

/** The sender block, top right: each field of the style's layout, blank lines kept. */
export function SenderAddress({ entries, dt }: { entries: SenderEntry[]; dt: TFunction }) {
  return (
    <ul className="invoice-paper__sender" aria-label={dt('invoices.document.senderAddress')}>
      {entries.map((e, i) =>
        'text' in e ? (
          <li key={i} className={e.bold ? 'invoice-paper__bold' : undefined}>
            {e.label && <span>{dt(`invoices.document.fields.${e.field}`)}: </span>}
            {e.text.map((line, j) => <span key={j} className={j > 0 ? 'invoice-paper__line' : undefined}>{line}</span>)}
          </li>
        ) : (
          <li key={i} className="invoice-paper__newline" />
        ),
      )}
    </ul>
  )
}

/** What the recipient block shows: the contact's name and address, or Moneybird's placeholder and first step. */
export function RecipientContent({ lines, chosen }: { lines: string[]; chosen: boolean }) {
  const { t } = useTranslation()
  const shown = chosen ? lines : [t('invoices.editor.placeholderName'), t('invoices.editor.placeholderAddress'), t('invoices.editor.placeholderCity')]
  return (
    <>
      {!chosen && <span className="invoice-paper__step">{t('invoices.editor.selectContact')}</span>}
      <span className={`invoice-paper__recipient-text${chosen ? '' : ' is-placeholder'}`}>
        {shown.map((l, i) => <span key={i} className="invoice-paper__line">{l}</span>)}
      </span>
      <span className="invoice-paper__edit"><ListIcon kind="edit" /></span>
    </>
  )
}

/** A label and value of the dates block on the right of the title, such as "Due date: 25-11-2026". */
export function DateView({ label, value, tentative }: { label: string; value: string; tentative?: boolean }) {
  return (
    <span className="invoice-paper__date">
      <span className="invoice-paper__date-label">{label}:</span>
      <span className={tentative ? 'invoice-paper__draft' : undefined}>{value}</span>
    </span>
  )
}

export type ShownLine = { quantity: string; description: string; price: string; total: string; vat: string }

type DetailsProps = {
  dt: TFunction
  lines: ShownLine[]
  /** The empty state's single row, as Moneybird shows it before any details are added. */
  empty: ShownLine
  onEdit: () => void
}

/** The lines as the document prints them; a click on them opens them for editing. */
export function DetailsView({ dt, lines, empty, onEdit }: DetailsProps) {
  const { t } = useTranslation()
  const shown = lines.length ? lines : [empty]
  return (
    <div className={`invoice-paper__details-view${lines.length ? '' : ' is-empty'}`} onClick={onEdit}>
      <button type="button" className={lines.length ? 'invoice-paper__edit-details' : 'invoice-paper__step'} onClick={(e) => { e.stopPropagation(); onEdit() }}>
        {lines.length ? <><ListIcon kind="edit" /><span className="sr-only">{t('invoices.editor.editDetails')}</span></> : t('invoices.editor.addDetails')}
      </button>
      <table className="invoice-paper__lines" aria-label={dt('invoices.document.lines')}>
        <thead>
          <tr>
            <th scope="col"><span className="sr-only">{dt('invoices.document.quantity')}</span></th>
            <th scope="col">{dt('invoices.document.description')}</th>
            <th scope="col" className="invoice-paper__money">{dt('invoices.document.price')}</th>
            <th scope="col" className="invoice-paper__money">{dt('invoices.document.total')}</th>
            <th scope="col" className="invoice-paper__money">{dt('invoices.document.vat')}</th>
          </tr>
        </thead>
        <tbody>
          {shown.map((l, i) => (
            <tr key={i} className={lines.length ? undefined : 'is-placeholder'}>
              <td className="invoice-paper__quantity">{l.quantity}</td>
              <td className="invoice-paper__description">{l.description}</td>
              <td className="invoice-paper__money">{l.price}</td>
              <td className="invoice-paper__money">{l.total}</td>
              <td className="invoice-paper__money">{l.vat}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
