import { useTranslation } from 'react-i18next'
import type { InvoiceState } from './queries'

/** The state of an invoice as a coloured label, after Moneybird. */
export function StateBadge({ state }: { state: InvoiceState | undefined }) {
  const { t } = useTranslation()
  const s = state ?? 'draft'
  return <span className={`badge badge--${s}`}>{t(`invoices.states.${s}`)}</span>
}
