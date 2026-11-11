import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable, listDate } from '../list/DocumentTable'
import { ListActions } from '../list/ListActions'
import { perPage, type TaskList, useTaskLists } from './queries'

/** The administration's task lists, newest first and paged (/task_lists/lists). */
export function TaskListsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const page = Math.max(1, Number(params.get('page')) || 1)
  const lists = useTaskLists(administrationId, page)
  const base = `/${administrationId}/task_lists`
  const shown = lists.data?.lists ?? []

  return (
    <Page wide title={t('taskLists.title')} actions={<ListActions exports={[]} primary={{ to: `${base}/new`, label: t('taskLists.add') }} />}>
      {!lists.data ? (
        lists.error ? <p className="empty">{t('taskLists.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{t('taskLists.empty')}</p>
      ) : (
        <DocumentTable<TaskList>
          label={t('taskLists.title')}
          rows={shown}
          rowKey={(l) => l.id ?? ''}
          href={(l) => `${base}/${l.id}`}
          total={lists.data.total}
          perPage={perPage}
          columns={[
            { key: 'name', header: t('taskLists.name'), link: true, cell: (l) => l.name },
            { key: 'period', header: t('taskLists.period'), kind: 'extra', cell: (l) => period(l) },
            { key: 'deadline', header: t('taskLists.deadline'), kind: 'date', cell: (l) => listDate(l.deadline_on) },
            { key: 'progress', header: t('taskLists.progress'), kind: 'amount', cell: (l) => t('taskLists.done', { done: l.completed_tasks_count, total: l.total_tasks_count }) },
            { key: 'state', header: t('taskLists.status'), kind: 'state', cell: (l) => <ListBadge list={l} /> },
          ]}
        />
      )}
    </Page>
  )
}

export function period(list: TaskList): string {
  return list.period_begin && list.period_end ? `${list.period_begin} – ${list.period_end}` : ''
}

export function ListBadge({ list }: { list: TaskList }) {
  const { t } = useTranslation()
  return <span className={`badge badge--${list.completed ? 'paid' : 'open'}`}>{t(`taskLists.states.${list.completed ? 'completed' : 'open'}`)}</span>
}
