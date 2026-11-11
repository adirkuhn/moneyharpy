import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import type { Contact } from '../contacts/queries'
import type { SalesInvoice } from '../invoices/queries'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, documentStyle, listing, server, signedIn, workflow } from '../test/server'
import type { RecurringInvoice } from './queries'

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

const recurring = (over: Partial<RecurringInvoice> = {}): RecurringInvoice => ({
  id: '941', administration_id: '11', contact_id: '701', contact: shop, contact_person_id: null, workflow_id: '601', document_style_id: '701',
  identity_id: '461', start_date: '2026-11-01', invoice_date: '2026-12-01', last_date: null, active: true, payment_conditions: null,
  reference: 'Contract 7', language: 'en', currency: 'EUR', discount: '0.0', first_due_interval: 14, auto_send: false,
  sending_scheduled_at: null, sending_scheduled_user_id: null, mergeable: false, frequency_type: 'month', frequency: 1, invoices_created: 1,
  created_at: stamp, updated_at: stamp, version: 1, prices_are_incl_tax: false,
  total_price_excl_tax: '100.0', total_price_excl_tax_base: '100.0', total_price_incl_tax: '121.0', total_price_incl_tax_base: '121.0', total_tax: '21.0',
  details: [{ id: '961', description: 'Hosting {period}', amount: '1', price: '100.0', tax_rate_id: '2101', ledger_account_id: '8001', total_price_excl_tax_with_discount: '100.0' }],
  custom_fields: [], notes: [], attachments: [], events: [], subscription: null, uri: '/11/recurring_sales_invoices/941',
  ...over,
})

const yearly = recurring({
  id: '942', reference: 'Licence', frequency_type: 'year', frequency: 2, auto_send: true, invoice_date: '2027-01-15', invoices_created: 0,
  total_price_incl_tax: '60.5', uri: '/11/recurring_sales_invoices/942',
})

const made: SalesInvoice = {
  id: '901', administration_id: '11', contact_id: '701', contact: shop, invoice_id: null, draft_id: 4, state: 'draft', invoice_date: '2026-11-01',
  due_date: '2026-11-15', reference: 'Contract 7', currency: 'EUR', prices_are_incl_tax: false, discount: null, workflow_id: '601',
  document_style_id: '701', identity_id: '461', payment_conditions: '', first_due_interval: 14, paused: false, original_sales_invoice_id: null,
  recurring_sales_invoice_id: '941',
  details: [{ id: '951', description: 'Hosting 2026-11-01 – 2026-11-30', amount: '1', price: '100.0', tax_rate_id: '2101', ledger_account_id: '8001', total_price_excl_tax_with_discount: '100.0' }],
  tax_totals: [], payments: [], notes: [], attachments: [], events: [],
  total_price_excl_tax: '100.0', total_tax: '21.0', total_price_incl_tax: '121.0', total_unpaid: '121.0', total_discount: '0.0',
  version: 1, uri: '/11/sales_invoices/901',
}

function loaded(list: RecurringInvoice[] = [recurring(), yearly], seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/recurring_sales_invoices', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/recurring_sales_invoices/941', () => HttpResponse.json(recurring())),
    http.get('*/api/v2/11/sales_invoices', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json([made], { headers: { 'X-Total-Count': '1' } })
    }),
    http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(made)),
    http.get('*/api/v2/11/tax_rates', () => HttpResponse.json([vat21])),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales])),
    http.get('*/api/v2/11/workflows', () => HttpResponse.json([workflow()])),
    http.get('*/api/v2/11/document_styles', () => HttpResponse.json([documentStyle({ id: '701' })])),
    http.get('*/api/v2/11/contacts', () => HttpResponse.json([shop as unknown as Contact], { headers: { 'X-Total-Count': '1' } })),
  ]
}

describe('the recurring invoice list', () => {
  it('lists recurring invoices with their contact, frequency, next date, auto send and total', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/recurring_sales_invoices')
    const table = await screen.findByRole('table', { name: 'Recurring invoices' })
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('Example Shop B.V.')
    expect(rows[1]).toHaveTextContent('Every month')
    expect(rows[1]).toHaveTextContent('Dec 1, 2026')
    expect(rows[1]).toHaveTextContent('€121.00')
    expect(rows[1]).toHaveTextContent('Contract 7')
    expect(rows[1]).toHaveTextContent('Active')
    expect(within(rows[1] as HTMLElement).queryByRole('img', { name: 'Sent automatically' })).toBeNull()
    expect(rows[2]).toHaveTextContent('Every 2 years')
    expect(within(rows[2] as HTMLElement).getByRole('img', { name: 'Sent automatically' })).toBeInTheDocument()
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/recurring_sales_invoices/941')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:active')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Recurring invoices' })).toHaveAttribute('aria-current', 'page')

    const toggle = screen.getByRole('button', { name: 'Active recurring invoices' })
    await userEvent.click(toggle)
    const panel = screen.getByRole('search', { name: 'Recurring invoice filters' })
    expect(within(panel).getByRole('link', { name: 'Show all active recurring invoices' })).toHaveAttribute('href', '/11/recurring_sales_invoices')
    await userEvent.click(within(panel).getByRole('radio', { name: 'Inactive' }))
    await userEvent.click(within(panel).getByRole('radio', { name: 'Annually' }))
    await userEvent.click(within(within(panel).getByRole('group', { name: 'Send automatically' })).getByRole('radio', { name: 'Yes' }))
    await userEvent.selectOptions(await within(panel).findByRole('combobox', { name: 'Workflow' }), await within(panel).findByRole('option', { name: 'Standaard (NL)' }))
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:inactive,frequency:year,auto_send:true,workflow_id:601'))
    expect(toggle).toHaveTextContent('Filtered recurring invoices')

    await userEvent.click(toggle)
    await userEvent.click(within(screen.getByRole('search', { name: 'Recurring invoice filters' })).getByRole('link', { name: 'Show all active recurring invoices' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:active'))
    expect(toggle).toHaveTextContent('Active recurring invoices')
  })

  it('says when there are none yet', async () => {
    server.use(...loaded([]))
    renderAt('/11/recurring_sales_invoices')
    expect(await screen.findByText(/No recurring invoices yet/)).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Send periodic invoices' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add recurring invoice' })).toHaveAttribute('href', '/11/recurring_sales_invoices/new')
    expect(screen.getByRole('button', { name: 'Export' })).toHaveAttribute('aria-disabled', 'true')
  })
})

