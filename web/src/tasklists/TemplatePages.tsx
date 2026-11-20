import { ActionLink } from '../layout/ActionLink'
import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { FormNotice, FormRow, FormSection, formErrors, TextArea, TextInput } from '../forms/Form'
import { InfoIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { DangerDialog } from '../layout/Modal'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { DocumentTable } from '../list/DocumentTable'
import { GettingStarted } from '../list/GettingStarted'
import { ActionMenu } from '../list/ListActions'
import { TaskGroups } from './TaskGroups'
import { type TaskGroup, type Template, type TemplateChanges, useDeleteTemplate, useSaveTemplate, useTemplate, useTemplates } from './queries'
import './tasklists.css'

type TemplatePeriod = 'none' | 'month' | 'quarter' | 'year'
const periods: TemplatePeriod[] = ['none', 'month', 'quarter', 'year']
const durations: Record<TemplatePeriod, string> = { none: '', month: 'P1M', quarter: 'P3M', year: 'P1Y' }

/** A template's period as one of Moneybird's choices. */
function periodOf(template?: Template): TemplatePeriod {
  return (Object.keys(durations) as TemplatePeriod[]).find((k) => durations[k] === (template?.period ?? '')) ?? 'none'
}

/** The days after the period's end a template's deadline falls, or null without one. */
function deadlineDays(template?: Template): number | null {
  const m = /^P(\d+)D$/.exec(template?.deadline_after ?? '')
  return m ? Number(m[1]) : null
}

/** The administration's task list templates (/task_lists/templates). */
export function TemplatesPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const templates = useTemplates(administrationId)
  const [info, setInfo] = useState<boolean | null>(null)
  const [deleting, setDeleting] = useState<Template | null>(null)
  const introId = useId()
  const base = `/${administrationId}/task_lists/templates`
  const rows = templates.data ?? []
  const firstUse = templates.data !== undefined && rows.length === 0
  const introShown = info ?? firstUse

  return (
    <Page
      wide
      title={t('taskTemplates.title')}
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
          <ActionLink className="btn btn--accent" to={`${base}/new`}>{t('taskTemplates.add')}</ActionLink>
        </>
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('taskTemplates.intro.title')}
          text={t('taskTemplates.intro.text')}
          steps={[
            { icon: 'check', title: t('taskTemplates.intro.trusted'), text: t('taskTemplates.intro.trustedText') },
            { icon: 'repeat', title: t('taskTemplates.intro.repeat'), text: t('taskTemplates.intro.repeatText') },
            { icon: 'person', title: t('taskTemplates.intro.together'), text: t('taskTemplates.intro.togetherText') },
          ]}
        />
      )}
      {!templates.data ? (
        templates.error ? <p className="empty">{t('taskTemplates.loadError')}</p> : <Loading />
      ) : (
        <div className="task-lists">
          <DocumentTable<Template>
            label={t('taskTemplates.title')}
            rows={rows}
            rowKey={(r) => r.id ?? ''}
            href={(r) => `${base}/${r.id}`}
            total={rows.length}
            perPage={Math.max(rows.length, 1)}
            footer={firstUse && (
              <ActionLink className="document-table__add" to={`${base}/new`}>
                <ListIcon kind="plus" />
                {t('taskTemplates.add')}
              </ActionLink>
            )}
            columns={[
              { key: 'name', header: t('taskLists.name'), link: true, cell: (r) => r.name },
              { key: 'period', header: t('taskLists.period'), cell: (r) => t(`taskLists.periodKinds.${periodOf(r)}`) },
              { key: 'deadline', header: t('taskLists.deadline'), cell: (r) => <DeadlineText template={r} /> },
              {
                key: 'menu', header: '', label: t('taskTemplates.options'), kind: 'icons',
                cell: (r) => (
                  <ActionMenu
                    iconOnly
                    icon={<ListIcon kind="more" />}
                    label={t('taskTemplates.options')}
                    items={[
                      { label: t('taskLists.edit'), to: `${base}/${r.id}/edit` },
                      { label: t('taskLists.delete'), danger: true, bookkeeping: true, onSelect: () => setDeleting(r) },
                    ]}
                  />
                ),
              },
            ]}
          />
        </div>
      )}
      {deleting && <DeleteTemplate administrationId={administrationId} template={deleting} onDone={() => setDeleting(null)} />}
    </Page>
  )
}

function DeleteTemplate({ administrationId, template, onDone }: { administrationId: string; template: Template; onDone: () => void }) {
  const { t } = useTranslation()
  const remove = useDeleteTemplate(administrationId, template.id ?? '')
  return <DangerDialog question={t('taskTemplates.confirmDelete')} busy={remove.isPending} onCancel={onDone} onConfirm={() => remove.mutate(undefined, { onSuccess: onDone })} />
}

function DeadlineText({ template }: { template: Template }) {
  const { t } = useTranslation()
  const days = deadlineDays(template)
  if (days !== null) {
    return <>{t('taskTemplates.daysAfter', { count: days })}</>
  }
  return <>{template.deadline_after ? template.deadline_after : t('taskLists.noDeadline')}</>
}

/** Adds a template (/task_lists/templates/new). */
export function NewTemplatePage() {
  return <TemplateForm />
}

/** Edits a template (/task_lists/templates/:templateId/edit). */
export function EditTemplatePage() {
  const { administrationId = '', templateId = '' } = useParams()
  const template = useTemplate(administrationId, templateId)
  if (template.error instanceof ApiError && template.error.status < 500) {
    return <NotFoundPage />
  }
  if (!template.data) {
    return <Loading />
  }
  return <TemplateForm key={template.data.id} template={template.data} />
}

