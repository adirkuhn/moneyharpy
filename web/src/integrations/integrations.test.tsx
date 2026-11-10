import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { Connection } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'
const conn = (over: Partial<Connection> = {}): Connection => ({
  id: '31', kind: 'api', name: 'Shop', scopes: ['bank'], read_only: true, granted_by: 'Ann de Vries', created_at: stamp, last_used_at: null, mine: true, ...over,
})

const loaded = (a = harpy, may = false) => [
  signedIn(),
  listing([a]),
  http.get('*/api/v2/administrations/11', () => HttpResponse.json(a)),
  http.get('*/api/v2/11/connection_settings', () => HttpResponse.json({ users_may_connect: may })),
  http.get('*/api/v2/11/connections', () => HttpResponse.json([conn(), conn({ id: '32', kind: 'ai', name: 'Claude', read_only: false, scopes: ['settings', 'bank'] })])),
  http.get('*/api/v2/11/webhooks', () =>
    HttpResponse.json([{ id: '41', administration_id: '11', url: 'https://example.com/hook', enabled_events: ['tax_rate'], last_http_status: 200, last_http_body: 'ok', token: 'tok', deactivated_at: null, uri: '/11/webhooks/41' }]),
  ),
  http.get('*/api/v2/11/webhooks/events', () => HttpResponse.json({ events: ['tax_rate_created', 'tax_rate_updated', 'booking_rule_created'], groups: ['tax_rate', 'booking_rule'] })),
]

describe('integrations settings', () => {
  it('shows every part with connections and webhooks', async () => {
    server.use(...loaded())
    renderAt('/11/settings/connections')
    expect(await screen.findByRole('heading', { name: 'Integrations', level: 1 })).toBeInTheDocument()
    for (const name of ['Available connections', 'Who can connect', 'External applications', 'AI connections', 'Webhooks']) {
      expect(screen.getByRole('region', { name })).toBeInTheDocument()
    }
    const apps = await screen.findByRole('table', { name: 'External applications' })
    expect(within(apps).getByText('Read only · Bank')).toBeInTheDocument()
    expect(within(await screen.findByRole('table', { name: 'AI connections' })).getByText('Claude')).toBeInTheDocument()
    expect(await screen.findByRole('link', { name: 'https://example.com/hook' })).toHaveAttribute('href', '/11/webhooks/41')
    expect(screen.getAllByText('Coming later')).toHaveLength(4)
    expect(screen.getByRole('link', { name: 'New AI connection' })).toHaveAttribute('href', '/11/connections/new?kind=ai')
  })

  it('lets owners open connecting to users', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/connection_settings', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ users_may_connect: true })
    }))
    renderAt('/11/settings/connections')
    await userEvent.click(await screen.findByRole('switch', { name: 'Users may create connections' }))
    await waitFor(() => expect(sent).toEqual({ users_may_connect: true }))
  })

  it('hides creating from users while connecting is closed', async () => {
    const limited = administration({ id: '11', name: 'Harpy B.V.', user_type: 'employee', permissions: ['bank'] })
    server.use(...loaded(limited))
    renderAt('/11/settings/connections')
    expect(await screen.findByRole('heading', { name: 'Integrations', level: 1 })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'New API token' })).not.toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Webhooks' })).not.toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Users may create connections' })).toBeDisabled()
  })

  it('revokes after confirming', async () => {
    let revoked = false
    server.use(...loaded(), http.delete('*/api/v2/11/connections/31', () => {
      revoked = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/settings/connections')
    const apps = await screen.findByRole('table', { name: 'External applications' })
    await userEvent.click(within(apps).getByRole('button', { name: 'Revoke Shop' }))
    await userEvent.click(within(apps).getByRole('button', { name: 'Confirm revoking Shop' }))
    await waitFor(() => expect(revoked).toBe(true))
  })
})

describe('connection form', () => {
  it('creates an AI connection and shows the token with the MCP setup', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/11/connections', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ ...conn({ kind: 'ai', name: 'Claude' }), token: 'mhc_secret', mcp_url: 'http://localhost/api/mcp' }, { status: 201 })
    }))
    renderAt('/11/connections/new?kind=ai')
    await userEvent.type(await screen.findByLabelText('Name'), 'Claude')
    await userEvent.selectOptions(screen.getByLabelText('Access'), 'read')
    await userEvent.click(screen.getByRole('checkbox', { name: 'Bank' }))
    await userEvent.click(screen.getByRole('button', { name: 'Create' }))
    await waitFor(() => expect(sent).toEqual({ connection: { kind: 'ai', name: 'Claude', scopes: ['bank'], read_only: true } }))
    expect(await screen.findByRole('status')).toHaveTextContent('mhc_secret')
    expect(screen.getByText(/"url": "http:\/\/localhost\/api\/mcp"/)).toBeInTheDocument()
  })

  it('offers only the scopes the user holds', async () => {
    server.use(...loaded(administration({ id: '11', name: 'Harpy B.V.', user_type: 'employee', permissions: ['bank'] }), true))
    renderAt('/11/connections/new?kind=api')
    expect(await screen.findByRole('checkbox', { name: 'Bank' })).toBeInTheDocument()
    expect(screen.queryByRole('checkbox', { name: 'Settings' })).not.toBeInTheDocument()
  })
})

describe('webhooks', () => {
  it('subscribes to a group and shows the secret once', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/11/webhooks', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ id: '42', administration_id: '11', url: 'https://example.com/new', enabled_events: ['tax_rate'], last_http_status: null, last_http_body: null, token: 't', secret: 'whsec', deactivated_at: null, uri: '/11/webhooks/42' }, { status: 201 })
    }))
    renderAt('/11/webhooks/new')
    await userEvent.type(await screen.findByLabelText('URL'), 'https://example.com/new')
    await userEvent.click(screen.getByRole('checkbox', { name: 'booking_rule_created' }))
    await userEvent.click(screen.getByRole('checkbox', { name: 'Everything about VAT rates' }))
    expect(screen.queryByRole('checkbox', { name: 'tax_rate_created' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Create webhook' }))
    await waitFor(() => expect(sent).toEqual({ url: 'https://example.com/new', enabled_events: ['booking_rule_created', 'tax_rate'] }))
    expect(await screen.findByRole('status')).toHaveTextContent('whsec')
  })

  it('deactivates a webhook', async () => {
    let off = false
    server.use(...loaded(), http.patch('*/api/v2/11/webhooks/41/deactivate', () => {
      off = true
      return HttpResponse.json({ id: '41', administration_id: '11', url: 'https://example.com/hook', enabled_events: [], last_http_status: 200, last_http_body: 'ok', token: 'tok', deactivated_at: stamp, uri: '/11/webhooks/41' })
    }))
    renderAt('/11/webhooks/41')
    expect(await screen.findByText('tax_rate')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Deactivate' }))
    await waitFor(() => expect(off).toBe(true))
  })
})
