import { Fragment, useEffect, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { contactName as nameOf, useContact } from '../contacts/queries'
import { InfoIcon, ListIcon, SearchIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { AdvancedFilter, ContactFilter, FilterPanel, RadioFilter } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ExportMenu, Unavailable } from '../list/ListActions'
import { allPeriods } from '../list/PeriodFilter'
import { isPeriod, parsePeriod, periodValue } from '../list/period'
import { PeriodPicker } from '../list/PeriodPicker'
import { contactName, TimeBadge } from './TimeEntriesPage'
import { type TimeEntry, timeStates, type TimeState, useReportEntries, workedMinutes } from './queries'
import { clock, dayOf, weekOf } from './week'
import './timesheets.css'

const defaultPeriod = 'this_year'

type Filter = { period: string; state: TimeState; contact: string }

/**
 * Moneybird's hours report (/time_entries_report): the time of everyone,
 * week by week with the hours of each week and in all, filtered by period,
 * status and contact and searched by description.
 */
export function HoursReportPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search, setSearch] = useSearchParams()
  const today = useMemo(() => new Date(), [])
  const administration = useAdministration(administrationId)
  const asked = search.get('period') ?? ''
  const period = asked === allPeriods || isPeriod(asked, today) ? asked : defaultPeriod
  const askedState = search.get('state') ?? ''
  const state: TimeState = (timeStates as readonly string[]).includes(askedState) ? (askedState as TimeState) : 'all'
  const contact = search.get('contact') ?? ''
  const [typed, setTyped] = useState(search.get('query') ?? '')
  const [query, setQuery] = useState(typed.trim())
  const filter = [
    ...(period === allPeriods ? [] : [`period:${period}`]), `state:${state}`, 'include_active:false', ...(contact ? [`contact_id:${contact}`] : []),
  ].join(',')
  const entries = useReportEntries(administrationId, filter, query)
  const chosen = useContact(administrationId, contact || undefined)
  const [info, setInfo] = useState(false)
  const infoId = useId()

  // A search waits for the typing to pause.
  useEffect(() => {
    const timer = setTimeout(() => setQuery(typed.trim()), 250)
    return () => clearTimeout(timer)
  }, [typed])

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const report = `/${administrationId}/time_entries_report`
  const address = (f: Filter) => {
    const next = new URLSearchParams()
    if (f.period !== defaultPeriod) next.set('period', f.period)
    if (f.state !== 'all') next.set('state', f.state)
    if (f.contact) next.set('contact', f.contact)
    return next
  }
  const linkTo = (f: Filter) => {
    const q = address(f).toString()
    return q ? `${report}?${q}` : report
  }
  const thisYear = period === defaultPeriod && state === 'all' && !contact
  const open = period === allPeriods && state === 'open' && !contact
  const label = thisYear ? t('timeEntries.report.views.thisYear')
    : open ? t('timeEntries.report.views.open')
    : contact && chosen.data ? t('timeEntries.report.views.contact', { name: nameOf(chosen.data) })
    : t('timeEntries.report.views.custom')

  const filterPanel = (
    <FilterPanel
      label={label}
      title={t('timeEntries.report.filters')}
      quick={[
        { label: t('timeEntries.report.views.thisYear'), to: linkTo({ period: defaultPeriod, state: 'all', contact: '' }), current: thisYear },
        { label: t('timeEntries.report.views.open'), to: linkTo({ period: allPeriods, state: 'open', contact: '' }), current: open },
      ]}
    >
      <ReportFilter
        administrationId={administrationId}
        filter={{ period, state, contact }}
        today={today}
        onApply={(f) => setSearch(address(f), { replace: true })}
      />
    </FilterPanel>
  )

  const actions = (
    <>
      <div className="list-search actionbar__search" role="search" aria-label={t('timeEntries.report.search')}>
        <SearchIcon />
        <input type="search" aria-label={t('timeEntries.report.search')} placeholder={t('timeEntries.report.search')} value={typed} onChange={(e) => setTyped(e.target.value)} />
      </div>
      <button
        type="button"
        className={`btn btn--secondary btn--icon${info ? ' is-active' : ''}`}
        aria-label={t('list.moreInfo')}
        title={t('list.moreInfo')}
        aria-expanded={info}
        aria-controls={infoId}
        onClick={() => setInfo(!info)}
      >
        <InfoIcon />
      </button>
      <Unavailable icon="chart" label={t('list.statistics')} />
      <ExportMenu entries={[t('timeEntries.report.excel'), t('timeEntries.report.pdf')]} />
      <Link className="btn btn--accent btn--with-icon" to={`/${administrationId}/time_entries_wizard`}>
        <ListIcon kind="send" />
        <span>{t('timeEntries.report.bill')}</span>
      </Link>
    </>
  )

  return (
    <Page wide hideTitle title={t('timeEntries.report.title')} lead={filterPanel} actions={actions}>
      {info && (
        <GettingStarted
          id={infoId}
          title={t('timeEntries.report.intro.title')}
          text={t('timeEntries.report.intro.text')}
          steps={[
            { icon: 'flow', title: t('timeEntries.report.intro.filter'), text: t('timeEntries.report.intro.filterText') },
            { icon: 'send', title: t('timeEntries.report.intro.bill'), text: t('timeEntries.report.intro.billText') },
            { icon: 'chart', title: t('timeEntries.report.intro.status'), text: t('timeEntries.report.intro.statusText') },
          ]}
        />
      )}
      {!entries.data ? (
        entries.error ? <p className="empty">{t('timeEntries.loadError')}</p> : <Loading />
      ) : (
        <WeekTable administrationId={administrationId} entries={entries.data} />
      )}
    </Page>
  )
}

