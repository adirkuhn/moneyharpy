import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable } from '../list/DocumentTable'
import { FilterPanel } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
import { perPage, type Project, type ProjectState, projectStates, useProjects } from './queries'

/** The administration's projects, active ones unless filtered otherwise, and paged (/projects). */
export function ProjectsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const asked = params.get('state') ?? ''
  const state: ProjectState = (projectStates as readonly string[]).includes(asked) ? (asked as ProjectState) : 'active'
  const page = Math.max(1, Number(params.get('page')) || 1)
  const projects = useProjects(administrationId, state, page)
  const [info, setInfo] = useState<boolean | null>(null)
  const introId = useId()
  const base = `/${administrationId}/projects`
  const shown = projects.data?.projects ?? []
  const firstUse = projects.data !== undefined && shown.length === 0 && state === 'active' && page === 1
  const introShown = info ?? firstUse

  const quick = projectStates.map((s) => ({ label: t(`projects.views.${s}`), to: s === 'active' ? base : `${base}?state=${s}`, current: s === state }))

  return (
    <Page
      wide
      title={t('projects.title')}
      lead={<FilterPanel label={t(`projects.views.${state}`)} title={t('projects.filters')} quick={quick} />}
      actions={
        <ListActions
          info={{ shown: introShown, onToggle: () => setInfo(!introShown), controls: introId }}
          exports={[]}
          primary={{ to: `${base}/new`, label: t('projects.add') }}
        />
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('projects.intro.title')}
          text={t('projects.intro.text')}
          steps={[
            { icon: 'plus', title: t('projects.intro.add'), text: t('projects.intro.addText') },
            { icon: 'file', title: t('projects.intro.book'), text: t('projects.intro.bookText') },
            { icon: 'chart', title: t('projects.intro.report'), text: t('projects.intro.reportText') },
          ]}
        />
      )}
      {!projects.data ? (
        projects.error ? <p className="empty">{t('projects.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{firstUse ? t('projects.empty') : t('projects.noMatches')}</p>
      ) : (
        <DocumentTable<Project>
          label={t('projects.title')}
          rows={shown}
          rowKey={(p) => p.id ?? ''}
          href={(p) => `${base}/${p.id}`}
          total={projects.data.total}
          perPage={perPage}
          columns={[
            { key: 'name', header: t('projects.name'), link: true, cell: (p) => p.name },
            { key: 'budget', header: t('projects.budget'), kind: 'amount', cell: (p) => (p.budget === null || p.budget === undefined ? '' : t('projects.hours', { count: p.budget })) },
            { key: 'state', header: t('projects.status'), kind: 'state', cell: (p) => <ProjectBadge project={p} /> },
          ]}
        />
      )}
    </Page>
  )
}

export function ProjectBadge({ project }: { project: Project }) {
  const { t } = useTranslation()
  const archived = project.state === 'archived'
  return <span className={`badge badge--${archived ? 'archived' : 'active'}`}>{t(`projects.states.${archived ? 'archived' : 'active'}`)}</span>
}
