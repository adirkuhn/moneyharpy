import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { TimeEntry } from './queries'
import { duration, workedMinutes } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

const entry = (over: Partial<TimeEntry> = {}): TimeEntry => ({
  id: '701', administration_id: '11', user_id: '1', description: 'Designing the website', started_at: '2026-11-10T09:00:00Z',
  ended_at: '2026-11-10T11:30:00Z', paused_duration: 1800, billable: true, contact_id: '5', project_id: '601', sales_invoice_id: null,
  contact: { id: '5', company_name: 'Example Shop B.V.', firstname: null, lastname: null }, project: { id: '601', name: 'Website redesign' },
  sales_invoice: null, user: { id: '1', name: 'Ann de Vries' }, notes: [], events: [], uri: '/11/time_entries/701',
  ...over,
})

const timer = entry({ id: '702', description: 'Support call', ended_at: null, paused_duration: 0, billable: false, contact: null, contact_id: null, project: null, project_id: null, uri: '/11/time_entries/702' })

function loaded(list: TimeEntry[] = [entry(), timer], seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/time_entries', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/time_entries/701', () => HttpResponse.json(entry())),
    http.get('*/api/v2/11/time_entries/702', () => HttpResponse.json(timer)),
  ]
}

const projects = [{ id: '601', administration_id: '11', name: 'Website redesign', state: 'active', budget: null }]

describe('the timesheet register', () => {
  it('shows the week with the hours of each day and the chosen day’s time', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/time_entries?date=2026-11-10')
    expect(await screen.findByRole('heading', { name: 'Timesheets November 10, 2026' })).toBeInTheDocument()
    const table = await screen.findByRole('table', { name: 'Timesheets' })
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('10:00 - 12:30')
    expect(rows[1]).toHaveTextContent('Designing the website')
    expect(rows[1]).toHaveTextContent('Example Shop B.V.')
    expect(rows[1]).toHaveTextContent('Website redesign')
    expect(rows[1]).toHaveTextContent('Open')
    expect(rows[1]).toHaveTextContent('00:30')
    expect(rows[1]).toHaveTextContent('02:00')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: '10:00 - 12:30' })).toHaveAttribute('href', '/11/time_entries/701')
    expect(rows[2]).toHaveTextContent('10:00 - Now')
    expect(within(rows[2] as HTMLElement).getByRole('button', { name: 'Stop timer' })).toBeInTheDocument()
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('period:20261109..20261115,include_active:true,user_id:1')

    const week = screen.getByRole('navigation', { name: 'Week' })
    const tuesday = within(week).getByRole('link', { name: 'Tuesday, November 10' })
    expect(tuesday).toHaveAttribute('aria-current', 'date')
    expect(within(week).getByRole('link', { name: 'Monday, November 9' })).toHaveTextContent('00:00')
    expect(within(week).getByRole('link', { name: 'Previous week' })).toHaveAttribute('href', '/11/time_entries?date=2026-11-03')
    expect(within(week).getByRole('link', { name: 'Next week' })).toHaveAttribute('href', '/11/time_entries?date=2026-11-17')

    await userEvent.click(within(week).getByRole('link', { name: 'Wednesday, November 11' }))
    expect(await within(screen.getByRole('table', { name: 'Timesheets' })).findByText(/No time logged on this day/)).toBeInTheDocument()
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Register' })).toHaveAttribute('aria-current', 'page')
  })

  it('logs time on the chosen day with its pause, contact and project', async () => {
    let sent: { time_entry?: Record<string, unknown> } = {}
    server.use(...loaded([]))
    server.use(
      http.get('*/api/v2/11/projects', () => HttpResponse.json(projects)),
      http.post('*/api/v2/11/time_entries', async ({ request }) => {
        sent = await request.json() as typeof sent
        return HttpResponse.json(entry(), { status: 201 })
      }),
    )
    renderAt('/11/time_entries?date=2026-11-10')
    const form = await screen.findByRole('form', { name: 'Log time' })
    expect(within(form).getByRole('button', { name: 'Start timer' })).toBeInTheDocument()
    await userEvent.clear(within(form).getByLabelText('Started at'))
    await userEvent.type(within(form).getByLabelText('Started at'), '09:00')
    await userEvent.type(within(form).getByLabelText('Ended at'), '11:30')
    await userEvent.type(within(form).getByLabelText('Pause duration'), '0:30')
    await userEvent.type(within(form).getByLabelText('Description'), 'Designing the website')
    await userEvent.click(within(form).getByRole('combobox', { name: 'Project' }))
    await userEvent.click(await screen.findByRole('option', { name: 'Website redesign' }))
    await userEvent.click(within(form).getByLabelText('Billable'))
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.time_entry).toBeDefined())
    expect(sent.time_entry).toMatchObject({
      user_id: '1', description: 'Designing the website', billable: false, project_id: '601', paused_duration: 1800,
      started_at: '2026-11-10T08:00:00.000Z', ended_at: '2026-11-10T10:30:00.000Z',
    })
    await waitFor(() => expect(within(form).getByLabelText('Description')).toHaveValue(''))
  })

  it('starts a timer without an end, and checks the times first', async () => {
    let sent: { time_entry?: Record<string, unknown> } | undefined
    server.use(...loaded([]))
    server.use(http.post('*/api/v2/11/time_entries', async ({ request }) => {
      sent = await request.json() as typeof sent
      return HttpResponse.json(timer, { status: 201 })
    }))
    renderAt('/11/time_entries?date=2026-11-10')
    const form = await screen.findByRole('form', { name: 'Log time' })
    await userEvent.type(within(form).getByLabelText('Ended at'), '9:99')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    expect(await within(form).findAllByText('Enter a time as hh:mm, such as 09:30.')).toHaveLength(1)
    expect(within(form).getByText('Describe what you worked on.')).toBeInTheDocument()
    expect(sent).toBeUndefined()

    await userEvent.clear(within(form).getByLabelText('Ended at'))
    await userEvent.type(within(form).getByLabelText('Description'), 'Support call')
    await userEvent.click(within(form).getByRole('button', { name: 'Start timer' }))
    await waitFor(() => expect(sent?.time_entry).toMatchObject({ description: 'Support call', billable: true }))
    expect(sent?.time_entry).not.toHaveProperty('ended_at')
  })

  it('counts the hours worked less the pauses', () => {
    expect(duration(workedMinutes(entry()))).toBe('2:00')
    expect(workedMinutes(timer, new Date('2026-11-10T09:05:30Z'))).toBe(5)
  })
})

