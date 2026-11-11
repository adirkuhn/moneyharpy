import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import type { Contact } from '../contacts/queries'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { ExternalInvoice } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const vat21: TaxRate = {
  id: '2101', administration_id: '11', name: '21% btw', percentage: '21', tax_rate_type: 'sales_invoice', show_tax: true, active: true,
  country: null, created_at: stamp, updated_at: stamp, uri: '/11/tax_rates/2101', tax_report_boxes: ['NL/1a'], position: 1,
}

const sales: LedgerAccount = {
  id: '8001', administration_id: '11', name: 'Sales', account_type: 'revenue', account_id: '8000', parent_id: null, active: true,
  allowed_document_types: ['sales_invoice'], financial_account_id: null, created_at: stamp, updated_at: stamp,
  taxonomy_item: null, uri: '/11/ledger_accounts/8001', description: null, system_role: null, template: null,
}

const shop = {
  id: '701', administration_id: '11', company_name: 'Example Shop B.V.', firstname: null, lastname: null, customer_id: '1',
  send_invoices_to_email: 'billing@example.com', email_ubl: false, delivery_method: 'Email' as const, country: 'NL',
}

const external = (over: Partial<ExternalInvoice> = {}): ExternalInvoice => ({
  id: '931', administration_id: '11', contact_id: '701', contact: shop, date: '2026-11-01', due_date: '2026-11-15', state: 'open',
  reference: 'WEB-1001', entry_number: 1, origin: 'api', source: 'Example webshop', source_url: 'https://shop.example.com/orders/1001',
  currency: 'EUR', prices_are_incl_tax: false, paid_at: null,
  details: [{ id: '961', description: 'Sofa', amount: '1', price: '100.0', tax_rate_id: '2101', ledger_account_id: '8001', total_price_excl_tax_with_discount: '100.0' }],
  tax_totals: [{ tax_rate_id: '2101', taxable_amount: '100.0', tax_amount: '21.0' }],
  payments: [], notes: [], attachments: [], events: [],
  total_price_excl_tax: '100.0', total_price_incl_tax: '121.0', total_paid: '0.0', total_unpaid: '121.0', total_tax: '21.0',
  marked_dubious_on: null, marked_uncollectible_on: null, version: 1, uri: '/11/external_sales_invoices/931',
  ...over,
})

const fresh = external({
  id: '932', contact_id: null, contact: null, state: 'new', reference: null, entry_number: 2, origin: 'upload', source: null, source_url: null,
  date: '2026-11-11', due_date: null, details: [], tax_totals: [], total_price_excl_tax: '0.0', total_price_incl_tax: '0.0', total_unpaid: '0.0', total_tax: '0.0',
  attachments: [{ id: '971', administration_id: '11', attachable_id: '932', attachable_type: 'ExternalSalesInvoice', filename: 'order.pdf', content_type: 'application/pdf', size: 2048, rotation: 0, created_at: stamp, updated_at: stamp }],
  uri: '/11/external_sales_invoices/932',
})

function loaded(list: ExternalInvoice[] = [external(), fresh], seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/external_sales_invoices', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/external_sales_invoices/931', () => HttpResponse.json(external())),
    http.get('*/api/v2/11/external_sales_invoices/932', () => HttpResponse.json(fresh)),
    http.get('*/api/v2/11/tax_rates', () => HttpResponse.json([vat21])),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales])),
    http.get('*/api/v2/11/contacts', () => HttpResponse.json([shop as unknown as Contact], { headers: { 'X-Total-Count': '1' } })),
  ]
}

