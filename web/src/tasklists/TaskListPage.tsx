import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { formErrors } from '../forms/Form'
import { useUsers } from '../management/queries'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { DangerDialog } from '../layout/Modal'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { ActionMenu } from '../list/ListActions'
import { listDate } from '../list/DocumentTable'
import { Progress } from './TaskListsPage'
import {
  type ReportType, reportTypes, type Task, type TaskChanges, type TaskGroup, type TaskList, useCompleteTask, useDeleteGroup, useDeleteTask,
  useDeleteTaskList, useSaveGroup, useSaveTask, useTaskList,
} from './queries'
import './tasklists.css'

/** A task list's canonical address (/task_lists/:taskListId) opens it where Moneybird keeps it. */
export function TaskListRedirect() {
  const { administrationId = '', taskListId = '' } = useParams()
  return <Navigate replace to={`/${administrationId}/task_lists/lists/${taskListId}`} />
}

/** One task list (/task_lists/lists/:taskListId). */
export function TaskListPage() {
  const { administrationId = '', taskListId } = useParams()
  const list = useTaskList(administrationId, taskListId)
  if (list.error instanceof ApiError && list.error.status < 500) {
    return <NotFoundPage />
  }
  if (!list.data) {
    return <Loading />
  }
  return <TaskListView key={list.data.id} administrationId={administrationId} list={list.data} />
}

/** Which inline form is open: a new task in a group, a task, a new group or a group's name. */
type Open = { kind: 'new-task'; group: string } | { kind: 'task'; id: string } | { kind: 'new-group' } | { kind: 'group'; id: string } | null

/**
 * Moneybird's task list: its progress, deadline and description on top, and
 * below them its tasks by group, each ticked off, edited or removed in place.
 */
