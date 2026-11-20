import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { InfoIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { Deactivated } from '../list/Deactivated'
import { DocumentTable } from '../list/DocumentTable'
import { GettingStarted } from '../list/GettingStarted'
import { can } from '../management/queries'
import { perPage, type Project, type ProjectState, useProjectCount, useProjects } from './queries'

/**
 * The administration's projects, after Moneybird's list: the active ones by
 * name, and below them how many are archived and a way to show those too
 * (/projects).
 */
export function ProjectsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const state: ProjectState = params.get('state') === 'all' ? 'all' : 'active'
  const page = Math.max(1, Number(params.get('page')) || 1)
  const projects = useProjects(administrationId, state, page)
  const archived = useProjectCount(administrationId, 'archived')
  const all = useProjectCount(administrationId, 'all')
  const [info, setInfo] = useState<boolean | null>(null)
  const introId = useId()
  const base = `/${administrationId}/projects`
  // Adding and changing projects is a setting; others only see them.
  const manage = can(useAdministration(administrationId).data, 'settings')
  const shown = projects.data?.projects ?? []
  const firstUse = all.data === 0
  const introShown = info ?? firstUse

  const footer = firstUse ? (manage && (
    <Link className="document-table__add" to={`${base}/new`}>
      <ListIcon kind="plus" />
      {t('projects.add')}
    </Link>
  )) : archived.data !== undefined && (
    <Deactivated count={archived.data} shown={state === 'all'} show={`${base}?state=all`} hide={base} />
  )

  return (
    <Page
      wide
      title={t('projects.title')}
      actions={
        <>
          <button
            type="button"
            className={`btn btn--secondary btn--icon${introShown ? ' is-active' : ''}`}
            aria-label={t('list.moreInfo')}
            title={t('list.moreInfo')}
            aria-expanded={introShown}
            aria-controls={introId}
            onClick={() => setInfo(!introShown)}
          >
            <InfoIcon />
          </button>
          {manage && (
            <Link className="btn btn--accent btn--with-icon" to={`${base}/new`}>
              <ListIcon kind="plus" />
              <span>{t('projects.addShort')}</span>
            </Link>
          )}
        </>
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('projects.intro.title')}
          text={t('projects.intro.text')}
          steps={[
            { icon: 'plus', title: t('projects.intro.add'), text: t('projects.intro.addText') },
            { icon: 'check', title: t('projects.intro.select'), text: t('projects.intro.selectText') },
            { icon: 'chart', title: t('projects.intro.insight'), text: t('projects.intro.insightText') },
          ]}
        />
      )}
      {!projects.data ? (
        projects.error ? <p className="empty">{t('projects.loadError')}</p> : <Loading />
      ) : (
        <DocumentTable<Project>
          label={t('projects.title')}
          rows={shown}
          rowKey={(p) => p.id ?? ''}
          href={(p) => (manage ? `${base}/${p.id}/edit` : undefined)}
          total={projects.data.total}
          perPage={perPage}
          footer={footer}
          columns={[
            {
              key: 'name', header: t('projects.name'), link: manage,
              cell: (p) => (p.state === 'archived' ? <span className="list-inactive">{p.name}</span> : p.name),
            },
          ]}
        />
      )}
    </Page>
  )
}
