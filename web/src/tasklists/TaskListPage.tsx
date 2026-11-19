import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { DangerDialog } from '../layout/Modal'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { listDate } from '../list/DocumentTable'
import { ActionMenu } from '../list/ListActions'
import { TaskGroups } from './TaskGroups'
import { Progress } from './TaskListsPage'
import { type TaskGroup, type TaskList, useDeleteTaskList, useTaskList } from './queries'
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

/**
 * Moneybird's task list: its progress, deadline and description on top, and
 * below them its tasks by group, each ticked off, edited or removed in place.
 */
function TaskListView({ administrationId, list }: { administrationId: string; list: TaskList }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const listId = list.id ?? ''
  const [showCompleted, setShowCompleted] = useState(true)
  const [confirming, setConfirming] = useState(false)
  const remove = useDeleteTaskList(administrationId, listId)
  const switchId = useId()
  const base = `/${administrationId}/task_lists/lists`

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
          <ActionMenu iconOnly icon={<ListIcon kind="more" />} label={t('taskLists.listOptions')} items={[{ label: t('taskLists.delete'), danger: true, onSelect: () => setConfirming(true) }]} />
        </>
      }
    >
      <Problems messages={formErrors(remove.error, {}).general} />
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
        <TaskGroups
          administrationId={administrationId}
          owner={{ kind: 'list', id: listId }}
          groups={(list.groups ?? []) as TaskGroup[]}
          showCompleted={showCompleted}
          startOpen={list.total_tasks_count === 0}
        />
      </section>
      {confirming && (
        <DangerDialog
          question={t('taskLists.confirmDelete')}
          busy={remove.isPending}
          onCancel={() => setConfirming(false)}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(base) })}
        />
      )}
    </Page>
  )
}
