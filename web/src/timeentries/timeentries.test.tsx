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

describe('the timesheet', () => {
  it('lists this year’s time with its hours and state, and filters on the state', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/time_entries')
    const table = await screen.findByRole('table', { name: 'Timesheets' })
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('2026-11-10')
    expect(rows[1]).toHaveTextContent('Example Shop B.V.')
    expect(rows[1]).toHaveTextContent('Website redesign')
    expect(rows[1]).toHaveTextContent('2:00')
    expect(rows[1]).toHaveTextContent('Open')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Designing the website' })).toHaveAttribute('href', '/11/time_entries/701')
    expect(rows[2]).toHaveTextContent('Running')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('period:this_year,include_active:true,state:all')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Register' })).toHaveAttribute('aria-current', 'page')

    await userEvent.click(screen.getByRole('button', { name: 'All time this year' }))
    await userEvent.click(within(screen.getByRole('search', { name: 'Time filters' })).getByRole('link', { name: 'Not billable' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('period:this_year,include_active:true,state:non_billable'))
  })

  it('says when there is none yet', async () => {
    server.use(...loaded([]))
    renderAt('/11/time_entries')
    expect(await screen.findByText(/No time recorded this year yet/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add time' })).toHaveAttribute('href', '/11/time_entries/new')
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
