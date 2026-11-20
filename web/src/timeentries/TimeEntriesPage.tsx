import { type FormEvent, useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { useCurrentUser } from '../auth/queries'
import { contactName as nameOf } from '../contacts/queries'
import { FormRow, formErrors, TextInput } from '../forms/Form'
import { SearchSelect } from '../forms/SearchSelect'
import { ContactPicker } from '../invoices/ContactPicker'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { GettingStarted } from '../list/GettingStarted'
import { ActionMenu } from '../list/ListActions'
import { can } from '../management/queries'
import { useActiveProjects } from '../projects/queries'
import { type TimeEntry, useCreateTimeEntry, useTimer, useWeekEntries, workedMinutes } from './queries'
import { addDays, atTime, clock, dayOf, isoDay, mondayOf, parseClock, parseDay, periodOf, stopwatch } from './week'
import './timesheets.css'

/**
 * Moneybird's timesheet register (/time_entries?date=): the week of the
 * signed-in member with the hours of each day, a form to log time on the
 * chosen day or start a timer, and that day's time.
 */
export function TimeEntriesPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search] = useSearchParams()
  const administration = useAdministration(administrationId)
  const me = useCurrentUser()
  const day = parseDay(search.get('date'))
  const monday = mondayOf(day)
  const week = useWeekEntries(administrationId, me.data?.id ?? '', periodOf(monday))
  const now = useNow()
  const [info, setInfo] = useState(false)
  const infoId = useId()

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data || !me.data) {
    return <Loading />
  }
  const base = `/${administrationId}/time_entries`
  const at = (d: string) => `${base}?date=${d}`
  const entries = week.data ?? []
  const minutesOn = (d: string) => entries.filter((e) => dayOf(e.started_at) === d).reduce((sum, e) => sum + workedMinutes(e, now), 0)
  const days = Array.from({ length: 7 }, (_, i) => addDays(monday, i))
  const today = isoDay(now)
  const long = new Intl.DateTimeFormat(i18n.language, { month: 'long', day: 'numeric', year: 'numeric' }).format(atTime(day, 0))
  const shown = entries.filter((e) => dayOf(e.started_at) === day).sort((a, b) => (a.started_at ?? '').localeCompare(b.started_at ?? ''))

  return (
    <Page
      wide
      title={t('timeEntries.register.title', { date: long })}
      actions={
        <div className="accounts-actions">
          <button
            type="button"
            className={`btn btn--secondary btn--icon${info ? ' is-active' : ''}`}
            aria-label={t('timeEntries.register.moreInfo')}
            title={t('timeEntries.register.moreInfo')}
            aria-expanded={info}
            aria-controls={infoId}
            onClick={() => setInfo(!info)}
          >
            <ListIcon kind="flow" />
          </button>
          <Link className="btn btn--secondary" to={at(today)}>{t('timeEntries.register.today')}</Link>
          <ActionMenu
            label={t('timeEntries.register.more')}
            iconOnly
            icon={<ListIcon kind="more" />}
            items={[
              { label: t('sidenav.items.hoursReport'), to: `/${administrationId}/time_entries_report` },
              { label: t('sidenav.items.projects'), to: `/${administrationId}/projects` },
            ]}
          />
        </div>
      }
    >
      {info && (
        <GettingStarted
          id={infoId}
          title={t('timeEntries.register.intro.title')}
          text={t('timeEntries.register.intro.text')}
          steps={[
            { icon: 'person', title: t('timeEntries.register.intro.contact'), text: t('timeEntries.register.intro.contactText') },
            { icon: 'briefcase', title: t('timeEntries.register.intro.project'), text: t('timeEntries.register.intro.projectText') },
            { icon: 'timer', title: t('timeEntries.register.intro.timer'), text: t('timeEntries.register.intro.timerText') },
          ]}
        />
      )}

      <nav className="day-picker" aria-label={t('timeEntries.register.week')}>
        <Link className="day-picker__week" to={at(addDays(day, -7))} aria-label={t('timeEntries.register.prevWeek')} title={t('timeEntries.register.prevWeek')}>‹</Link>
        <ol className="day-picker__days">
          {days.map((d) => {
            const date = atTime(d, 0)
            return (
              <li key={d}>
                <Link
                  className={`day-picker__day${d === today ? ' day-picker__day--today' : ''}`}
                  to={at(d)}
                  aria-current={d === day ? 'date' : undefined}
                  aria-label={new Intl.DateTimeFormat(i18n.language, { weekday: 'long', day: 'numeric', month: 'long' }).format(date)}
                >
                  <span className="day-picker__weekday">{new Intl.DateTimeFormat(i18n.language, { weekday: 'short' }).format(date)}</span>
                  <span className="day-picker__date">{date.getDate()}</span>
                  <span className="day-picker__hours">{clock(minutesOn(d))}</span>
                </Link>
              </li>
            )
          })}
        </ol>
        <Link className="day-picker__week" to={at(addDays(day, 7))} aria-label={t('timeEntries.register.nextWeek')} title={t('timeEntries.register.nextWeek')}>›</Link>
        <span className="day-picker__total">
          <span className="day-picker__total-title">{t('timeEntries.register.weekTotal')}</span>
          <span className="day-picker__total-value">{clock(days.reduce((sum, d) => sum + minutesOn(d), 0))}</span>
        </span>
      </nav>

      <TimerForm key={day} administrationId={administrationId} userId={me.data.id ?? ''} day={day} addProjects={can(administration.data, 'settings')} />

      {!week.data ? (
        <Loading />
      ) : (
        <DayTable administrationId={administrationId} entries={shown} now={now} today={today} />
      )}
    </Page>
  )
}

