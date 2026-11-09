import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const dev = administration({ id: '22', name: 'Developer company', favorite: true })

describe('selecting an administration', () => {
  it('links every row to the administration URI and its settings', async () => {
    server.use(signedIn(), listing([dev, harpy]))
    renderAt('/administrations')
    expect(await screen.findByRole('heading', { name: 'Select an administration' })).toBeInTheDocument()

    const list = await screen.findByRole('list', { name: 'Administrations' })
    expect(within(list).getByRole('link', { name: 'Harpy B.V.' })).toHaveAttribute('href', '/11')
    expect(within(list).getByRole('link', { name: 'Settings for Harpy B.V.' })).toHaveAttribute('href', '/11/settings')
    expect(screen.getByRole('link', { name: 'Add administration' })).toHaveAttribute('href', '/administrations/new')
  })

  it('invites the user to add the first one', async () => {
    server.use(signedIn(), listing([]))
    renderAt('/administrations')
    expect(await screen.findByText('Add your first administration to start bookkeeping.')).toBeInTheDocument()
  })

  it('tells the user when the list cannot load', async () => {
    server.use(signedIn(), http.get('*/api/v2/administrations', () => HttpResponse.json({ error: 'x' }, { status: 500 })))
    renderAt('/administrations')
    expect(await screen.findByRole('alert')).toHaveTextContent('Administrations could not be loaded. Reload the page to try again.')
  })

  it('marks and unmarks favorites', async () => {
    const calls: string[] = []
    server.use(
      signedIn(),
      listing([dev, harpy]),
      http.put('*/api/v2/administrations/:id/favorite', ({ params }) => {
        calls.push(`PUT ${params.id}`)
        return new HttpResponse(null, { status: 204 })
      }),
      http.delete('*/api/v2/administrations/:id/favorite', ({ params }) => {
        calls.push(`DELETE ${params.id}`)
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/administrations')

    const mark = await screen.findByRole('button', { name: 'Mark Harpy B.V. as favorite' })
    expect(mark).toHaveAttribute('aria-pressed', 'false')
    await userEvent.click(mark)
    await userEvent.click(screen.getByRole('button', { name: 'Remove Developer company from favorites' }))
    await waitFor(() => expect(calls).toEqual(['PUT 11', 'DELETE 22']))
  })
})

describe('adding an administration', () => {
  const type = async (name: string) => userEvent.type(await screen.findByLabelText('Company name'), name)

  it('opens the new administration', async () => {
    server.use(
      signedIn(),
      listing([]),
      http.post('*/api/v2/administrations', async ({ request }) => {
        expect(await request.json()).toEqual({
          administration: { name: 'New Co', language: 'en', currency: 'EUR', country: 'NL', time_zone: 'Europe/Amsterdam' },
        })
        return HttpResponse.json(administration({ id: '33', name: 'New Co' }), { status: 201 })
      }),
      http.get('*/api/v2/administrations/33', () => HttpResponse.json(administration({ id: '33', name: 'New Co' }))),
    )
    renderAt('/administrations/new')
    await type('New Co')
    await userEvent.click(screen.getByRole('button', { name: 'Add administration' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/33'))
  })

  it('shows which field is wrong', async () => {
    server.use(
      signedIn(),
      listing([]),
      http.post('*/api/v2/administrations', () =>
        HttpResponse.json({ error: { name: ["can't be blank"] } }, { status: 422 }),
      ),
    )
    renderAt('/administrations/new')
    await type('x')
    await userEvent.click(screen.getByRole('button', { name: 'Add administration' }))
    expect(await screen.findByText("Company name can't be blank")).toBeInTheDocument()
  })
})

describe('opening resources by URI', () => {
  it('opens an administration directly', async () => {
    server.use(signedIn(), listing([harpy]), http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)))
    renderAt('/11')
    expect(await screen.findByRole('heading', { name: 'Harpy B.V.' })).toBeInTheDocument()
  })

  it('reports an administration the user cannot open', async () => {
    server.use(
      signedIn(),
      listing([]),
      http.get('*/api/v2/administrations/99', () => HttpResponse.json({ error: 'Not found' }, { status: 404 })),
    )
    renderAt('/99')
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  })

  it('follows permalinks to the canonical URI', async () => {
    server.use(
      signedIn(),
      listing([harpy]),
      http.get('*/api/resources/11', () => HttpResponse.json({ id: '11', type: 'administrations', uri: '/11' })),
      http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    )
    renderAt('/r/11')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11$/))
  })

  it('reports a permalink that leads nowhere', async () => {
    server.use(
      signedIn(),
      listing([]),
      http.get('*/api/resources/77', () => HttpResponse.json({ error: 'Not found' }, { status: 404 })),
    )
    renderAt('/r/77')
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  })
})
