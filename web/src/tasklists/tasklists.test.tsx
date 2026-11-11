import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { Task, TaskList } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

const task = (over: Partial<Task> = {}): Task => ({
  id: '901', administration_id: '11', name: 'Reconcile the bank', description: 'Every account', row_order: 0, completed_at: null,
  report_type: null, task_lists_group_id: '801', parent_task_id: null, assignee_id: null, subtasks: [], notes: [], events: [],
  ...over,
})

const list = (over: Partial<TaskList> = {}): TaskList => ({
  id: '701', administration_id: '11', name: 'Month end', description: null, deadline_on: '2026-11-10', period_begin: '2026-10-01',
  period_end: '2026-10-31', completed: false, template_id: null, total_tasks_count: 2, completed_tasks_count: 1, uri: '/11/task_lists/701',
  groups: [
    { id: '801', name: null, row_order: 0, report_type: null, tasks: [task()] },
    { id: '802', name: 'VAT', row_order: 1, report_type: null, tasks: [task({ id: '902', name: 'File the return', description: null, task_lists_group_id: '802', completed_at: '2026-11-02T10:00:00Z' })] },
  ],
  ...over,
})

function loaded(lists: TaskList[] = [list()]) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/task_lists', () => HttpResponse.json(lists, { headers: { 'X-Total-Count': String(lists.length) } })),
    http.get('*/api/v2/11/task_lists/701', () => HttpResponse.json(list())),
  ]
}

describe('the task lists', () => {
  it('lists task lists with their period and progress', async () => {
    server.use(...loaded())
    renderAt('/11/task_lists/lists')
    const table = await screen.findByRole('table', { name: 'Task lists' })
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('Month end')
    expect(rows[1]).toHaveTextContent('2026-10-01 – 2026-10-31')
    expect(rows[1]).toHaveTextContent('1 of 2')
    expect(rows[1]).toHaveTextContent('Open')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Month end' })).toHaveAttribute('href', '/11/task_lists/701')
  })

  it('says when there are none yet', async () => {
    server.use(...loaded([]))
    renderAt('/11/task_lists/lists')
    expect(await screen.findByText(/No task lists yet/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add task list' })).toHaveAttribute('href', '/11/task_lists/new')
  })
})

describe('a task list', () => {
  it('shows its tasks by group, completes one and adds one', async () => {
    let completed = ''
    let added: unknown
    server.use(...loaded())
    server.use(
      http.post('*/api/v2/11/task_list_tasks/:id/completion', ({ params }) => {
        completed = String(params.id)
        return HttpResponse.json(task({ completed_at: '2026-11-11T10:00:00Z' }))
      }),
      http.post('*/api/v2/11/task_list_groups/802/tasks', async ({ request }) => {
        added = await request.json()
        return HttpResponse.json(task({ id: '903', name: 'Pay the VAT' }), { status: 201 })
      }),
    )
    renderAt('/11/task_lists/701')
    expect(await screen.findByRole('heading', { name: 'Month end' })).toBeInTheDocument()
    const tasks = screen.getByRole('list', { name: 'Tasks' })
    expect(within(tasks).getByText('Every account')).toBeInTheDocument()
    const vat = screen.getByRole('list', { name: 'VAT' })
    expect(within(vat).getByRole('checkbox', { name: 'File the return' })).toBeChecked()
    await userEvent.click(within(tasks).getByRole('checkbox', { name: 'Reconcile the bank' }))
    await waitFor(() => expect(completed).toBe('901'))

    const form = screen.getByRole('form', { name: 'Add a task to VAT' })
    await userEvent.type(within(form).getByLabelText('New task'), 'Pay the VAT')
    await userEvent.click(within(form).getByRole('button', { name: 'Add task' }))
    await waitFor(() => expect(added).toEqual({ task: { name: 'Pay the VAT' } }))
  })

  it('is added with a name', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/task_lists', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(list(), { status: 201 })
    }))
    renderAt('/11/task_lists/new')
    const form = await screen.findByRole('form', { name: 'Task list' })
    await userEvent.type(within(form).getByLabelText('Name'), 'Month end')
    await userEvent.click(within(form).getByRole('button', { name: 'Save task list' }))
    await waitFor(() => expect(sent).toEqual({ task_list: { name: 'Month end' } }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/task_lists/701'))
  })
})