describe('the external invoice list', () => {
  it('lists invoices with their date, contact, reference, amount and status', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/external_sales_invoices')
    const table = await screen.findByRole('table', { name: 'External invoices' })
    expect(within(table).getByRole('link', { name: 'WEB-1001' })).toHaveAttribute('href', '/11/external_sales_invoices/931')
    expect(within(table).getByRole('link', { name: 'External invoice 2' })).toHaveAttribute('href', '/11/external_sales_invoices/932')
    expect(within(table).getByText('Example Shop B.V.')).toBeInTheDocument()
    expect(within(table).getByText('€121.00')).toBeInTheDocument()
    expect(within(table).getByText('New')).toHaveClass('badge')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('period:this_year')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'External invoices' })).toHaveAttribute('aria-current', 'page')
    expect(within(table).getByRole('img', { name: 'Has attachments' })).toBeInTheDocument()
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Reference', '', 'Contact', 'Date', 'Amount', 'Status'])

    const toggle = screen.getByRole('button', { name: 'External invoices from this year' })
    await userEvent.click(toggle)
    const panel = screen.getByRole('search', { name: 'External invoice filters' })
    expect(within(panel).getByRole('link', { name: 'Unpaid external invoices' })).toHaveAttribute('href', '/11/external_sales_invoices?filter=state%3Aopen%7Clate')
    expect(within(panel).queryByRole('combobox', { name: 'Workflow' })).toBeNull()
    await userEvent.click(within(panel).getByRole('radio', { name: 'New' }))
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:new,period:this_year'))
    expect(toggle).toHaveTextContent('Filtered external invoices')
  })

  it('introduces the list and says emailing files is not available yet', async () => {
    server.use(...loaded([]))
    renderAt('/11/external_sales_invoices')
    expect(await screen.findByText(/No external invoices yet/)).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Getting started with external invoices' })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Add invoice files' })).toHaveTextContent('Drag files here to add or select files (Maximum size 10 MB per file)')
    const email = screen.getByRole('button', { name: 'Add external invoice by emailing files' })
    await userEvent.click(email)
    const box = screen.getByRole('region', { name: 'E-mail files for external invoices' })
    expect(box).toHaveTextContent('Not available yet.')
    await userEvent.click(within(box).getByRole('button', { name: 'Close' }))
    expect(email).toHaveAttribute('aria-expanded', 'false')
  })

  it('makes an invoice of every uploaded file and opens the first', async () => {
    // jsdom's FormData does not carry files into Node's fetch, so the parts are checked where they are added.
    const append = vi.spyOn(FormData.prototype, 'append')
    let calls = 0
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/external_sales_invoices/attachment', () => {
      calls++
      return HttpResponse.json(calls === 1 ? fresh : external({ id: '933', state: 'new', uri: '/11/external_sales_invoices/933' }))
    }))
    renderAt('/11/external_sales_invoices')
    const upload = await screen.findByLabelText('Upload invoices')
    await userEvent.upload(upload, [new File(['%PDF-1.4 a'], 'order-a.pdf', { type: 'application/pdf' }), new File(['%PDF-1.4 b'], 'order-b.pdf', { type: 'application/pdf' })])
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/external_sales_invoices/932'))
    expect(calls).toBe(2)
    expect(append).toHaveBeenCalledWith('file', expect.objectContaining({ name: 'order-a.pdf' }))
    expect(append).toHaveBeenCalledWith('file', expect.objectContaining({ name: 'order-b.pdf' }))
    append.mockRestore()
  })
})

