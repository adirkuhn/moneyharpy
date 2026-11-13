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
    http.get('*/api/v2/11/users', () => HttpResponse.json([])),
  ]
}

describe('the external invoice list', () => {
  it('lists invoices with their date, contact, reference, amount and status', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/external_sales_invoices')
    const table = await screen.findByRole('table', { name: 'External invoices' })
    // As at Moneybird the reference follows the software the invoice comes from.
    expect(within(table).getByRole('link', { name: 'Example webshop › WEB-1001' })).toHaveAttribute('href', '/11/external_sales_invoices/931')
    expect(within(table).getByRole('link', { name: 'External invoice 2' })).toHaveAttribute('href', '/11/external_sales_invoices/932')
    expect(within(table).getByText('Example Shop B.V.')).toBeInTheDocument()
    expect(within(table).getByText('€121.00')).toBeInTheDocument()
    expect(within(table).getByText('New')).toHaveClass('badge')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('period:this_year')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'External invoices' })).toHaveAttribute('aria-current', 'page')
    expect(within(table).getByRole('img', { name: 'Has attachments' })).toBeInTheDocument()
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['', 'Reference', '', 'Contact', 'Date', 'Amount', 'Status'])
    expect(within(table).getByRole('checkbox', { name: 'Select all external invoices' })).toBeInTheDocument()

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

describe('the selection of external invoices', () => {
  it('deletes the ticked invoices', async () => {
    const deleted: string[] = []
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/external_sales_invoices/:id', ({ params }) => {
      deleted.push(String(params.id))
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/external_sales_invoices')
    const table = await screen.findByRole('table', { name: 'External invoices' })
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select external invoice Example webshop › WEB-1001' }))
    const bar = screen.getByRole('group', { name: 'Selected external invoices' })
    expect(bar).toHaveTextContent('1 external invoice selected')
    await userEvent.click(within(bar).getByRole('button', { name: 'Delete (1)' }))
    await userEvent.click(within(bar).getByRole('button', { name: 'Delete 1 external invoice permanently' }))
    await waitFor(() => expect(deleted).toEqual(['931']))
  })
})

const created = { administration_id: '11', user_id: '1', action: 'external_sales_invoice_created', link_entity_id: null, link_entity_type: null, data: {}, created_at: stamp, updated_at: stamp }