describe('a time entry', () => {
  it('shows its details and stops a running timer', async () => {
    let stopped = false
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/time_entries/702/stop', () => {
      stopped = true
      return HttpResponse.json({ ...timer, ended_at: '2026-11-10T10:00:00Z' })
    }))
    renderAt('/11/time_entries/702')
    expect(await screen.findByRole('heading', { name: 'Support call' })).toBeInTheDocument()
    const details = screen.getByRole('region', { name: 'Time details' })
    expect(within(details).getByText('Running')).toBeInTheDocument()
    expect(within(details).getByText('Ann de Vries')).toBeInTheDocument()
    await chooseMore('Stop timer')
    await waitFor(() => expect(stopped).toBe(true))
    expect(await within(details).findByText('Not billable')).toBeInTheDocument()
  })

  it('is recorded for the signed-in user, as a timer without an end', async () => {
    let sent: { time_entry?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/time_entries', async ({ request }) => {
      sent = await request.json() as typeof sent
      return HttpResponse.json(timer, { status: 201 })
    }))
    renderAt('/11/time_entries/new')
    const form = await screen.findByRole('form', { name: 'Time entry' })
    await userEvent.type(within(form).getByLabelText('Description'), 'Support call')
    await userEvent.click(within(form).getByLabelText('Billable to the client'))
    await waitFor(() => expect(within(form).getByRole('button', { name: 'Save time' })).toBeEnabled())
    await userEvent.click(within(form).getByRole('button', { name: 'Save time' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/time_entries/702'))
    expect(sent.time_entry).toMatchObject({ user_id: '1', description: 'Support call', billable: false })
    expect(sent.time_entry?.started_at).toBeTypeOf('string')
    expect(sent.time_entry).not.toHaveProperty('ended_at')
  })
})
