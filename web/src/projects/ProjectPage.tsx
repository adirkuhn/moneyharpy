import { ActionLink } from '../layout/ActionLink'
import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { FormNotice, FormSection, formErrors, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { DangerDialog } from '../layout/Modal'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type MenuItem, MoreMenu } from '../list/ListActions'
import { type Project, type ProjectChanges, useDeleteProject, useProject, useSaveProject } from './queries'

/** Adds a project (/projects/new). */
export function NewProjectPage() {
  const { t } = useTranslation()
  return (
    <Page wide title={t('projects.newTitle')}>
      <ProjectForm project={null} />
    </Page>
  )
}

/** A project's own address (/projects/:projectId) opens its form, as on Moneybird. */
export function ProjectRedirect() {
  const { administrationId = '', projectId = '' } = useParams()
  return <Navigate replace to={`/${administrationId}/projects/${projectId}/edit`} />
}

/** Edits a project (/projects/:projectId/edit), and archives, reactivates or deletes it from the ⋯ menu. */
export function EditProjectPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '', projectId = '' } = useParams()
  const project = useProject(administrationId, projectId)
  const save = useSaveProject(administrationId, projectId)
  const remove = useDeleteProject(administrationId, projectId)
  const [confirming, setConfirming] = useState(false)
  const [kept, setKept] = useState(false)
  const list = `/${administrationId}/projects`
  if (project.error instanceof ApiError && project.error.status < 500) {
    return <NotFoundPage />
  }
  if (!project.data) {
    return <Loading />
  }
  const p = project.data
  const archived = p.state === 'archived'
  const more: MenuItem[] = [
    archived
      ? { label: t('projects.reactivate'), disabled: save.isPending, onSelect: () => save.mutate({ state: 'active' }) }
      : { label: t('projects.archive'), disabled: save.isPending, onSelect: () => save.mutate({ state: 'archived' }) },
    { label: t('projects.delete'), danger: true, onSelect: () => setConfirming(true) },
  ]
  return (
    <Page wide title={t('projects.editTitle')} actions={<MoreMenu items={more} />}>
      <Problems messages={formErrors(remove.error ?? save.error, {}).general} />
      {kept ? <FormNotice>{t('projects.keptNotice')}</FormNotice> : archived && <FormNotice>{t('projects.archivedNotice')}</FormNotice>}
      <ProjectForm key={`${p.name}-${p.budget}`} project={p} />
      {confirming && (
        <DangerDialog
          question={t('projects.confirmDelete')}
          busy={remove.isPending}
          onCancel={() => setConfirming(false)}
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
        />
      )}
    </Page>
  )
}

/** Moneybird's project form: its name and its budget in hours, then Save and Cancel. */
function ProjectForm({ project }: { project: Project | null }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const save = useSaveProject(administrationId, project?.id)
  const [name, setName] = useState(project?.name ?? '')
  const [budget, setBudget] = useState(project?.budget === null || project?.budget === undefined ? '' : String(project.budget))
  const labels: Record<string, string> = { name: t('projects.name'), budget: t('projects.budget') }
  const errors = formErrors(save.error, labels)
  const list = `/${administrationId}/projects`

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: ProjectChanges = { name: name.trim(), budget: budget.trim() === '' ? null : Number(budget) || budget.trim() }
    save.mutate(body, { onSuccess: () => navigate(list) })
  }

  return (
    <form className="form-sheet" onSubmit={submit} noValidate aria-label={t('projects.form')}>
      <Problems messages={errors.general} />
      <FormSection>
        <TextInput label={labels.name ?? ''} width="medium" maxLength={255} required value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
        <TextInput
          label={labels.budget ?? ''}
          inputMode="decimal"
          width="small"
          value={budget}
          errors={errors.field('budget')}
          onChange={(e) => setBudget(e.target.value)}
        />
      </FormSection>
      <div className="form-sheet__actions form-sheet__actions--center">
        <button className="btn btn--primary" disabled={save.isPending}>{t('projects.save')}</button>
        <ActionLink className="btn btn--secondary" to={list}>{t('common.cancel')}</ActionLink>
      </div>
    </form>
  )
}