describe('a recurring invoice', () => {
  it('shows its schedule and the invoices it made, and changes', async () => {
    const seen: URL[] = []
    let sent: unknown
    server.use(...loaded(undefined, seen))
    server.use(http.patch('*/api/v2/11/recurring_sales_invoices/941', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(recurring({ frequency_type: 'quarter', updated_at: '2026-11-11T09:00:00Z' }))
    }))
    renderAt('/11/recurring_sales_invoices/941')
    expect(await screen.findByRole('heading', { name: 'Example Shop B.V.' })).toBeInTheDocument()
    expect(screen.getByText('Next invoice on', { selector: 'dt' }).nextSibling).toHaveTextContent('Dec 1, 2026')
    const madeTable = await screen.findByRole('table', { name: 'Invoices made' })
    expect(within(madeTable).getByRole('link', { name: 'Draft 4' })).toHaveAttribute('href', '/11/sales_invoices/901')
    expect(seen.some((u) => u.pathname.endsWith('/sales_invoices') && u.searchParams.get('filter') === 'recurring_sales_invoice_id:941')).toBe(true)

    const form = screen.getByRole('form', { name: 'Recurring invoice' })
    await userEvent.selectOptions(within(form).getByLabelText('Unit'), 'quarter')
    await userEvent.type(within(form).getByLabelText('Last invoice on or before'), '2027-12-31')
    await userEvent.click(within(form).getByLabelText('Send each invoice automatically'))
    await userEvent.click(within(form).getByRole('button', { name: 'Save recurring invoice' }))
    await waitFor(() => expect(sent).toEqual({
      recurring_sales_invoice: {
        contact_id: '701', reference: 'Contract 7', frequency: 1, frequency_type: 'quarter', last_date: '2027-12-31', auto_send: true,
        prices_are_incl_tax: false, discount: '', payment_conditions: '', first_due_interval: 14, workflow_id: '601', document_style_id: '701',
        details_attributes: [{ id: '961', description: 'Hosting {period}', amount: '1', price: '100', tax_rate_id: '2101', ledger_account_id: '8001' }],
      },
    }))
    expect(await screen.findByText('Every quarter', { selector: 'dd' })).toBeInTheDocument()
  })

  it('deactivates, after which it no longer changes', async () => {
    let current = recurring()
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/recurring_sales_invoices/941', () => HttpResponse.json(current)),
      http.patch('*/api/v2/11/recurring_sales_invoices/941/deactivate', () => {
        current = recurring({ active: false, updated_at: '2026-11-11T09:00:00Z' })
        return HttpResponse.json(current)
      }),
    )
    renderAt('/11/recurring_sales_invoices/941')
    await chooseMore('Deactivate')
    expect(await screen.findByText('This recurring invoice is deactivated and makes no more invoices.')).toBeInTheDocument()
    // It made invoices, so it can no longer be deleted either: nothing is left to do with it.
    expect(screen.queryByRole('button', { name: 'More actions' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Save recurring invoice' })).toBeNull()
    expect(within(screen.getByRole('form', { name: 'Recurring invoice' })).getByLabelText('Unit')).toBeDisabled()
  })

  it('is made from a draft invoice', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/recurring_sales_invoices', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(recurring(), { status: 201 })
    }))
    renderAt('/11/sales_invoices/901')
    await chooseMore('Make recurring')
    const form = await screen.findByRole('form', { name: 'Recurring invoice' })
    expect(screen.getByRole('heading', { name: 'New recurring invoice' })).toBeInTheDocument()
    expect(within(form).getByLabelText('Reference')).toHaveValue('Contract 7')
    expect(within(form).getByLabelText('Description')).toHaveValue('Hosting 2026-11-01 – 2026-11-30')
    await userEvent.clear(within(form).getByLabelText('Description'))
    await userEvent.type(within(form).getByLabelText('Description'), 'Hosting {{period}')
    await userEvent.click(within(form).getByRole('button', { name: 'Save recurring invoice' }))
    await waitFor(() => expect(sent).toMatchObject({
      recurring_sales_invoice: {
        contact_id: '701', reference: 'Contract 7', frequency: 1, frequency_type: 'month', auto_send: false, last_date: '',
        details_attributes: [{ description: 'Hosting {period}', amount: '1', price: '100', tax_rate_id: '2101', ledger_account_id: '8001' }],
      },
    }))
    expect((sent as { recurring_sales_invoice: { invoice_date?: string } }).recurring_sales_invoice.invoice_date).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/recurring_sales_invoices/941'))
  })
})