type FilterProps = { administrationId: string; filter: Filter; today: Date; onApply: (f: Filter) => void }

/** Moneybird's advanced filter of the hours: a period, the status and a contact. */
function ReportFilter({ administrationId, filter, today, onApply }: FilterProps) {
  const { t } = useTranslation()
  const [range, setRange] = useState(() => (filter.period === allPeriods ? null : parsePeriod(filter.period, today)))
  const [state, setState] = useState<TimeState>(filter.state)
  const [contact, setContact] = useState(filter.contact)
  return (
    <AdvancedFilter value={{ range, state, contact }} onApply={() => onApply({ state, contact, period: range ? periodValue(range, today) : allPeriods })}>
      <PeriodPicker value={range} today={today} onChange={setRange} />
      <RadioFilter
        legend={t('timeEntries.status')}
        value={state}
        onChange={(v) => setState(v as TimeState)}
        options={timeStates.map((s) => ({ value: s, label: t(`timeEntries.report.states.${s}`) }))}
      />
      <ContactFilter administrationId={administrationId} contactId={contact} onChange={setContact} />
    </AdvancedFilter>
  )
}

type Week = { key: string; label: number; entries: TimeEntry[] }

/** The time of each week, newest first, as the API lists it. */
function byWeek(entries: TimeEntry[]): Week[] {
  const weeks: Week[] = []
  for (const e of entries) {
    const { year, week } = weekOf(dayOf(e.started_at))
    const key = `${year}-${week}`
    const last = weeks.at(-1)
    if (last?.key === key) {
      last.entries.push(e)
    } else {
      weeks.push({ key, label: week, entries: [e] })
    }
  }
  return weeks
}

/** Moneybird's table of the hours: a group per week that folds, with its total, and the total of all. */
function WeekTable({ administrationId, entries }: { administrationId: string; entries: TimeEntry[] }) {
  const { t } = useTranslation()
  const [folded, setFolded] = useState<Set<string>>(new Set())
  const time = (iso: string | null | undefined) => (iso ? clock(new Date(iso).getHours() * 60 + new Date(iso).getMinutes()) : '')
  const sum = (list: TimeEntry[]) => list.reduce((total, e) => total + workedMinutes(e), 0)
  const toggle = (key: string) => {
    const next = new Set(folded)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    setFolded(next)
  }
  return (
    <table className="data-table timesheet-table hours-report" aria-label={t('timeEntries.title')}>
      <thead>
        <tr>
          <th scope="col">{t('timeEntries.date')}</th>
          <th scope="col">{t('timeEntries.report.period')}</th>
          <th scope="col">{t('timeEntries.description')}</th>
          <th scope="col">{t('timeEntries.contact')}</th>
          <th scope="col">{t('timeEntries.project')}</th>
          <th scope="col">{t('timeEntries.report.employee')}</th>
          <th scope="col">{t('timeEntries.status')}</th>
          <th scope="col" className="timesheet-table__amount">{t('timeEntries.report.total')}</th>
        </tr>
      </thead>
      <tbody>
        {entries.length === 0 && (
          <tr>
            <td colSpan={8} className="timesheet-table__empty">{t('timeEntries.report.empty')}</td>
          </tr>
        )}
        {byWeek(entries).map((w) => {
          const open = !folded.has(w.key)
          return (
            <Fragment key={w.key}>
              <tr className="hours-report__week">
                <th scope="rowgroup" colSpan={7}>
                  <button type="button" className="hours-report__fold" aria-expanded={open} onClick={() => toggle(w.key)}>
                    {t('timeEntries.report.week', { week: w.label })}
                  </button>
                </th>
                <td className="timesheet-table__amount">{clock(sum(w.entries))}</td>
              </tr>
              {open && w.entries.map((e) => (
                <tr key={e.id}>
                  <td><Link to={`/${administrationId}/time_entries/${e.id}`}>{dayOf(e.started_at)}</Link></td>
                  <td>{`${time(e.started_at)} - ${time(e.ended_at)}`}</td>
                  <td>{e.description}</td>
                  <td>{contactName(e)}</td>
                  <td>{e.project?.name ?? ''}</td>
                  <td>{e.user?.name ?? ''}</td>
                  <td className="document-table__state"><TimeBadge entry={e} /></td>
                  <td className="timesheet-table__amount">{clock(workedMinutes(e))}</td>
                </tr>
              ))}
            </Fragment>
          )
        })}
      </tbody>
      <tfoot>
        <tr>
          <th scope="row" colSpan={7}>{t('timeEntries.register.total')}</th>
          <td className="timesheet-table__amount">{clock(sum(entries))}</td>
        </tr>
      </tfoot>
    </table>
  )
}