/** The time now, ticking each second while a timer could be shown. */
function useNow() {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(timer)
  }, [])
  return now
}

const nowMinutes = () => {
  const d = new Date()
  return d.getHours() * 60 + d.getMinutes()
}

/**
 * Moneybird's start timer form: started, ended and paused as hours and
 * minutes, the description, contact, project and billable. With an end it
 * saves the time; without, it starts a timer.
 */
function TimerForm({ administrationId, userId, day, addProjects }: { administrationId: string; userId: string; day: string; addProjects: boolean }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const save = useCreateTimeEntry(administrationId)
  const projects = useActiveProjects(administrationId)
  const [start, setStart] = useState(() => clock(nowMinutes()))
  const [end, setEnd] = useState('')
  const [pause, setPause] = useState('')
  const [description, setDescription] = useState('')
  const [contact, setContact] = useState<{ id: string; name: string } | null>(null)
  const [project, setProject] = useState('')
  const [billable, setBillable] = useState(true)
  const [invalid, setInvalid] = useState<Record<string, string[]>>({})
  const labels: Record<string, string> = {
    started_at: t('timeEntries.register.startedAt'), ended_at: t('timeEntries.register.endedAt'), paused_duration: t('timeEntries.register.pause'),
    description: t('timeEntries.description'), contact_id: t('timeEntries.contact'), project_id: t('timeEntries.project'),
  }
  const errors = formErrors(save.error, labels)
  const field = (name: string) => invalid[name] ?? errors.field(name)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const from = parseClock(start)
    const until = end.trim() ? parseClock(end) : undefined
    const paused = pause.trim() ? parseClock(pause) : 0
    const wrong: Record<string, string[]> = {}
    if (from === null) wrong.started_at = [t('timeEntries.register.notATime')]
    if (until === null) wrong.ended_at = [t('timeEntries.register.notATime')]
    if (paused === null) wrong.paused_duration = [t('timeEntries.register.notATime')]
    if (!description.trim()) wrong.description = [t('timeEntries.register.describe')]
    setInvalid(wrong)
    if (Object.keys(wrong).length > 0 || from === null || until === null || paused === null) {
      return
    }
    save.mutate({
      user_id: userId, description: description.trim(), billable, started_at: atTime(day, from).toISOString(),
      ...(until !== undefined ? { ended_at: atTime(day, until).toISOString() } : {}),
      ...(paused ? { paused_duration: paused * 60 } : {}),
      ...(contact ? { contact_id: contact.id } : {}),
      ...(project ? { project_id: project } : {}),
    }, {
      onSuccess: () => {
        setStart(clock(nowMinutes()))
        setEnd('')
        setPause('')
        setDescription('')
      },
    })
  }

  return (
    <form className="timer-form" onSubmit={submit} noValidate aria-label={t('timeEntries.register.form')}>
      <Problems messages={errors.general} />
      <div className="timer-form__times">
        <TextInput label={labels.started_at ?? ''} placeholder="00:00" inputMode="numeric" maxLength={5} value={start} errors={field('started_at')} onChange={(e) => setStart(e.target.value)} />
        <TextInput label={labels.ended_at ?? ''} placeholder="00:00" inputMode="numeric" maxLength={5} value={end} errors={field('ended_at')} onChange={(e) => setEnd(e.target.value)} />
        <TextInput label={labels.paused_duration ?? ''} placeholder="00:00" inputMode="numeric" maxLength={5} value={pause} errors={field('paused_duration')} onChange={(e) => setPause(e.target.value)} />
      </div>
      <div className="timer-form__description">
        <TextInput label={labels.description ?? ''} maxLength={10000} value={description} errors={field('description')} onChange={(e) => setDescription(e.target.value)} />
      </div>
      <div className="timer-form__contact">
        <ContactPicker
          administrationId={administrationId}
          label={labels.contact_id ?? ''}
          value={contact?.name ?? ''}
          errors={field('contact_id')}
          onChange={(c) => setContact({ id: c.id ?? '', name: nameOf(c) })}
          action={contact ? (
            <button type="button" className="icon-btn" aria-label={t('list.clearContact')} title={t('list.clearContact')} onClick={() => setContact(null)}>×</button>
          ) : undefined}
        />
      </div>
      <div className="timer-form__project">
        <FormRow label={labels.project_id ?? ''} errors={field('project_id')}>
          {({ labelId, ...control }) => (
            <SearchSelect
              labelledBy={labelId}
              searchLabel={t('timeEntries.register.findProject')}
              emptyLabel={t('timeEntries.register.noProject')}
              placeholder={t('timeEntries.register.chooseProject')}
              value={project}
              options={[{ value: '', label: t('timeEntries.register.noneProject') }, ...(projects.data ?? []).map((p) => ({ value: p.id ?? '', label: p.name ?? '' }))]}
              add={addProjects ? { label: t('timeEntries.register.addProject'), onAdd: () => navigate(`/${administrationId}/projects/new`) } : undefined}
              aria-invalid={control['aria-invalid']}
              aria-describedby={control['aria-describedby']}
              onChange={setProject}
            />
          )}
        </FormRow>
      </div>
      <label className="timer-form__billable">
        <span className="form-input__label">{t('timeEntries.billableShort')}</span>
        <input type="checkbox" checked={billable} onChange={(e) => setBillable(e.target.checked)} />
      </label>
      <div className="timer-form__button">
        {end.trim() ? (
          <button className="btn btn--primary" disabled={save.isPending}>{t('timeEntries.register.save')}</button>
        ) : (
          <button className="btn btn--accent btn--with-icon" disabled={save.isPending}>
            <ListIcon kind="play" />
            <span>{t('timeEntries.register.start')}</span>
          </button>
        )}
      </div>
    </form>
  )
}

