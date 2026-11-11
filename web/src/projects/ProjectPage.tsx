import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { ConfirmPanel, DetailBlock, DetailList, type DetailRow } from '../detail/Detail'
import { FormSection, formErrors, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type MenuItem, MoreMenu } from '../list/ListActions'
import { ProjectBadge } from './ProjectsPage'
import { type Project, type ProjectChanges, useDeleteProject, useProject, useSaveProject } from './queries'

/** One project (/projects/:projectId), or a new one (/projects/new). */
export function ProjectPage() {
  const { t } = useTranslation()
  const { administrationId = '', projectId } = useParams()
  const project = useProject(administrationId, projectId)
  if (projectId && project.error instanceof ApiError && project.error.status < 500) {
    return <NotFoundPage />
  }
  if (!projectId) {
    return (
      <Page title={t('projects.newTitle')} back={{ to: `/${administrationId}/projects`, label: t('projects.backToList') }}>
        <ProjectForm administrationId={administrationId} project={null} />
      </Page>
    )
  }
  if (!project.data) {
    return <Loading />
  }
  return <ProjectView key={project.data.id} administrationId={administrationId} project={project.data} />
}

function ProjectView({ administrationId, project }: { administrationId: string; project: Project }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveProject(administrationId, project.id)
  const remove = useDeleteProject(administrationId, project.id ?? '')
  const list = `/${administrationId}/projects`
  const [confirming, setConfirming] = useState(false)
  const [kept, setKept] = useState(false)
  const archived = project.state === 'archived'

  const more: MenuItem[] = [
    archived
      ? { label: t('projects.reactivate'), disabled: save.isPending, onSelect: () => save.mutate({ state: 'active' }) }
      : { label: t('projects.archive'), disabled: save.isPending, onSelect: () => save.mutate({ state: 'archived' }) },
    { label: t('projects.delete'), danger: true, onSelect: () => setConfirming(true) },
  ]
  const rows: DetailRow[] = [
    [t('projects.status'), <ProjectBadge key="state" project={project} />],
    [t('projects.budget'), project.budget === null || project.budget === undefined ? '' : t('projects.hours', { count: project.budget })],
  ]

  return (
    <Page title={project.name ?? ''} back={{ to: list, label: t('projects.backToList') }} actions={<MoreMenu items={more} />}>
      <Problems messages={formErrors(remove.error ?? save.error, {}).general} />
      {confirming && (
        <ConfirmPanel
          message={t('projects.confirmDeleteText')}
          confirmLabel={t('projects.confirmDelete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, {
            onSuccess: (keptAs) => {
              setConfirming(false)
              if (keptAs) {
                setKept(true)
              } else {
                navigate(list)
              }
            },
          })}
          onCancel={() => setConfirming(false)}
        />
      )}
      {kept && <p className="notice" role="status">{t('projects.keptNotice')}</p>}
      {archived && !kept && <p className="notice">{t('projects.archivedNotice')}</p>}
      <DetailBlock title={t('projects.detailsTitle')}>
        <DetailList rows={rows} />
      </DetailBlock>
      <DetailBlock title={t('projects.editTitle')}>
        <ProjectForm key={`${project.name}-${project.budget}-${project.state}`} administrationId={administrationId} project={project} />
      </DetailBlock>
    </Page>
  )
}

/** A project's name and its budget in hours. */
function ProjectForm({ administrationId, project }: { administrationId: string; project: Project | null }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveProject(administrationId, project?.id)
  const [name, setName] = useState(project?.name ?? '')
  const [budget, setBudget] = useState(project?.budget === null || project?.budget === undefined ? '' : String(project.budget))
  const labels: Record<string, string> = { name: t('projects.name'), budget: t('projects.budget') }
  const errors = formErrors(save.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: ProjectChanges = { name: name.trim(), budget: budget.trim() === '' ? null : Number(budget) || budget.trim() }
    save.mutate(body, {
      onSuccess: (saved) => {
        if (!project) {
          navigate(`/${administrationId}/projects/${saved.id}`)
        }
      },
    })
  }

  return (
    <form className="form-sheet" onSubmit={submit} noValidate aria-label={t('projects.form')}>
      <Problems messages={errors.general} />
      <FormSection>
        <TextInput label={labels.name ?? ''} maxLength={255} required value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
        <TextInput
          label={labels.budget ?? ''}
          type="number"
          inputMode="numeric"
          min={0}
          width="small"
          help={t('projects.budgetHelp')}
          value={budget}
          errors={errors.field('budget')}
          onChange={(e) => setBudget(e.target.value)}
        />
      </FormSection>
      <div className="form-sheet__actions">
        <button className="btn btn--primary" disabled={save.isPending}>{t('projects.save')}</button>
        {!project && <Link className="btn btn--secondary" to={`/${administrationId}/projects`}>{t('common.cancel')}</Link>}
      </div>
    </form>
  )
}
