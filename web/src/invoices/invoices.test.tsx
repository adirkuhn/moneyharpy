import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import type { Contact } from '../contacts/queries'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, documentStyle, listing, server, signedIn, workflow } from '../test/server'
import type { SalesInvoice } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const rate = (over: Partial<TaxRate> = {}): TaxRate => ({
  id: '2101', administration_id: '11', name: '21% btw', percentage: '21', tax_rate_type: 'sales_invoice', show_tax: true, active: true,
  country: null, created_at: stamp, updated_at: stamp, uri: '/11/tax_rates/2101', tax_report_boxes: ['NL/1a'], position: 1, ...over,
})
const rates = [
  rate(),
  rate({ id: '2109', name: '9% btw', percentage: '9', position: 2, uri: '/11/tax_rates/2109' }),
  rate({ id: '2150', name: 'Old rate', percentage: '6', active: false, position: 3, uri: '/11/tax_rates/2150' }),
  rate({ id: '2201', name: '21% purchase', tax_rate_type: 'purchase_invoice', uri: '/11/tax_rates/2201' }),
]

const sales: LedgerAccount = {
  id: '8001', administration_id: '11', name: 'Sales', account_type: 'revenue', account_id: '8000', parent_id: null, active: true,
  allowed_document_types: ['sales_invoice'], financial_account_id: null, created_at: stamp, updated_at: stamp,
  taxonomy_item: null, uri: '/11/ledger_accounts/8001', description: null, system_role: null, template: null,
}

const shop = {
  id: '701', administration_id: '11', company_name: 'Example Shop B.V.', firstname: null, lastname: null, customer_id: '1',
  send_invoices_to_email: 'billing@example.com', email_ubl: false, delivery_method: 'Email' as const, country: 'NL',
}

const invoice = (over: Partial<SalesInvoice> = {}): SalesInvoice => ({
  id: '901', administration_id: '11', contact_id: '701', contact: shop, invoice_id: '2026-0001', draft_id: 1, state: 'open',
  invoice_date: '2026-11-01', due_date: '2026-11-15', reference: 'Order 12', currency: 'EUR', prices_are_incl_tax: false, discount: null,
  workflow_id: '601', document_style_id: '701', identity_id: '461', payment_conditions: '', first_due_interval: 14, paused: false, original_sales_invoice_id: null,
  details: [{ id: '951', description: 'Consulting', amount: '2', price: '100.0', tax_rate_id: '2101', ledger_account_id: '8001', total_price_excl_tax_with_discount: '200.0' }],
  tax_totals: [{ tax_rate_id: '2101', taxable_amount: '200.0', tax_amount: '42.0' }],
  payments: [], notes: [], attachments: [], events: [],
  total_price_excl_tax: '200.0', total_tax: '42.0', total_price_incl_tax: '242.0', total_unpaid: '242.0', total_discount: '0.0',
  version: 1, uri: '/11/sales_invoices/901',
  ...over,
})
const draft = invoice({ id: '902', invoice_id: null, draft_id: 3, state: 'draft', invoice_date: null, uri: '/11/sales_invoices/902' })

function loaded(list: SalesInvoice[] = [invoice(), draft], total = list.length, seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/sales_invoices', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(total) } })
    }),
    http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice())),
    http.get('*/api/v2/11/sales_invoices/902', () => HttpResponse.json(draft)),
    http.get('*/api/v2/11/tax_rates', () => HttpResponse.json(rates)),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales])),
    http.get('*/api/v2/11/workflows', () => HttpResponse.json([workflow(), workflow({ id: '602', type: 'EstimateWorkflow', name: 'Quotes' })])),
    http.get('*/api/v2/11/document_styles', () => HttpResponse.json([documentStyle({ id: '701' })])),
    http.get('*/api/v2/11/contacts', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json([shop as unknown as Contact], { headers: { 'X-Total-Count': '1' } })
    }),
  ]
}

