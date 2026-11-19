import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { todayDay } from '../forms/calendar'
import { DateInput } from '../forms/DatePicker'
import { FormRow, FormSection, formErrors, TextArea, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type PeriodKind, periodRange, previousPeriod } from './period'
import { type TaskList, type TaskListChanges, useSaveTaskList, useTaskList } from './queries'
import './tasklists.css'

/** Adds a task list (/task_lists/lists/new). */
export function NewTaskListPage() {
  return <TaskListForm />
}

/** Edits a task list's name, description and deadline (/task_lists/lists/:taskListId/edit). */
export function EditTaskListPage() {
  const { administrationId = '', taskListId = '' } = useParams()
  const list = useTaskList(administrationId, taskListId)
  if (list.error instanceof ApiError && list.error.status < 500) {
    return <NotFoundPage />
  }
  if (!list.data) {
    return <Loading />
  }
  return <TaskListForm key={list.data.id} list={list.data} />
}

const kinds: PeriodKind[] = ['none', 'month', 'quarter', 'year']

/**
 * Moneybird's task list form: a name and description, then for a new list
 * its period type and period, and a deadline. A list keeps its period.
 */
function TaskListForm({ list }: { list?: TaskList }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const save = useSaveTaskList(administrationId, list?.id)
  const today = todayDay()
  const proposed = previousPeriod('month', today)
  const [name, setName] = useState(list?.name ?? '')
  const [description, setDescription] = useState(list?.description ?? '')
  const [kind, setKind] = useState<PeriodKind>('month')
  const [year, setYear] = useState(proposed.year)
  const [part, setPart] = useState(proposed.part)
  const [deadline, setDeadline] = useState(list?.deadline_on ?? '')
  const kindLabel = useId()
  const base = `/${administrationId}/task_lists/lists`
  const labels: Record<string, string> = {
    name: t('taskLists.name'), description: t('taskLists.description'), period_begin: t('taskLists.period'), period_end: t('taskLists.period'), deadline_on: t('taskLists.deadline'),
  }
  const errors = formErrors(save.error, labels)
  const thisYear = Number(today.slice(0, 4))
  const years = [thisYear - 2, thisYear - 1, thisYear, thisYear + 1]

  const chooseKind = (k: PeriodKind) => {
    setKind(k)
    const p = previousPeriod(k, today)
    setYear(p.year)
    setPart(p.part)
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: TaskListChanges = { name: name.trim(), description: description.trim(), deadline_on: deadline || null }
    if (!list) {
      const range = kind === 'none' ? { begin: null, end: null } : periodRange(kind, year, part)
      body.period_begin = range.begin
      body.period_end = range.end
    }
    save.mutate(body, { onSuccess: (saved) => navigate(`${base}/${saved.id}`) })
  }

  const month = (m: number) => new Date(Date.UTC(2000, m - 1, 1)).toLocaleDateString(i18n.language, { month: 'long', timeZone: 'UTC' })

  return (
    <Page wide title={list ? t('taskLists.editTitle') : t('taskLists.newTitle')}>
      <form className="form-sheet task-list-form" onSubmit={submit} noValidate aria-label={t('taskLists.form')}>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name ?? ''} placeholder={t('taskLists.name')} maxLength={255} value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
          <TextArea
            label={labels.description ?? ''} rows={2} placeholder={t('taskLists.descriptionPlaceholder')}
            value={description} errors={errors.field('description')} onChange={(e) => setDescription(e.target.value)}
          />
          {!list && (
            <>
              <div className="form-input form-input--long">
                <span className="form-input__label" id={kindLabel}>{t('taskLists.periodType')}</span>
                <div className="form-input__body">
                  <div className="radio-list" role="radiogroup" aria-labelledby={kindLabel}>
                    {kinds.map((k) => (
                      <label key={k} className="radio-list__item">
                        <input type="radio" name={kindLabel} checked={kind === k} onChange={() => chooseKind(k)} />
                        <span>{t(`taskLists.periodKinds.${k}`)}</span>
                      </label>
                    ))}
                  </div>
                </div>
              </div>
              {kind !== 'none' && (
                <FormRow label={t('taskLists.period')} errors={errors.field('period_begin') ?? errors.field('period_end')}>
                  {({ labelId: _, ...control }) => (
                    <div className="task-list-form__period">
                      <select {...control} aria-label={t('taskLists.periodYear')} value={year} onChange={(e) => setYear(Number(e.target.value))}>
                        {years.map((y) => <option key={y} value={y}>{y}</option>)}
                      </select>
                      {kind !== 'year' && (
                        <select aria-label={kind === 'month' ? t('taskLists.month') : t('taskLists.quarter')} value={part} onChange={(e) => setPart(Number(e.target.value))}>
                          {(kind === 'month' ? [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12] : [1, 2, 3, 4]).map((n) => (
                            <option key={n} value={n}>{kind === 'month' ? month(n) : t('list.quarterShort', { n })}</option>
                          ))}
                        </select>
                      )}
                    </div>
                  )}
                </FormRow>
              )}
            </>
          )}
          <DateInput label={labels.deadline_on ?? ''} value={deadline} errors={errors.field('deadline_on')} onChange={setDeadline} />
        </FormSection>
        <div className="form-sheet__actions form-sheet__actions--center">
          <button className="btn btn--primary" disabled={save.isPending}>{t('taskLists.save')}</button>
          <Link className="btn btn--secondary" to={list ? `${base}/${list.id}` : base}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
