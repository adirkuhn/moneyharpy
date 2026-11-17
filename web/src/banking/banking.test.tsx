import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { LedgerAccount } from '../bookkeeping/queries'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { BookingRule, FinancialAccount, LinkSuggestion, Mutation, PurchaseTransaction } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const account = (over: Partial<FinancialAccount> = {}): FinancialAccount => ({
  id: '501', administration_id: '11', type: 'BankAccount', name: 'Main bank', identifier: 'NL91ABNA0417164300', currency: 'EUR', provider: null,
  moneybird_account: false, active: true, created_at: stamp, updated_at: stamp, uri: '/11/financial_accounts/501', position: 1, ledger_account_id: '1101', last_import_at: null, ...over,
})
const cash = account({ id: '502', type: 'CashAccount', name: 'Cash', identifier: '', position: 2, uri: '/11/financial_accounts/502' })
const old = account({ id: '503', name: 'Old bank', identifier: '', active: false, position: 3, uri: '/11/financial_accounts/503' })

const mutation = (over: Partial<Mutation> = {}): Mutation => ({
  id: '701', administration_id: '11', financial_account_id: '501', financial_statement_id: '601', date: '2026-11-01', valutation_date: null,
  message: 'Monthly hosting plan', amount: '-25.5', amount_open: '-25.5', currency: 'EUR', contra_account_name: 'Hosting Company', contra_account_number: 'NL44RABO0123456789',
  state: 'unprocessed', settlement_state: 'settled', payments: [], ledger_account_bookings: [], version: 1, created_at: stamp, updated_at: stamp, uri: '/11/financial_mutations/701', ...over,
})

const software: LedgerAccount = {
  id: '1201', administration_id: '11', name: 'Software', account_type: 'expenses', account_id: '4430', parent_id: null, active: true,
  allowed_document_types: ['purchase_invoice', 'financial_mutation'], financial_account_id: null, created_at: stamp, updated_at: stamp,
  taxonomy_item: null, uri: '/11/ledger_accounts/1201', description: null, system_role: null, template: null,
}
const salesOnly: LedgerAccount = { ...software, id: '1202', name: 'Invoices only', allowed_document_types: ['sales_invoice'], uri: '/11/ledger_accounts/1202' }

const rule: BookingRule = {
  id: '801', administration_id: '11', name: 'Hosting', active: true, position: 1, match_all: true, direction: 'debit', financial_account_id: null,
  conditions: [{ field: 'contra_account_name', operator: 'contains', value: 'hosting' }], ledger_account_id: '1201', description: '', uri: '/11/booking_rules/801', created_at: stamp, updated_at: stamp,
}

const loaded = () => [
  signedIn(),
  listing([harpy]),
  http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
  http.get('*/api/v2/11/bank_settings', () => HttpResponse.json({ automatic_booking: true })),
  http.get('*/api/v2/11/financial_accounts', () => HttpResponse.json([account(), cash, old])),
  http.get('*/api/v2/11/booking_rules', () => HttpResponse.json([rule])),
  http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([software, salesOnly])),
  http.get('*/api/v2/11/financial_mutations', () => HttpResponse.json([mutation(), mutation({ id: '702', amount: '1000.0', amount_open: '0.0', state: 'processed', contra_account_name: null, message: 'Invoice 1', uri: '/11/financial_mutations/702' })])),
  http.get('*/api/v2/11/financial_mutations/701', () => HttpResponse.json(mutation())),
  http.get('*/api/v2/11/sales_invoices', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
]

