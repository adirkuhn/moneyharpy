import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable, listDate } from '../list/DocumentTable'
import { FilterPanel } from '../list/FilterPanel'
import { ListActions } from '../list/ListActions'
import { duration, perPage, type TimeEntry, type TimeState, timeStates, useTimeEntries, workedMinutes } from './queries'

/** The time recorded this year, newest first and paged (/time_entries). */
export function TimeEntriesPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const asked = params.get('state') ?? ''
  const state: TimeState = (timeStates as readonly string[]).includes(asked) ? (asked as TimeState) : 'all'
  const page = Math.max(1, Number(params.get('page')) || 1)
  const entries = useTimeEntries(administrationId, state, page)
  const base = `/${administrationId}/time_entries`
  const shown = entries.data?.entries ?? []
  const quick = timeStates.map((s) => ({ label: t(`timeEntries.views.${s}`), to: s === 'all' ? base : `${base}?state=${s}`, current: s === state }))

  return (
    <Page
      wide
      title={t('timeEntries.title')}
      lead={<FilterPanel label={t(`timeEntries.views.${state}`)} title={t('timeEntries.filters')} quick={quick} />}
      actions={<ListActions exports={[]} primary={{ to: `${base}/new`, label: t('timeEntries.add') }} />}
    >
      {!entries.data ? (
        entries.error ? <p className="empty">{t('timeEntries.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{state === 'all' && page === 1 ? t('timeEntries.empty') : t('timeEntries.noMatches')}</p>
      ) : (
        <DocumentTable<TimeEntry>
          label={t('timeEntries.title')}
          rows={shown}
          rowKey={(e) => e.id ?? ''}
          href={(e) => `${base}/${e.id}`}
          total={entries.data.total}
          perPage={perPage}
          columns={[
            { key: 'date', header: t('timeEntries.date'), kind: 'date', cell: (e) => listDate(e.started_at) },
            { key: 'description', header: t('timeEntries.description'), link: true, cell: (e) => e.description },
            { key: 'contact', header: t('timeEntries.contact'), kind: 'extra', cell: (e) => contactName(e) },
            { key: 'project', header: t('timeEntries.project'), kind: 'extra', cell: (e) => e.project?.name ?? '' },
            { key: 'duration', header: t('timeEntries.duration'), kind: 'amount', cell: (e) => duration(workedMinutes(e)) },
            { key: 'state', header: t('timeEntries.status'), kind: 'state', cell: (e) => <TimeBadge entry={e} /> },
          ]}
        />
      )}
    </Page>
  )
}

export function contactName(entry: TimeEntry): string {
  const c = entry.contact
  return c ? c.company_name || [c.firstname, c.lastname].filter(Boolean).join(' ') : ''
}

/** Whether the time runs, is billed, open to bill or not billable. */
export function TimeBadge({ entry }: { entry: TimeEntry }) {
  const { t } = useTranslation()
  const state = !entry.ended_at ? 'running' : entry.sales_invoice_id ? 'invoiced' : entry.billable ? 'open' : 'nonBillable'
  const look = { running: 'open', invoiced: 'paid', open: 'pending', nonBillable: 'archived' }[state]
  return <span className={`badge badge--${look}`}>{t(`timeEntries.states.${state}`)}</span>
}
