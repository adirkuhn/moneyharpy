import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { LedgerAccount } from '../bookkeeping/queries'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { BookingRule, FinancialAccount, Mutation } from './queries'

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
  it('lists transactions to book with filters in the address', async () => {
    let filter = ''
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/financial_mutations', ({ request }) => {
      filter = new URL(request.url).searchParams.get('filter') ?? ''
      return HttpResponse.json([mutation()])
    }))
    renderAt('/11/financial_mutations')
    const table = await screen.findByRole('table', { name: 'Transactions' })
    expect(within(table).getByRole('link', { name: 'Hosting Company' })).toHaveAttribute('href', '/11/financial_mutations/701')
    expect(filter).toBe('period:this_year,state:unprocessed')
    await userEvent.selectOptions(screen.getByLabelText('Account'), '501')
    await userEvent.click(screen.getByRole('radio', { name: 'All' }))
    await waitFor(() => expect(filter).toBe('period:this_year,state:all,financial_account_id:501'))
    expect(screen.getByRole('navigation', { name: 'Main' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Bank' })).toHaveAttribute('aria-current', 'page')
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
})
