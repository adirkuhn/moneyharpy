import { useTranslation } from 'react-i18next'

/** The state of an invoice, sales or external, as a coloured label, after Moneybird. */
export function StateBadge({ state }: { state: string | undefined }) {
  const { t } = useTranslation()
  const s = state ?? 'draft'
  return <span className={`badge badge--${s}`}>{t(`invoices.states.${s}`)}</span>
}
