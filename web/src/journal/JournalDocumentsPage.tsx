import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable, listDate } from '../list/DocumentTable'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
import { allPeriods, PeriodFilter } from '../list/PeriodFilter'
import { isPeriod } from '../list/period'
import { documentsPerPage, reportPeriods, useJournalDocuments } from './queries'

const defaultPeriod = 'this_year'

/** The journal entries made by hand, newest first, in a period (/documents/general_journal_documents). */
export function JournalDocumentsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search, setSearch] = useSearchParams()
  const today = useMemo(() => new Date(), [])
  const asked = search.get('period') ?? ''
  const period = asked === allPeriods || isPeriod(asked, today) ? asked : defaultPeriod
  const page = Math.max(1, Number(search.get('page')) || 1)
  const administration = useAdministration(administrationId)
  const docs = useJournalDocuments(administrationId, period, page)
  const [info, setInfo] = useState<boolean | null>(null)
  const introId = useId()
  const currency = administration.data?.currency ?? 'EUR'
  const base = `/${administrationId}/documents/general_journal_documents`
  const shown = docs.data?.documents ?? []
  const firstUse = docs.data !== undefined && shown.length === 0 && period === defaultPeriod && page === 1
  const introShown = info ?? firstUse
  const linkTo = (p: string) => (p === defaultPeriod ? base : `${base}?period=${p}`)

  return (
    <Page
      wide
      title={t('journal.entries')}
      lead={
        <PeriodFilter
          allowAll
          period={period}
          today={today}
          title={t('journal.entriesFilters')}
          quick={reportPeriods.map((p) => ({ value: p, label: t(`journal.periods.${p}`) }))}
          linkTo={linkTo}
          onApply={(p) => setSearch(p === defaultPeriod ? {} : { period: p }, { replace: true })}
        />
      }
      actions={
        <ListActions
          info={{ shown: introShown, onToggle: () => setInfo(!introShown), controls: introId }}
          exports={[]}
          primary={{ to: `${base}/new`, label: t('journal.newEntry') }}
        />
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('journal.intro.title')}
          text={t('journal.intro.text')}
          steps={[
            { icon: 'repeat', title: t('journal.intro.correct'), text: t('journal.intro.correctText') },
            { icon: 'coins', title: t('journal.intro.private'), text: t('journal.intro.privateText') },
            { icon: 'chart', title: t('journal.intro.check'), text: t('journal.intro.checkText') },
          ]}
        />
      )}
      {!docs.data ? (
        docs.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{t('journal.noEntries')}</p>
      ) : (
        <DocumentTable
          label={t('journal.entries')}
          rows={shown}
          rowKey={(d) => d.id ?? ''}
          href={(d) => `${base}/${d.id}`}
          total={docs.data.total}
          perPage={documentsPerPage}
          columns={[
            { key: 'date', header: t('journal.date'), kind: 'date', cell: (d) => listDate(d.date) },
            { key: 'reference', header: t('journal.reference'), link: true, cell: (d) => d.reference },
            {
              key: 'amount', header: t('journal.amount'), kind: 'amount',
              cell: (d) => formatAmount((d.general_journal_document_entries ?? []).reduce((sum, r) => sum + Number(r.debit ?? 0), 0), currency, i18n.language),
            },
          ]}
        />
      )}
    </Page>
  )
}
