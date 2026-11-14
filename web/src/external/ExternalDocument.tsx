import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ContactHoverCard } from '../contacts/ContactHoverCard'
import { formatAmount } from '../banking/queries'
import type { TaxRate } from '../bookkeeping/queries'
import { parseCents, parseQuantity } from '../invoices/totals'
import { type ExternalInvoice, externalTitle } from './queries'

type Props = {
  administrationId: string
  invoice: ExternalInvoice
  rates: TaxRate[]
  /** The category whose lines are marked, as when pointing at its label in the sidebar. */
  highlight?: string
}

/** "9%" of a rate's percentage such as "9.0". */
const percentage = (rate: TaxRate | undefined) => (rate ? `${Number(rate.percentage ?? 0).toLocaleString('en', { maximumFractionDigits: 3 })}%` : '')

/** A line's total as typed: its price times its amount, in cents. */
function lineTotal(price: string | undefined, amount: string | undefined) {
  const cents = parseCents(String(Number(price ?? 0)))
  const quantity = parseQuantity(amount ?? '1')
  return cents === null || quantity === null ? 0 : Math.round((cents * quantity) / 1000)
}

/**
 * An external invoice as Moneybird shows it: not a rendered invoice, but a
 * summary of what it is, its reference, contact, dates and source, over a
 * table of its lines with the subtotal, VAT and total.
 */
export function ExternalDocument({ administrationId, invoice: inv, rates, highlight }: Props) {
  const { t, i18n } = useTranslation()
  const money = (v: string | number | null | undefined) => formatAmount(v, inv.currency ?? 'EUR', i18n.language)
  const rate = (id: string | number | null | undefined) => rates.find((r) => r.id === String(id ?? ''))
  const lines = [...(inv.details ?? [])].sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
  const source = inv.source_url
    ? <a href={inv.source_url} target="_blank" rel="noreferrer">{inv.source || inv.source_url}</a>
    : inv.source
  const facts: [string, ReactNode][] = [
    [t('external.document.contact'), inv.contact && <ContactHoverCard administrationId={administrationId} contact={{ ...inv.contact, id: String(inv.contact_id ?? '') }} />],
    [t('external.document.invoiceDate'), inv.date],
    [t('external.document.dueDate'), inv.due_date],
    [t('external.document.source'), source],
  ]
  return (
    <article className="external-document" aria-label={t('external.externalInvoice')}>
      <div className="external-document__summary">
        <p className="external-document__type">{t('external.externalInvoice')}</p>
        <h1 className="external-document__title" title={externalTitle(inv, t)}>{externalTitle(inv, t)}</h1>
        <ul className="external-document__meta" aria-label={t('external.document.facts')}>
          {facts.filter(([, value]) => value).map(([label, value]) => (
            <li key={label} className="external-document__item">
              <span className="external-document__label">{label}</span>
              <span className="external-document__value">{value}</span>
            </li>
          ))}
        </ul>
      </div>
      <table className="external-document__details" aria-label={t('external.document.lines')}>
        <thead>
          <tr>
            <th scope="col" className="external-document__amount" aria-label={t('invoices.quantity')} />
            <th scope="col">{t('external.document.description')}</th>
            <th scope="col" className="external-document__price">{t('external.document.price')}</th>
            <th scope="col" className="external-document__price">{t('external.document.total')}</th>
            <th scope="col" className="external-document__price">{t('external.document.vat')}</th>
          </tr>
        </thead>
        <tbody>
          {lines.map((d) => (
            <tr key={d.id} className={highlight && d.ledger_account_id === highlight ? 'is-highlighted' : undefined}>
              <td className="external-document__amount">{d.amount}</td>
              <td className="external-document__description">{d.description}</td>
              <td className="external-document__price">{money(d.price)}</td>
              <td className="external-document__price">{money(lineTotal(d.price, d.amount ?? undefined) / 100)}</td>
              <td className="external-document__price">{percentage(rate(d.tax_rate_id))}</td>
            </tr>
          ))}
        </tbody>
        <tfoot>
          <tr className="external-document__subtotal">
            <td colSpan={2} />
            <th scope="row">{t('external.document.subtotal')}</th>
            <td className="external-document__price">{money(inv.total_price_excl_tax)}</td>
            <td />
          </tr>
          {(inv.tax_totals ?? []).map((r) => (
            <tr key={String(r.tax_rate_id)} className="external-document__tax">
              <td colSpan={2} />
              <th scope="row">{rate(r.tax_rate_id)?.name ?? ''}</th>
              <td className="external-document__price">{money(r.tax_amount)}</td>
              <td />
            </tr>
          ))}
          <tr className="external-document__total">
            <td colSpan={2} />
            <th scope="row">{t('external.document.total')}</th>
            <td className="external-document__price">{money(inv.total_price_incl_tax)}</td>
            <td />
          </tr>
        </tfoot>
      </table>
    </article>
  )
}
