import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { Template } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

const template = (over: Partial<Template> = {}): Template => ({
  id: '601', administration_id: '11', name: 'Month close', description: 'Every month', period: 'P1M', deadline_after: 'P10D', uri: '/11/task_list_templates/601',
  groups: [
    { id: '811', name: null, row_order: 0, report_type: null, tasks: [] },
    {
      id: '812', name: 'Bank', row_order: 1, report_type: null,
      tasks: [{
        id: '911', administration_id: '11', name: 'Reconcile', description: null, row_order: 0, completed_at: null, report_type: null,
        task_lists_group_id: '812', parent_task_id: null, assignee_id: null, subtasks: [], notes: [], events: [],
      }],
    },
  ],
  ...over,
})

function loaded(templates: Template[] = [template()]) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/task_list_templates', () => HttpResponse.json(templates)),
    http.get('*/api/v2/11/task_list_templates/601', () => HttpResponse.json(template())),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([])),
  ]
}

describe('the templates', () => {
  it('lists templates with their period and deadline', async () => {
    server.use(...loaded())
    renderAt('/11/task_lists/templates')
    const table = await screen.findByRole('table', { name: 'Templates' })
    const row = within(table).getAllByRole('row')[1] as HTMLElement
    expect(row).toHaveTextContent('Month close')
    expect(row).toHaveTextContent('Month')
    expect(row).toHaveTextContent('10 days after the period')
    expect(within(row).getByRole('link', { name: 'Month close' })).toHaveAttribute('href', '/11/task_lists/templates/601')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Templates' })).toHaveAttribute('aria-current', 'page')
  })

  it('introduces templates until the first one is added', async () => {
    server.use(...loaded([]))
    renderAt('/11/task_lists/templates')
    expect(await screen.findByRole('region', { name: 'Getting started with templates' })).toBeInTheDocument()
    expect(within(screen.getByRole('table', { name: 'Templates' })).getByRole('link', { name: 'Add template' })).toHaveAttribute('href', '/11/task_lists/templates/new')
  })
})

describe('a template', () => {
  it('is added with its period and a deadline some days after it', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/task_list_templates', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(template(), { status: 201 })
    }))
    renderAt('/11/task_lists/templates/new')
    const form = await screen.findByRole('form', { name: 'Template' })
    expect(within(form).getByRole('radio', { name: 'Month' })).toBeChecked()
    await userEvent.type(within(form).getByLabelText('Name'), 'Quarter close')
    await userEvent.click(within(form).getByRole('radio', { name: 'Quarter' }))
    await userEvent.click(within(form).getByRole('switch', { name: 'Deadline' }))
    await userEvent.clear(within(form).getByRole('spinbutton', { name: 'Days' }))
    await userEvent.type(within(form).getByRole('spinbutton', { name: 'Days' }), '30')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ task_list_template: { name: 'Quarter close', description: '', period: 'P3M', deadline_after: 'P30D' } }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/task_lists/templates/601'))
  })

  it('shows its period, deadline and tasks, and adds a task without an assignee', async () => {
    let added: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/task_list_groups/812/tasks', async ({ request }) => {
      added = await request.json()
      return HttpResponse.json({}, { status: 201 })
    }))
    renderAt('/11/task_lists/templates/601')
    expect(await screen.findByRole('heading', { name: 'Month close' })).toBeInTheDocument()
    expect(screen.getByRole('note')).toHaveTextContent(/will not be directly applied to existing task lists/)
    expect(screen.getByText('10 days after the period')).toBeInTheDocument()
    const bank = screen.getByRole('group', { name: 'Bank' })
    expect(within(bank).getByText('Reconcile')).toBeInTheDocument()
    expect(within(bank).queryByRole('checkbox')).toBeNull()
    await userEvent.click(within(bank).getByRole('button', { name: 'Add task' }))
    const form = within(bank).getByRole('form', { name: 'Add task' })
    expect(within(form).queryByRole('combobox', { name: 'Assignee' })).toBeNull()
    await userEvent.type(within(form).getByRole('textbox', { name: 'Task' }), 'Book the fees')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(added).toEqual({ task: { name: 'Book the fees', description: '' } }))
  })

  it('adds a group to the template', async () => {
    let added: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/task_list_templates/601/groups', async ({ request }) => {
      added = await request.json()
      return HttpResponse.json({ id: '813', name: 'VAT', row_order: 2, report_type: null, tasks: [] }, { status: 201 })
    }))
    renderAt('/11/task_lists/templates/601')
    await userEvent.click(await screen.findByRole('button', { name: 'Add group' }))
    const form = screen.getByRole('form', { name: 'Add group' })
    await userEvent.type(within(form).getByRole('textbox', { name: 'Group name' }), 'VAT')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(added).toEqual({ group: { name: 'VAT' } }))
  })

  it('edits its deadline away and is deleted', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(
      http.patch('*/api/v2/11/task_list_templates/601', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(template({ deadline_after: '' }))
      }),
      http.delete('*/api/v2/11/task_list_templates/601', () => new HttpResponse(null, { status: 204 })),
    )
    renderAt('/11/task_lists/templates/601/edit')
    const form = await screen.findByRole('form', { name: 'Template' })
    expect(within(form).getByRole('spinbutton', { name: 'Days' })).toHaveValue(10)
    await userEvent.click(within(form).getByRole('switch', { name: 'Deadline' }))
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ task_list_template: { name: 'Month close', description: 'Every month', period: 'P1M', deadline_after: '' } }))

    await userEvent.click(await screen.findByRole('button', { name: 'Template options' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/task_lists\/templates$/))
  })
})
