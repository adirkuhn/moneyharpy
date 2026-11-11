import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { formatAmount } from '../banking/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable, listDate } from '../list/DocumentTable'
import { MoreMenu } from '../list/ListActions'
import { PeriodFilter } from '../list/PeriodFilter'
import { isPeriod } from '../list/period'
import { type JournalEntry, reportPeriods, useLedgerEntries } from './queries'

/** The bookings on one category in a period, from every kind of document (/report/journal_entries). */
export function LedgerEntriesPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search, setSearch] = useSearchParams()
  const today = useMemo(() => new Date(), [])
  const ledgerAccountId = search.get('ledger_account_id') ?? ''
  const asked = search.get('period') ?? ''
  const period = isPeriod(asked, today) ? asked : 'this_month'
  const page = Math.max(1, Number(search.get('page')) || 1)
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const entries = useLedgerEntries(administrationId, ledgerAccountId, period, page)
  const account = accounts.data?.find((a) => a.id === ledgerAccountId)
  const currency = administration.data?.currency ?? 'EUR'
  const money = (n: string | number) => formatAmount(n, currency, i18n.language)
  const back = `/${administrationId}/report/general_ledger?period=${period}`
  const linkTo = (p: string) => `/${administrationId}/report/journal_entries?${new URLSearchParams({ ledger_account_id: ledgerAccountId, period: p })}`

  return (
    <Page
      wide
      title={account?.name ?? t('journal.bookings')}
      back={{ to: back, label: t('journal.backToTrialBalance') }}
      lead={
        <PeriodFilter
          period={period}
          today={today}
          title={t('journal.filters')}
          quick={reportPeriods.map((p) => ({ value: p, label: t(`journal.periods.${p}`) }))}
          linkTo={linkTo}
          onApply={(p) => setSearch({ ledger_account_id: ledgerAccountId, period: p }, { replace: true })}
        />
      }
      actions={<MoreMenu items={[{ to: `/${administrationId}/ledger_accounts/${ledgerAccountId}`, label: t('journal.categorySettings') }]} />}
    >
      <p className="list-intro">{t('journal.bookingsSubtitle')}</p>
      {!entries.data ? (
        entries.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />
      ) : entries.data.entries.length === 0 ? (
        <p className="empty">{t('journal.noBookings')}</p>
      ) : (
        <DocumentTable
          label={t('journal.bookings')}
          rows={entries.data.entries}
          rowKey={(e) => e.id}
          href={(e) => documentPath(administrationId, e)}
          total={entries.data.total}
          perPage={100}
          columns={[
            { key: 'date', header: t('journal.date'), kind: 'date', cell: (e) => listDate(e.date) },
            { key: 'description', header: t('journal.description'), cell: (e) => e.description },
            { key: 'document', header: t('journal.document'), link: true, cell: (e) => t(`journal.documentTypes.${e.document_type}`) },
            { key: 'debit', header: t('journal.debit'), kind: 'amount', cell: (e) => (Number(e.amount) > 0 ? money(e.amount) : '') },
            { key: 'credit', header: t('journal.credit'), kind: 'amount', cell: (e) => (Number(e.amount) < 0 ? money(-Number(e.amount)) : '') },
          ]}
        />
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

/** The page of the document a booking comes from, when it has one. */
function documentPath(administrationId: string, entry: JournalEntry) {
  const to = documentPaths[entry.document_type]
  return to ? `/${administrationId}/${to}/${entry.document_id}` : undefined
}