function TaskListView({ administrationId, list }: { administrationId: string; list: TaskList }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const listId = list.id ?? ''
  const groups = (list.groups ?? []) as TaskGroup[]
  const first = groups[0]
  const [open, setOpen] = useState<Open>(() => (list.total_tasks_count === 0 && first ? { kind: 'new-task', group: first.id ?? '' } : null))
  const [showCompleted, setShowCompleted] = useState(true)
  const [confirm, setConfirm] = useState<{ kind: 'list' } | { kind: 'group'; id: string } | { kind: 'task'; id: string } | null>(null)
  const removeList = useDeleteTaskList(administrationId, listId)
  const removeGroup = useDeleteGroup(administrationId, listId)
  const removeTask = useDeleteTask(administrationId, listId)
  const complete = useCompleteTask(administrationId, listId)
  const switchId = useId()
  const base = `/${administrationId}/task_lists/lists`
  const failure = removeList.error ?? removeGroup.error ?? removeTask.error ?? complete.error

  const confirmed = () => {
    if (confirm?.kind === 'list') {
      removeList.mutate(undefined, { onSuccess: () => navigate(base) })
    } else if (confirm?.kind === 'group') {
      removeGroup.mutate(confirm.id, { onSettled: () => setConfirm(null) })
    } else if (confirm?.kind === 'task') {
      removeTask.mutate(confirm.id, { onSettled: () => setConfirm(null) })
    }
  }

  const tasks = (g: TaskGroup) => (
    <TaskRows
      administrationId={administrationId}
      listId={listId}
      tasks={(g.tasks ?? []) as Task[]}
      showCompleted={showCompleted}
      open={open}
      setOpen={setOpen}
      onToggle={(id, done) => complete.mutate({ task: id, done })}
      onDelete={(id) => setConfirm({ kind: 'task', id })}
    />
  )

  const addTask = (g: TaskGroup) =>
    !g.report_type && (open?.kind === 'new-task' && open.group === g.id ? (
      <TaskForm administrationId={administrationId} listId={listId} group={g.id ?? ''} onDone={() => setOpen(null)} />
    ) : (
      <button type="button" className="task-row task-row--add" onClick={() => setOpen({ kind: 'new-task', group: g.id ?? '' })}>
        <ListIcon kind="plus" />
        <span>{t('taskLists.addTask')}</span>
      </button>
    ))

  return (
    <Page
      wide
      title={t('taskLists.viewTitle', { name: list.name })}
      back={{ to: base, label: t('taskLists.backToList') }}
      actions={
        <>
          <Link className="btn btn--secondary btn--with-icon" to={`${base}/${listId}/edit`}>
            <ListIcon kind="edit" />
            <span>{t('taskLists.editList')}</span>
          </Link>
          <ActionMenu iconOnly icon={<ListIcon kind="more" />} label={t('taskLists.listOptions')} items={[{ label: t('taskLists.delete'), danger: true, onSelect: () => setConfirm({ kind: 'list' }) }]} />
        </>
      }
    >
      <Problems messages={formErrors(failure, {}).general} />
      <section className="task-sheet" aria-label={list.name ?? ''}>
        <div className="task-sheet__top">
          <div className="task-sheet__progress">
            <h2 className="task-sheet__label">{t('taskLists.progress')}</h2>
            <Progress large done={list.completed_tasks_count ?? 0} total={list.total_tasks_count ?? 0} />
          </div>
          <div className="task-sheet__deadline">
            <h2 className="task-sheet__label task-sheet__label--plain">{t('taskLists.deadline')}</h2>
            <p>{list.deadline_on ? listDate(list.deadline_on) : t('taskLists.noDeadline')}</p>
          </div>
          <div className="task-sheet__about">
            <h2 className="task-sheet__label task-sheet__label--plain">{t('taskLists.about')}</h2>
            {list.description && <p>{list.description}</p>}
          </div>
          <label className="task-sheet__switch" htmlFor={switchId}>
            <input id={switchId} className="switch" type="checkbox" role="switch" checked={showCompleted} onChange={(e) => setShowCompleted(e.target.checked)} />
            <span>{t('taskLists.showCompleted')}</span>
          </label>
        </div>
        <div className="task-sheet__body">
          {groups.map((g, i) => (
            <div key={g.id} className="task-group" role="group" aria-label={g.name ?? t('taskLists.tasks')}>
              {i > 0 && (open?.kind === 'group' && open.id === g.id ? (
                <GroupForm administrationId={administrationId} listId={listId} group={g} onDone={() => setOpen(null)} />
              ) : (
                <div className="task-group__head">
                  <h3 className="task-group__title">{g.name}</h3>
                  <hr aria-hidden="true" />
                  <ActionMenu
                    iconOnly
                    icon={<ListIcon kind="more" />}
                    label={t('taskLists.groupOptions', { name: g.name })}
                    items={[
                      { label: t('taskLists.editGroup'), onSelect: () => setOpen({ kind: 'group', id: g.id ?? '' }) },
                      { label: t('taskLists.deleteGroup'), danger: true, onSelect: () => setConfirm({ kind: 'group', id: g.id ?? '' }) },
                    ]}
                  />
                </div>
              ))}
              <div className={i > 0 ? 'task-group__tasks' : undefined}>
                {tasks(g)}
                {addTask(g)}
              </div>
            </div>
          ))}
          {open?.kind === 'new-group' ? (
            <GroupForm administrationId={administrationId} listId={listId} onDone={() => setOpen(null)} />
          ) : (
            <button type="button" className="task-group__head task-group__head--add" onClick={() => setOpen({ kind: 'new-group' })}>
              <ListIcon kind="plus" />
              <span className="task-group__title">{t('taskLists.addGroup')}</span>
              <hr aria-hidden="true" />
            </button>
          )}
        </div>
      </section>
      {confirm && (
        <DangerDialog
          question={t(confirm.kind === 'list' ? 'taskLists.confirmDelete' : confirm.kind === 'group' ? 'taskLists.confirmDeleteGroup' : 'taskLists.confirmDeleteTask')}
          busy={removeList.isPending || removeGroup.isPending || removeTask.isPending}
          onCancel={() => setConfirm(null)}
          onConfirm={confirmed}
        />
      )}
    </Page>
  )
}

