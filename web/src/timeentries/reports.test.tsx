import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import { projectTotals } from './ProjectsReportPage'
import type { TimeEntry } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

const entry = (over: Partial<TimeEntry> = {}): TimeEntry => ({
  id: '701', administration_id: '11', user_id: '1', description: 'Designing the website', started_at: '2026-11-17T09:00:00Z',
  ended_at: '2026-11-17T11:30:00Z', paused_duration: 1800, billable: true, contact_id: '5', project_id: '601', sales_invoice_id: null,
  contact: { id: '5', company_name: 'Example Shop B.V.', firstname: null, lastname: null }, project: { id: '601', name: 'Website redesign' },
  sales_invoice: null, user: { id: '1', name: 'Ann de Vries' }, notes: [], events: [], uri: '/11/time_entries/701',
  ...over,
})

const entries = [
  entry(),
  entry({ id: '702', description: 'Support call', started_at: '2026-11-16T13:00:00Z', ended_at: '2026-11-16T14:00:00Z', paused_duration: 0, billable: false }),
  entry({ id: '703', description: 'Planning', started_at: '2026-11-10T08:00:00Z', ended_at: '2026-11-10T09:15:00Z', paused_duration: 0, project_id: null, project: null }),
]

function loaded(seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/time_entries', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(entries, { headers: { 'X-Total-Count': String(entries.length) } })
    }),
  ]
}

describe('the hours report', () => {
  it('shows this year’s time week by week with the totals', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/time_entries_report')
    const table = await screen.findByRole('table', { name: 'Timesheets' })
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('Week 47')
    expect(rows[1]).toHaveTextContent('03:00')
    expect(rows[2]).toHaveTextContent('2026-11-17')
    expect(rows[2]).toHaveTextContent('10:00 - 12:30')
    expect(rows[2]).toHaveTextContent('Ann de Vries')
    expect(rows[2]).toHaveTextContent('02:00')
    expect(within(rows[2] as HTMLElement).getByRole('link', { name: '2026-11-17' })).toHaveAttribute('href', '/11/time_entries/701')
    expect(rows[3]).toHaveTextContent('Not billable')
    expect(rows[4]).toHaveTextContent('Week 46')
    expect(rows.at(-1)).toHaveTextContent('04:15')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('period:this_year,state:all,include_active:false')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Hours report' })).toHaveAttribute('aria-current', 'page')

    await userEvent.click(within(rows[1] as HTMLElement).getByRole('button', { name: 'Week 47' }))
    expect(within(table).queryByText('Designing the website')).not.toBeInTheDocument()
  })

  it('filters on open time and searches the descriptions', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/time_entries_report')
    await screen.findByRole('table', { name: 'Timesheets' })
    await userEvent.click(screen.getByRole('button', { name: 'Time entries from this year' }))
    await userEvent.click(within(screen.getByRole('search', { name: 'Hours filters' })).getByRole('link', { name: 'Open time entries' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:open,include_active:false'))
    await userEvent.type(screen.getByRole('searchbox', { name: 'Search for description..' }), 'call')
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('query')).toBe('call'))
  })
})

describe('the projects report', () => {
  it('shows each project’s hours against its budget', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    server.use(http.get('*/api/v2/11/projects', () => HttpResponse.json([
      { id: '601', administration_id: '11', name: 'Website redesign', state: 'active', budget: 10 },
      { id: '602', administration_id: '11', name: 'Shop launch', state: 'active', budget: null },
    ])))
    renderAt('/11/time_entries_report/projects')
    const table = await screen.findByRole('table', { name: 'Projects' })
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('Website redesign')
    expect(rows[1]).toHaveTextContent('10:00')
    expect(rows[1]).toHaveTextContent('03:00')
    expect(rows[1]).toHaveTextContent('30% spent')
    expect(rows[2]).toHaveTextContent('Shop launch')
    expect(rows[2]).not.toHaveTextContent('spent')
    expect(seen.at(-1)?.searchParams.get('filter')).toMatch(/^period:\d{6}\.\.\d{6},state:all,include_active:true$/)
    expect(screen.queryByText('See all worked hours by project')).not.toBeInTheDocument()
  })

  it('introduces itself while there is no project', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/projects', () => HttpResponse.json([])))
    renderAt('/11/time_entries_report/projects')
    expect(await screen.findByText('See all worked hours by project')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('adds up the billable hours per project', () => {
    expect(projectTotals(entries).get('601')).toEqual({ worked: 180, billable: 120 })
  })
})
