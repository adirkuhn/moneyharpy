import { useTranslation } from 'react-i18next'
import { useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { Loading } from '../layout/Loading'
import { type InvoiceKind, type JournalEntry, type JournalSource, useInvoiceJournalEntries } from './queries'

/** Categories of the profit and loss statement; every other kind is on the balance sheet. */
const profitAndLoss = ['revenue', 'direct_costs', 'expenses', 'other_income_expenses']

type Group = { key: string; document: JournalEntry['document_type']; date: string; entries: JournalEntry[] }

/** The entries by document and day, as Moneybird heads them: "Invoice - 2026-11-09", then "Payment - 2026-11-11". */
export function groupEntries(entries: JournalEntry[]): Group[] {
  const groups: Group[] = []
  for (const e of entries) {
    const key = `${e.document_type}/${e.document_id}/${e.date}`
    const last = groups.at(-1)
    if (last?.key === key) {
      last.entries.push(e)
    } else {
      groups.push({ key, document: e.document_type, date: e.date, entries: [e] })
    }
  }
  return groups
}

/**
 * Moneybird's Journal entries of an invoice: the bookings MoneyHarpy made
 * for it in the background, by document, with their category, debit,
 * credit and VAT rate.
 */
export function JournalEntriesPanel({ administrationId, invoice, kind = 'sales' }: { administrationId: string; invoice: JournalSource; kind?: InvoiceKind }) {
  const { t, i18n } = useTranslation()
  const entries = useInvoiceJournalEntries(administrationId, invoice, true, kind)
  const accounts = useLedgerAccounts(administrationId)
  const rates = useTaxRates(administrationId)
  const number = (v: number) => v.toLocaleString(i18n.language, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  const account = (id: string) => accounts.data?.find((a) => a.id === id)
  const rate = (id: string | null | undefined) => (id ? rates.data?.find((r) => r.id === id)?.name ?? '' : '')

  return (
    <section className="journal-panel" aria-labelledby="journal-entries-title">
      <div className="journal-panel__intro">
        <h3 id="journal-entries-title" className="journal-panel__title">{t('invoices.journal.title')}</h3>
        <p className="journal-panel__text">{t('invoices.journal.intro')}</p>
      </div>
      {!entries.data ? (
        entries.error ? <p className="journal-panel__empty">{t('invoices.journal.failed')}</p> : <Loading />
      ) : entries.data.length === 0 ? (
        <p className="journal-panel__empty">{t('invoices.journal.empty')}</p>
      ) : (
        <table className="journal-panel__table" aria-label={t('invoices.journal.title')}>
          <thead>
            <tr>
              <th scope="col">{t('invoices.journal.category')}</th>
              <th scope="col" className="journal-panel__amount">{t('invoices.journal.debit')}</th>
              <th scope="col" className="journal-panel__amount">{t('invoices.journal.credit')}</th>
              <th scope="col">{t('invoices.journal.vat')}</th>
            </tr>
          </thead>
          {groupEntries(entries.data).map((g) => (
            <tbody key={g.key}>
              <tr className="journal-panel__group">
                <th scope="colgroup" colSpan={4}>{t(`invoices.journal.documents.${g.document}`, { defaultValue: g.document })} - {g.date}</th>
              </tr>
              {g.entries.map((e) => {
                const amount = Number(e.amount)
                const category = account(e.ledger_account_id)
                const statement = category && t(profitAndLoss.includes(category.account_type ?? '') ? 'invoices.journal.profitAndLoss' : 'invoices.journal.balanceSheet')
                return (
                  <tr key={e.id}>
                    <td className="journal-panel__category" title={category ? `${category.name} (${statement})` : undefined}>
                      {category?.name ?? ''} {statement && <span className="journal-panel__quiet">({statement})</span>}
                    </td>
                    <td className="journal-panel__amount">{amount > 0 ? number(amount) : ''}</td>
                    <td className="journal-panel__amount">{amount < 0 ? number(-amount) : ''}</td>
                    <td>{rate(e.tax_rate_id == null ? null : String(e.tax_rate_id))}</td>
                  </tr>
                )
              })}
            </tbody>
          ))}
        </table>
      )}
    </section>
  )
}
