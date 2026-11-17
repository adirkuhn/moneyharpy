import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import { type BankConnection, bankRedirect, type FinancialAccount } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'
const account: FinancialAccount = {
  id: '501', administration_id: '11', type: 'BankAccount', name: 'Business', identifier: 'NL91ABNA0417164300', currency: 'EUR', provider: 'enable_banking',
  moneybird_account: false, active: true, created_at: stamp, updated_at: stamp, uri: '/11/financial_accounts/501', position: 1, ledger_account_id: '1101', last_import_at: stamp,
}
const connection = (over: Partial<BankConnection> = {}): BankConnection => ({
  id: '901', administration_id: '11', provider: 'enable_banking', bank: 'Example Bank', country: 'NL', state: 'active', valid_until: '2027-05-10T09:00:00Z',
  needs_reconnect: false, last_sync_at: stamp, last_error: '', financial_account_ids: ['501'], created_at: stamp, updated_at: stamp, ...over,
})

const loaded = (connections: BankConnection[]) => [
  signedIn(),
  listing([harpy]),
  http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
  http.get('*/api/v2/11/bank_settings', () => HttpResponse.json({ automatic_booking: true })),
  http.get('*/api/v2/11/financial_accounts', () => HttpResponse.json([account])),
  http.get('*/api/v2/11/booking_rules', () => HttpResponse.json([])),
  http.get('*/api/v2/11/overview', () => HttpResponse.json({})),
  http.get('*/api/v2/11/bank_connections', () => HttpResponse.json(connections)),
]

afterEach(() => vi.restoreAllMocks())

describe('connecting a bank', () => {
  it('picks a bank from the list and goes there to authorise', async () => {
    const go = vi.spyOn(bankRedirect, 'go').mockImplementation(() => {})
    let asked: unknown
    server.use(
      ...loaded([]),
      http.get('*/api/v2/11/bank_connections/banks', () =>
        HttpResponse.json([{ name: 'Example Bank', country: 'NL', logo: 'https://example.com/bank.png' }, { name: 'Other Bank', country: 'NL', logo: '' }]),
      ),
      http.post('*/api/v2/11/bank_connections', async ({ request }) => {
        asked = await request.json()
        return HttpResponse.json({ authorization_url: 'https://bank.example.com/login', bank_connection_id: '901' }, { status: 201 })
      }),
    )
    renderAt('/11/financial_accounts')
    await userEvent.click(await screen.findByRole('button', { name: 'More actions' }))
    await userEvent.click(screen.getByRole('button', { name: 'Connect a bank' }))
    const panel = await screen.findByRole('region', { name: 'Connect a bank' })
    const banks = await within(panel).findByRole('list', { name: 'Banks' })
    expect(within(banks).getAllByRole('button')).toHaveLength(2)
    await userEvent.type(within(panel).getByRole('searchbox', { name: 'Search your bank' }), 'example')
    expect(within(banks).getAllByRole('button')).toHaveLength(1)
    await userEvent.click(within(banks).getByRole('button', { name: 'Example Bank' }))
    await waitFor(() => expect(go).toHaveBeenCalledWith('https://bank.example.com/login'))
    expect(asked).toEqual({ bank: 'Example Bank', country: 'NL' })
  })

  it('says when bank connections are turned off', async () => {
    server.use(
      ...loaded([]),
      http.get('*/api/v2/11/bank_connections/banks', () =>
        HttpResponse.json({ error: { base: ['Bank connections are turned off on this installation; an instance administrator can turn them on'] } }, { status: 422 }),
      ),
    )
    renderAt('/11/financial_accounts')
    await userEvent.click(await screen.findByRole('button', { name: 'More actions' }))
    await userEvent.click(screen.getByRole('button', { name: 'Connect a bank' }))
    expect(await screen.findByText(/turned off on this installation/)).toBeInTheDocument()
  })

  it('shows the bank an account comes through, and reconnects in one click when its consent ends', async () => {
    const go = vi.spyOn(bankRedirect, 'go').mockImplementation(() => {})
    server.use(
      ...loaded([connection({ needs_reconnect: true, valid_until: '2026-11-20T09:00:00Z' })]),
      http.post('*/api/v2/11/bank_connections/901/reconnect', () => HttpResponse.json({ authorization_url: 'https://bank.example.com/again', bank_connection_id: '901' })),
    )
    renderAt('/11/financial_accounts')
    const row = await screen.findByRole('link', { name: 'NL91 ABNA 0417 1643 00' })
    await waitFor(() => expect(row).toHaveTextContent('Connected to Example Bank · reconnect needed'))
    expect(screen.getByText(/The connection with Example Bank ends on/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Reconnect Example Bank' }))
    await waitFor(() => expect(go).toHaveBeenCalledWith('https://bank.example.com/again'))
  })

  it('offers to fetch again after a failed fetch', async () => {
    let synced = false
    server.use(
      ...loaded([connection({ last_error: 'the bank connection failed: Bank unavailable' })]),
      http.post('*/api/v2/11/bank_connections/901/sync', () => {
        synced = true
        return HttpResponse.json(connection())
      }),
    )
    renderAt('/11/financial_accounts')
    expect(await screen.findByText('Fetching from Example Bank failed: the bank connection failed: Bank unavailable')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }))
    await waitFor(() => expect(synced).toBe(true))
  })
})