describe('an external invoice', () => {
  it('reads as Moneybird’s document, with the action bar and the sidebar', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/external_sales_invoices/931', () => HttpResponse.json(external({ events: [created] }))))
    renderAt('/11/external_sales_invoices/931')
    const document = await screen.findByRole('article', { name: 'External invoice' })
    expect(within(document).getByRole('heading', { level: 1, name: 'WEB-1001' })).toBeInTheDocument()
    const facts = within(document).getByRole('list', { name: 'Invoice facts' })
    expect(within(facts).getByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/contacts/701')
    expect(facts).toHaveTextContent('Invoice date2026-11-01')
    expect(facts).toHaveTextContent('Due date2026-11-15')
    expect(within(facts).getByRole('link', { name: 'Example webshop' })).toHaveAttribute('href', 'https://shop.example.com/orders/1001')
    const lines = within(document).getByRole('table', { name: 'Invoice lines' })
    expect(within(lines).getAllByRole('row').map((r) => r.textContent)).toEqual([
      'DescriptionAmountTotalVAT',
      '1Sofa€100.00€100.0021%',
      'Subtotal€100.00',
      '21% btw€21.00',
      'Total€121.00',
    ])

    expect(screen.getByRole('link', { name: 'Register payment' })).toHaveAttribute('href', '/11/external_sales_invoices/931/payments/new')
    expect(screen.getByRole('link', { name: 'Edit' })).toHaveAttribute('href', '/11/external_sales_invoices/931/edit')
    expect(screen.getByRole('button', { name: 'Journal entries' })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: 'Delete external invoice' })).toBeInTheDocument()
    expect(screen.getByText('Open')).toHaveClass('badge')

    const sidebar = screen.getByRole('complementary', { name: 'About this invoice' })
    expect(await within(within(sidebar).getByRole('list', { name: 'Category' })).findByText('Sales')).toBeInTheDocument()
    expect(within(sidebar).getByRole('heading', { name: 'Notes and tasks' })).toBeInTheDocument()
    expect(within(sidebar).getByText('External invoice created')).toBeInTheDocument()
  })

  it('shows its journal entries as Moneybird heads them', async () => {
    const seen: string[] = []
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/reports/journal_entries', ({ request }) => {
      const url = new URL(request.url)
      seen.push(`${url.searchParams.get('document_type')}/${url.searchParams.get('document_id')}`)
      return HttpResponse.json([
        { id: '1', date: '2026-11-01', document_type: 'ExternalSalesInvoice', document_id: '931', ledger_account_id: '8001', amount: '-100.0', tax_rate_id: '2101', contact_id: '701', project_id: null, financial_account_id: null, description: null },
      ])
    }))
    renderAt('/11/external_sales_invoices/931')
    const button = await screen.findByRole('button', { name: 'Journal entries' })
    await userEvent.click(button)
    expect(button).toHaveAttribute('aria-pressed', 'true')
    const table = await screen.findByRole('table', { name: 'Journal entries' })
    await waitFor(() => expect(within(table).getAllByRole('row').map((r) => r.textContent)).toEqual([
      'CategoryDebitCreditVAT', 'External invoice - 2026-11-01', 'Sales (Profit and loss statement)100.0021% btw',
    ]))
    expect(seen).toEqual(['ExternalSalesInvoice/931'])
  })

  it('shows its files under the document', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/external_sales_invoices/931', () => HttpResponse.json(external({ attachments: fresh.attachments }))))
    renderAt('/11/external_sales_invoices/931')
    const files = await screen.findByRole('region', { name: 'Attachments' })
    expect(within(files).getByRole('link', { name: 'order.pdf' })).toHaveAttribute('href', '/api/v2/11/external_sales_invoices/931/attachments/971/download')
  })

  it('still offers Register payment once paid, as Moneybird does', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/external_sales_invoices/931', () => HttpResponse.json(external({ state: 'paid', total_paid: '121.0', total_unpaid: '0.0', paid_at: '2026-11-05' }))))
    renderAt('/11/external_sales_invoices/931')
    expect(await screen.findByRole('link', { name: 'Register payment' })).toHaveAttribute('href', '/11/external_sales_invoices/931/payments/new')
  })

  it('can be marked dubious while it is unpaid', async () => {
    let dubious: unknown
    let current = external({ state: 'late' })
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/external_sales_invoices/931', () => HttpResponse.json(current)),
      http.patch('*/api/v2/11/external_sales_invoices/931/mark_as_dubious', async ({ request }) => {
        dubious = await request.json()
        current = { ...current, marked_dubious_on: '2026-11-08' }
        return HttpResponse.json(current)
      }),
    )
    renderAt('/11/external_sales_invoices/931')
    await chooseMore('Mark as dubious')
    const panel = screen.getByRole('region', { name: 'Mark as dubious' })
    await userEvent.clear(within(panel).getByLabelText('Date'))
    await userEvent.type(within(panel).getByLabelText('Date'), '2026-11-08')
    await userEvent.click(within(panel).getByRole('button', { name: 'Mark as dubious' }))
    await waitFor(() => expect(dubious).toEqual({ dubious_date: '2026-11-08' }))
    expect(await screen.findByText('This invoice is marked as dubious.')).toBeInTheDocument()
  })

  it('registers a payment through Moneybird’s pages', async () => {
    let paid: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/external_sales_invoices/931/payments', async ({ request }) => {
      paid = await request.json()
      return HttpResponse.json({ id: '981' }, { status: 201 })
    }))
    renderAt('/11/external_sales_invoices/931')
    await userEvent.click(await screen.findByRole('link', { name: 'Register payment' }))
    expect(await screen.findByText(/external invoice WEB-1001/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('link', { name: 'Business cash payment' }))
    const form = await screen.findByRole('form', { name: 'Register payment' })
    await userEvent.clear(within(form).getByLabelText('Amount'))
    await userEvent.type(within(form).getByLabelText('Amount'), '21')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(paid).toMatchObject({ payment: { price: '21', manual_payment_action: 'cash_payment' } }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/external_sales_invoices\/931$/))
  })

  it('deletes after asking and returns to the list', async () => {
    let deleted = false
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/external_sales_invoices/931', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/external_sales_invoices/931')
    await userEvent.click(await screen.findByRole('button', { name: 'Delete external invoice' }))
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'Delete this external invoice? This cannot be undone.' })).getByRole('button', { name: 'Delete permanently' }))
    await waitFor(() => expect(deleted).toBe(true))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/external_sales_invoices$/))
  })
})