describe('bank settings', () => {
  it('shows every part and the bank menu', async () => {
    server.use(...loaded())
    renderAt('/11/settings/bank')
    expect(await screen.findByRole('heading', { name: 'Bank', level: 1 })).toBeInTheDocument()
    for (const name of ['Accounts', 'Bank connections', 'Automatic booking', 'Booking rules', 'Payment cards', 'Smart transfers']) {
      expect(screen.getByRole('region', { name })).toBeInTheDocument()
    }
    const nav = screen.getByRole('navigation', { name: 'Settings' })
    expect(within(nav).getByRole('link', { name: 'Booking rules' })).toHaveAttribute('href', '/11/settings/bank#booking-rules')
    expect(screen.getByText(/Direct connections to banks come later/)).toBeInTheDocument()
  })

  it('lists accounts, hides deactivated ones and reorders', async () => {
    let order: unknown
    server.use(...loaded(), http.put('*/api/v2/11/financial_accounts/order', async ({ request }) => {
      order = await request.json()
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/settings/bank')
    const table = await screen.findByRole('table', { name: 'Financial accounts' })
    expect(await within(table).findByRole('link', { name: 'Main bank' })).toHaveAttribute('href', '/11/financial_accounts/501')
    expect(within(table).queryByRole('link', { name: 'Old bank' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Show 1 deactivated account' }))
    expect(within(table).getByRole('button', { name: 'Reactivate Old bank' })).toBeInTheDocument()
    expect(within(table).getByRole('button', { name: 'Move Main bank up' })).toBeDisabled()
    await userEvent.click(within(table).getByRole('button', { name: 'Move Main bank down' }))
    await waitFor(() => expect(order).toEqual({ ids: ['502', '501', '503'] }))
  })

  it('turns automatic booking off and lists rules', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/bank_settings', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ automatic_booking: false })
    }))
    renderAt('/11/settings/bank')
    await userEvent.click(await screen.findByRole('switch', { name: 'Book automatically' }))
    await waitFor(() => expect(sent).toEqual({ bank_settings: { automatic_booking: false } }))
    const rules = screen.getByRole('table', { name: 'Booking rules' })
    expect(await within(rules).findByText('Name contains "hosting"')).toBeInTheDocument()
  })
})

describe('financial account', () => {
  it('adds an account', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/11/financial_accounts', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(account({ id: '504' }), { status: 201 })
    }))
    renderAt('/11/financial_accounts/new')
    await userEvent.type(await screen.findByLabelText('Name'), 'Savings')
    await userEvent.type(screen.getByLabelText('Account number'), 'NL02ABNA0123456789')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ financial_account: { name: 'Savings', type: 'BankAccount', identifier: 'NL02ABNA0123456789', currency: 'EUR', active: true } }))
  })

  it('imports a statement file and reports what happened', async () => {
    let body = ''
    let type = ''
    server.use(...loaded(), http.post('*/api/v2/11/financial_accounts/501/import', async ({ request }) => {
      body = await request.text()
      type = request.headers.get('Content-Type') ?? ''
      return HttpResponse.json({ format: 'camt', imported: 2, duplicates: 1, locked: 0, auto_booked: 1, financial_statements: [] }, { status: 201 })
    }))
    renderAt('/11/financial_accounts/501')
    const input = await screen.findByLabelText('Choose file')
    await userEvent.upload(input, new File(['<Document/>'], 'statement.xml', { type: 'text/xml' }))
    expect(await screen.findByRole('status')).toHaveTextContent('2 new transactions. 1 you had already. 1 booked by a rule.')
    expect(body).toBe('<Document/>')
    expect(type).toBe('application/octet-stream')
  })

  it('adds a transaction by hand as money out', async () => {
    let sent: { financial_statement?: { manual?: boolean; financial_mutations_attributes?: { amount?: string }[] } } = {}
    server.use(...loaded(), http.post('*/api/v2/11/financial_statements', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json({}, { status: 201 })
    }))
    renderAt('/11/financial_accounts/502')
    const form = await screen.findByRole('form', { name: 'Add a transaction' })
    await userEvent.type(within(form).getByLabelText('Amount'), '3.50')
    await userEvent.type(within(form).getByLabelText('Description'), 'Coffee')
    await userEvent.click(within(form).getByRole('button', { name: 'Add transaction' }))
    await waitFor(() => expect(sent.financial_statement?.manual).toBe(true))
    expect(sent.financial_statement?.financial_mutations_attributes?.[0]?.amount).toBe('-3.50')
  })
})

describe('booking rule form', () => {
  it('creates a rule with conditions and a category', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/11/booking_rules', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(rule, { status: 201 })
    }))
    renderAt('/11/booking_rules/new')
    await userEvent.type(await screen.findByLabelText('Name'), 'Rent')
    await userEvent.type(screen.getByLabelText('Value'), 'landlord')
    await userEvent.click(screen.getByRole('button', { name: 'Add condition' }))
    const second = screen.getByRole('group', { name: 'Condition 2' })
    await userEvent.selectOptions(within(second).getByLabelText('Field'), 'amount')
    expect(within(second).getByLabelText('Comparison')).toHaveValue('equals')
    await userEvent.type(within(second).getByLabelText('Value'), '1200')
    const category = screen.getByLabelText('Category')
    expect(within(category).queryByRole('option', { name: /Invoices only/ })).not.toBeInTheDocument()
    await userEvent.selectOptions(category, '1201')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(sent).toEqual({
        booking_rule: {
          name: 'Rent', active: true, match_all: true, direction: 'any', financial_account_id: '', ledger_account_id: '1201', description: '',
          conditions: [{ field: 'contra_account_name', operator: 'contains', value: 'landlord' }, { field: 'amount', operator: 'equals', value: '1200' }],
        },
      }),
    )
  })
})

