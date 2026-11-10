import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { formatAmount } from '../banking/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { Pagination } from '../layout/Pagination'
import { type JournalEntry, reportPeriods, useLedgerEntries } from './queries'

/** The bookings on one category in a period, from every kind of document. */
export function LedgerEntriesPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search, setSearch] = useSearchParams()
  const ledgerAccountId = search.get('ledger_account_id') ?? ''
  const period = (reportPeriods as readonly string[]).includes(search.get('period') ?? '') ? search.get('period')! : 'this_month'
  const page = Math.max(1, Number(search.get('page')) || 1)
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const entries = useLedgerEntries(administrationId, ledgerAccountId, period, page)
  const account = accounts.data?.find((a) => a.id === ledgerAccountId)
  const currency = administration.data?.currency ?? 'EUR'
  const money = (n: string | number) => formatAmount(n, currency, i18n.language)
  const date = (d: string) => new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })
  const back = `/${administrationId}/report/general_ledger?period=${period}`

  return (
    <Page title={account?.name ?? t('journal.bookings')} subtitle={t('journal.bookingsSubtitle')} back={{ to: back, label: t('journal.backToTrialBalance') }}>
      <div className="filter-bar" role="search" aria-label={t('journal.filters')}>
        <label className="filter-bar__field">
          <span>{t('journal.period')}</span>
          <select value={period} onChange={(e) => setSearch({ ledger_account_id: ledgerAccountId, period: e.target.value }, { replace: true })}>
            {reportPeriods.map((p) => <option key={p} value={p}>{t(`journal.periods.${p}`)}</option>)}
          </select>
        </label>
      </div>
      {!entries.data ? (
        entries.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />
      ) : entries.data.entries.length === 0 ? (
        <p className="empty">{t('journal.noBookings')}</p>
      ) : (
        <>
          <table className="data-table ledger-report" aria-label={t('journal.bookings')}>
            <thead>
              <tr>
                <th scope="col" className="transactions__date">{t('journal.date')}</th>
                <th scope="col">{t('journal.description')}</th>
                <th scope="col" className="data-table__extra">{t('journal.document')}</th>
                <th scope="col" className="ledger-report__amount">{t('journal.debit')}</th>
                <th scope="col" className="ledger-report__amount">{t('journal.credit')}</th>
              </tr>
            </thead>
            <tbody>
              {entries.data.entries.map((e) => {
                const amount = Number(e.amount)
                return (
                  <tr key={e.id}>
                    <td className="transactions__date">{date(e.date)}</td>
                    <td>{e.description}</td>
                    <td className="data-table__extra"><DocumentLink administrationId={administrationId} entry={e} /></td>
                    <td className="ledger-report__amount">{amount > 0 ? money(amount) : ''}</td>
                    <td className="ledger-report__amount">{amount < 0 ? money(-amount) : ''}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
          <Pagination total={entries.data.total} perPage={100} />
        </>
      )}
    </Page>
  )
}

const documentPaths: Record<string, string> = {
  FinancialMutation: 'financial_mutations',
  GeneralJournalDocument: 'documents/general_journal_documents',
  SalesInvoice: 'sales_invoices',
  ExternalSalesInvoice: 'external_sales_invoices',
}

function DocumentLink({ administrationId, entry }: { administrationId: string; entry: JournalEntry }) {
  const { t } = useTranslation()
  const label = t(`journal.documentTypes.${entry.document_type}`)
  const to = documentPaths[entry.document_type]
  return to ? <Link to={`/${administrationId}/${to}/${entry.document_id}`}>{label}</Link> : <>{label}</>
}
