import { ActionLink } from '../layout/ActionLink'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { InfoIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { DangerDialog } from '../layout/Modal'
import { Page } from '../layout/Page'
import { DocumentTable, listDate } from '../list/DocumentTable'
import { GettingStarted } from '../list/GettingStarted'
import { ActionMenu } from '../list/ListActions'
import { periodLabel } from './period'
import { perPage, type TaskList, useDeleteTaskList, useTaskLists } from './queries'
import './tasklists.css'

/**
 * The administration's task lists, after Moneybird's: each with its period,
 * deadline and progress, and its own menu to edit or delete it (/task_lists/lists).
 */
export function TaskListsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const page = Math.max(1, Number(params.get('page')) || 1)
  const lists = useTaskLists(administrationId, page)
  const [info, setInfo] = useState<boolean | null>(null)
  const [deleting, setDeleting] = useState<TaskList | null>(null)
  const introId = useId()
  const base = `/${administrationId}/task_lists/lists`
  const shown = lists.data?.lists ?? []
  const firstUse = lists.data !== undefined && lists.data.total === 0
  const introShown = info ?? firstUse

  return (
    <Page
      wide
      title={t('taskLists.title')}
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
          <ActionMenu
            primary
            label={t('taskLists.add')}
            items={[
              { label: t('taskLists.addEmpty'), to: `${base}/new` },
              { label: t('taskLists.addFromTemplate'), to: `${base}/templates_selection` },
            ]}
          />
        </>
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('taskLists.intro.title')}
          text={t('taskLists.intro.text')}
          steps={[
            { icon: 'check', title: t('taskLists.intro.tasks'), text: t('taskLists.intro.tasksText') },
            { icon: 'chart', title: t('taskLists.intro.progress'), text: t('taskLists.intro.progressText') },
            { icon: 'person', title: t('taskLists.intro.together'), text: t('taskLists.intro.togetherText') },
          ]}
        />
      )}
      {!lists.data ? (
        lists.error ? <p className="empty">{t('taskLists.loadError')}</p> : <Loading />
      ) : (
        <div className="task-lists">
          <DocumentTable<TaskList>
            label={t('taskLists.title')}
            rows={shown}
            rowKey={(l) => l.id ?? ''}
            href={(l) => `${base}/${l.id}`}
            total={lists.data.total}
            perPage={perPage}
            footer={firstUse && (
              <ActionLink className="document-table__add" to={`${base}/new`}>
                <ListIcon kind="plus" />
                {t('taskLists.add')}
              </ActionLink>
            )}
            columns={[
              { key: 'name', header: t('taskLists.name'), link: true, cell: (l) => l.name },
              { key: 'period', header: t('taskLists.period'), cell: (l) => periodLabel(l.period_begin, l.period_end, i18n.language) },
              { key: 'deadline', header: t('taskLists.deadline'), cell: (l) => listDate(l.deadline_on) },
              { key: 'status', header: t('taskLists.status'), cell: (l) => <Progress done={l.completed_tasks_count ?? 0} total={l.total_tasks_count ?? 0} /> },
              {
                key: 'menu', header: '', label: t('taskLists.options'), kind: 'icons',
                cell: (l) => (
                  <ActionMenu
                    iconOnly
                    icon={<ListIcon kind="more" />}
                    label={t('taskLists.options')}
                    items={[
                      { label: t('taskLists.edit'), to: `${base}/${l.id}/edit` },
                      { label: t('taskLists.delete'), danger: true, bookkeeping: true, onSelect: () => setDeleting(l) },
                    ]}
                  />
                ),
              },
            ]}
          />
        </div>
      )}
      {deleting && <DeleteList administrationId={administrationId} list={deleting} onClose={() => setDeleting(null)} />}
    </Page>
  )
}

function DeleteList({ administrationId, list, onClose }: { administrationId: string; list: TaskList; onClose: () => void }) {
  const { t } = useTranslation()
  const remove = useDeleteTaskList(administrationId, list.id ?? '')
  return <DangerDialog question={t('taskLists.confirmDelete')} busy={remove.isPending} onCancel={onClose} onConfirm={() => remove.mutate(undefined, { onSuccess: onClose })} />
}

/** Moneybird's progress line: how many of the tasks are done, as a bar and in words. */
export function Progress({ done, total, large = false }: { done: number; total: number; large?: boolean }) {
  const { t } = useTranslation()
  const share = total === 0 ? 1 : done / total
  return (
    <span className={`progress-line${large ? ' progress-line--large' : ''}`}>
      <span className="progress-line__track" role="progressbar" aria-label={t('taskLists.progress')} aria-valuemin={0} aria-valuemax={total} aria-valuenow={done}>
        <span className={`progress-line__fill${share === 1 ? ' is-complete' : ''}`} style={{ transform: `scaleX(${share})` }} />
      </span>
      <span className="progress-line__text">{t('taskLists.tasksDone', { done, total })}</span>
    </span>
  )
}
