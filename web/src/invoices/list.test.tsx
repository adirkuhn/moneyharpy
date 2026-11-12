import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { loaded, shop } from './testkit'

describe('the invoice list', () => {
  it('lists invoices with their number, contact, amount and status', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, undefined, seen))
    renderAt('/11/sales_invoices')
    const table = await screen.findByRole('table', { name: 'Invoices' })
    expect(within(table).getByRole('link', { name: '2026-0001' })).toHaveAttribute('href', '/11/sales_invoices/901')
    const draft = within(table).getByRole('link', { name: 'Draft #3' })
    expect(draft).toHaveAttribute('href', '/11/sales_invoices/902')
    expect(draft.querySelector('em')).toHaveTextContent('Draft #3')
    expect(within(table).getAllByText('Example Shop B.V.')).toHaveLength(2)
    expect(within(table).getAllByText('€242.00')).toHaveLength(2)
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('2026-11-01')
    expect(within(rows[1]!).getByText('Open')).toHaveClass('badge')
    expect(within(rows[2]!).getByText('Draft')).toHaveClass('badge')
    expect(screen.getByRole('link', { name: 'New invoice' })).toHaveAttribute('href', '/11/sales_invoices/new')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('period:this_year')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Sales invoices' })).toHaveAttribute('aria-current', 'page')
  })

  it('filters with the quick filters and the advanced filter', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, undefined, seen))
    server.use(http.get('*/api/v2/11/contacts/701', () => HttpResponse.json(shop)))
    renderAt('/11/sales_invoices')
    await screen.findByRole('table', { name: 'Invoices' })
    const toggle = screen.getByRole('button', { name: 'Invoices from this year' })
    await userEvent.click(toggle)
    await userEvent.click(within(screen.getByRole('search', { name: 'Invoice filters' })).getByRole('link', { name: 'Draft invoices' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:draft'))
    expect(toggle).toHaveTextContent('Draft invoices')
    await userEvent.click(toggle)
    await userEvent.click(within(screen.getByRole('search', { name: 'Invoice filters' })).getByRole('link', { name: 'Unpaid invoices' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:open|late|reminded|pending_payment'))

    await userEvent.click(toggle)
    const panel = screen.getByRole('search', { name: 'Invoice filters' })
    expect(within(panel).getByRole('radio', { name: 'Unpaid' })).toBeChecked()
    await userEvent.click(within(panel).getByRole('radio', { name: 'Paid' }))
    await userEvent.click(within(panel).getByRole('button', { name: String(new Date().getFullYear() - 1) }))
    await userEvent.click(within(panel).getByRole('combobox', { name: 'Contact' }))
    await userEvent.type(within(panel).getByRole('searchbox', { name: 'Find a contact' }), 'shop')
    await userEvent.click(await within(panel).findByRole('option', { name: 'Example Shop B.V.' }))
    await userEvent.selectOptions(await within(panel).findByRole('combobox', { name: 'Workflow' }), await within(panel).findByRole('option', { name: 'Standaard (NL)' }))
    expect(within(panel).queryByRole('option', { name: 'Quotes' })).toBeNull()
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:paid,period:prev_year,contact_id:701,workflow_id:601'))
    expect(screen.queryByRole('search', { name: 'Invoice filters' })).toBeNull()
    expect(toggle).toHaveTextContent('Filtered invoices')

    await userEvent.click(toggle)
    const again = screen.getByRole('search', { name: 'Invoice filters' })
    expect(await within(again).findByRole('combobox', { name: 'Contact' })).toHaveTextContent('Example Shop B.V.')
    await userEvent.click(within(again).getByRole('button', { name: 'Clear contact' }))
    await userEvent.click(within(again).getByRole('radio', { name: 'All' }))
    await userEvent.click(within(again).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('period:prev_year,workflow_id:601'))
  })

  it('offers statistics and exports as not available yet, and no more actions', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices')
    await screen.findByRole('table', { name: 'Invoices' })
    expect(screen.getByRole('button', { name: 'Show statistics' })).toHaveAccessibleDescription('Not available yet.')
    await userEvent.click(screen.getByRole('button', { name: 'Export' }))
    expect(screen.getByText('Excel per invoice').closest('[aria-disabled]')).toHaveAttribute('aria-disabled', 'true')
    expect(screen.queryByRole('button', { name: 'More actions' })).toBeNull()
  })

  it('opens an invoice from anywhere in its row', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices')
    const table = await screen.findByRole('table', { name: 'Invoices' })
    await userEvent.click(within(table).getAllByText('€242.00')[0]!)
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/sales_invoices/901'))
  })

  it('pages through many invoices', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, 120, seen))
    renderAt('/11/sales_invoices')
    const pages = await screen.findByRole('navigation', { name: 'Pagination' })
    await userEvent.click(within(pages).getByRole('link', { name: 'Page 3' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('page')).toBe('3'))
  })

  it('invites creating the first invoice', async () => {
    server.use(...loaded([]))
    renderAt('/11/sales_invoices')
    expect(await screen.findByText('No invoices yet. Create your first invoice.')).toBeInTheDocument()
    const intro = screen.getByRole('region', { name: 'Send your invoice' })
    expect(within(intro).getAllByRole('heading', { level: 3 }).map((h) => h.textContent)).toEqual(['Create and send invoice', 'Set up your house style', 'Create a workflow'])
    expect(within(intro).getByText('Create your sales invoice and send it straight to your customer. Decide the settings and how you send the invoice yourself.')).toBeInTheDocument()
    expect(within(intro).getByText('Give your invoices your own look. Use your logo, colors and font. Choose a layout that suits your business.')).toBeInTheDocument()
    expect(within(intro).getByText('Determine the process your invoice follows: whether reminders are sent automatically and which payment methods your customer gets.')).toBeInTheDocument()
    const info = screen.getByRole('button', { name: 'More information' })
    expect(info).toHaveAttribute('aria-expanded', 'true')
    await userEvent.click(info)
    expect(screen.queryByRole('region', { name: 'Send your invoice' })).toBeNull()
  })
})