/** Moneybird's template form: name, description, period and, when switched on, a deadline some days after the period. */
function TemplateForm({ template }: { template?: Template }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const save = useSaveTemplate(administrationId, template?.id)
  const [name, setName] = useState(template?.name ?? '')
  const [description, setDescription] = useState(template?.description ?? '')
  const [period, setPeriod] = useState<TemplatePeriod>(template ? periodOf(template) : 'month')
  const [deadline, setDeadline] = useState(!!template?.deadline_after)
  const [days, setDays] = useState(String(deadlineDays(template) ?? 10))
  const periodLabel = useId()
  const base = `/${administrationId}/task_lists/templates`
  const labels: Record<string, string> = { name: t('taskLists.name'), description: t('taskLists.description'), period: t('taskLists.period'), deadline_after: t('taskLists.deadline') }
  const errors = formErrors(save.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: TemplateChanges = {
      name: name.trim(), description: description.trim(), period: durations[period] || 'none',
      deadline_after: deadline ? `P${Number(days) || 0}D` : '',
    }
    save.mutate(body, { onSuccess: (saved) => navigate(`${base}/${saved.id}`) })
  }

  return (
    <Page wide title={template ? t('taskTemplates.editTitle') : t('taskTemplates.newTitle')}>
      <form className="form-sheet task-list-form" onSubmit={submit} noValidate aria-label={t('taskTemplates.form')}>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name ?? ''} placeholder={t('taskLists.name')} maxLength={255} value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
          <TextArea
            label={labels.description ?? ''} rows={2} placeholder={t('taskLists.descriptionPlaceholder')}
            value={description} errors={errors.field('description')} onChange={(e) => setDescription(e.target.value)}
          />
          <div className="form-input form-input--long">
            <span className="form-input__label" id={periodLabel}>{labels.period}</span>
            <div className="form-input__body">
              <div className="radio-list" role="radiogroup" aria-labelledby={periodLabel}>
                {periods.map((k) => (
                  <label key={k} className="radio-list__item">
                    <input type="radio" name={periodLabel} checked={period === k} onChange={() => setPeriod(k)} />
                    <span>{t(`taskLists.periodKinds.${k}`)}</span>
                  </label>
                ))}
              </div>
            </div>
          </div>
          <FormRow label={labels.deadline_after ?? ''} errors={errors.field('deadline_after')}>
            {(control) => (
              <div className="template-deadline">
                <input {...control} className="switch" type="checkbox" role="switch" aria-labelledby={control.labelId} checked={deadline} onChange={(e) => setDeadline(e.target.checked)} />
                {deadline && (
                  <>
                    <input type="number" min={0} max={365} inputMode="numeric" aria-label={t('taskTemplates.days')} value={days} onChange={(e) => setDays(e.target.value)} />
                    <span>{t('taskTemplates.daysAfterPeriod')}</span>
                  </>
                )}
              </div>
            )}
          </FormRow>
        </FormSection>
        <div className="form-sheet__actions form-sheet__actions--center">
          <button className="btn btn--primary" disabled={save.isPending}>{t('taskLists.save')}</button>
          <ActionLink className="btn btn--secondary" to={template ? `${base}/${template.id}` : base}>{t('common.cancel')}</ActionLink>
        </div>
      </form>
    </Page>
  )
}

/** One template (/task_lists/templates/:templateId): its period and deadline, and its groups and tasks to edit in place. */
export function TemplatePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '', templateId = '' } = useParams()
  const template = useTemplate(administrationId, templateId)
  const remove = useDeleteTemplate(administrationId, templateId)
  const [confirming, setConfirming] = useState(false)
  const base = `/${administrationId}/task_lists/templates`
  if (template.error instanceof ApiError && template.error.status < 500) {
    return <NotFoundPage />
  }
  if (!template.data) {
    return <Loading />
  }
  const tpl = template.data
  const groups = (tpl.groups ?? []) as TaskGroup[]
  const empty = groups.every((g) => (g.tasks ?? []).length === 0)
  return (
    <Page
      wide
      title={tpl.name ?? ''}
      back={{ to: base, label: t('taskTemplates.backToList') }}
      actions={
        <>
          <ActionLink className="btn btn--secondary btn--with-icon" to={`${base}/${tpl.id}/edit`}>
            <ListIcon kind="edit" />
            <span>{t('taskLists.edit')}</span>
          </ActionLink>
          <ActionMenu iconOnly icon={<ListIcon kind="more" />} label={t('taskTemplates.options')} items={[{ label: t('taskLists.delete'), danger: true, bookkeeping: true, onSelect: () => setConfirming(true) }]} />
        </>
      }
    >
      <FormNotice>{t('taskTemplates.notice')}</FormNotice>
      <Problems messages={formErrors(remove.error, {}).general} />
      <section className="task-sheet" aria-label={tpl.name ?? ''}>
        <div className="task-sheet__top">
          <div className="task-sheet__about">
            <h2 className="task-sheet__label task-sheet__label--plain">{t('taskTemplates.about')}</h2>
            {tpl.description && <p>{tpl.description}</p>}
          </div>
          <div className="task-sheet__deadline">
            <p>{t(`taskLists.periodKinds.${periodOf(tpl)}`)}</p>
            <p><DeadlineText template={tpl} /></p>
          </div>
        </div>
        <TaskGroups key={tpl.id} administrationId={administrationId} owner={{ kind: 'template', id: tpl.id ?? '' }} groups={groups} startOpen={empty} />
      </section>
      {confirming && (
        <DangerDialog
          question={t('taskTemplates.confirmDelete')}
          busy={remove.isPending}
          onCancel={() => setConfirming(false)}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(base) })}
        />
      )}
    </Page>
  )
}