describe('the invoice list', () => {
  it('lists invoices with their number, contact, amount and status', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, undefined, seen))
    renderAt('/11/sales_invoices')
    const table = await screen.findByRole('table', { name: 'Invoices' })
    expect(within(table).getByRole('link', { name: '2026-0001' })).toHaveAttribute('href', '/11/sales_invoices/901')
    expect(within(table).getByRole('link', { name: 'Draft 3' })).toHaveAttribute('href', '/11/sales_invoices/902')
    expect(within(table).getAllByText('Example Shop B.V.')).toHaveLength(2)
    expect(within(table).getAllByText('€242.00')).toHaveLength(2)
    const rows = within(table).getAllByRole('row')
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
    await userEvent.click(within(again).getByRole('button', { name: 'All periods' }))
    await userEvent.click(within(again).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('workflow_id:601'))
  })

  it('offers statistics and exports as not available yet, and more actions', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices')
    await screen.findByRole('table', { name: 'Invoices' })
    expect(screen.getByRole('button', { name: 'Show statistics' })).toHaveAccessibleDescription('Not available yet.')
    await userEvent.click(screen.getByRole('button', { name: 'Export' }))
    expect(screen.getByText('Excel per invoice').closest('[aria-disabled]')).toHaveAttribute('aria-disabled', 'true')
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(screen.getByRole('link', { name: 'Invoice settings' })).toHaveAttribute('href', '/11/settings/invoices_and_estimates')
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
    expect(within(intro).getAllByRole('heading', { level: 3 })).toHaveLength(3)
    const info = screen.getByRole('button', { name: 'More information' })
    expect(info).toHaveAttribute('aria-expanded', 'true')
    await userEvent.click(info)
    expect(screen.queryByRole('region', { name: 'Send your invoice' })).toBeNull()
  })
})

async function pickContact() {
  await userEvent.click(await screen.findByRole('combobox', { name: 'Contact' }))
  await userEvent.type(screen.getByRole('searchbox', { name: 'Find a contact' }), 'shop')
  await userEvent.click(await screen.findByRole('option', { name: 'Example Shop B.V.' }))
}

