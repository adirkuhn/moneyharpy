import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { MoneybirdImport } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

const base = [
  signedIn(),
  listing([harpy]),
  http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
  http.get('*/api/v2/11/connection_settings', () => HttpResponse.json({ users_may_connect: false })),
  http.get('*/api/v2/11/connections', () => HttpResponse.json([])),
  http.get('*/api/v2/11/webhooks', () => HttpResponse.json([])),
  http.get('*/api/v2/11/webhooks/events', () => HttpResponse.json({ events: [], groups: [] })),
]

const running: MoneybirdImport = {
  source_administration_id: '123', source_name: 'Example Shop B.V.', state: 'running', error: '', waiting_until: '2026-11-20T09:05:00Z',
  started_at: '2026-11-20T09:00:00Z', finished_at: null, updated_at: '2026-11-20T09:01:00Z',
  steps: [
    { kind: 'chart', total: 40, imported: 40, skipped: 0, failed: 0, done: true, problems: [] },
    { kind: 'contacts', total: 300, imported: 120, skipped: 0, failed: 1, done: false, problems: ['contact 502: send_invoices_to_email is invalid'] },
    { kind: 'products', total: 0, imported: 0, skipped: 0, failed: 0, done: false, problems: [] },
    { kind: 'projects', total: 0, imported: 0, skipped: 0, failed: 0, done: false, problems: [] },
  ],
}

describe('import from Moneybird', () => {
  it('checks the token, picks the administration and starts the import', async () => {
    let started: unknown
    let current: MoneybirdImport | null = null
    server.use(
      ...base,
      http.get('*/api/v2/11/moneybird_import', () => (current ? HttpResponse.json(current) : HttpResponse.json({ error: 'No import' }, { status: 404 }))),
      http.post('*/api/v2/11/moneybird_import/administrations', () => HttpResponse.json([{ id: '123', name: 'Example Shop B.V.' }, { id: '456', name: 'Beispiel GmbH' }])),
      http.post('*/api/v2/11/moneybird_import', async ({ request }) => {
        started = await request.json()
        current = running
        return HttpResponse.json(running, { status: 201 })
      }),
    )
    renderAt('/11/settings/connections')
    const block = await screen.findByRole('region', { name: 'Import from Moneybird' })
    await userEvent.type(await within(block).findByLabelText('Moneybird API token'), 'secret-token')
    await userEvent.click(within(block).getByRole('button', { name: 'Check token' }))
    await userEvent.click(await within(block).findByRole('radio', { name: 'Beispiel GmbH' }))
    await userEvent.click(within(block).getByRole('button', { name: 'Start import' }))
    await waitFor(() => expect(started).toEqual({ moneybird_import: { token: 'secret-token', administration_id: '456' } }))

    const progress = await within(block).findByRole('region', { name: 'Import from Moneybird' })
    expect(progress).toHaveTextContent('From Example Shop B.V.')
    expect(within(progress).getByRole('status')).toHaveTextContent(/Waiting for Moneybird's rate limit until/)
    const steps = within(progress).getByRole('table', { name: 'Steps of the import' })
    expect(within(steps).getByRole('row', { name: /Contacts/ })).toHaveTextContent('120 of 300')
    expect(within(progress).getByText('contact 502: send_invoices_to_email is invalid')).toBeInTheDocument()
  })

  it('is only there for owners', async () => {
    const clerk = administration({ id: '11', name: 'Harpy B.V.', user_type: 'employee', permissions: ['settings'] })
    server.use(...base)
    server.use(listing([clerk]), http.get('*/api/v2/administrations/11', () => HttpResponse.json(clerk)))
    renderAt('/11/settings/connections')
    expect(await screen.findByRole('region', { name: 'Webhooks' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Import from Moneybird' })).toBeNull()
  })
})