describe('an external invoice', () => {
  it('fills in an uploaded invoice so it is booked', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/external_sales_invoices/932', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(external({ id: '932', reference: 'WEB-1002', uri: '/11/external_sales_invoices/932' }))
    }))
    renderAt('/11/external_sales_invoices/932')
    expect(await screen.findByRole('link', { name: 'View order.pdf' })).toHaveAttribute('href', '/api/v2/11/external_sales_invoices/932/attachments/971/download')
    const form = screen.getByRole('form', { name: 'External invoice' })
    await userEvent.click(within(form).getByRole('combobox', { name: 'Contact' }))
    await userEvent.type(screen.getByRole('searchbox', { name: 'Find a contact' }), 'shop')
    await userEvent.click(await screen.findByRole('option', { name: 'Example Shop B.V.' }))
    await userEvent.type(within(form).getByLabelText('Reference'), 'WEB-1002')
    await userEvent.type(within(form).getByLabelText('Due date'), '2026-11-25')
    await userEvent.type(within(form).getByLabelText('Description'), 'Chair')
    await userEvent.type(within(form).getByLabelText('Price'), '50')
    expect(within(form).getByText('€60.50')).toBeInTheDocument()
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({
      external_sales_invoice: {
        contact_id: '701', reference: 'WEB-1002', date: '2026-11-11', due_date: '2026-11-25', prices_are_incl_tax: false,
        details_attributes: [{ description: 'Chair', amount: '1', price: '50', tax_rate_id: '2101' }],
      },
    }))
    expect(await screen.findByRole('heading', { name: 'WEB-1002' })).toBeInTheDocument()
  })

  it('takes a payment and can be marked dubious', async () => {
    let paid: unknown
    let dubious: unknown
    let current = external({ state: 'late' })
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/external_sales_invoices/931', () => HttpResponse.json(current)),
      http.post('*/api/v2/11/external_sales_invoices/931/payments', async ({ request }) => {
        paid = await request.json()
        current = external({ state: 'late', total_unpaid: '100.0', total_paid: '21.0', payments: [
          { id: '981', administration_id: '11', invoice_type: 'ExternalSalesInvoice', invoice_id: '931', price: '21.0', price_base: '21.0', payment_date: '2026-11-05', manual_payment_action: 'cash_payment', created_at: stamp, updated_at: stamp },
        ] })
        return HttpResponse.json(current.payments?.[0], { status: 201 })
      }),
      http.patch('*/api/v2/11/external_sales_invoices/931/mark_as_dubious', async ({ request }) => {
        dubious = await request.json()
        current = { ...current, marked_dubious_on: '2026-11-08' }
        return HttpResponse.json(current)
      }),
    )
    renderAt('/11/external_sales_invoices/931')
    const block = await screen.findByRole('region', { name: 'Payments' })
    const form = within(block).getByRole('form', { name: 'Register payment' })
    await userEvent.clear(within(form).getByLabelText('Amount'))
    await userEvent.type(within(form).getByLabelText('Amount'), '21')
    await userEvent.clear(within(form).getByLabelText('Payment date'))
    await userEvent.type(within(form).getByLabelText('Payment date'), '2026-11-05')
    await userEvent.selectOptions(within(form).getByLabelText('Paid by'), 'cash_payment')
    await userEvent.click(within(form).getByRole('button', { name: 'Register payment' }))
    await waitFor(() => expect(paid).toEqual({ payment: { payment_date: '2026-11-05', price: '21', manual_payment_action: 'cash_payment' } }))
    expect(await within(block).findByRole('row', { name: /Cash/ })).toHaveTextContent('€21.00')
    // With a payment it can no longer change.
    expect(screen.queryByRole('form', { name: 'External invoice' })).toBeNull()

    await chooseMore('Mark as dubious')
    const panel = screen.getByRole('region', { name: 'Mark as dubious' })
    await userEvent.clear(within(panel).getByLabelText('Date'))
    await userEvent.type(within(panel).getByLabelText('Date'), '2026-11-08')
    await userEvent.click(within(panel).getByRole('button', { name: 'Mark as dubious' }))
    await waitFor(() => expect(dubious).toEqual({ dubious_date: '2026-11-08' }))
    expect(await screen.findByText('This invoice is marked as dubious.')).toBeInTheDocument()
  })

  it('creates an invoice typed in by hand', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/external_sales_invoices', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(external(), { status: 201 })
    }))
    renderAt('/11/external_sales_invoices/new')
    const form = await screen.findByRole('form', { name: 'External invoice' })
    await userEvent.type(within(form).getByLabelText('Reference'), 'WEB-1001')
    await userEvent.click(within(form).getByLabelText('Prices include VAT'))
    await userEvent.type(within(form).getByLabelText('Price'), '121')
    await userEvent.selectOptions(within(form).getByLabelText('Category'), '8001')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toMatchObject({
      external_sales_invoice: { reference: 'WEB-1001', prices_are_incl_tax: true, details_attributes: [{ price: '121', tax_rate_id: '2101', ledger_account_id: '8001' }] },
    }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/external_sales_invoices/931'))
  })

  it('deletes an invoice and returns to the list', async () => {
    let deleted = false
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/external_sales_invoices/931', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/external_sales_invoices/931')
    await chooseMore('Delete')
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'Delete this external invoice? This cannot be undone.' })).getByRole('button', { name: 'Delete permanently' }))
    await waitFor(() => expect(deleted).toBe(true))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/external_sales_invoices$/))
  })
})