function TaskRows({ administrationId, listId, tasks, showCompleted, open, setOpen, onToggle, onDelete }: {
  administrationId: string; listId: string; tasks: Task[]; showCompleted: boolean; open: Open; setOpen: (o: Open) => void
  onToggle: (id: string, done: boolean) => void; onDelete: (id: string) => void
}) {
  const { t } = useTranslation()
  const shown = tasks.filter((task) => showCompleted || !task.completed_at)
  if (shown.length === 0) {
    return null
  }
  return (
    <ul className="task-rows">
      {shown.map((task) => {
        const done = !!task.completed_at
        const id = task.id ?? ''
        return (
          <li key={id}>
            {open?.kind === 'task' && open.id === id ? (
              <TaskForm administrationId={administrationId} listId={listId} task={task} onDone={() => setOpen(null)} />
            ) : (
              <div className="task-row">
                <input type="checkbox" aria-label={task.name ?? ''} checked={done} onChange={() => onToggle(id, !done)} />
                <span className={`task-row__name${done ? ' is-done' : ''}`}>{task.name}</span>
                <ActionMenu
                  iconOnly
                  icon={<ListIcon kind="more" />}
                  label={t('taskLists.taskOptions', { name: task.name })}
                  items={[
                    { label: t('taskLists.editTask'), onSelect: () => setOpen({ kind: 'task', id }) },
                    ...(task.generated ? [] : [{ label: t('taskLists.deleteTask'), danger: true, onSelect: () => onDelete(id) }]),
                  ]}
                />
              </div>
            )}
            {(task.subtasks ?? []).length > 0 && (
              <div className="task-rows__sub">
                <TaskRows
                  administrationId={administrationId} listId={listId} tasks={(task.subtasks ?? []) as Task[]} showCompleted={showCompleted}
                  open={open} setOpen={setOpen} onToggle={onToggle} onDelete={onDelete}
                />
              </div>
            )}
          </li>
        )
      })}
    </ul>
  )
}

type Kind = 'none' | 'category' | 'report'

/**
 * Moneybird's inline task form: the task and its description, who does it,
 * and whether it is about a category (with a subtask per sub-category) or a report.
 */
