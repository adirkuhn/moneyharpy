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

const old = project({ id: '602', name: 'Old shop', state: 'archived', budget: null, uri: '/11/projects/602' })

function loaded(list: Project[] = [project(), old], seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/projects', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/projects/601', () => HttpResponse.json(project())),
  ]
}

describe('the project list', () => {
  it('lists projects with their budget and state, and filters on the state', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/projects')
    const table = await screen.findByRole('table', { name: 'Projects' })
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('Website redesign')
    expect(rows[1]).toHaveTextContent('40 hours')
    expect(rows[1]).toHaveTextContent('Active')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Website redesign' })).toHaveAttribute('href', '/11/projects/601')
    expect(rows[2]).toHaveTextContent('Archived')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:active')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Projects' })).toHaveAttribute('aria-current', 'page')

    await userEvent.click(screen.getByRole('button', { name: 'Active projects' }))
    await userEvent.click(within(screen.getByRole('search', { name: 'Project filters' })).getByRole('link', { name: 'Archived projects' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:archived'))
    await userEvent.click(screen.getByRole('button', { name: 'Archived projects' }))
    await userEvent.click(within(screen.getByRole('search', { name: 'Project filters' })).getByRole('link', { name: 'All projects' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:all'))
  })

  it('says when there are none yet', async () => {
    server.use(...loaded([]))
    renderAt('/11/projects')
    expect(await screen.findByText(/No projects yet/)).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Keep track of your projects' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add project' })).toHaveAttribute('href', '/11/projects/new')
  })
})

describe('a project', () => {
  it('is added with a name and a budget', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/projects', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(project(), { status: 201 })
    }))
    renderAt('/11/projects/new')
    const form = await screen.findByRole('form', { name: 'Project' })
    expect(screen.getByRole('heading', { name: 'New project' })).toBeInTheDocument()
    await userEvent.type(within(form).getByLabelText('Name'), 'Website redesign')
    await userEvent.type(within(form).getByLabelText('Budget'), '40')
    await userEvent.click(within(form).getByRole('button', { name: 'Save project' }))
    await waitFor(() => expect(sent).toEqual({ project: { name: 'Website redesign', budget: 40 } }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/projects/601'))
  })

  it('shows the problems the server finds beside their fields', async () => {
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/projects', () => HttpResponse.json({ error: { name: ['has already been taken'] } }, { status: 422 })))
    renderAt('/11/projects/new')
    const form = await screen.findByRole('form', { name: 'Project' })
    await userEvent.type(within(form).getByLabelText('Name'), 'Website redesign')
    await userEvent.click(within(form).getByRole('button', { name: 'Save project' }))
    expect(await within(form).findByText(/^Name has already been taken/)).toBeInTheDocument()
  })

  it('changes its name and removes its budget', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/projects/601', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(project({ name: 'Website relaunch', budget: null }))
    }))
    renderAt('/11/projects/601')
    expect(await screen.findByRole('heading', { name: 'Website redesign' })).toBeInTheDocument()
    expect(screen.getByText('Budget', { selector: 'dt' }).nextSibling).toHaveTextContent('40 hours')
    const form = screen.getByRole('form', { name: 'Project' })
    await userEvent.clear(within(form).getByLabelText('Name'))
    await userEvent.type(within(form).getByLabelText('Name'), 'Website relaunch')
    await userEvent.clear(within(form).getByLabelText('Budget'))
    await userEvent.click(within(form).getByRole('button', { name: 'Save project' }))
    await waitFor(() => expect(sent).toEqual({ project: { name: 'Website relaunch', budget: null } }))
    expect(await screen.findByRole('heading', { name: 'Website relaunch' })).toBeInTheDocument()
  })

  it('is archived and reactivated from the menu', async () => {
    const sent: unknown[] = []
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/projects/601', async ({ request }) => {
      const body = (await request.json()) as { project: { state: 'active' | 'archived' } }
      sent.push(body)
      return HttpResponse.json(project({ state: body.project.state }))
    }))
    renderAt('/11/projects/601')
    await chooseMore('Archive')
    expect(await screen.findByText('This project is archived and is not offered for new bookings.')).toBeInTheDocument()
    await chooseMore('Reactivate')
    await waitFor(() => expect(screen.queryByText('This project is archived and is not offered for new bookings.')).toBeNull())
    expect(sent).toEqual([{ project: { state: 'archived' } }, { project: { state: 'active' } }])
  })

  it('is deleted and the list shows again', async () => {
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/projects/601', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/projects/601')
    await chooseMore('Delete')
    await userEvent.click(screen.getByRole('button', { name: 'Delete project' }))
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
    renderAt('/11/projects/601')
    await chooseMore('Delete')
    await userEvent.click(screen.getByRole('button', { name: 'Delete project' }))
    expect(await screen.findByText('Something is booked on this project, so it was archived instead of deleted.')).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent('/11/projects/601')
    await waitFor(() => expect(within(screen.getByRole('region', { name: 'Project details' })).getByText('Archived')).toBeInTheDocument())
  })
})
