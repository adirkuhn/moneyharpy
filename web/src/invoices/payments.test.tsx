import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { choose, invoice, loaded, sales, sidebar, stamp, stateLabel } from './testkit'

describe('getting paid', () => {
  it('registers a cash payment on the "How was the invoice paid?" page, and removes it again', async () => {
    let sent: unknown
    let removed = false
    const payment = { id: '981', administration_id: '11', invoice_type: 'SalesInvoice' as const, invoice_id: '901', price: '242.0', price_base: '242.0', payment_date: '2026-11-05', manual_payment_action: 'cash_payment', created_at: stamp, updated_at: stamp }
    let current = invoice()
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(current)),
      http.post('*/api/v2/11/sales_invoices/901/payments', async ({ request }) => {
        sent = await request.json()
        current = invoice({ state: 'paid', paid_at: '2026-11-05', total_unpaid: '0.0', total_paid: '242.0', payments: [payment] })
        return HttpResponse.json(payment, { status: 201 })
      }),
      http.delete('*/api/v2/11/sales_invoices/901/payments/981', () => {
        removed = true
        current = invoice()
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/sales_invoices/901')
    await userEvent.click(await screen.findByRole('link', { name: 'Register payment' }))
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/901\/payments\/new$/)
    expect(await screen.findByRole('heading', { level: 1, name: 'Register payment' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 2, name: 'How was the invoice paid?' })).toBeInTheDocument()
    expect(screen.getByText('You have indicated that you want to register a payment for invoice 2026-0001. Please indicate below how this invoice was paid.')).toBeInTheDocument()
    const ways = screen.getByRole('list', { name: 'How was the invoice paid?' })
    expect(within(ways).getAllByRole('link').map((l) => l.textContent)).toEqual([
      'Bank transferLink your bank transaction', 'Business cash paymentThrough ledger', 'Rounding difference', 'Balance settlement',
      'Mark as paid without proofNo proof of payment present in MoneyHarpy',
    ])
    expect(screen.getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/11/sales_invoices/901')

    await userEvent.click(within(ways).getByRole('link', { name: 'Business cash payment' }))
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/901\/payments\/cash_payment$/)
    expect(screen.getByRole('heading', { level: 2, name: 'Business cash payment' })).toBeInTheDocument()
    expect(screen.getByText('You are going to register a cash payment for invoice 2026-0001 with reference Order 12, which will create a transaction in the cash book and automatically link it.')).toBeInTheDocument()
    const form = screen.getByRole('form', { name: 'Register payment' })
    expect(within(form).getByLabelText('Amount')).toHaveValue('242.00')
    expect(within(form).getByLabelText('Payment date')).toHaveValue('2026-11-11')
    expect(screen.getByRole('link', { name: 'Back' })).toHaveAttribute('href', '/11/sales_invoices/901/payments/new')
    const date = within(form).getByLabelText('Payment date')
    await userEvent.clear(date)
    await userEvent.type(date, '2026-11-05')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ payment: { payment_date: '2026-11-05', price: '242.00', manual_payment_action: 'cash_payment' } }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/901$/))
    expect(await stateLabel('Paid')).toBeInTheDocument()

    const summary = within(sidebar()).getByRole('list', { name: 'Payment summary' })
    expect(within(summary).getByText('To receive').parentElement).toHaveTextContent('€0.00')
    const toggle = within(summary).getByRole('button', { name: 'View registered payment' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await userEvent.click(toggle)
    const paid = within(summary).getByRole('list', { name: 'Registered payments' })
    expect(within(paid).getByRole('listitem')).toHaveTextContent(/Nov 5, 2026.*€242\.00.*Cash/)
    expect(within(sidebar()).getByText(/^Received payment of €242\.00/)).toBeInTheDocument()
    await userEvent.click(within(paid).getByRole('button', { name: 'Remove payment of Nov 5, 2026' }))
    await userEvent.click(within(paid).getByRole('button', { name: 'Confirm remove payment of Nov 5, 2026' }))
    await waitFor(() => expect(removed).toBe(true))
    expect(await stateLabel('Open')).toBeInTheDocument()
  })

  it('shows what is wrong with a payment', async () => {
    server.use(...loaded(), http.post('*/api/v2/11/sales_invoices/901/payments', () =>
      HttpResponse.json({ error: { price: ['must not be more than what is unpaid, 242.0'] } }, { status: 422 })))
    renderAt('/11/sales_invoices/901/payments/cash_payment')
    const form = await screen.findByRole('form', { name: 'Register payment' })
    await userEvent.clear(within(form).getByLabelText('Amount'))
    await userEvent.type(within(form).getByLabelText('Amount'), '500')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    expect(await within(form).findByText(/must not be more than what is unpaid/)).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent(/payments\/cash_payment$/)
  })

  it('settles with a balance category, marks paid without proof and books a rounding difference', async () => {
    const sent: unknown[] = []
    const loan = { ...sales, id: '1601', name: 'Loan from the owner', account_type: 'current_liabilities' as const, allowed_document_types: [], uri: '/11/ledger_accounts/1601' }
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales, loan])),
      http.get('*/api/v2/11/financial_accounts', () => HttpResponse.json([{ id: '301', administration_id: '11', name: 'Main bank', identifier: 'NL91ABNA0417164300', type: 'BankAccount', active: true, currency: 'EUR' }])),
      http.post('*/api/v2/11/sales_invoices/901/payments', async ({ request }) => {
        sent.push(await request.json())
        return HttpResponse.json({ id: '985' }, { status: 201 })
      }),
    )
    renderAt('/11/sales_invoices/901/payments/balance_settlement')
    expect(await screen.findByRole('heading', { level: 2, name: 'Settlement with balance ledger account' })).toBeInTheDocument()
    expect(screen.getByText('You are going to register a balance settlement for invoice 2026-0001 with reference Order 12')).toBeInTheDocument()
    let form = screen.getByRole('form', { name: 'Register payment' })
    const category = within(form).getByLabelText('Category')
    await waitFor(() => expect(within(category).getAllByRole('option').map((o) => o.textContent)).toEqual(['Loan from the owner']))
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0]).toEqual({ payment: { payment_date: '2026-11-11', price: '242.00', manual_payment_action: 'balance_settlement', ledger_account_id: '1601' } })

    await userEvent.click(await screen.findByRole('link', { name: 'Register payment' }))
    await userEvent.click(screen.getByRole('link', { name: 'Mark as paid without proof' }))
    expect(screen.getByText('You are going to register a payment for invoice 2026-0001 with reference Order 12')).toBeInTheDocument()
    form = screen.getByRole('form', { name: 'Register payment' })
    const method = within(form).getByLabelText('Payment method')
    expect(method).toHaveValue('')
    await waitFor(() => expect(within(method).getAllByRole('option').map((o) => o.textContent)).toEqual(['Without financial account', 'Main bank']))
    await userEvent.selectOptions(method, 'Main bank')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toHaveLength(2))
    expect(sent[1]).toEqual({ payment: { payment_date: '2026-11-11', price: '242.00', manual_payment_action: 'payment_without_proof', financial_account_id: '301' } })

    await userEvent.click(await screen.findByRole('link', { name: 'Register payment' }))
    await userEvent.click(screen.getByRole('link', { name: 'Rounding difference' }))
    expect(screen.getByText('Register a rounding difference for invoice 2026-0001 with reference Order 12')).toBeInTheDocument()
    form = screen.getByRole('form', { name: 'Register payment' })
    expect(within(form).queryByLabelText('Amount')).toBeNull()
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toHaveLength(3))
    expect(sent[2]).toEqual({ payment: { payment_date: '2026-11-11', price: '242.00', manual_payment_action: 'rounding_error' } })
  })

  it('links the bank transaction that pays the invoice, or says none was found', async () => {
    let linked: unknown
    let mutations = [{ id: '771', administration_id: '11', financial_account_id: '301', date: '2026-11-10', amount: '242.0', amount_open: '242.0', message: 'Invoice 2026-0001', contra_account_name: 'Example Shop B.V.', state: 'unprocessed', currency: 'EUR' }]
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/financial_mutations', () => HttpResponse.json(mutations)),
      http.patch('*/api/v2/11/financial_mutations/771/link_booking', async ({ request }) => {
        linked = await request.json()
        return HttpResponse.json({ ...mutations[0], state: 'processed' })
      }),
    )
    const { unmount } = renderAt('/11/sales_invoices/901/payments/bank_transfer')
    const found = await screen.findByRole('list', { name: 'Matching transactions' })
    expect(within(found).getByRole('listitem')).toHaveTextContent(/Example Shop B\.V\..*Invoice 2026-0001.*€242\.00/)
    await userEvent.click(within(found).getByRole('button', { name: 'Link' }))
    await waitFor(() => expect(linked).toEqual({ booking_type: 'SalesInvoice', booking_id: '901' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/901$/))
    unmount()

    mutations = []
    renderAt('/11/sales_invoices/901/payments/bank_transfer')
    expect(await screen.findByRole('heading', { level: 2, name: 'No matching transactions were found' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Bank' })).toHaveAttribute('href', '/11/financial_mutations')
  })

  it('links a payment to its bank transaction', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice({
      state: 'paid', total_unpaid: '0.0',
      payments: [{ id: '982', administration_id: '11', invoice_type: 'SalesInvoice', invoice_id: '901', price: '242.0', price_base: '242.0', payment_date: '2026-11-03', financial_mutation_id: '771', manual_payment_action: null, created_at: stamp, updated_at: stamp }],
    }))))
    renderAt('/11/sales_invoices/901')
    await stateLabel('Paid')
    const summary = within(sidebar()).getByRole('list', { name: 'Payment summary' })
    await userEvent.click(within(summary).getByRole('button', { name: 'View registered payment' }))
    const paid = within(summary).getByRole('list', { name: 'Registered payments' })
    expect(within(paid).getByRole('link', { name: 'Bank transaction' })).toHaveAttribute('href', '/11/financial_mutations/771')
    expect(within(paid).getByRole('button', { name: /^Remove payment of/ })).toHaveTextContent('Unlink')
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
    await userEvent.click(await screen.findByRole('link', { name: 'Register payment' }))
    await userEvent.click(await screen.findByRole('button', { name: 'Settle with the original invoice' }))
    await waitFor(() => expect(called).toBe(true))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/903$/))
    expect(await stateLabel('Paid')).toBeInTheDocument()
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
    await choose('Extra', 'Mark as dubious')
    let panel = screen.getByRole('region', { name: 'Mark as dubious' })
    await userEvent.clear(within(panel).getByLabelText('Date'))
    await userEvent.type(within(panel).getByLabelText('Date'), '2026-11-08')
    await userEvent.click(within(panel).getByRole('button', { name: 'Mark as dubious' }))
    expect(await screen.findByText('This invoice is marked as dubious.')).toBeInTheDocument()
    expect(within(sidebar()).getByText('Marked dubious on').nextElementSibling).toHaveTextContent('Nov 8, 2026')
    await userEvent.click(screen.getByRole('button', { name: 'Extra' }))
    expect(screen.queryByRole('button', { name: 'Mark as dubious' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Mark as uncollectible' }))
    panel = screen.getByRole('region', { name: 'Mark as uncollectible' })
    expect(within(panel).getByText(/cannot be undone/)).toBeInTheDocument()
    await userEvent.clear(within(panel).getByLabelText('Date'))
    await userEvent.type(within(panel).getByLabelText('Date'), '2026-11-09')
    await userEvent.click(within(panel).getByRole('button', { name: 'Write off as uncollectible' }))
    expect(await stateLabel('Irrecoverable')).toBeInTheDocument()
    expect(bodies).toEqual([{ dubious_date: '2026-11-08' }, { uncollectible_date: '2026-11-09', book_method: 'revenue' }])
    // Nothing is sent or paid any more.
    expect(screen.queryByRole('button', { name: 'Send' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Register payment' })).toBeNull()
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
    expect(await stateLabel('Expired')).toBeInTheDocument()
    expect(within(sidebar()).getByText('Next reminder').nextElementSibling).toHaveTextContent('Nov 22, 2026')
    await choose('Send', 'Send reminder')
    const panel = screen.getByRole('region', { name: 'Send reminder' })
    expect(within(panel).getByLabelText('Email address')).toHaveValue('billing@example.com')
    await userEvent.type(within(panel).getByLabelText('Reminder text'), 'Please pay {{invoices}')
    await userEvent.click(within(panel).getByRole('button', { name: 'Send reminder' }))
    await waitFor(() => expect(sent).toEqual({ sales_invoice_reminders: [{
      contact_id: '701', workflow_id: '601', document_style_id: '701', identity_id: '461', sales_invoice_ids: ['901'],
      delivery_method: 'Email', email_address: 'billing@example.com', reminder_text: 'Please pay {invoices}',
    }] }))
    expect(await stateLabel('Reminded')).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Send reminder' })).toBeNull()
    expect(within(sidebar()).getByText('Reminders sent').nextElementSibling).toHaveTextContent('1')
  })
})
