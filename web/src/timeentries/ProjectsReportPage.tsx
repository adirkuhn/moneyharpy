import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { InfoIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { GettingStarted } from '../list/GettingStarted'
import { ActionMenu } from '../list/ListActions'
import { can } from '../management/queries'
import { type Project, useActiveProjects } from '../projects/queries'
import { type TimeEntry, useReportEntries, workedMinutes } from './queries'
import { clock } from './week'
import './timesheets.css'

/** The last ten years, the longest period the API filters on: a project's time from its start. */
function lastTenYears(today: Date): string {
  const year = today.getFullYear()
  return `${year - 9}01..${year}12`
}

type Totals = { worked: number; billable: number }

/** The minutes worked on each project, and how many of them are billable. */
export function projectTotals(entries: TimeEntry[]): Map<string, Totals> {
  const totals = new Map<string, Totals>()
  for (const e of entries) {
    const id = String(e.project_id ?? '')
    if (!id) {
      continue
    }
    const sum = totals.get(id) ?? { worked: 0, billable: 0 }
    const minutes = workedMinutes(e)
    sum.worked += minutes
    if (e.billable) {
      sum.billable += minutes
    }
    totals.set(id, sum)
  }
  return totals
}

/**
 * Moneybird's projects report (/time_entries_report/projects): the hours of
 * each active project against its budget, how much of the budget is spent,
 * and which hours are billable.
 */
export function ProjectsReportPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const today = useMemo(() => new Date(), [])
  const administration = useAdministration(administrationId)
  const projects = useActiveProjects(administrationId)
  const entries = useReportEntries(administrationId, `period:${lastTenYears(today)},state:all,include_active:true`)
  const [info, setInfo] = useState<boolean | null>(null)
  const infoId = useId()

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data || !projects.data) {
    return <Loading />
  }
  // As on Moneybird, the introduction shows until there is a project.
  const shown = info ?? projects.data.length === 0
  const totals = projectTotals(entries.data ?? [])

  return (
    <Page
      wide
      title={t('timeEntries.projects.title')}
      actions={
        <div className="accounts-actions">
          <button
            type="button"
            className={`btn btn--secondary btn--icon${shown ? ' is-active' : ''}`}
            aria-label={t('list.moreInfo')}
            title={t('list.moreInfo')}
            aria-expanded={shown}
            aria-controls={infoId}
            onClick={() => setInfo(!shown)}
          >
            <InfoIcon />
          </button>
          <ActionMenu
            label={t('timeEntries.register.more')}
            iconOnly
            icon={<ListIcon kind="more" />}
            items={[
              { label: t('sidenav.items.projects'), to: `/${administrationId}/projects` },
              ...(can(administration.data, 'settings') ? [{ label: t('timeEntries.projects.add'), to: `/${administrationId}/projects/new` }] : []),
            ]}
          />
        </div>
      }
    >
      {shown && (
        <GettingStarted
          id={infoId}
          title={t('timeEntries.projects.intro.title')}
          text={t('timeEntries.projects.intro.text')}
          steps={[
            { icon: 'timer', title: t('timeEntries.projects.intro.budget'), text: t('timeEntries.projects.intro.budgetText') },
            { icon: 'chart', title: t('timeEntries.projects.intro.progress'), text: t('timeEntries.projects.intro.progressText') },
            { icon: 'coins', title: t('timeEntries.projects.intro.billable'), text: t('timeEntries.projects.intro.billableText') },
          ]}
        />
      )}
      {projects.data.length > 0 && (
        !entries.data ? <Loading /> : <ProjectTable administrationId={administrationId} projects={projects.data} totals={totals} manage={can(administration.data, 'settings')} />
      )}
    </Page>
  )
}

type TableProps = { administrationId: string; projects: Project[]; totals: Map<string, Totals>; manage: boolean }

function ProjectTable({ administrationId, projects, totals, manage }: TableProps) {
  const { t } = useTranslation()
  return (
    <table className="data-table timesheet-table" aria-label={t('timeEntries.projects.title')}>
      <thead>
        <tr>
          <th scope="col">{t('timeEntries.project')}</th>
          <th scope="col" className="timesheet-table__amount">{t('timeEntries.projects.budget')}</th>
          <th scope="col" className="timesheet-table__amount">{t('timeEntries.projects.worked')}</th>
          <th scope="col" className="timesheet-table__amount">{t('timeEntries.projects.billable')}</th>
          <th scope="col" className="timesheet-table__amount">{t('timeEntries.projects.nonBillable')}</th>
          <th scope="col">{t('timeEntries.projects.progress')}</th>
        </tr>
      </thead>
      <tbody>
        {projects.map((p) => {
          const sum = totals.get(p.id ?? '') ?? { worked: 0, billable: 0 }
          const budget = p.budget ? Number(p.budget) * 60 : 0
          const spent = budget > 0 ? Math.round((sum.worked / budget) * 100) : null
          return (
            <tr key={p.id}>
              <td>{manage ? <Link to={`/${administrationId}/projects/${p.id}`}>{p.name}</Link> : p.name}</td>
              <td className="timesheet-table__amount">{budget > 0 ? clock(budget) : ''}</td>
              <td className="timesheet-table__amount">{clock(sum.worked)}</td>
              <td className="timesheet-table__amount">{clock(sum.billable)}</td>
              <td className="timesheet-table__amount">{clock(sum.worked - sum.billable)}</td>
              <td>
                {spent !== null && (
                  <span className="project-progress">
                    <span className="project-progress__bar" aria-hidden="true">
                      <span className={`project-progress__part${spent > 100 ? ' is-over' : ''}`} style={{ width: `${Math.min(100, spent)}%` }} />
                    </span>
                    <span className="project-progress__value">{t('timeEntries.projects.spent', { percent: spent })}</span>
                  </span>
                )}
              </td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}