describe('transactions', () => {
  it('shows every transaction of this year as a card, filtered by the address', async () => {
    let filter = ''
    server.use(...loaded())
    const paid = mutation({
      id: '702', amount: '39.0', amount_open: '0.0', state: 'processed', contra_account_name: null, message: 'Order 12', uri: '/11/financial_mutations/702',
      links: [{ payment_id: '951', document_type: 'SalesInvoice', document_id: '901', label: 'INV', reference: '2026-0001', contact_name: 'Example Shop B.V.', date: '2026-10-30', total: '39.0', currency: 'EUR', state: 'paid', payment_reference: 'RF18 0000 0000 0000 01', details: '2 Coffee', price: '39.0' }],
    })
    server.use(http.get('*/api/v2/11/financial_mutations', ({ request }) => {
      filter = new URL(request.url).searchParams.get('filter') ?? ''
      return HttpResponse.json([mutation(), paid])
    }))
    renderAt('/11/financial_mutations')
    const feed = await screen.findByRole('list', { name: 'Transactions' })
    expect(filter).toBe('period:this_year,state:all')
    const open = within(feed).getByRole('article', { name: 'Hosting Company' })
    expect(open).toHaveTextContent('2026-11-01 - NL44 RABO 0123 4567 89')
    expect(open).toHaveTextContent('Payment reference: Monthly hosting plan')
    expect(open).toHaveTextContent('€-25.50')
    expect(within(open).getByRole('img', { name: 'Unprocessed' })).toBeInTheDocument()
    expect(within(open).getByRole('button', { name: 'Link open amount' })).toBeInTheDocument()
    // A processed one shows what it pays, as Moneybird's card does.
    const done = within(feed).getByRole('article', { name: 'Payment reference: Order 12' })
    expect(within(done).getByRole('img', { name: 'Processed' })).toBeInTheDocument()
    const linked = within(done).getByRole('list', { name: 'Linked to' })
    expect(within(linked).getByRole('link', { name: 'INV 2026-0001 · Example Shop B.V.' })).toHaveAttribute('href', '/11/sales_invoices/901')
    expect(linked).toHaveTextContent('2026-10-30 · €39.00')
    expect(linked).toHaveTextContent('2 Coffee')
    expect(within(done).queryByRole('button', { name: 'Link open amount' })).toBeNull()

    await userEvent.click(screen.getByRole('button', { name: 'Transactions from this year' }))
    const panel = screen.getByRole('search', { name: 'Filter transactions' })
    await userEvent.selectOptions(within(panel).getByLabelText('Account'), '501')
    await userEvent.click(within(panel).getByRole('radio', { name: 'Unprocessed' }))
    await userEvent.click(within(panel).getByRole('radio', { name: 'Debit' }))
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(filter).toBe('period:this_year,state:unprocessed,mutation_type:debit,financial_account_id:501'))
    expect(screen.getByRole('button', { name: 'Transactions of Main bank' })).toBeInTheDocument()
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Transactions' })).toHaveAttribute('aria-current', 'page')
  })

  it('offers quick filters and a period over months, quarters and years', async () => {
    let filter = ''
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/financial_mutations', ({ request }) => {
      filter = new URL(request.url).searchParams.get('filter') ?? ''
      return HttpResponse.json([mutation()])
    }))
    renderAt('/11/financial_mutations')
    await screen.findByRole('list', { name: 'Transactions' })
    await userEvent.click(screen.getByRole('button', { name: 'Transactions from this year' }))
    const panel = screen.getByRole('search', { name: 'Filter transactions' })
    expect(within(panel).getByRole('link', { name: 'Processed transactions' })).toHaveAttribute('href', '/11/financial_mutations?state=processed')
    expect(within(panel).getByRole('link', { name: 'All periods' })).toHaveAttribute('href', '/11/financial_mutations?period=all')
    await userEvent.click(within(panel).getByRole('link', { name: 'All periods' }))
    await waitFor(() => expect(filter).toBe('state:all'))
    await userEvent.click(screen.getByRole('button', { name: 'Filtered transactions' }))
    await userEvent.click(screen.getByRole('link', { name: 'Unprocessed transactions' }))
    await waitFor(() => expect(filter).toBe('period:this_year,state:unprocessed'))
  })

  it('reads a range of months from the address', async () => {
    let filter = ''
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/financial_mutations', ({ request }) => {
      filter = new URL(request.url).searchParams.get('filter') ?? ''
      return HttpResponse.json([])
    }))
    renderAt('/11/financial_mutations?state=processed&period=202601..202603')
    await waitFor(() => expect(filter).toBe('period:202601..202603,state:processed'))
    expect(await screen.findByRole('button', { name: 'Filtered transactions' })).toBeInTheDocument()
  })

  it('books part of a transaction on a category', async () => {
    let sent: unknown
    const booked = mutation({ amount_open: '-5.5', ledger_account_bookings: [{ id: '901', administration_id: '11', financial_mutation_id: '701', ledger_account_id: '1201', description: null, price: '20.0', project_id: null, booking_rule_id: null, created_at: stamp, updated_at: stamp }] as Mutation['ledger_account_bookings'] })
    let current = mutation()
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/financial_mutations/701', () => HttpResponse.json(current)),
      http.patch('*/api/v2/11/financial_mutations/701/link_booking', async ({ request }) => {
        sent = await request.json()
        current = booked
        return HttpResponse.json(booked)
      }),
    )
    renderAt('/11/financial_mutations/701')
    const form = await screen.findByRole('form', { name: 'Book' })
    expect(within(form).getByLabelText('Amount')).toHaveValue('25.5')
    await within(form).findByRole('option', { name: /Software/ })
    await userEvent.selectOptions(within(form).getByLabelText('Category'), '1201')
    await userEvent.clear(within(form).getByLabelText('Amount'))
    await userEvent.type(within(form).getByLabelText('Amount'), '20')
    await userEvent.click(within(form).getByRole('button', { name: 'Book' }))
    await waitFor(() => expect(sent).toEqual({ booking_type: 'LedgerAccount', booking_id: '1201', price_base: '20', description: '' }))
    const bookings = await screen.findByRole('table', { name: 'Bookings' })
    expect(within(bookings).getByText('Software')).toBeInTheDocument()
    expect(within(bookings).getByRole('button', { name: 'Remove the booking on Software' })).toBeInTheDocument()
  })

  it('pays an open sales invoice with an incoming transaction and unlinks it', async () => {
    let sent: unknown
    let unlinked: unknown
    const incoming = mutation({ amount: '242.0', amount_open: '242.0', contra_account_name: 'Example Shop B.V.', message: 'Invoice 2026-0001' })
    const shop = { id: '801', company_name: 'Example Shop B.V.', firstname: null, lastname: null }
    const invoice = (id: string, number: string, reference: string) => ({
      id, administration_id: '11', contact_id: '801', contact: shop, invoice_id: number, reference, state: 'open', currency: 'EUR', total_unpaid: '242.0',
      details: [], tax_totals: [], payments: [], notes: [], attachments: [], events: [], total_price_excl_tax: '200.0', total_price_incl_tax: '242.0', version: 1, uri: `/11/sales_invoices/${id}`,
    })
    const paid = {
      ...incoming, amount_open: '0.0', state: 'processed' as const,
      payments: [{ id: '991', administration_id: '11', invoice_type: 'SalesInvoice' as const, invoice_id: '901', price: '242.0', price_base: '242.0', payment_date: '2026-11-01', created_at: stamp, updated_at: stamp }],
    }
    let current: Mutation = incoming
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/financial_mutations/701', () => HttpResponse.json(current)),
      http.get('*/api/v2/11/sales_invoices', () => HttpResponse.json([invoice('901', '2026-0001', 'Order 12'), invoice('902', '2026-0002', 'Order 13')], { headers: { 'X-Total-Count': '2' } })),
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice('901', '2026-0001', 'Order 12'))),
      http.patch('*/api/v2/11/financial_mutations/701/link_booking', async ({ request }) => {
        sent = await request.json()
        current = paid as Mutation
        return HttpResponse.json(current)
      }),
      http.delete('*/api/v2/11/financial_mutations/701/unlink_booking', async ({ request }) => {
        unlinked = await request.json()
        current = incoming
        return HttpResponse.json(current)
      }),
    )
    renderAt('/11/financial_mutations/701')
    const form = await screen.findByRole('form', { name: 'Link to an invoice' })
    await userEvent.type(within(form).getByLabelText('Find an invoice'), 'order 13')
    expect(await within(form).findByRole('radio', { name: /2026-0002/ })).toBeInTheDocument()
    expect(within(form).queryByRole('radio', { name: /2026-0001/ })).toBeNull()
    await userEvent.clear(within(form).getByLabelText('Find an invoice'))
    await userEvent.type(within(form).getByLabelText('Find an invoice'), '0001')
    await userEvent.click(await within(form).findByRole('radio', { name: /2026-0001/ }))
    await userEvent.click(within(form).getByRole('button', { name: 'Link invoice' }))
    await waitFor(() => expect(sent).toEqual({ booking_type: 'SalesInvoice', booking_id: '901' }))
    const paidTable = await screen.findByRole('table', { name: 'Invoices paid' })
    expect(await within(paidTable).findByRole('link', { name: '2026-0001' })).toHaveAttribute('href', '/11/sales_invoices/901')
    expect(screen.queryByRole('form', { name: 'Link to an invoice' })).toBeNull()
    await userEvent.click(within(paidTable).getByRole('button', { name: 'Unlink invoice 2026-0001' }))
    await waitFor(() => expect(unlinked).toEqual({ booking_type: 'Payment', booking_id: '991' }))
    expect(await screen.findByRole('form', { name: 'Link to an invoice' })).toBeInTheDocument()
  })

  it('suggests first the invoice whose payment reference the transaction quotes', async () => {
    let sent: unknown
    const shop = { id: '801', company_name: 'Example Shop B.V.', firstname: null, lastname: null }
    const invoice = (id: string, number: string, paymentReference: string) => ({
      id, administration_id: '11', contact_id: '801', contact: shop, invoice_id: number, reference: '', payment_reference: paymentReference, state: 'open', currency: 'EUR',
      total_unpaid: '242.0', details: [], tax_totals: [], payments: [], notes: [], attachments: [], events: [], total_price_excl_tax: '200.0', total_price_incl_tax: '242.0',
      version: 1, uri: `/11/sales_invoices/${id}`,
    })
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/financial_mutations/701', () => HttpResponse.json(mutation({ amount: '242.0', amount_open: '242.0', message: 'Betaling rf13 pv96 x5kp' }))),
      http.get('*/api/v2/11/sales_invoices', () => HttpResponse.json([
        invoice('901', '2026-0001', 'RF180000000000000SKE5TSYN'), invoice('902', '2026-0002', 'RF130000000000000PV96X5KP'),
      ], { headers: { 'X-Total-Count': '2' } })),
      http.patch('*/api/v2/11/financial_mutations/701/link_booking', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(mutation())
      }),
    )
    renderAt('/11/financial_mutations/701')
    const form = await screen.findByRole('form', { name: 'Link to an invoice' })
    const radios = await within(form).findAllByRole('radio')
    expect(radios[0]).toHaveAccessibleName(/2026-0002.*quotes payment reference RF13PV96X5KP/)
    expect(radios[0]).toBeChecked()
    await userEvent.click(within(form).getByRole('button', { name: 'Link invoice' }))
    await waitFor(() => expect(sent).toEqual({ booking_type: 'SalesInvoice', booking_id: '902' }))
  })

  it('pays an open external invoice too', async () => {
    let sent: unknown
    const incoming = mutation({ amount: '121.0', amount_open: '121.0', contra_account_name: 'Example Shop B.V.', message: 'Order WEB-1001' })
    const shop = { id: '801', company_name: 'Example Shop B.V.', firstname: null, lastname: null }
    const external = {
      id: '931', administration_id: '11', contact_id: '801', contact: shop, reference: 'WEB-1001', entry_number: 1, date: '2026-11-01', state: 'open', currency: 'EUR',
      total_unpaid: '121.0', total_paid: '0.0', details: [], tax_totals: [], payments: [], notes: [], attachments: [], events: [], total_price_excl_tax: '100.0',
      total_price_incl_tax: '121.0', version: 1, uri: '/11/external_sales_invoices/931',
    }
    const paid = {
      ...incoming, amount_open: '0.0', state: 'processed' as const,
      payments: [{ id: '992', administration_id: '11', invoice_type: 'ExternalSalesInvoice' as const, invoice_id: '931', price: '121.0', price_base: '121.0', payment_date: '2026-11-01', created_at: stamp, updated_at: stamp }],
    }
    let current: Mutation = incoming
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/financial_mutations/701', () => HttpResponse.json(current)),
      http.get('*/api/v2/11/sales_invoices', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
      http.get('*/api/v2/11/external_sales_invoices', () => HttpResponse.json([external], { headers: { 'X-Total-Count': '1' } })),
      http.get('*/api/v2/11/external_sales_invoices/931', () => HttpResponse.json(external)),
      http.patch('*/api/v2/11/financial_mutations/701/link_booking', async ({ request }) => {
        sent = await request.json()
        current = paid as Mutation
        return HttpResponse.json(current)
      }),
    )
    renderAt('/11/financial_mutations/701')
    const form = await screen.findByRole('form', { name: 'Link to an invoice' })
    await userEvent.click(await within(form).findByRole('radio', { name: /WEB-1001/ }))
    await userEvent.click(within(form).getByRole('button', { name: 'Link invoice' }))
    await waitFor(() => expect(sent).toEqual({ booking_type: 'ExternalSalesInvoice', booking_id: '931' }))
    const paidTable = await screen.findByRole('table', { name: 'Invoices paid' })
    expect(await within(paidTable).findByRole('link', { name: 'WEB-1001' })).toHaveAttribute('href', '/11/external_sales_invoices/931')
  })
})