type DayProps = { administrationId: string; entries: TimeEntry[]; now: Date; today: string }

/** Moneybird's time of the day: from and until, what, for whom, its state, pauses and hours, with the timer's button. */
function DayTable({ administrationId, entries, now, today }: DayProps) {
  const { t } = useTranslation()
  const time = (iso: string | null | undefined) => (iso ? clock(new Date(iso).getHours() * 60 + new Date(iso).getMinutes()) : '')
  const worked = (e: TimeEntry) => {
    if (e.ended_at) {
      return clock(workedMinutes(e))
    }
    const seconds = Math.max(0, Math.floor((now.getTime() - new Date(e.started_at ?? '').getTime()) / 1000) - (e.paused_duration ?? 0))
    return stopwatch(seconds)
  }
  const total = entries.reduce((sum, e) => sum + workedMinutes(e, now), 0)
  return (
    <table className="data-table timesheet-table" aria-label={t('timeEntries.title')}>
      <thead>
        <tr>
          <th scope="col">{t('timeEntries.register.fromUntil')}</th>
          <th scope="col">{t('timeEntries.description')}</th>
          <th scope="col">{t('timeEntries.contact')}</th>
          <th scope="col">{t('timeEntries.project')}</th>
          <th scope="col">{t('timeEntries.status')}</th>
          <th scope="col" className="timesheet-table__amount">{t('timeEntries.register.paused')}</th>
          <th scope="col" className="timesheet-table__amount">{t('timeEntries.register.worked')}</th>
          <th scope="col"><span className="sr-only">{t('timeEntries.register.timer')}</span></th>
        </tr>
      </thead>
      <tbody>
        {entries.length === 0 ? (
          <tr>
            <td colSpan={8} className="timesheet-table__empty">{t('timeEntries.register.empty')}</td>
          </tr>
        ) : (
          entries.map((e) => (
            <tr key={e.id} className={e.ended_at ? undefined : 'is-running'}>
              <td>
                <Link to={`/${administrationId}/time_entries/${e.id}`}>
                  {`${time(e.started_at)} - ${e.ended_at ? time(e.ended_at) : t('timeEntries.register.now')}`}
                </Link>
              </td>
              <td>{e.description}</td>
              <td>{contactName(e)}</td>
              <td>{e.project?.name ?? ''}</td>
              <td className="document-table__state"><TimeBadge entry={e} /></td>
              <td className="timesheet-table__amount">{clock(Math.floor((e.paused_duration ?? 0) / 60))}</td>
              <td className="timesheet-table__amount">{worked(e)}</td>
              <td className="timesheet-table__action">
                <TimerButton administrationId={administrationId} entry={e} canResume={dayOf(e.started_at) === today} />
              </td>
            </tr>
          ))
        )}
      </tbody>
      <tfoot>
        <tr>
          <th scope="row" colSpan={6}><span className="sr-only">{t('timeEntries.register.total')}</span></th>
          <td className="timesheet-table__amount">{clock(total)}</td>
          <td />
        </tr>
      </tfoot>
    </table>
  )
}

/** Stops a running timer, or resumes time of today that is not invoiced. */
function TimerButton({ administrationId, entry, canResume }: { administrationId: string; entry: TimeEntry; canResume: boolean }) {
  const { t } = useTranslation()
  const timer = useTimer(administrationId, entry.id ?? '')
  if (entry.ended_at && (!canResume || entry.sales_invoice_id)) {
    return null
  }
  const action = entry.ended_at ? 'resume' : 'stop'
  const label = t(action === 'stop' ? 'timeEntries.stop' : 'timeEntries.resume')
  return (
    <button
      type="button"
      className={`icon-btn timer-button timer-button--${action}`}
      aria-label={label}
      title={label}
      disabled={timer.isPending}
      onClick={() => timer.mutate(action)}
    >
      <ListIcon kind={action === 'stop' ? 'stop' : 'play'} />
    </button>
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
