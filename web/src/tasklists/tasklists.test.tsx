import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
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
    http.get('*/api/v2/11/users', () => HttpResponse.json([{ id: '5', name: 'Ann de Vries', active: true }])),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([{ id: '61', name: 'Bank', account_type: 'current_assets', active: true }])),
  ]
}

describe('the task lists', () => {
  it('lists task lists with their period, deadline and progress, like Moneybird', async () => {
    server.use(...loaded())
    renderAt('/11/task_lists/lists')
    const table = await screen.findByRole('table', { name: 'Task lists' })
    const rows = within(table).getAllByRole('row')
    expect(rows[0]).toHaveTextContent(/Name\s*Period\s*Deadline\s*Status/)
    expect(rows[1]).toHaveTextContent('Month end')
    expect(rows[1]).toHaveTextContent('Oct 2026')
    expect(rows[1]).toHaveTextContent('1/2 tasks')
    expect(within(rows[1] as HTMLElement).getByRole('progressbar', { name: 'Progress' })).toHaveAttribute('aria-valuenow', '1')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Month end' })).toHaveAttribute('href', '/11/task_lists/lists/701')
    expect(screen.queryByRole('region', { name: 'Getting started with task lists.' })).toBeNull()

    await userEvent.click(within(rows[1] as HTMLElement).getByRole('button', { name: 'Task list options' }))
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Edit' })).toHaveAttribute('href', '/11/task_lists/lists/701/edit')

    await userEvent.click(screen.getByRole('button', { name: 'Add task list' }))
    expect(screen.getByRole('link', { name: 'Add empty task list' })).toHaveAttribute('href', '/11/task_lists/lists/new')
    expect(screen.getByRole('link', { name: 'Add from template' })).toHaveAttribute('href', '/11/task_lists/lists/templates_selection')
  })

  it('introduces task lists until the first one is added', async () => {
    server.use(...loaded([]))
    renderAt('/11/task_lists/lists')
    expect(await screen.findByRole('region', { name: 'Getting started with task lists.' })).toBeInTheDocument()
    const table = await screen.findByRole('table', { name: 'Task lists' })
    expect(within(table).getByRole('link', { name: 'Add task list' })).toHaveAttribute('href', '/11/task_lists/lists/new')
  })

  it('deletes a list from its menu after confirming', async () => {
    let deleted = false
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/task_lists/701', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/task_lists/lists')
    const table = await screen.findByRole('table', { name: 'Task lists' })
    await userEvent.click(within(table).getByRole('button', { name: 'Task list options' }))
    await userEvent.click(within(table).getByRole('button', { name: 'Delete' }))
    await userEvent.click(within(screen.getByRole('dialog', { name: 'Are you sure you want to delete this task list?' })).getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(deleted).toBe(true))
  })
})