describe('the invoice editor', () => {
  it('creates an invoice with lines and shows the totals as you type', async () => {
    let sent: { sales_invoice?: Record<string, unknown> } = {}
    server.use(...loaded(), http.post('*/api/v2/11/sales_invoices', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(draft, { status: 201 })
    }))
    renderAt('/11/sales_invoices/new')
    expect(await screen.findByRole('heading', { level: 1, name: 'New invoice' })).toBeInTheDocument()
    await pickContact()
    await userEvent.type(screen.getByLabelText('Reference'), 'Order 12')
    const lines = screen.getByRole('table', { name: 'Lines' })
    let rows = within(lines).getAllByRole('row')
    await userEvent.type(within(rows[1]!).getByLabelText('Description'), 'Consulting')
    await userEvent.clear(within(rows[1]!).getByLabelText('Quantity'))
    await userEvent.type(within(rows[1]!).getByLabelText('Quantity'), '2')
    await userEvent.type(within(rows[1]!).getByLabelText('Price'), '100')
    expect(within(rows[1]!).getByLabelText('VAT rate')).toHaveValue('2101')
    expect(within(within(rows[1]!).getByLabelText('VAT rate')).queryByRole('option', { name: 'Old rate' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Add line' }))
    rows = within(lines).getAllByRole('row')
    await userEvent.type(within(rows[2]!).getByLabelText('Description'), 'Book')
    await userEvent.type(within(rows[2]!).getByLabelText('Price'), '50')
    await userEvent.selectOptions(within(rows[2]!).getByLabelText('VAT rate'), '2109')
    await userEvent.selectOptions(within(rows[2]!).getByLabelText('Category'), '8001')
    await userEvent.type(screen.getByLabelText('Discount'), '10')

    const totals = screen.getByRole('table', { name: 'Totals' })
    expect(within(totals).getByRole('row', { name: /Subtotal/ })).toHaveTextContent('€250.00')
    expect(within(totals).getByRole('row', { name: /Discount/ })).toHaveTextContent('-€25.00')
    expect(within(totals).getByRole('row', { name: /21% btw/ })).toHaveTextContent('€37.80')
    expect(within(totals).getByRole('row', { name: /9% btw/ })).toHaveTextContent('€4.05')
    expect(within(totals).getByRole('row', { name: /Total/ })).toHaveTextContent('€266.85')

    await userEvent.click(screen.getByRole('button', { name: 'Save invoice' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/902$/))
    expect(sent.sales_invoice).toMatchObject({ contact_id: '701', reference: 'Order 12', discount: '10', prices_are_incl_tax: false })
    expect(sent.sales_invoice?.details_attributes).toEqual([
      { description: 'Consulting', amount: '2', price: '100', tax_rate_id: '2101' },
      { description: 'Book', amount: '1', price: '50', tax_rate_id: '2109', ledger_account_id: '8001' },
    ])
  })

  it('takes VAT out of prices that include it', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/new')
    const lines = await screen.findByRole('table', { name: 'Lines' })
    await userEvent.click(screen.getByRole('checkbox', { name: 'Prices include VAT' }))
    await userEvent.type(within(within(lines).getAllByRole('row')[1]!).getByLabelText('Price'), '121')
    const totals = screen.getByRole('table', { name: 'Totals' })
    expect(within(totals).getByRole('row', { name: /21% btw/ })).toHaveTextContent('€21.00')
    expect(within(totals).getByRole('row', { name: /Total/ })).toHaveTextContent('€121.00')
  })

  it('shows what is wrong beside the fields', async () => {
    server.use(...loaded(), http.post('*/api/v2/11/sales_invoices', () => HttpResponse.json({ error: { contact_id: ["can't be blank"], 'details_attributes.0.price': ['is not an amount'] } }, { status: 422 })))
    renderAt('/11/sales_invoices/new')
    const lines = await screen.findByRole('table', { name: 'Lines' })
    await userEvent.type(within(within(lines).getAllByRole('row')[1]!).getByLabelText('Description'), 'Consulting')
    await userEvent.click(screen.getByRole('button', { name: 'Save invoice' }))
    expect(await screen.findByText("Contact can't be blank")).toBeInTheDocument()
    expect(screen.getByText('Price on line 1 is not an amount')).toBeInTheDocument()
    expect(within(lines).getByLabelText('Price')).toHaveAttribute('aria-invalid', 'true')
  })

  it('edits an invoice, changing and removing lines', async () => {
    let sent: { sales_invoice?: Record<string, unknown> } = {}
    const two = invoice({
      details: [
        ...(invoice().details ?? []),
        { id: '952', description: 'Travel', amount: '1', price: '30.0', tax_rate_id: '2101', ledger_account_id: '8001', total_price_excl_tax_with_discount: '30.0' },
      ],
    })
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(two)),
      http.patch('*/api/v2/11/sales_invoices/901', async ({ request }) => {
        sent = (await request.json()) as typeof sent
        return HttpResponse.json(invoice())
      }),
    )
    renderAt('/11/sales_invoices/901/edit')
    expect(await screen.findByRole('heading', { level: 1, name: 'Edit invoice 2026-0001' })).toBeInTheDocument()
    expect(screen.getByLabelText('Reference')).toHaveValue('Order 12')
    expect(screen.getByRole('combobox', { name: 'Contact' })).toHaveTextContent('Example Shop B.V.')
    const lines = screen.getByRole('table', { name: 'Lines' })
    const price = within(within(lines).getAllByRole('row')[1]!).getByLabelText('Price')
    expect(price).toHaveValue('100')
    await userEvent.clear(price)
    await userEvent.type(price, '120')
    await userEvent.click(screen.getByRole('button', { name: 'Remove line 2' }))
    expect(within(screen.getByRole('table', { name: 'Totals' })).getByRole('row', { name: /Total/ })).toHaveTextContent('€290.40')
    await userEvent.click(screen.getByRole('button', { name: 'Save invoice' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/901$/))
    expect(sent.sales_invoice).toMatchObject({ contact_id: '701', workflow_id: '601', first_due_interval: 14 })
    expect(sent.sales_invoice?.details_attributes).toEqual([
      { id: '951', description: 'Consulting', amount: '2', price: '120', tax_rate_id: '2101', ledger_account_id: '8001' },
      { id: '952', _destroy: true },
    ])
  })
})

describe('an invoice', () => {
  it('shows its details, lines and totals with download links', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/901')
    expect(await screen.findByRole('heading', { level: 1, name: '2026-0001' })).toBeInTheDocument()
    expect(screen.getByText('Open', { selector: '.badge' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/contacts/701')
    const lines = screen.getByRole('table', { name: 'Lines' })
    expect(within(lines).getByRole('row', { name: /Consulting/ })).toHaveTextContent('€200.00')
    const totals = screen.getByRole('table', { name: 'Totals' })
    expect(await within(totals).findByRole('row', { name: /21% btw/ })).toHaveTextContent('€42.00')
    expect(within(totals).getByRole('row', { name: /^Total/ })).toHaveTextContent('€242.00')
    expect(within(totals).getByRole('row', { name: /Unpaid/ })).toHaveTextContent('€242.00')
    expect(screen.getByRole('link', { name: 'Download PDF' })).toHaveAttribute('href', '/api/v2/11/sales_invoices/901/download_pdf')
    expect(screen.getByRole('link', { name: 'Edit' })).toHaveAttribute('href', '/11/sales_invoices/901/edit')
    // The other actions wait under ⋯.
    expect(screen.queryByRole('link', { name: 'Download UBL' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(screen.getByRole('link', { name: 'Download UBL' })).toHaveAttribute('href', '/api/v2/11/sales_invoices/901/download_ubl')
    expect(screen.getByRole('link', { name: 'Packing slip' })).toHaveAttribute('href', '/api/v2/11/sales_invoices/901/download_packing_slip_pdf')
    expect(screen.getByRole('button', { name: 'Create credit invoice' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull()
  })

  it('sends a draft by email', async () => {
    let sent: unknown
    let current = draft
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/902', () => HttpResponse.json(current)),
      http.patch('*/api/v2/11/sales_invoices/902/send_invoice', async ({ request }) => {
        sent = await request.json()
        current = invoice({ id: '902', invoice_id: '2026-0002', state: 'open' })
        return HttpResponse.json(current)
      }),
    )
    renderAt('/11/sales_invoices/902')
    expect(await screen.findByRole('heading', { level: 1, name: 'Draft 3' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(screen.queryByRole('link', { name: 'Download UBL' })).toBeNull()
    expect(screen.getByRole('link', { name: 'Make recurring' })).toHaveAttribute('href', '/11/recurring_sales_invoices/new?from=902')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    const panel = screen.getByRole('region', { name: 'Send invoice' })
    expect(within(panel).getByLabelText('Email address')).toHaveValue('billing@example.com')
    await userEvent.type(within(panel).getByLabelText('Message'), 'Thanks for your order')
    await userEvent.click(within(panel).getByRole('button', { name: 'Send invoice' }))
    expect(await screen.findByRole('heading', { level: 1, name: '2026-0002' })).toBeInTheDocument()
    expect(sent).toEqual({ sales_invoice_sending: { delivery_method: 'Email', email_address: 'billing@example.com', email_message: 'Thanks for your order', deliver_ubl: false } })
    expect(screen.getByText('Open', { selector: '.badge' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Send invoice' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Send again' })).toBeInTheDocument()
  })

  it('schedules a draft for a later date', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/sales_invoices/902/send_invoice', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ ...draft, state: 'scheduled' })
    }))
    renderAt('/11/sales_invoices/902')
    await userEvent.click(await screen.findByRole('button', { name: 'Send' }))
    const panel = screen.getByRole('region', { name: 'Send invoice' })
    await userEvent.selectOptions(within(panel).getByLabelText('Send by'), 'Manual')
    expect(within(panel).queryByLabelText('Email address')).toBeNull()
    await userEvent.click(within(panel).getByRole('checkbox', { name: 'Send on a later date' }))
    const date = within(panel).getByLabelText('Send on')
    await userEvent.clear(date)
    await userEvent.type(date, '2030-01-15')
    await userEvent.click(within(panel).getByRole('button', { name: 'Schedule invoice' }))
    expect(await screen.findByText('Scheduled', { selector: '.badge' })).toBeInTheDocument()
    expect(sent).toEqual({ sales_invoice_sending: { delivery_method: 'Manual', sending_scheduled: true, invoice_date: '2030-01-15' } })
  })

  it('explains when the email could not be sent', async () => {
    server.use(...loaded(), http.patch('*/api/v2/11/sales_invoices/902/send_invoice', () =>
      HttpResponse.json({ error: { base: ['The invoice is booked but could not be emailed; check the mail settings and send it again'] } }, { status: 422 })))
    renderAt('/11/sales_invoices/902')
    await userEvent.click(await screen.findByRole('button', { name: 'Send' }))
    await userEvent.click(within(screen.getByRole('region', { name: 'Send invoice' })).getByRole('button', { name: 'Send invoice' }))
    expect(await screen.findByText(/booked but could not be emailed/)).toBeInTheDocument()
  })

  it('creates a credit invoice and opens it', async () => {
    let called = false
    server.use(...loaded())
    server.use(
      http.patch('*/api/v2/11/sales_invoices/901/duplicate_creditinvoice', () => {
        called = true
        return HttpResponse.json({ ...draft, id: '903', draft_id: 4, original_sales_invoice_id: '901', uri: '/11/sales_invoices/903' })
      }),
      http.get('*/api/v2/11/sales_invoices/903', () => HttpResponse.json({ ...draft, id: '903', draft_id: 4, original_sales_invoice_id: '901', uri: '/11/sales_invoices/903' })),
    )
    renderAt('/11/sales_invoices/901')
    await chooseMore('Create credit invoice')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/sales_invoices/903'))
    expect(called).toBe(true)
    expect(await screen.findByRole('heading', { level: 1, name: 'Draft 4' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'the original invoice' })).toHaveAttribute('href', '/11/sales_invoices/901')
  })

  it('pauses and resumes reminders', async () => {
    let paused = false
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice({ paused }))),
      http.post('*/api/v2/11/sales_invoices/901/pause', () => {
        paused = true
        return HttpResponse.json(invoice({ paused }), { status: 201 })
      }),
      http.post('*/api/v2/11/sales_invoices/901/resume', () => {
        paused = false
        return HttpResponse.json(invoice({ paused }), { status: 201 })
      }),
    )
    renderAt('/11/sales_invoices/901')
    await chooseMore('Pause reminders')
    expect(await screen.findByText('Reminders for this invoice are paused.')).toBeInTheDocument()
    await chooseMore('Resume reminders')
    await waitFor(() => expect(screen.queryByText('Reminders for this invoice are paused.')).toBeNull())
  })

  it('deletes a draft and returns to the list', async () => {
    server.use(...loaded(), http.delete('*/api/v2/11/sales_invoices/902', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/sales_invoices/902')
    expect(await screen.findByRole('heading', { level: 1, name: 'Draft 3' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(screen.queryByRole('button', { name: 'Create credit invoice' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'Delete this draft invoice? This cannot be undone.' })).getByRole('button', { name: 'Delete permanently' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices$/))
  })

  it('cannot be edited once paid', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice({ state: 'paid', total_unpaid: '0.0' }))))
    renderAt('/11/sales_invoices/901')
    expect(await screen.findByText('Paid', { selector: '.badge' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Edit' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Pause reminders' })).toBeNull()
  })

  it('keeps notes', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/11/sales_invoices/901/notes', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({}, { status: 201 })
    }))
    renderAt('/11/sales_invoices/901')
    const notes = await screen.findByRole('region', { name: 'Notes' })
    await userEvent.type(within(notes).getByLabelText('New note'), 'Called about payment')
    await userEvent.click(within(notes).getByRole('checkbox', { name: 'This is a to-do' }))
    await userEvent.click(within(notes).getByRole('button', { name: 'Add note' }))
    await waitFor(() => expect(sent).toEqual({ note: { note: 'Called about payment', todo: true } }))
  })

  it('uploads, lists and removes attachments', async () => {
    // jsdom's FormData does not carry files into Node's fetch, so the part is checked where it is added.
    const append = vi.spyOn(FormData.prototype, 'append')
    let contentType = ''
    let removed = false
    const attachment = { id: '991', administration_id: '11', attachable_id: '901', attachable_type: 'SalesInvoice' as const, filename: 'order.pdf', content_type: 'application/pdf', size: 2048, rotation: null, created_at: stamp, updated_at: stamp }
    let current = invoice()
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(current)),
      http.post('*/api/v2/11/sales_invoices/901/attachments', async ({ request }) => {
        contentType = request.headers.get('content-type') ?? ''
        current = invoice({ attachments: [attachment] })
        return HttpResponse.json(attachment)
      }),
      http.delete('*/api/v2/11/sales_invoices/901/attachments/991', () => {
        removed = true
        current = invoice()
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/sales_invoices/901')
    const block = await screen.findByRole('region', { name: 'Attachments' })
    await userEvent.upload(within(block).getByLabelText('Add attachment'), new File(['%PDF-1.4'], 'order.pdf', { type: 'application/pdf' }))
    await waitFor(() => expect(contentType).toMatch(/^multipart\/form-data; boundary=/))
    expect(append).toHaveBeenCalledWith('file', expect.objectContaining({ name: 'order.pdf' }))
    const link = await within(block).findByRole('link', { name: 'order.pdf' })
    expect(link).toHaveAttribute('href', '/api/v2/11/sales_invoices/901/attachments/991/download')
    await userEvent.click(within(block).getByRole('button', { name: 'Remove order.pdf' }))
    await userEvent.click(within(block).getByRole('button', { name: 'Confirm remove order.pdf' }))
    await waitFor(() => expect(removed).toBe(true))
    await waitFor(() => expect(within(block).queryByRole('link', { name: 'order.pdf' })).toBeNull())
    append.mockRestore()
  })
})

describe('getting paid', () => {
  it('registers a payment and removes it again', async () => {
    let sent: unknown
    let removed = false
    const payment = { id: '981', administration_id: '11', invoice_type: 'SalesInvoice' as const, invoice_id: '901', price: '242.0', price_base: '242.0', payment_date: '2026-11-05', manual_payment_action: 'cash_payment', created_at: stamp, updated_at: stamp }
    let current = invoice()
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(current)),
      http.post('*/api/v2/11/sales_invoices/901/payments', async ({ request }) => {
        sent = await request.json()
        current = invoice({ state: 'paid', paid_at: '2026-11-05', total_unpaid: '0.0', payments: [payment] })
        return HttpResponse.json(payment, { status: 201 })
      }),
      http.delete('*/api/v2/11/sales_invoices/901/payments/981', () => {
        removed = true
        current = invoice()
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/sales_invoices/901')
    const block = await screen.findByRole('region', { name: 'Payments' })
    expect(within(block).getByText('No payments yet.')).toBeInTheDocument()
    const form = within(block).getByRole('form', { name: 'Register payment' })
    expect(within(form).getByLabelText('Amount')).toHaveValue('242')
    const date = within(form).getByLabelText('Payment date')
    await userEvent.clear(date)
    await userEvent.type(date, '2026-11-05')
    await userEvent.selectOptions(within(form).getByLabelText('Paid by'), 'cash_payment')
    await userEvent.click(within(form).getByRole('button', { name: 'Register payment' }))
    await waitFor(() => expect(sent).toEqual({ payment: { payment_date: '2026-11-05', price: '242', manual_payment_action: 'cash_payment' } }))
    expect(await screen.findByText('Paid', { selector: '.badge' })).toBeInTheDocument()
    const table = within(block).getByRole('table', { name: 'Payments' })
    expect(within(table).getByRole('row', { name: /Cash/ })).toHaveTextContent('€242.00')
    expect(within(block).queryByRole('form', { name: 'Register payment' })).toBeNull()
    await userEvent.click(within(table).getByRole('button', { name: /^Remove payment of/ }))
    await userEvent.click(within(table).getByRole('button', { name: /^Confirm remove payment of/ }))
    await waitFor(() => expect(removed).toBe(true))
    expect(await screen.findByText('Open', { selector: '.badge' })).toBeInTheDocument()
  })

  it('shows what is wrong with a payment', async () => {
    server.use(...loaded(), http.post('*/api/v2/11/sales_invoices/901/payments', () =>
      HttpResponse.json({ error: { price: ['must not be more than what is unpaid, 242.0'] } }, { status: 422 })))
    renderAt('/11/sales_invoices/901')
    const form = await screen.findByRole('form', { name: 'Register payment' })
    await userEvent.clear(within(form).getByLabelText('Amount'))
    await userEvent.type(within(form).getByLabelText('Amount'), '500')
    await userEvent.click(within(form).getByRole('button', { name: 'Register payment' }))
    expect(await within(form).findByText(/must not be more than what is unpaid/)).toBeInTheDocument()
  })

  it('links a payment to its bank transaction', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice({
      state: 'paid', total_unpaid: '0.0',
      payments: [{ id: '982', administration_id: '11', invoice_type: 'SalesInvoice', invoice_id: '901', price: '242.0', price_base: '242.0', payment_date: '2026-11-03', financial_mutation_id: '771', manual_payment_action: null, created_at: stamp, updated_at: stamp }],
    }))))
    renderAt('/11/sales_invoices/901')
    const table = await screen.findByRole('table', { name: 'Payments' })
    expect(within(table).getByRole('link', { name: 'Bank transaction' })).toHaveAttribute('href', '/11/financial_mutations/771')
    expect(within(table).getByRole('button', { name: /^Remove payment of/ })).toHaveTextContent('Unlink')
  })

  it('settles a credit invoice with the invoice it credits', async () => {
    let called = false
    const credit = invoice({ id: '903', invoice_id: '2026-0003', original_sales_invoice_id: '901', total_price_incl_tax: '-242.0', total_unpaid: '-242.0', uri: '/11/sales_invoices/903' })
    let current = credit
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/903', () => HttpResponse.json(current)),
      http.patch('*/api/v2/11/sales_invoices/903/register_payment_creditinvoice', () => {
        called = true
        current = { ...credit, state: 'paid', total_unpaid: '0.0' }
        return HttpResponse.json(current)
      }),
    )
    renderAt('/11/sales_invoices/903')
    const block = await screen.findByRole('region', { name: 'Payments' })
    expect(within(block).queryByRole('form', { name: 'Register payment' })).toBeNull()
    await userEvent.click(within(block).getByRole('button', { name: 'Settle with the original invoice' }))
    await waitFor(() => expect(called).toBe(true))
    expect(await screen.findByText('Paid', { selector: '.badge' })).toBeInTheDocument()
  })

  it('marks an invoice dubious and then uncollectible', async () => {
    const bodies: unknown[] = []
    let current = invoice({ state: 'late' })
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(current)),
      http.patch('*/api/v2/11/sales_invoices/901/mark_as_dubious', async ({ request }) => {
        bodies.push(await request.json())
        current = invoice({ state: 'late', marked_dubious_on: '2026-11-08' })
        return HttpResponse.json(current)
      }),
      http.patch('*/api/v2/11/sales_invoices/901/mark_as_uncollectible', async ({ request }) => {
        bodies.push(await request.json())
        current = invoice({ state: 'uncollectible', marked_dubious_on: '2026-11-08', marked_uncollectible_on: '2026-11-09' })
        return HttpResponse.json(current)
      }),
    )
    renderAt('/11/sales_invoices/901')
    await chooseMore('Mark as dubious')
    let panel = screen.getByRole('region', { name: 'Mark as dubious' })
    await userEvent.clear(within(panel).getByLabelText('Date'))
    await userEvent.type(within(panel).getByLabelText('Date'), '2026-11-08')
    await userEvent.click(within(panel).getByRole('button', { name: 'Mark as dubious' }))
    expect(await screen.findByText('This invoice is marked as dubious.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(screen.queryByRole('button', { name: 'Mark as dubious' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Mark as uncollectible' }))
    panel = screen.getByRole('region', { name: 'Mark as uncollectible' })
    expect(within(panel).getByText(/cannot be undone/)).toBeInTheDocument()
    await userEvent.clear(within(panel).getByLabelText('Date'))
    await userEvent.type(within(panel).getByLabelText('Date'), '2026-11-09')
    await userEvent.click(within(panel).getByRole('button', { name: 'Write off as uncollectible' }))
    expect(await screen.findByText('Irrecoverable', { selector: '.badge' })).toBeInTheDocument()
    expect(bodies).toEqual([{ dubious_date: '2026-11-08' }, { uncollectible_date: '2026-11-09', book_method: 'revenue' }])
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(screen.queryByRole('button', { name: 'Send reminder' })).toBeNull()
    expect(screen.queryByRole('form', { name: 'Register payment' })).toBeNull()
  })

  it('sends a reminder and shows when the next one is due', async () => {
    let sent: unknown
    let current = invoice({ state: 'late', next_reminder: '2026-11-22' })
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(current)),
      http.post('*/api/v2/11/sales_invoices/send_reminders', async ({ request }) => {
        sent = await request.json()
        current = invoice({ state: 'reminded', reminder_count: 1 })
        return HttpResponse.json([current])
      }),
    )
    renderAt('/11/sales_invoices/901')
    expect(await screen.findByText('Nov 22, 2026')).toBeInTheDocument()
    await chooseMore('Send reminder')
    const panel = screen.getByRole('region', { name: 'Send reminder' })
    expect(within(panel).getByLabelText('Email address')).toHaveValue('billing@example.com')
    await userEvent.type(within(panel).getByLabelText('Reminder text'), 'Please pay {{invoices}')
    await userEvent.click(within(panel).getByRole('button', { name: 'Send reminder' }))
    await waitFor(() => expect(sent).toEqual({ sales_invoice_reminders: [{
      contact_id: '701', workflow_id: '601', document_style_id: '701', identity_id: '461', sales_invoice_ids: ['901'],
      delivery_method: 'Email', email_address: 'billing@example.com', reminder_text: 'Please pay {invoices}',
    }] }))
    expect(await screen.findByText('Reminded', { selector: '.badge' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Send reminder' })).toBeNull()
  })
})

