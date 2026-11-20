import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { DangerDialog } from '../layout/Modal'
import { ActionMenu } from '../list/ListActions'
import { useUsers } from '../management/queries'
import {
  type Owner, type ReportType, reportTypes, type Task, type TaskChanges, type TaskGroup, useCompleteTask, useDeleteGroup, useDeleteTask, useSaveGroup, useSaveTask,
} from './queries'

/** Which inline form is open: a new task in a group, a task, a new group or a group's name. */
type Open = { kind: 'new-task'; group: string } | { kind: 'task'; id: string } | { kind: 'new-group' } | { kind: 'group'; id: string } | null

/**
 * The groups of a task list or template, after Moneybird: the first group's
 * tasks without a heading, then each named group with its tasks, every one
 * added, edited, ticked off or removed in place, and Add group at the end.
 * A template's tasks have no tick box and no assignee.
 */
export function TaskGroups({ administrationId, owner, groups, showCompleted = true, startOpen = false }: {
  administrationId: string; owner: Owner; groups: TaskGroup[]; showCompleted?: boolean; startOpen?: boolean
}) {
  const { t } = useTranslation()
  const template = owner.kind === 'template'
  const first = groups[0]
  const [open, setOpen] = useState<Open>(() => (startOpen && first ? { kind: 'new-task', group: first.id ?? '' } : null))
  const [confirm, setConfirm] = useState<{ kind: 'group' | 'task'; id: string } | null>(null)
  const removeGroup = useDeleteGroup(administrationId, owner)
  const removeTask = useDeleteTask(administrationId, owner)
  const complete = useCompleteTask(administrationId, owner)
  const ctx: Ctx = { administrationId, owner, template, showCompleted, open, setOpen, onToggle: (id, done) => complete.mutate({ task: id, done }), onDelete: (id) => setConfirm({ kind: 'task', id }) }

  const addTask = (g: TaskGroup) =>
    !g.report_type && (open?.kind === 'new-task' && open.group === g.id ? (
      <TaskForm ctx={ctx} group={g.id ?? ''} onDone={() => setOpen(null)} />
    ) : (
      <button type="button" className="task-row task-row--add" onClick={() => setOpen({ kind: 'new-task', group: g.id ?? '' })}>
        <ListIcon kind="plus" />
        <span>{t('taskLists.addTask')}</span>
      </button>
    ))

  return (
    <div className="task-sheet__body">
      <Problems messages={formErrors(removeGroup.error ?? removeTask.error ?? complete.error, {}).general} />
      {groups.map((g, i) => (
        <div key={g.id} className="task-group" role="group" aria-label={g.name ?? t('taskLists.tasks')}>
          {i > 0 && (open?.kind === 'group' && open.id === g.id ? (
            <GroupForm administrationId={administrationId} owner={owner} group={g} onDone={() => setOpen(null)} />
          ) : (
            <div className="task-group__head">
              <h3 className="task-group__title">{g.name}</h3>
              <hr aria-hidden="true" />
              <ActionMenu
                iconOnly
                icon={<ListIcon kind="more" />}
                label={t('taskLists.groupOptions', { name: g.name })}
                items={[
                  { label: t('taskLists.editGroup'), bookkeeping: true, onSelect: () => setOpen({ kind: 'group', id: g.id ?? '' }) },
                  { label: t('taskLists.deleteGroup'), danger: true, bookkeeping: true, onSelect: () => setConfirm({ kind: 'group', id: g.id ?? '' }) },
                ]}
              />
            </div>
          ))}
          <div className={i > 0 ? 'task-group__tasks' : undefined}>
            <TaskRows ctx={ctx} tasks={(g.tasks ?? []) as Task[]} />
            {addTask(g)}
          </div>
        </div>
      ))}
      {open?.kind === 'new-group' ? (
        <GroupForm administrationId={administrationId} owner={owner} onDone={() => setOpen(null)} />
      ) : (
        <button type="button" className="task-group__head task-group__head--add" onClick={() => setOpen({ kind: 'new-group' })}>
          <ListIcon kind="plus" />
          <span className="task-group__title">{t('taskLists.addGroup')}</span>
          <hr aria-hidden="true" />
        </button>
      )}
      {confirm && (
        <DangerDialog
          question={t(confirm.kind === 'group' ? 'taskLists.confirmDeleteGroup' : 'taskLists.confirmDeleteTask')}
          busy={removeGroup.isPending || removeTask.isPending}
          onCancel={() => setConfirm(null)}
          onConfirm={() => (confirm.kind === 'group' ? removeGroup : removeTask).mutate(confirm.id, { onSettled: () => setConfirm(null) })}
        />
      )}
    </div>
  )
}

