import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { Overview } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

const full: Overview = {
  bank: { balance: '1250.5', currency: 'EUR', unprocessed_count: 3 },
  sales: { to_receive: { count: 3, amount: '363.0' }, to_remind: { count: 1, amount: '121.0' }, currency: 'EUR' },
  incoming: { to_pay: { count: 2, amount: '242.0' }, to_process: 4, currency: 'EUR' },
  recently_processed: [
    {
      type: 'financial_mutation', id: '701', uri: '/11/financial_mutations/701', title: 'Hosting Company', description: 'Invoice 12',
      amount: '-25.5', currency: 'EUR', date: '2026-11-09', state: 'processed', processed_at: '2026-11-10T09:00:00Z',
    },
  ],
}

function open(admin = harpy, overview: Overview = full) {
  server.use(
    signedIn(),
    listing([admin]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(admin)),
    http.get('*/api/v2/11/overview', () => HttpResponse.json(overview)),
  )
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-11-10T13:30:00Z'))
})
afterEach(() => vi.useRealTimers())

describe('administration overview', () => {
  it('opens on the overview with a greeting', async () => {
    open()
    renderAt('/11')
    expect(await screen.findByRole('heading', { level: 1, name: 'Good afternoon Ann!' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent('/11/feed')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Overview' })).toHaveAttribute('aria-current', 'page')
  })

  it('shows the bank balance and what is left to link', async () => {
    open()
    renderAt('/11/feed')
    const bank = await screen.findByRole('region', { name: 'Bank' })
    expect(await within(bank).findByRole('link', { name: /Balance\s*€1,250.50/ })).toHaveAttribute('href', '/11/settings/bank')
    expect(within(bank).getByRole('link', { name: /To link\s*3/ })).toHaveAttribute('href', '/11/financial_mutations?state=unprocessed&period=all')
  })

  it('shows the invoices to receive and to remind, linking to them', async () => {
    open()
    renderAt('/11/feed')
    const sales = await screen.findByRole('region', { name: 'Sales invoices and external invoices' })
    expect(await within(sales).findByRole('link', { name: /To receive\s*— 3\s*€363.00$/ })).toHaveAttribute('href', '/11/sales_invoices?filter=state%3Aopen%7Clate%7Creminded%7Cpending_payment')
    expect(within(sales).getByRole('link', { name: /To remind\s*1$/ })).toHaveAttribute('href', '/11/sales_invoices?filter=state%3Alate%7Creminded')
    expect(within(sales).queryByText('Not available yet')).toBeNull()
  })

  it('shows the documents to pay and the uploads to process, linking to them', async () => {
    open()
    renderAt('/11/feed')
    const incoming = await screen.findByRole('region', { name: 'Incoming' })
    expect(await within(incoming).findByRole('link', { name: /To pay\s*— 2\s*€242.00$/ })).toHaveAttribute('href', '/11/documents/purchase_invoices?filter=state%3Aopen%7Clate%7Cpending_payment')
    expect(within(incoming).getByRole('link', { name: /To process\s*4$/ })).toHaveAttribute('href', '/11/documents/typeless_documents')
    expect(within(incoming).queryByText('Not available yet')).toBeNull()
  })

  it('opens the filtered invoice list from a tile', async () => {
    const seen: URL[] = []
    open()
    server.use(http.get('*/api/v2/11/sales_invoices', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })
    }))
    renderAt('/11/feed')
    const sales = await screen.findByRole('region', { name: 'Sales invoices and external invoices' })
    await userEvent.click(await within(sales).findByRole('link', { name: /To remind/ }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:late|reminded'))
  })

  it('lists what was processed last', async () => {
    open()
    renderAt('/11/feed')
    const recent = await screen.findByRole('region', { name: 'Recently processed' })
    const row = await within(recent).findByRole('link', { name: /Hosting Company/ })
    expect(row).toHaveAttribute('href', '/11/financial_mutations/701')
    expect(within(row).getByText('Invoice 12')).toBeInTheDocument()
    expect(within(row).getByText('Booked')).toBeInTheDocument()
    expect(within(row).getByText('-€25.50')).toBeInTheDocument()
  })

  it('invites booking when nothing is processed yet', async () => {
    open(harpy, { bank: full.bank, recently_processed: [] })
    renderAt('/11/feed')
    expect(await screen.findByText('Nothing processed yet. Booked transactions appear here.')).toBeInTheDocument()
  })

  it('shows only the blocks a member may see', async () => {
    const clerk = administration({ id: '11', user_type: 'employee', permissions: ['sales_invoices'] })
    open(clerk, { recently_processed: [] })
    renderAt('/11/feed')
    expect(await screen.findByRole('region', { name: 'Sales invoices and external invoices' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Bank' })).toBeNull()
    expect(screen.queryByRole('region', { name: 'Incoming' })).toBeNull()
    expect(screen.queryByRole('region', { name: 'Recently processed' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Add' }))
    const menu = screen.getByRole('navigation', { name: 'Add to this administration' })
    expect(within(menu).getByRole('link', { name: 'Invoice' })).toHaveAttribute('href', '/11/sales_invoices/new')
    expect(within(menu).queryByRole('link', { name: 'Financial account' })).toBeNull()
    expect(within(menu).queryByRole('link', { name: 'Manual journal' })).toBeNull()
  })

  it('adds nothing for a member without rights to add', async () => {
    open(administration({ id: '11', user_type: 'employee', permissions: ['time_entries'] }), { recently_processed: [] })
    renderAt('/11/feed')
    expect(await screen.findByText(/You have no access to the parts of this administration/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Add' })).toBeNull()
  })

  it('explains an overview with nothing to show', async () => {
    open(administration({ id: '11', user_type: 'employee', permissions: ['time_entries'] }), { recently_processed: [] })
    renderAt('/11/feed')
    expect(await screen.findByText(/You have no access to the parts of this administration/)).toBeInTheDocument()
  })

  it('adds what MoneyHarpy can create today', async () => {
    open()
    renderAt('/11/feed')
    await userEvent.click(await screen.findByRole('button', { name: 'Add' }))
    const menu = screen.getByRole('navigation', { name: 'Add to this administration' })
    expect(within(menu).getByRole('link', { name: 'Invoice' })).toHaveAttribute('href', '/11/sales_invoices/new')
    expect(within(menu).getByRole('link', { name: 'External invoice' })).toHaveAttribute('href', '/11/external_sales_invoices/new')
    expect(within(menu).getByRole('link', { name: 'Recurring invoice' })).toHaveAttribute('href', '/11/recurring_sales_invoices/new')
    expect(within(menu).getByRole('link', { name: 'Manual journal' })).toHaveAttribute('href', '/11/documents/general_journal_documents/new')
    expect(within(menu).getByRole('link', { name: 'Contact' })).toHaveAttribute('href', '/11/contacts/new')
    expect(within(menu).getByRole('link', { name: 'Financial account' })).toHaveAttribute('href', '/11/financial_accounts/new')
    expect(within(menu).getByRole('link', { name: 'Booking rule' })).toHaveAttribute('href', '/11/booking_rules/new')
    expect(within(menu).getByRole('link', { name: 'Category' })).toHaveAttribute('href', '/11/ledger_accounts/new')
  })

  it('reports an overview that failed to load', async () => {
    open()
    server.use(http.get('*/api/v2/11/overview', () => HttpResponse.json({ error: 'boom' }, { status: 500 })))
    renderAt('/11/feed')
    expect(await screen.findByText('The overview could not be loaded. Reload the page to try again.')).toBeInTheDocument()
  })
})

describe('logo', () => {
  it('leads to the open administration', async () => {
    open()
    renderAt('/11/settings')
    await waitFor(() => expect(screen.getByRole('link', { name: 'MoneyHarpy home' })).toHaveAttribute('href', '/11'))
  })

  it('leads to the administration opened last from other pages', async () => {
    localStorage.setItem('moneyharpy.lastAdministration', '11')
    open()
    renderAt('/user')
    await waitFor(() => expect(screen.getByRole('link', { name: 'MoneyHarpy home' })).toHaveAttribute('href', '/11'))
  })

  it('remembers the administration opened last', async () => {
    open()
    renderAt('/11/feed')
    await screen.findByRole('heading', { level: 1, name: 'Good afternoon Ann!' })
    expect(localStorage.getItem('moneyharpy.lastAdministration')).toBe('11')
  })

  it('falls back to the list when the last administration is gone', async () => {
    localStorage.setItem('moneyharpy.lastAdministration', '99')
    open()
    renderAt('/user')
    await userEvent.click(await screen.findByRole('button', { name: 'Select administration' }))
    await screen.findByRole('link', { name: 'Harpy B.V.' })
    expect(screen.getByRole('link', { name: 'MoneyHarpy home' })).toHaveAttribute('href', '/administrations')
  })
})

describe('start page', () => {
  it('opens the administration opened last', async () => {
    localStorage.setItem('moneyharpy.lastAdministration', '11')
    open()
    renderAt('/')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/feed'))
  })

  it('opens the list without one', async () => {
    open()
    renderAt('/')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/administrations'))
  })
})