describe('bulk actions in the list', () => {
  it('registers full payments, sends reminders and deletes drafts of the selected invoices', async () => {
    const paid: unknown[] = []
    let reminders: unknown
    let deleted = false
    const late = invoice({ id: '904', invoice_id: '2026-0004', state: 'late', total_unpaid: '100.0', uri: '/11/sales_invoices/904' })
    server.use(...loaded([invoice(), late, draft]))
    server.use(
      http.post('*/api/v2/11/sales_invoices/:id/payments', async ({ request, params }) => {
        paid.push({ id: params.id, ...((await request.json()) as object) })
        return HttpResponse.json({ id: '1' }, { status: 201 })
      }),
      http.post('*/api/v2/11/sales_invoices/send_reminders', async ({ request }) => {
        reminders = await request.json()
        return HttpResponse.json([late])
      }),
      http.delete('*/api/v2/11/sales_invoices/902', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/sales_invoices')
    const table = await screen.findByRole('table', { name: 'Invoices' })
    expect(screen.queryByRole('group', { name: 'Selected invoices' })).toBeNull()
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select 2026-0004' }))
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select Draft 3' }))
    let bar = screen.getByRole('group', { name: 'Selected invoices' })
    expect(within(bar).getByText('2 selected')).toBeInTheDocument()
    await userEvent.click(within(bar).getByRole('button', { name: 'Send reminders (1)' }))
    await waitFor(() => expect(reminders).toEqual({ sales_invoice_reminders: [{ contact_id: '701', workflow_id: '601', document_style_id: '701', identity_id: '461', sales_invoice_ids: ['904'] }] }))
    expect(await screen.findByText('Sent a reminder for 1 invoice.')).toBeInTheDocument()

    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select all invoices' }))
    bar = screen.getByRole('group', { name: 'Selected invoices' })
    await userEvent.click(within(bar).getByRole('button', { name: 'Register full payment (2)' }))
    await waitFor(() => expect(paid).toHaveLength(2))
    expect(paid).toEqual([
      { id: '901', payment: { payment_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/), price: '242.0' } },
      { id: '904', payment: { payment_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/), price: '100.0' } },
    ])
    expect(await screen.findByText('Registered the payments of 2 invoices.')).toBeInTheDocument()

    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select Draft 3' }))
    bar = screen.getByRole('group', { name: 'Selected invoices' })
    expect(within(bar).getByRole('button', { name: 'Register full payment (0)' })).toBeDisabled()
    await userEvent.click(within(bar).getByRole('button', { name: 'Delete drafts (1)' }))
    await userEvent.click(within(bar).getByRole('button', { name: 'Delete 1 draft permanently' }))
    await waitFor(() => expect(deleted).toBe(true))
    expect(await screen.findByText('Deleted 1 draft.')).toBeInTheDocument()
  })
})