/** What every row and form of the groups needs. */
type Ctx = {
  administrationId: string; owner: Owner; template: boolean; showCompleted: boolean; open: Open; setOpen: (o: Open) => void
  onToggle: (id: string, done: boolean) => void; onDelete: (id: string) => void
}

function TaskRows({ ctx, tasks }: { ctx: Ctx; tasks: Task[] }) {
  const { t } = useTranslation()
  const { showCompleted, open, setOpen, onToggle, onDelete, template } = ctx
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
              <TaskForm ctx={ctx} task={task} onDone={() => setOpen(null)} />
            ) : (
              <div className="task-row">
                {template ? <span className="task-row__box" aria-hidden="true" /> : <input type="checkbox" aria-label={task.name ?? ''} checked={done} onChange={() => onToggle(id, !done)} />}
                <span className={`task-row__name${done ? ' is-done' : ''}`}>{task.name}</span>
                <ActionMenu
                  iconOnly
                  icon={<ListIcon kind="more" />}
                  label={t('taskLists.taskOptions', { name: task.name })}
                  items={[
                    { label: t('taskLists.editTask'), bookkeeping: true, onSelect: () => setOpen({ kind: 'task', id }) },
                    ...(task.generated ? [] : [{ label: t('taskLists.deleteTask'), danger: true, bookkeeping: true, onSelect: () => onDelete(id) }]),
                  ]}
                />
              </div>
            )}
            {(task.subtasks ?? []).length > 0 && (
              <div className="task-rows__sub">
                <TaskRows ctx={ctx} tasks={(task.subtasks ?? []) as Task[]} />
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
function TaskForm({ ctx, group, task, onDone }: { ctx: Ctx; group?: string; task?: Task; onDone: () => void }) {
  const { t } = useTranslation()
  const { administrationId, owner, template } = ctx
  const save = useSaveTask(administrationId, owner)
  const users = useUsers(administrationId, !template).data ?? []
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
        {template ? <span className="task-row__box" aria-hidden="true" /> : <input type="checkbox" disabled checked={!!task?.completed_at} aria-hidden="true" tabIndex={-1} />}
        <input type="text" autoFocus aria-label={t('taskLists.taskName')} placeholder={t('taskLists.taskPlaceholder')} maxLength={255} value={name} onChange={(e) => setName(e.target.value)} />
        <button className="btn btn--primary" disabled={save.isPending}>{t('taskLists.save')}</button>
        <button type="button" className="btn btn--secondary" onClick={onDone}>{t('common.cancel')}</button>
      </div>
      <input
        type="text" className="task-form__description" aria-label={t('taskLists.description')} placeholder={t('taskLists.descriptionPlaceholder')}
        value={description} onChange={(e) => setDescription(e.target.value)}
      />
      <div className="task-form__lower">
        {!template && (
          <select aria-label={t('taskLists.assignee')} value={assignee} onChange={(e) => setAssignee(e.target.value)}>
            <option value="">{t('taskLists.assignTo')}</option>
            {users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
          </select>
        )}
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
function GroupForm({ administrationId, owner, group, onDone }: { administrationId: string; owner: Owner; group?: TaskGroup; onDone: () => void }) {
  const { t } = useTranslation()
  const save = useSaveGroup(administrationId, owner)
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
