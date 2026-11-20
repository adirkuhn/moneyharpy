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
  source_administration_id: '123', source_name: 'Example Shop B.V.', state: 'running', error: '', waiting_until: '2026-11-20T09:05:00Z', next_attempt_at: null,
  started_at: '2026-11-20T09:00:00Z', finished_at: null, updated_at: '2026-11-20T09:01:00Z',
  steps: [
    { kind: 'chart', total: 40, imported: 40, skipped: 0, failed: 0, done: true, problems: [] },
    { kind: 'contacts', total: 300, imported: 120, skipped: 0, failed: 1, done: false, problems: ['contact 502: send_invoices_to_email is invalid'] },
    { kind: 'products', total: 0, imported: 0, skipped: 0, failed: 0, done: false, problems: [] },
    { kind: 'projects', total: 0, imported: 0, skipped: 0, failed: 0, done: false, problems: [] },
    { kind: 'custom_fields', total: 2, imported: 2, skipped: 0, failed: 0, done: true, problems: [] },
    { kind: 'identities', total: 1, imported: 1, skipped: 0, failed: 0, done: true, problems: [] },
    { kind: 'workflows', total: 3, imported: 3, skipped: 0, failed: 0, done: true, problems: [] },
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
      http.get('*/api/v2/11/moneybird_import/problems', () => HttpResponse.json(
        [{ step: 'contacts', kind: 'contact', source_id: '504000000000000502', message: 'send_invoices_to_email: is not a valid email address', partial: false, created_at: '2026-11-20T09:01:00Z' }],
        { headers: { 'X-Total-Count': '1' } },
      )),
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
    const start = within(block).getByRole('button', { name: 'Start import' })
    await userEvent.click(start)
    // Once triggered it cannot be triggered again.
    await waitFor(() => expect(started).toEqual({ moneybird_import: { token: 'secret-token', administration_id: '456' } }))

    const progress = await within(block).findByRole('region', { name: 'Import from Moneybird' })
    expect(progress).toHaveTextContent('From Example Shop B.V.')
    expect(within(progress).getByRole('status')).toHaveTextContent(/Waiting for Moneybird's rate limit until/)
    const steps = within(progress).getByRole('table', { name: 'Steps of the import' })
    expect(within(steps).getByRole('row', { name: /Contacts/ })).toHaveTextContent('120 of 300')
    // The progress over all steps, and the step at work.
    const overall = within(progress).getByRole('progressbar', { name: 'Import progress' })
    expect(overall).toHaveAttribute('aria-valuenow', String(Math.floor(((1 + 120 / 300 + 3) / 7) * 100)))
    expect(progress).toHaveTextContent('Step 2 of 7: Contacts')
    expect(within(steps).getByRole('progressbar', { name: 'Contacts' })).toHaveAttribute('aria-valuenow', '40')
    expect(screen.queryByRole('button', { name: 'Start import' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Import again' })).toBeNull()
    // Every record that could not come in, with why.
    await userEvent.click(within(progress).getByText('1 record could not be imported'))
    const problems = await within(progress).findByRole('table', { name: 'Records that could not be imported' })
    expect(within(problems).getByRole('row', { name: /504000000000000502/ })).toHaveTextContent('send_invoices_to_email: is not a valid email address')
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

describe('a queued import', () => {
  it('says when it tries again, and keeps the start buttons away', async () => {
    const queued: MoneybirdImport = { ...running, waiting_until: null, error: 'contacts: moneybird is down', next_attempt_at: '2026-11-20T10:30:00Z' }
    server.use(...base, http.get('*/api/v2/11/moneybird_import', () => HttpResponse.json(queued)))
    renderAt('/11/settings/connections')
    const block = await screen.findByRole('region', { name: 'Import from Moneybird' })
    expect(await within(block).findByText(/stays queued and tries again at/)).toBeInTheDocument()
    expect(within(block).queryByRole('button', { name: 'Import again' })).toBeNull()
    expect(within(block).queryByLabelText('Moneybird API token')).toBeNull()
  })

  it('tries again now on request', async () => {
    let tried = false
    const queued: MoneybirdImport = { ...running, waiting_until: null, error: 'chart: the bank is down', next_attempt_at: '2026-11-20T10:30:00Z' }
    server.use(
      ...base,
      http.get('*/api/v2/11/moneybird_import', () => HttpResponse.json(tried ? { ...queued, error: '', next_attempt_at: null } : queued)),
      http.post('*/api/v2/11/moneybird_import/try_now', () => {
        tried = true
        return HttpResponse.json({ ...queued, error: '', next_attempt_at: null })
      }),
    )
    renderAt('/11/settings/connections')
    const block = await screen.findByRole('region', { name: 'Import from Moneybird' })
    await userEvent.click(await within(block).findByRole('button', { name: 'Try now' }))
    await waitFor(() => expect(tried).toBe(true))
    await waitFor(() => expect(within(block).queryByRole('button', { name: 'Try now' })).toBeNull())
  })
})