function TaskForm({ administrationId, listId, group, task, onDone }: { administrationId: string; listId: string; group?: string; task?: Task; onDone: () => void }) {
  const { t } = useTranslation()
  const save = useSaveTask(administrationId, listId)
  const users = useUsers(administrationId).data ?? []
  const accounts = (useLedgerAccounts(administrationId).data ?? []).filter((a) => a.active !== false)
  const [name, setName] = useState(task?.name ?? '')
  const [description, setDescription] = useState(task?.description ?? '')
  const [assignee, setAssignee] = useState(task?.assignee_id ?? '')
  const [kind, setKind] = useState<Kind>(task?.related_entity_id ? 'category' : task?.report_type ? 'report' : 'none')
  const [category, setCategory] = useState(task?.related_entity_id ?? '')
  const [subtasks, setSubtasks] = useState(false)
  const [report, setReport] = useState<ReportType | ''>((task?.report_type ?? '') as ReportType | '')
  const kindName = useId()
  const errors = formErrors(save.error, { name: t('taskLists.taskName'), description: t('taskLists.description'), assignee_id: t('taskLists.assignee') })
  const problems = [...errors.general, ...['name', 'description', 'assignee_id', 'related_entity_id', 'report_type', 'generate_subtasks'].flatMap((f) => errors.field(f) ?? [])]

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: TaskChanges = { name: name.trim(), description: description.trim() }
    if (assignee) {
      body.assignee_id = assignee
    }
    if (kind === 'category' && category) {
      body.related_entity_id = category
      body.generate_subtasks = subtasks
    } else if (task?.related_entity_id) {
      body.related_entity_id = null
    }
    if (kind === 'report' && report) {
      body.report_type = report
    } else if (task?.report_type) {
      body.report_type = null
    }
    save.mutate({ group, id: task?.id, task: body, unassign: !assignee && !!task?.assignee_id }, { onSuccess: onDone })
  }

  return (
    <form className="task-form" onSubmit={submit} noValidate aria-label={task ? t('taskLists.editTask') : t('taskLists.addTask')}>
      <div className="task-form__upper">
        <input type="checkbox" disabled checked={!!task?.completed_at} aria-hidden="true" tabIndex={-1} />
        <input type="text" autoFocus aria-label={t('taskLists.taskName')} placeholder={t('taskLists.taskPlaceholder')} maxLength={255} value={name} onChange={(e) => setName(e.target.value)} />
        <button className="btn btn--primary" disabled={save.isPending}>{t('taskLists.save')}</button>
        <button type="button" className="btn btn--secondary" onClick={onDone}>{t('common.cancel')}</button>
      </div>
      <input
        type="text" className="task-form__description" aria-label={t('taskLists.description')} placeholder={t('taskLists.descriptionPlaceholder')}
        value={description} onChange={(e) => setDescription(e.target.value)}
      />
      <div className="task-form__lower">
        <select aria-label={t('taskLists.assignee')} value={assignee} onChange={(e) => setAssignee(e.target.value)}>
          <option value="">{t('taskLists.assignTo')}</option>
          {users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </select>
        <fieldset className="segmented-control" role="radiogroup" aria-label={t('taskLists.taskType')}>
          {(['none', 'category', 'report'] as const).map((k) => (
            <label key={k} className={`segmented-control__option${kind === k ? ' is-checked' : ''}`}>
              <input type="radio" name={kindName} checked={kind === k} onChange={() => setKind(k)} />
              {t(`taskLists.types.${k}`)}
            </label>
          ))}
        </fieldset>
        {kind === 'category' && (
          <>
            <select aria-label={t('taskLists.category')} value={category} onChange={(e) => setCategory(e.target.value)}>
              <option value="">{t('taskLists.noCategory')}</option>
              {accounts.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
            </select>
            <label className="task-form__check">
              <input type="checkbox" checked={subtasks} onChange={(e) => setSubtasks(e.target.checked)} />
              <span>{t('taskLists.addSubcategories')}</span>
            </label>
          </>
        )}
        {kind === 'report' && (
          <select aria-label={t('taskLists.report')} value={report} onChange={(e) => setReport(e.target.value as ReportType | '')}>
            <option value="">{t('taskLists.noReport')}</option>
            {reportTypes.map((r) => <option key={r} value={r}>{t(`taskLists.reports.${r}`)}</option>)}
          </select>
        )}
      </div>
      <Problems messages={problems} />
    </form>
  )
}

/** A group's name, inline in the list, as Moneybird's Add group and Edit group. */
function GroupForm({ administrationId, listId, group, onDone }: { administrationId: string; listId: string; group?: TaskGroup; onDone: () => void }) {
  const { t } = useTranslation()
  const save = useSaveGroup(administrationId, listId)
  const [name, setName] = useState(group?.name ?? '')
  const errors = formErrors(save.error, { name: t('taskLists.groupName') })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate({ id: group?.id, name: name.trim() }, { onSuccess: onDone })
  }
  return (
    <form className="task-form task-form--group" onSubmit={submit} noValidate aria-label={group ? t('taskLists.editGroup') : t('taskLists.addGroup')}>
      <div className="task-form__upper">
        <input type="text" autoFocus aria-label={t('taskLists.groupName')} placeholder={t('taskLists.groupName')} maxLength={255} value={name} onChange={(e) => setName(e.target.value)} />
        <button className="btn btn--primary" disabled={save.isPending}>{t('taskLists.save')}</button>
        <button type="button" className="btn btn--secondary" onClick={onDone}>{t('common.cancel')}</button>
      </div>
      <Problems messages={[...errors.general, ...(errors.field('name') ?? [])]} />
    </form>
  )
}
