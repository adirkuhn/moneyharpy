import { useTranslation } from 'react-i18next'

export type TotalRow = { label: string; amount: string; strong?: boolean }

/** The sums under an invoice's lines: subtotal, discount, VAT per rate and total. */
export function TotalsTable({ rows }: { rows: TotalRow[] }) {
  const { t } = useTranslation()
  return (
    <table className="invoice-totals" aria-label={t('invoices.totals')}>
      <tbody>
        {rows.map((r, i) => (
          // Labels can repeat while VAT rate names load, so rows are keyed by position.
          <tr key={i} className={r.strong ? 'invoice-totals__strong' : undefined}>
            <th scope="row">{r.label}</th>
            <td>{r.amount}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