describe('the task list form', () => {
  it('adds a list for the month before, as Moneybird proposes', async () => {
    vi.setSystemTime(new Date('2026-11-20T10:00:00'))
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/task_lists', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(list(), { status: 201 })
    }))
    renderAt('/11/task_lists/lists/new')
    const form = await screen.findByRole('form', { name: 'Task list' })
    expect(screen.getByRole('heading', { name: 'Add new task list' })).toBeInTheDocument()
    expect(within(form).getByRole('radio', { name: 'Month' })).toBeChecked()
    expect(within(form).getByRole('combobox', { name: 'Period year' })).toHaveValue('2026')
    expect(within(form).getByRole('combobox', { name: 'Month' })).toHaveValue('10')
    await userEvent.type(within(form).getByLabelText('Name'), 'Month end')
    await userEvent.type(within(form).getByLabelText('Description'), 'Closing')
    await userEvent.click(within(form).getByRole('radio', { name: 'Quarter' }))
    expect(within(form).getByRole('combobox', { name: 'Quarter' })).toHaveValue('3')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ task_list: { name: 'Month end', description: 'Closing', deadline_on: null, period_begin: '2026-07-01', period_end: '2026-09-30' } }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/task_lists/lists/701'))
    vi.useRealTimers()
  })

  it('adds a list without a period', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/task_lists', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(list(), { status: 201 })
    }))
    renderAt('/11/task_lists/lists/new')
    const form = await screen.findByRole('form', { name: 'Task list' })
    await userEvent.type(within(form).getByLabelText('Name'), 'Ad hoc')
    await userEvent.click(within(form).getByRole('radio', { name: 'No period' }))
    expect(within(form).queryByRole('combobox', { name: 'Period year' })).toBeNull()
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ task_list: { name: 'Ad hoc', description: '', deadline_on: null, period_begin: null, period_end: null } }))
  })

  it('edits the name, description and deadline, but not the period', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/task_lists/701', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(list())
    }))
    renderAt('/11/task_lists/lists/701/edit')
    const form = await screen.findByRole('form', { name: 'Task list' })
    expect(within(form).queryByRole('radio', { name: 'Month' })).toBeNull()
    await userEvent.clear(within(form).getByLabelText('Name'))
    await userEvent.type(within(form).getByLabelText('Name'), 'Month closing')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ task_list: { name: 'Month closing', description: '', deadline_on: '2026-11-10' } }))
  })

  it('makes a list from a template', async () => {
    let from = ''
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/task_list_templates', () => HttpResponse.json([{ id: '601', administration_id: '11', name: 'Quarter close', description: 'Each quarter', period: 'P3M', deadline_after: '', groups: [], uri: '/11/task_list_templates/601' }])),
      http.post('*/api/v2/11/task_list_templates/:id/task_lists', ({ params }) => {
        from = String(params.id)
        return HttpResponse.json(list(), { status: 201 })
      }),
    )
    renderAt('/11/task_lists/lists/templates_selection')
    await userEvent.click(await screen.findByRole('button', { name: /Quarter close/ }))
    await waitFor(() => expect(from).toBe('601'))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/task_lists/lists/701'))
  })
})

