import { useTranslation } from 'react-i18next'
import { bookable } from '../banking/queries'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import type { FormErrors } from '../forms/Form'
import { categoryOption } from '../bookkeeping/queries'

export type LineRow = { key: string; id?: string; description: string; amount: string; price: string; tax_rate_id: string; ledger_account_id: string }

type Props = {
  rows: LineRow[]
  rates: TaxRate[]
  accounts: LedgerAccount[]
  /** Each line's amount, formatted, in the order of rows. */
  amounts: string[]
  /** Whether each line's price and quantity can be read. */
  invalid: { price: boolean; amount: boolean }[]
  errors: FormErrors
  onChange: (index: number, change: Partial<LineRow>) => void
  onRemove: (index: number) => void
  onAdd: () => void
}

/** The lines of an invoice as an editable table, after Moneybird's editor. */
export function LinesEditor({ rows, rates, accounts, amounts, invalid, errors, onChange, onRemove, onAdd }: Props) {
  const { t } = useTranslation()
  const groups = bookable(accounts, 'sales_invoice')
  const cell = (i: number, field: keyof LineRow, invalidInput = false) => {
    const problems = errors.field(`details_attributes.${i}.${field}`)
    const id = `line-${i}-${field}-message`
    return {
      control: {
        'aria-invalid': invalidInput || !!problems || undefined,
        'aria-describedby': problems ? id : undefined,
      },
      message: problems && <p id={id} className="form-input__message form-input__message--error">{problems.join(' ')}</p>,
    }
  }

  return (
    <>
      <table className="data-table invoice-lines" aria-label={t('invoices.lines')}>
        <thead>
          <tr>
            <th scope="col" className="invoice-lines__description">{t('invoices.description')}</th>
            <th scope="col" className="invoice-lines__quantity">{t('invoices.quantity')}</th>
            <th scope="col" className="invoice-lines__price">{t('invoices.price')}</th>
            <th scope="col" className="invoice-lines__rate">{t('invoices.vatRate')}</th>
            <th scope="col" className="invoice-lines__category">{t('invoices.category')}</th>
            <th scope="col" className="invoice-lines__amount">{t('invoices.amount')}</th>
            <th scope="col" className="invoice-lines__remove"><span className="sr-only">{t('invoices.actions')}</span></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => {
            const description = cell(i, 'description')
            const amount = cell(i, 'amount', invalid[i]?.amount)
            const price = cell(i, 'price', invalid[i]?.price)
            const rate = cell(i, 'tax_rate_id')
            const category = cell(i, 'ledger_account_id')
            return (
              <tr key={r.key}>
                <td className="invoice-lines__description" data-label={t('invoices.description')}>
                  <textarea rows={2} aria-label={t('invoices.description')} maxLength={10000} value={r.description} {...description.control} onChange={(e) => onChange(i, { description: e.target.value })} />
                  {description.message}
                </td>
                <td className="invoice-lines__quantity" data-label={t('invoices.quantity')}>
                  <input type="text" aria-label={t('invoices.quantity')} maxLength={255} value={r.amount} {...amount.control} onChange={(e) => onChange(i, { amount: e.target.value })} />
                  {amount.message}
                </td>
                <td className="invoice-lines__price" data-label={t('invoices.price')}>
                  <input type="text" inputMode="decimal" aria-label={t('invoices.price')} value={r.price} {...price.control} onChange={(e) => onChange(i, { price: e.target.value })} />
                  {price.message}
                </td>
                <td className="invoice-lines__rate" data-label={t('invoices.vatRate')}>
                  <select aria-label={t('invoices.vatRate')} value={r.tax_rate_id} {...rate.control} onChange={(e) => onChange(i, { tax_rate_id: e.target.value })}>
                    {rates.map((tr) => <option key={tr.id} value={tr.id}>{tr.name}</option>)}
                  </select>
                  {rate.message}
                </td>
                <td className="invoice-lines__category" data-label={t('invoices.category')}>
                  <select aria-label={t('invoices.category')} value={r.ledger_account_id} {...category.control} onChange={(e) => onChange(i, { ledger_account_id: e.target.value })}>
                    <option value="">{t('invoices.uncategorized')}</option>
                    {groups.map((g) => (
                      <optgroup key={g.type} label={t(`bookkeeping.accountTypes.${g.type}`)}>
                        {g.rows.map(({ account: a, depth }) => (
                          <option key={a.id} value={a.id}>{categoryOption({ account: a, depth })}</option>
                        ))}
                      </optgroup>
                    ))}
                  </select>
                  {category.message}
                </td>
                <td className="invoice-lines__amount" data-label={t('invoices.amount')}>{amounts[i]}</td>
                <td className="invoice-lines__remove">
                  {rows.length > 1 && (
                    <button type="button" className="btn btn--secondary btn--small invoice-lines__remove-button" aria-label={t('invoices.removeLine', { n: i + 1 })} title={t('invoices.removeLine', { n: i + 1 })} onClick={() => onRemove(i)}>
                      <span aria-hidden="true">×</span>
                    </button>
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      <button type="button" className="icon-link icon-link--button" onClick={onAdd}>{t('invoices.addLine')}</button>
    </>
  )
}
