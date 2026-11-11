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
import { ListBadge, period } from './TaskListsPage'
import { type Task, type TaskGroup, type TaskList, useAddTask, useCompleteTask, useCreateTaskList, useDeleteTaskList, useTaskList } from './queries'

/** One task list (/task_lists/:taskListId), or a new one (/task_lists/new). */
export function TaskListPage() {
  const { t } = useTranslation()
  const { administrationId = '', taskListId } = useParams()
  const list = useTaskList(administrationId, taskListId)
  if (taskListId && list.error instanceof ApiError && list.error.status < 500) {
    return <NotFoundPage />
  }
  if (!taskListId) {
    return (
      <Page title={t('taskLists.newTitle')} back={{ to: `/${administrationId}/task_lists/lists`, label: t('taskLists.backToList') }}>
        <NewTaskList administrationId={administrationId} />
      </Page>
    )
  }
  if (!list.data) {
    return <Loading />
  }
  return <TaskListView administrationId={administrationId} list={list.data} />
}

function TaskListView({ administrationId, list }: { administrationId: string; list: TaskList }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const remove = useDeleteTaskList(administrationId, list.id ?? '')
  const complete = useCompleteTask(administrationId, list.id ?? '')
  const [confirming, setConfirming] = useState(false)
  const back = `/${administrationId}/task_lists/lists`
  const more: MenuItem[] = [{ label: t('taskLists.delete'), danger: true, onSelect: () => setConfirming(true) }]
  const rows: DetailRow[] = [
    [t('taskLists.status'), <ListBadge key="state" list={list} />],
    [t('taskLists.period'), period(list)],
    [t('taskLists.deadline'), list.deadline_on ?? ''],
    [t('taskLists.progress'), t('taskLists.done', { done: list.completed_tasks_count, total: list.total_tasks_count })],
    [t('taskLists.description'), list.description ?? ''],
  ]

  return (
    <Page title={list.name ?? ''} back={{ to: back, label: t('taskLists.backToList') }} actions={<MoreMenu items={more} />}>
      <Problems messages={formErrors(remove.error ?? complete.error, {}).general} />
      {confirming && (
        <ConfirmPanel
          message={t('taskLists.confirmDeleteText')}
          confirmLabel={t('taskLists.confirmDelete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(back) })}
          onCancel={() => setConfirming(false)}
        />
      )}
      <DetailBlock title={t('taskLists.detailsTitle')}>
        <DetailList rows={rows} />
      </DetailBlock>
      {(list.groups ?? []).map((g) => (
        <DetailBlock key={g.id} title={g.name ?? t('taskLists.tasks')}>
          <ul className="task-list" aria-label={g.name ?? t('taskLists.tasks')}>
            {(g.tasks ?? []).map((task) => (
              <TaskItem key={task.id} task={task} disabled={complete.isPending} onToggle={(id, done) => complete.mutate({ task: id, done })} />
            ))}
          </ul>
          {!g.report_type && <AddTask administrationId={administrationId} listId={list.id ?? ''} group={g} />}
        </DetailBlock>
      ))}
    </Page>
  )
}

function TaskItem({ task, disabled, onToggle }: { task: Task; disabled: boolean; onToggle: (id: string, done: boolean) => void }) {
  const done = !!task.completed_at
  return (
    <li className="task-list__item">
      <label className="check">
        <input type="checkbox" checked={done} disabled={disabled} onChange={() => onToggle(task.id ?? '', !done)} />
        <span>{task.name}</span>
      </label>
      {task.description && <p className="task-list__description">{task.description}</p>}
      {(task.subtasks ?? []).length > 0 && (
        <ul className="task-list task-list--sub">
          {(task.subtasks ?? []).map((sub) => <TaskItem key={sub.id} task={sub} disabled={disabled} onToggle={onToggle} />)}
        </ul>
      )}
    </li>
  )
}

function AddTask({ administrationId, listId, group }: { administrationId: string; listId: string; group: TaskGroup }) {
  const { t } = useTranslation()
  const add = useAddTask(administrationId, listId)
  const [name, setName] = useState('')
  const errors = formErrors(add.error, { name: t('taskLists.taskName') })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    add.mutate({ group: group.id ?? '', name: name.trim() }, { onSuccess: () => setName('') })
  }
  return (
    <form className="task-list__add" onSubmit={submit} noValidate aria-label={t('taskLists.addTaskTo', { group: group.name ?? t('taskLists.tasks') })}>
      <Problems messages={errors.general} />
      <TextInput label={t('taskLists.taskName')} maxLength={255} value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
      <button className="btn btn--secondary" disabled={add.isPending || name.trim() === ''}>{t('taskLists.addTask')}</button>
    </form>
  )
}

function NewTaskList({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const save = useCreateTaskList(administrationId)
  const [name, setName] = useState('')
  const labels: Record<string, string> = { name: t('taskLists.name') }
  const errors = formErrors(save.error, labels)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(name.trim(), { onSuccess: (saved) => navigate(`/${administrationId}/task_lists/${saved.id}`) })
  }
  return (
    <form className="form-sheet" onSubmit={submit} noValidate aria-label={t('taskLists.form')}>
      <Problems messages={errors.general} />
      <FormSection>
        <TextInput label={labels.name ?? ''} required maxLength={255} value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
      </FormSection>
      <div className="form-sheet__actions">
        <button className="btn btn--primary" disabled={save.isPending}>{t('taskLists.save')}</button>
        <Link className="btn btn--secondary" to={`/${administrationId}/task_lists/lists`}>{t('common.cancel')}</Link>
      </div>
    </form>
  )
}