describe('a task list', () => {
  it('shows its progress and tasks by group, and ticks one off', async () => {
    let completed = ''
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/task_list_tasks/:id/completion', ({ params }) => {
      completed = String(params.id)
      return HttpResponse.json(task({ completed_at: '2026-11-11T10:00:00Z' }))
    }))
    renderAt('/11/task_lists/701')
    expect(await screen.findByRole('heading', { name: 'Task list Month end' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent('/11/task_lists/lists/701')
    expect(screen.getByRole('link', { name: 'To Task lists' })).toHaveAttribute('href', '/11/task_lists/lists')
    expect(screen.getByRole('link', { name: 'Edit task list' })).toHaveAttribute('href', '/11/task_lists/lists/701/edit')
    expect(screen.getByText('1/2 tasks')).toBeInTheDocument()
    const vat = screen.getByRole('group', { name: 'VAT' })
    expect(within(vat).getByRole('heading', { name: 'VAT' })).toBeInTheDocument()
    expect(within(vat).getByRole('checkbox', { name: 'File the return' })).toBeChecked()
    await userEvent.click(screen.getByRole('checkbox', { name: 'Reconcile the bank' }))
    await waitFor(() => expect(completed).toBe('901'))

    await userEvent.click(screen.getByRole('switch', { name: 'Show completed tasks' }))
    expect(screen.queryByRole('checkbox', { name: 'File the return' })).toBeNull()
  })

  it('adds a task with its description, assignee and category', async () => {
    let added: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/task_list_groups/802/tasks', async ({ request }) => {
      added = await request.json()
      return HttpResponse.json(task({ id: '903', name: 'Pay the VAT' }), { status: 201 })
    }))
    renderAt('/11/task_lists/lists/701')
    const vat = await screen.findByRole('group', { name: 'VAT' })
    await userEvent.click(within(vat).getByRole('button', { name: 'Add task' }))
    const form = within(vat).getByRole('form', { name: 'Add task' })
    await userEvent.type(within(form).getByRole('textbox', { name: 'Task' }), 'Pay the VAT')
    await userEvent.type(within(form).getByRole('textbox', { name: 'Description' }), 'Before the 30th')
    await userEvent.selectOptions(within(form).getByRole('combobox', { name: 'Assignee' }), await within(form).findByRole('option', { name: 'Ann de Vries' }))
    await userEvent.click(within(form).getByRole('radio', { name: 'Category' }))
    await userEvent.selectOptions(within(form).getByRole('combobox', { name: 'Category' }), await within(form).findByRole('option', { name: 'Bank' }))
    await userEvent.click(within(form).getByRole('checkbox', { name: 'Add subcategories' }))
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(added).toEqual({
      task: { name: 'Pay the VAT', description: 'Before the 30th', assignee_id: '5', related_entity_id: '61', generate_subtasks: true },
    }))
  })

  it('links a task to a report and unlinks it again', async () => {
    const sent: unknown[] = []
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/task_list_tasks/901', async ({ request }) => {
      sent.push(await request.json())
      return HttpResponse.json(task())
    }))
    renderAt('/11/task_lists/lists/701')
    await userEvent.click(await screen.findByRole('button', { name: 'Task settings of Reconcile the bank' }))
    await userEvent.click(screen.getByRole('button', { name: 'Edit task' }))
    const form = screen.getByRole('form', { name: 'Edit task' })
    await userEvent.click(within(form).getByRole('radio', { name: 'Report' }))
    await userEvent.selectOptions(within(form).getByRole('combobox', { name: 'Report' }), 'debtor')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent[0]).toEqual({ task: { name: 'Reconcile the bank', description: 'Every account', report_type: 'debtor' } }))
  })

  it('adds, renames and deletes a group', async () => {
    let added: unknown
    let renamed: unknown
    let deleted = ''
    server.use(...loaded())
    server.use(
      http.post('*/api/v2/11/task_lists/701/groups', async ({ request }) => {
        added = await request.json()
        return HttpResponse.json({ id: '803', name: 'Payroll', row_order: 2, report_type: null, tasks: [] }, { status: 201 })
      }),
      http.patch('*/api/v2/11/task_list_groups/802', async ({ request }) => {
        renamed = await request.json()
        return HttpResponse.json({ id: '802', name: 'Sales tax', row_order: 1, report_type: null, tasks: [] })
      }),
      http.delete('*/api/v2/11/task_list_groups/:id', ({ params }) => {
        deleted = String(params.id)
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/task_lists/lists/701')
    await userEvent.click(await screen.findByRole('button', { name: 'Add group' }))
    const form = screen.getByRole('form', { name: 'Add group' })
    await userEvent.type(within(form).getByRole('textbox', { name: 'Group name' }), 'Payroll')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(added).toEqual({ group: { name: 'Payroll' } }))

    await userEvent.click(screen.getByRole('button', { name: 'Group settings of VAT' }))
    await userEvent.click(screen.getByRole('button', { name: 'Edit group' }))
    const rename = screen.getByRole('form', { name: 'Edit group' })
    await userEvent.clear(within(rename).getByRole('textbox', { name: 'Group name' }))
    await userEvent.type(within(rename).getByRole('textbox', { name: 'Group name' }), 'Sales tax')
    await userEvent.click(within(rename).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(renamed).toEqual({ group: { name: 'Sales tax' } }))

    await userEvent.click(screen.getByRole('button', { name: 'Group settings of VAT' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete group' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(deleted).toBe('802'))
  })

  it('is deleted from its menu and the lists show again', async () => {
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/task_lists/701', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/task_lists/lists/701')
    await userEvent.click(await screen.findByRole('button', { name: 'List options' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/task_lists\/lists$/))
  })
})
