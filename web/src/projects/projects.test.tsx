import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { Project } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

const project = (over: Partial<Project> = {}): Project => ({
  id: '601', administration_id: '11', name: 'Website redesign', state: 'active', budget: 40, uri: '/11/projects/601',
  ...over,
})

function loaded(list: Project[] = [project()], seen: URL[] = [], counts = { all: 2, archived: 1 }) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/projects', ({ request }) => {
      const url = new URL(request.url)
      if (url.searchParams.get('per_page') === '1') {
        const state = url.searchParams.get('filter') === 'state:all' ? 'all' : 'archived'
        return HttpResponse.json([], { headers: { 'X-Total-Count': String(counts[state]) } })
      }
      seen.push(url)
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/projects/601', () => HttpResponse.json(project())),
  ]
}

describe('the project list', () => {
  it('lists the active projects by name, like Moneybird, and shows the archived ones on request', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/projects')
    const table = await screen.findByRole('table', { name: 'Projects' })
    const rows = within(table).getAllByRole('row')
    expect(rows[0]).toHaveTextContent('Project name')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Website redesign' })).toHaveAttribute('href', '/11/projects/601/edit')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:active')
    expect(screen.queryByRole('region', { name: 'Add projects' })).toBeNull()
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Projects' })).toHaveAttribute('aria-current', 'page')

    expect(await within(table).findByText('There is 1 deactivated item.')).toBeInTheDocument()
    await userEvent.click(within(table).getByRole('link', { name: 'Show deactivated items.' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:all'))
    expect(await within(screen.getByRole('table', { name: 'Projects' })).findByRole('link', { name: 'Hide deactivated items.' })).toHaveAttribute('href', '/11/projects')
  })

  it('introduces projects until the first one is added', async () => {
    server.use(...loaded([], [], { all: 0, archived: 0 }))
    renderAt('/11/projects')
    expect(await screen.findByRole('region', { name: 'Add projects' })).toBeInTheDocument()
    const table = await screen.findByRole('table', { name: 'Projects' })
    expect(await within(table).findByRole('link', { name: 'Add project' })).toHaveAttribute('href', '/11/projects/new')
    expect(screen.getByRole('link', { name: 'Add' })).toHaveAttribute('href', '/11/projects/new')
  })
})

describe('a project', () => {
  it('is added with a name and a budget, and the list shows again', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/projects', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(project(), { status: 201 })
    }))
    renderAt('/11/projects/new')
    const form = await screen.findByRole('form', { name: 'Project' })
    expect(screen.getByRole('heading', { name: 'Add project' })).toBeInTheDocument()
    expect(within(form).getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/11/projects')
    await userEvent.type(within(form).getByLabelText('Project name'), 'Website redesign')
    await userEvent.type(within(form).getByLabelText('Budget (hours)'), '40')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ project: { name: 'Website redesign', budget: 40 } }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/projects$/))
  })

  it('shows the problems the server finds beside their fields', async () => {
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/projects', () => HttpResponse.json({ error: { name: ['has already been taken'] } }, { status: 422 })))
    renderAt('/11/projects/new')
    const form = await screen.findByRole('form', { name: 'Project' })
    await userEvent.type(within(form).getByLabelText('Project name'), 'Website redesign')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    expect(await within(form).findByText(/^Project name has already been taken/)).toBeInTheDocument()
  })

  it('opens its form from its own address and changes its name and removes its budget', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/projects/601', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(project({ name: 'Website relaunch', budget: null }))
    }))
    renderAt('/11/projects/601')
    expect(await screen.findByRole('heading', { name: 'Edit project' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent('/11/projects/601/edit')
    const form = screen.getByRole('form', { name: 'Project' })
    expect(within(form).getByLabelText('Budget (hours)')).toHaveValue('40')
    await userEvent.clear(within(form).getByLabelText('Project name'))
    await userEvent.type(within(form).getByLabelText('Project name'), 'Website relaunch')
    await userEvent.clear(within(form).getByLabelText('Budget (hours)'))
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ project: { name: 'Website relaunch', budget: null } }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/projects$/))
  })

  it('is archived and reactivated from the menu', async () => {
    const sent: unknown[] = []
    let current = project()
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/projects/601', () => HttpResponse.json(current)),
      http.patch('*/api/v2/11/projects/601', async ({ request }) => {
        const body = (await request.json()) as { project: { state: 'active' | 'archived' } }
        sent.push(body)
        current = project({ state: body.project.state })
        return HttpResponse.json(current)
      }),
    )
    renderAt('/11/projects/601/edit')
    await chooseMore('Archive')
    expect(await screen.findByText('This project is archived and is not offered for new bookings.')).toBeInTheDocument()
    await chooseMore('Reactivate')
    await waitFor(() => expect(screen.queryByText('This project is archived and is not offered for new bookings.')).toBeNull())
    expect(sent).toEqual([{ project: { state: 'archived' } }, { project: { state: 'active' } }])
  })

  it('is deleted after confirming and the list shows again', async () => {
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/projects/601', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/projects/601/edit')
    await chooseMore('Delete')
    const dialog = screen.getByRole('dialog', { name: 'Are you sure you want to delete this project?' })
    await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/projects$/))
  })

  it('is archived instead when something is booked on it', async () => {
    let current = project()
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/projects/601', () => HttpResponse.json(current)),
      http.delete('*/api/v2/11/projects/601', () => {
        current = project({ state: 'archived' })
        return new HttpResponse(null, { status: 204, headers: { 'X-Kept-As': 'archived' } })
      }),
    )
    renderAt('/11/projects/601/edit')
    await chooseMore('Delete')
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Confirm' }))
    expect(await screen.findByText('Something is booked on this project, so it was archived instead of deleted.')).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent('/11/projects/601/edit')
  })
})