describe('the bank connection callback', () => {
  it('completes the connection and opens the accounts', async () => {
    let sent: unknown
    server.use(
      ...loaded([connection()]),
      http.post('*/api/bank_connections/complete', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(connection())
      }),
    )
    renderAt('/bank_connection_callback?state=s1&code=c1')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/financial_accounts'))
    expect(sent).toEqual({ state: 's1', code: 'c1' })
    expect(await screen.findByText('Connected to Example Bank. Its transactions are in.')).toBeInTheDocument()
  })

  it('says what the bank answered when it gave no access', async () => {
    server.use(...loaded([]))
    renderAt('/bank_connection_callback?state=s1&error=access_denied&error_description=User+cancelled')
    expect(await screen.findByText('The bank did not give access: User cancelled')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Back to MoneyHarpy' })).toBeInTheDocument()
  })
})

describe('bank connections in the settings', () => {
  it('lists each connection with its accounts and lets the user fetch, reconnect and disconnect', async () => {
    const go = vi.spyOn(bankRedirect, 'go').mockImplementation(() => {})
    let synced = false
    let deleted = false
    server.use(
      ...loaded([connection()]),
      http.post('*/api/v2/11/bank_connections/901/sync', () => {
        synced = true
        return HttpResponse.json(connection())
      }),
      http.post('*/api/v2/11/bank_connections/901/reconnect', () => HttpResponse.json({ authorization_url: 'https://bank.example.com/again', bank_connection_id: '901' })),
      http.delete('*/api/v2/11/bank_connections/901', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/settings/bank')
    const card = await screen.findByRole('article', { name: 'Example Bank' })
    expect(card).toHaveTextContent('Connected')
    expect(card).toHaveTextContent('NL91 ABNA 0417 1643 00')
    await userEvent.click(within(card).getByRole('button', { name: 'Fetch now' }))
    await waitFor(() => expect(synced).toBe(true))
    await userEvent.click(within(card).getByRole('button', { name: 'Reconnect' }))
    await waitFor(() => expect(go).toHaveBeenCalledWith('https://bank.example.com/again'))
    await userEvent.click(within(card).getByRole('button', { name: 'Disconnect' }))
    expect(within(card).getByText(/the ones fetched stay/)).toBeInTheDocument()
    await userEvent.click(within(card).getByRole('button', { name: 'Yes, disconnect' }))
    await waitFor(() => expect(deleted).toBe(true))
  })
})

describe('adding transactions', () => {
  it('offers to connect the bank instead', async () => {
    server.use(...loaded([]), http.get('*/api/v2/11/bank_connections/banks', () => HttpResponse.json([{ name: 'Example Bank', country: 'NL', logo: '' }])))
    renderAt('/11/financial_accounts')
    await userEvent.click(await screen.findByRole('button', { name: 'Add transactions' }))
    await userEvent.click(screen.getByRole('button', { name: 'connect your bank' }))
    expect(await screen.findByRole('region', { name: 'Connect a bank' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Add transactions' })).toBeNull()
  })
})

describe('Revolut Business with an API key', () => {
  const revolut = (over: Partial<BankConnection> = {}) => connection({ id: '902', provider: 'revolut', bank: 'Revolut Business', country: '', valid_until: null, ...over })

  it('connects with a key pasted in the connect panel, also while other banks are off', async () => {
    let sent: unknown
    server.use(
      ...loaded([]),
      http.get('*/api/v2/11/bank_connections/banks', () => HttpResponse.json({ error: { base: ['Bank connections are turned off on this installation; an instance administrator can turn them on'] } }, { status: 422 })),
      http.post('*/api/v2/11/bank_connections/api_key', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(revolut(), { status: 201 })
      }),
    )
    renderAt('/11/financial_accounts')
    await userEvent.click(await screen.findByRole('button', { name: 'More actions' }))
    await userEvent.click(screen.getByRole('button', { name: 'Connect a bank' }))
    const panel = await screen.findByRole('region', { name: 'Connect a bank' })
    await userEvent.click(within(panel).getByRole('button', { name: 'Revolut Business (API key)' }))
    expect(within(panel).getByText(/without request signing/)).toBeInTheDocument()
    await userEvent.type(within(panel).getByLabelText('API key'), 'secret-key')
    await userEvent.click(within(panel).getByRole('button', { name: 'Connect' }))
    expect(await within(panel).findByText('Connected to Revolut Business. Its transactions are in.')).toBeInTheDocument()
    expect(sent).toEqual({ provider: 'revolut', api_key: 'secret-key' })
  })

  it('asks for a new key when Revolut no longer accepts it', async () => {
    let sent: unknown
    server.use(
      ...loaded([revolut({ state: 'expired', needs_reconnect: true, last_error: 'the bank no longer gives access; reconnect the bank' })]),
      http.put('*/api/v2/11/bank_connections/902/api_key', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(revolut())
      }),
    )
    renderAt('/11/financial_accounts')
    expect(await screen.findByText(/Revolut Business no longer accepts the API key/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Enter a new Revolut Business API key' }))
    await userEvent.type(screen.getByLabelText('API key'), 'new-key')
    await userEvent.click(screen.getByRole('button', { name: 'Replace API key' }))
    await waitFor(() => expect(sent).toEqual({ api_key: 'new-key' }))
  })

  it('replaces the key from the settings, where it has no consent date', async () => {
    let sent: unknown
    server.use(
      ...loaded([revolut()]),
      http.put('*/api/v2/11/bank_connections/902/api_key', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(revolut())
      }),
    )
    renderAt('/11/settings/bank')
    const card = await screen.findByRole('article', { name: 'Revolut Business' })
    expect(card).not.toHaveTextContent('Access until')
    expect(within(card).queryByRole('button', { name: 'Reconnect' })).toBeNull()
    await userEvent.click(within(card).getByRole('button', { name: 'Replace API key' }))
    await userEvent.type(within(card).getByLabelText('API key'), 'new-key')
    await userEvent.click(within(card).getAllByRole('button', { name: 'Replace API key' }).at(-1)!)
    await waitFor(() => expect(sent).toEqual({ api_key: 'new-key' }))
  })
})