describe('financial accounts', () => {
  const overview = http.get('*/api/v2/11/overview', () => HttpResponse.json({ bank: { balance: '0.0', currency: 'EUR', unprocessed_count: 431, recently_processed: [] } }))
  const accounts = http.get('*/api/v2/11/financial_accounts', () => HttpResponse.json([
    account({ balance: '40912.61', last_import_at: '2026-09-10T09:00:00Z' }), { ...cash, balance: '0.0' }, { ...old, balance: '0.0' },
  ]))

  it('lists each account with its balance and opens its transactions, as Moneybird does', async () => {
    server.use(accounts, overview, ...loaded())
    renderAt('/11/financial_accounts')
    expect(await screen.findByRole('heading', { level: 1, name: 'Financial accounts' })).toBeInTheDocument()
    const list = screen.getByRole('region', { name: 'Financial accounts' })
    const bank = within(list).getByRole('link', { name: 'NL91 ABNA 0417 1643 00' })
    expect(bank).toHaveAttribute('href', '/11/financial_mutations?account=501&state=all')
    expect(bank).toHaveTextContent('€40,912.61')
    expect(bank).toHaveTextContent(/Last update: .*ago/)
    expect(within(list).getByRole('link', { name: 'Cash' })).toHaveTextContent('Last update: never')
    // The deactivated account is folded away.
    expect(within(list).queryByRole('link', { name: 'Old bank' })).toBeNull()
    expect(list).toHaveTextContent('There is 1 deactivated account.')
    await userEvent.click(within(list).getByRole('button', { name: 'Show deactivated account.' }))
    expect(within(list).getByRole('link', { name: 'Old bank' })).toHaveTextContent('This account is disabled')
    expect(await screen.findByRole('link', { name: 'Link unprocessed transactions (431)' })).toHaveAttribute('href', '/11/financial_mutations?state=unprocessed')
  })

  it('imports a dropped statement file into its account and shows its transactions', async () => {
    let body: unknown
    server.use(accounts, overview, ...loaded(), http.post('*/api/v2/11/financial_statements/import', async ({ request }) => {
      body = await request.text()
      return HttpResponse.json({ format: 'camt', imported: 2, duplicates: 0, locked: 0, auto_booked: 0, financial_statements: [], financial_account_id: '501', financial_account_ids: ['501'] }, { status: 201 })
    }))
    renderAt('/11/financial_accounts')
    await userEvent.click(await screen.findByRole('button', { name: 'Add transactions' }))
    const add = screen.getByRole('region', { name: 'Add transactions' })
    expect(add).toHaveTextContent('CAMT.053 or MT940, max. 10 MB')
    expect(within(add).getByRole('link', { name: 'add transactions manually' })).toHaveAttribute('href', '/11/financial_accounts/501')
    await userEvent.upload(within(add).getByLabelText('Drag a transaction file here or click to select one'), new File(['<Document/>'], 'statement.xml', { type: 'text/xml' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/financial_mutations'))
    expect(screen.getByTestId('search')).toHaveTextContent('?account=501')
    expect(body).toBe('<Document/>')
  })

  it('shows all transactions after a file for several accounts', async () => {
    server.use(accounts, overview, ...loaded(), http.post('*/api/v2/11/financial_statements/import', () =>
      HttpResponse.json({ format: 'mt940', imported: 3, duplicates: 0, locked: 0, auto_booked: 0, financial_statements: [], financial_account_id: '501', financial_account_ids: ['501', '502'] }, { status: 201 }),
    ))
    renderAt('/11/financial_accounts')
    await userEvent.click(await screen.findByRole('button', { name: 'Add transactions' }))
    const add = screen.getByRole('region', { name: 'Add transactions' })
    await userEvent.upload(within(add).getByLabelText('Drag a transaction file here or click to select one'), new File([':20:A'], 'statement.sta', { type: 'text/plain' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/financial_mutations'))
    expect(screen.getByTestId('search')).toBeEmptyDOMElement()
  })
})

describe('linking a transaction in its card', () => {
  const suggestion = (over: Partial<LinkSuggestion> = {}): LinkSuggestion => ({
    booking_type: 'Document', booking_id: '583', label: 'PUR', reference: '583', contact_name: 'Example Supplier', date: '2026-10-30', total: '25.5', open: '25.5',
    currency: 'EUR', state: 'open', payment_reference: null, details: '1 Hosting plan', price: '25.5', ...over,
  })

  it('links the best match, searches and books on a category with a description', async () => {
    let linked: unknown
    let asked = ''
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/financial_mutations', () => HttpResponse.json([mutation()])),
      http.get('*/api/v2/11/financial_mutations/701/link_suggestions', ({ request }) => {
        asked = new URL(request.url).search
        return HttpResponse.json(new URL(request.url).searchParams.get('query') === 'soft'
          ? [suggestion({ booking_type: 'LedgerAccount', booking_id: '1201', label: '', reference: 'Software', contact_name: '', details: '', price: '25.5' })]
          : [suggestion(), suggestion({ booking_type: 'SalesInvoice', booking_id: '901', label: 'INV', reference: '2026-0001', state: 'late', price: '-25.5' })])
      }),
      http.patch('*/api/v2/11/financial_mutations/701/link_booking', async ({ request }) => {
        linked = await request.json()
        return HttpResponse.json(mutation({ amount_open: '0.0', state: 'processed' }))
      }),
    )
    renderAt('/11/financial_mutations')
    const card = await screen.findByRole('article', { name: 'Hosting Company' })
    await userEvent.click(within(card).getByRole('button', { name: 'Link open amount' }))
    const panel = within(card).getByRole('form', { name: 'Link transaction' })
    const results = await within(panel).findByRole('radiogroup', { name: 'Matches' })
    // The best match is chosen, with what linking it books.
    expect(within(results).getByRole('radio', { name: /PUR 583 · Example Supplier/ })).toBeChecked()
    expect(results).toHaveTextContent('€+25.50')
    expect(results).toHaveTextContent('€-25.50')
    expect(within(panel).getByRole('textbox', { name: 'Amount to link' })).toHaveValue('25.5')
    expect(within(panel).getByRole('combobox', { name: 'Search in' })).toHaveValue('all')

    await userEvent.type(within(panel).getByRole('searchbox', { name: 'Find invoices or categories…' }), 'soft')
    await waitFor(() => expect(asked).toContain('query=soft'))
    await userEvent.click(await within(panel).findByRole('radio', { name: /Software/ }))
    await userEvent.type(within(panel).getByRole('textbox', { name: 'Description' }), 'Yearly plan')
    await userEvent.clear(within(panel).getByRole('textbox', { name: 'Amount to link' }))
    await userEvent.type(within(panel).getByRole('textbox', { name: 'Amount to link' }), '20')
    expect(panel).toHaveTextContent('Left to link: €5.50')
    await userEvent.click(within(panel).getByRole('button', { name: 'Link' }))
    await waitFor(() => expect(linked).toEqual({ booking_type: 'LedgerAccount', booking_id: '1201', price_base: '20', description: 'Yearly plan' }))
  })

  it('makes a new receipt for an uploaded document and attaches the file', async () => {
    const calls: string[] = []
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/financial_mutations', () => HttpResponse.json([mutation()])),
      http.patch('*/api/v2/11/financial_mutations/701/link_booking', async ({ request }) => {
        calls.push(`link ${JSON.stringify(await request.json())}`)
        return HttpResponse.json(mutation({ amount_open: '0.0', state: 'processed', payments: [{ id: '961', invoice_type: 'Document', invoice_id: '777' }] as Mutation['payments'] }))
      }),
      http.post('*/api/v2/11/documents/receipts/777/attachments', () => {
        calls.push('attach 777')
        return HttpResponse.json({}, { status: 201 })
      }),
    )
    renderAt('/11/financial_mutations')
    const card = await screen.findByRole('article', { name: 'Hosting Company' })
    await userEvent.upload(within(card).getByLabelText('Upload a document'), new File(['%PDF'], 'receipt.pdf', { type: 'application/pdf' }))
    await waitFor(() => expect(calls).toEqual(['link {"booking_type":"NewReceipt"}', 'attach 777']))
  })
})

describe('payments', () => {
  const tx = (over: Partial<PurchaseTransaction> = {}): PurchaseTransaction => ({
    id: '1001', administration_id: '11', financial_account_id: '501', amount: '25.5', state: 'open', sepa_iban: 'NL44RABO0123456789', sepa_iban_account_name: 'Hosting Company',
    sepa_bic: 'RABONL2U', end_to_end_id: 'E2E-1001', description: 'Invoice 583', created_at: stamp, updated_at: stamp, uri: '/11/purchase_transactions/1001',
    payable_id: '583', payable_type: 'Document', purchase_transaction_batch_id: null, date: null, ...over,
  } as PurchaseTransaction)

  it('batches the payments to send and lists the earlier ones, as Moneybird does', async () => {
    let batched: unknown
    const filters: string[] = []
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/purchase_transactions', ({ request }) => {
        const filter = new URL(request.url).searchParams.get('filter') ?? ''
        filters.push(filter)
        return HttpResponse.json(filter.includes('unbatched:true')
          ? [tx(), tx({ id: '1002', sepa_iban_account_name: 'Other Supplier', amount: '10.0' })]
          : [tx({ id: '999', state: 'pending_payment', purchase_transaction_batch_id: '77', date: '2026-11-01' })])
      }),
      http.post('*/api/v2/11/purchase_transaction_batches', async ({ request }) => {
        batched = await request.json()
        return HttpResponse.json({ id: '78' }, { status: 201 })
      }),
    )
    renderAt('/11/purchase_transaction_batches')
    expect(await screen.findByRole('heading', { level: 1, name: 'Payments' })).toBeInTheDocument()
    const toSend = await screen.findByRole('list', { name: 'To send' })
    expect(toSend).toHaveTextContent('Hosting Company')
    expect(toSend).toHaveTextContent('NL44 RABO 0123 4567 89 · Invoice 583')
    expect(toSend).toHaveTextContent('€25.50')
    expect(filters).toContain('state:open|pending_payment,period:this_year,unbatched:false')

    await userEvent.click(within(toSend).getByRole('checkbox', { name: 'Choose the payment to Hosting Company' }))
    await userEvent.click(screen.getByRole('button', { name: 'Create a batch of 1 payment' }))
    await waitFor(() => expect(batched).toEqual({ purchase_transaction_batch: { financial_account_id: '501', purchase_transaction_ids: ['1001'] } }))
    expect(await screen.findByRole('link', { name: 'download the SEPA file' })).toHaveAttribute('href', '/api/v2/11/purchase_transaction_batches/78/download')

    const earlier = screen.getByRole('table', { name: 'Earlier payments' })
    expect(earlier).toHaveTextContent('Pending payment')
    expect(within(earlier).getByRole('link', { name: 'SEPA file' })).toHaveAttribute('href', '/api/v2/11/purchase_transaction_batches/77/download')
  })

  it('says how to start while there is nothing to send', async () => {
    server.use(...loaded(), http.get('*/api/v2/11/purchase_transactions', () => HttpResponse.json([])))
    renderAt('/11/purchase_transaction_batches')
    expect(await screen.findByText('Collect payments here to send them in one go. Set up a payment on a purchase invoice.')).toBeInTheDocument()
    expect(await screen.findByText('Your sent payments appear here, with their status.')).toBeInTheDocument()
  })
})