describe('editing an external invoice', () => {
  it('changes its details on Moneybird’s edit page', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/external_sales_invoices/931', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(external({ reference: 'WEB-1009' }))
    }))
    renderAt('/11/external_sales_invoices/931/edit')
    expect(await screen.findByRole('heading', { level: 1, name: 'Edit external invoice' })).toBeInTheDocument()
    const form = screen.getByRole('form', { name: 'Edit external invoice' })
    expect(within(form).getByRole('group', { name: 'Contact' })).toHaveTextContent('Example Shop B.V.')
    const details = within(form).getByRole('group', { name: 'Add details' })
    expect(within(details).getByLabelText('Invoice id')).toHaveValue('WEB-1001')
    expect(within(details).getByLabelText('Date', { selector: 'input[type="date"]' })).toHaveValue('2026-11-01')
    expect(within(details).getByLabelText('Source (optional)')).toHaveValue('Example webshop')
    expect(within(details).getByLabelText('Source URL (optional)')).toHaveValue('https://shop.example.com/orders/1001')
    await userEvent.clear(within(details).getByLabelText('Invoice id'))
    await userEvent.type(within(details).getByLabelText('Invoice id'), 'WEB-1009')
    // The due date can be given as a number of days after the date.
    await userEvent.click(within(details).getByRole('radio', { name: 'Days' }))
    await userEvent.type(within(details).getByLabelText('Days until due'), '30')
    await userEvent.click(within(form).getByRole('radio', { name: 'Including VAT' }))
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({
      external_sales_invoice: {
        contact_id: '701', reference: 'WEB-1009', date: '2026-11-01', due_date: '2026-12-01', source: 'Example webshop', source_url: 'https://shop.example.com/orders/1001',
        currency: 'EUR', prices_are_incl_tax: true,
        details_attributes: [{ id: '961', description: 'Sofa', amount: '1', price: '100', tax_rate_id: '2101', ledger_account_id: '8001', row_order: 1 }],
      },
    }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/external_sales_invoices\/931$/))
  })

  it('fills in an uploaded invoice beside its file', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/external_sales_invoices/932', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(external({ id: '932', reference: 'WEB-1002', uri: '/11/external_sales_invoices/932' }))
    }))
    renderAt('/11/external_sales_invoices/932')
    expect(await screen.findByRole('heading', { level: 1, name: 'Edit external invoice' })).toBeInTheDocument()
    const files = screen.getByRole('region', { name: 'Attachments' })
    expect(within(files).getByRole('link', { name: 'order.pdf' })).toHaveAttribute('href', '/api/v2/11/external_sales_invoices/932/attachments/971/download')
    expect(files).toHaveTextContent('(Maximum 10 MB per file and 15 MB total)')
    const form = screen.getByRole('form', { name: 'Edit external invoice' })
    await userEvent.click(within(form).getByRole('combobox', { name: 'Contact' }))
    await userEvent.type(screen.getByRole('searchbox', { name: 'Find a contact' }), 'shop')
    await userEvent.click(await screen.findByRole('option', { name: 'Example Shop B.V.' }))
    expect(within(form).getByRole('group', { name: 'Contact' })).toHaveTextContent('Example Shop B.V.')
    await userEvent.type(within(form).getByLabelText('Invoice id'), 'WEB-1002')
    await userEvent.type(within(form).getByLabelText('Description'), 'Chair')
    await userEvent.type(within(form).getByLabelText('Price'), '50')
    expect(within(form).getByText('€60.50')).toBeInTheDocument()
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toMatchObject({
      external_sales_invoice: {
        contact_id: '701', reference: 'WEB-1002', date: '2026-11-11', due_date: '', prices_are_incl_tax: false,
        details_attributes: [{ description: 'Chair', amount: '1', price: '50', tax_rate_id: '2101' }],
      },
    }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/external_sales_invoices\/932$/))
  })

  it('creates an invoice typed in by hand', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/external_sales_invoices', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(external(), { status: 201 })
    }))
    renderAt('/11/external_sales_invoices/new')
    expect(await screen.findByRole('heading', { level: 1, name: 'New external invoice' })).toBeInTheDocument()
    const form = screen.getByRole('form', { name: 'New external invoice' })
    expect(screen.queryByRole('button', { name: 'Delete external invoice' })).toBeNull()
    await userEvent.type(within(form).getByLabelText('Invoice id'), 'WEB-1001')
    await userEvent.click(within(form).getByRole('radio', { name: 'Including VAT' }))
    await userEvent.type(within(form).getByLabelText('Price'), '121')
    await userEvent.selectOptions(within(form).getByLabelText('Category'), '8001')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toMatchObject({
      external_sales_invoice: { reference: 'WEB-1001', prices_are_incl_tax: true, details_attributes: [{ price: '121', tax_rate_id: '2101', ledger_account_id: '8001' }] },
    }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/external_sales_invoices/931'))
  })
})
