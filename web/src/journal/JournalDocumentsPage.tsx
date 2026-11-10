import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { Pagination } from '../layout/Pagination'
import { documentsPerPage, reportPeriods, useJournalDocuments } from './queries'

/** The journal entries made by hand, newest first. */
export function JournalDocumentsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search, setSearch] = useSearchParams()
  const period = (reportPeriods as readonly string[]).includes(search.get('period') ?? '') ? search.get('period')! : 'this_year'
  const page = Math.max(1, Number(search.get('page')) || 1)
  const administration = useAdministration(administrationId)
  const docs = useJournalDocuments(administrationId, period, page)
  const currency = administration.data?.currency ?? 'EUR'
  const date = (d: string) => new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })
  const base = `/${administrationId}/documents/general_journal_documents`

  return (
    <Page title={t('journal.entries')} subtitle={t('journal.entriesSubtitle')} actions={<Link className="btn btn--accent" to={`${base}/new`}>{t('journal.newEntry')}</Link>}>
      <div className="filter-bar" role="search" aria-label={t('journal.filters')}>
        <label className="filter-bar__field">
          <span>{t('journal.period')}</span>
          <select value={period} onChange={(e) => setSearch({ period: e.target.value }, { replace: true })}>
            {reportPeriods.map((p) => <option key={p} value={p}>{t(`journal.periods.${p}`)}</option>)}
          </select>
        </label>
      </div>
      {!docs.data ? (
        docs.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />
      ) : docs.data.documents.length === 0 ? (
        <p className="empty">{t('journal.noEntries')}</p>
      ) : (
        <>
          <table className="data-table" aria-label={t('journal.entries')}>
            <thead>
              <tr>
                <th scope="col" className="transactions__date">{t('journal.date')}</th>
                <th scope="col">{t('journal.reference')}</th>
                <th scope="col" className="ledger-report__amount">{t('journal.amount')}</th>
              </tr>
            </thead>
            <tbody>
              {docs.data.documents.map((d) => (
                <tr key={d.id}>
                  <td className="transactions__date">{date(d.date ?? '')}</td>
                  <td><Link to={`${base}/${d.id}`}>{d.reference}</Link></td>
                  <td className="ledger-report__amount">
                    {formatAmount((d.general_journal_document_entries ?? []).reduce((sum, r) => sum + Number(r.debit ?? 0), 0), currency, i18n.language)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pagination total={docs.data.total} perPage={documentsPerPage} />
        </>
      )}
    </Page>
  )
}
