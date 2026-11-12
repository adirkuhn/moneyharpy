import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import type { LedgerAccount } from '../bookkeeping/queries'
import { renderAt } from '../test/render'
import { defaultConditions, server, workflow } from '../test/server'
import { cells, choose, contactCounts, creating, draft, invoice, invoicePaper, loaded, sales, senderStyle, sidebar, stamp, stateLabel } from './testkit'

describe('an invoice', () => {
  it("shows the invoice as its document with Moneybird's action bar and sidebar", async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/901')
    expect(await screen.findByRole('heading', { level: 1, name: '2026-0001' })).toBeInTheDocument()
    expect(await stateLabel('Open')).toBeInTheDocument()

    // The document, written in the workflow's language.
    const doc = invoicePaper('Invoice 2026-0001')
    expect(within(doc).getByRole('heading', { level: 2 })).toHaveTextContent('Factuur 2026-0001')
    expect(within(doc).getByText('Order 12')).toBeInTheDocument()
    // Moneybird's default layout prints the reference and the dates, no client number.
    expect(within(doc).queryByText(/Klantnummer/)).toBeNull()
    const lines = within(doc).getByRole('table', { name: 'Factuurregels' })
    expect(cells(within(lines).getAllByRole('row')[1]!)).toEqual(['2 x', 'Consulting', '€ 100,00', '€ 200,00', '21%'])
    const totals = within(doc).getByRole('table', { name: 'Totals' })
    expect(within(totals).getByRole('row', { name: /Subtotaal excl. btw/ })).toHaveTextContent('€ 200,00')
    expect(await within(totals).findByRole('row', { name: /21% btw over €\s200,00/ })).toHaveTextContent('€ 42,00')
    expect(within(totals).getByRole('row', { name: /^Totaal/ })).toHaveTextContent('€ 242,00')
    // Its blocks open the editor, as Moneybird's blue markers do.
    expect(within(doc).getByRole('button', { name: 'Edit contact' })).toBeInTheDocument()
    expect(within(doc).getByRole('button', { name: 'Invoice settings' })).toBeInTheDocument()
    expect(within(doc).getByRole('button', { name: 'Edit payment conditions' })).toBeInTheDocument()

    // The action bar.
    expect(screen.getByRole('link', { name: 'Register payment' })).toHaveAttribute('href', '/11/sales_invoices/901/payments/new')
    expect(screen.getByRole('button', { name: 'Journal entries' })).toHaveAttribute('aria-pressed', 'false')
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(screen.getByRole('button', { name: 'Send invoice again' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Send reminder' })).toBeInTheDocument()
    // A sent invoice is not sent in the future.
    expect(screen.queryByRole('button', { name: 'Send in future' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Download as...' }))
    expect(screen.getByRole('link', { name: 'Invoice (PDF)' })).toHaveAttribute('href', '/api/v2/11/sales_invoices/901/download_pdf')
    expect(screen.getByRole('link', { name: 'Invoice (UBL)' })).toHaveAttribute('href', '/api/v2/11/sales_invoices/901/download_ubl')
    expect(screen.getByRole('link', { name: 'Packing slip (PDF)' })).toHaveAttribute('href', '/api/v2/11/sales_invoices/901/download_packing_slip_pdf')
    expect(screen.queryByText('Payment receipt (PDF)')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Extra' }))
    expect(screen.getByRole('button', { name: 'Duplicate invoice' })).toBeInTheDocument()
    expect(screen.getByText('Duplicate to quote').closest('.dropdown__item')).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('button', { name: 'Duplicate to credit note' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Duplicate to recurring invoice' })).toHaveAttribute('href', '/11/recurring_sales_invoices/new?from=901')
    expect(screen.getByRole('button', { name: 'Pause reminders' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Mark as dubious' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Mark as uncollectible' })).toBeInTheDocument()
    // A sent invoice is usually credited, but can be deleted after a warning.
    expect(screen.getByRole('button', { name: 'Delete invoice' })).toBeInTheDocument()

    // The sidebar.
    const side = sidebar()
    expect(within(side).getByRole('heading', { name: 'Products' })).toBeInTheDocument()
    expect(within(side).getByRole('link', { name: 'Add product' })).toHaveAttribute('href', '/11/products/new')
    const summary = within(side).getByRole('list', { name: 'Payment summary' })
    expect(within(summary).getByText('Total').parentElement).toHaveTextContent('€242.00')
    expect(within(summary).getByText('To receive').parentElement).toHaveTextContent('€242.00')
    expect(within(summary).queryByRole('button', { name: /View registered/ })).toBeNull()
    expect(within(side).getByText('Invoice id').nextElementSibling).toHaveTextContent('2026-0001')
    expect(within(side).getByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/contacts/701')
    expect(within(within(side).getByRole('list', { name: 'Category' })).getByRole('listitem')).toHaveTextContent('Sales')
    expect(within(side).getByRole('region', { name: 'Notes and tasks' })).toBeInTheDocument()
  })

  it('shows the contact in a card when its link is hovered or focused', async () => {
    server.use(...loaded(), ...contactCounts)
    renderAt('/11/sales_invoices/901')
    await screen.findByRole('heading', { level: 1, name: '2026-0001' })
    const link = within(sidebar()).getByRole('link', { name: 'Example Shop B.V.' })
    expect(screen.queryByRole('dialog', { name: 'Example Shop B.V.' })).toBeNull()
    await userEvent.hover(link)
    const card = await screen.findByRole('dialog', { name: 'Example Shop B.V.' })
    expect(within(card).getByText('Created 2 hours ago')).toBeInTheDocument()
    expect(within(card).getByText('Address').nextElementSibling).toHaveTextContent('Dam 21012AB Amsterdam')
    expect(within(card).getByText('Email').nextElementSibling).toHaveTextContent('info@example.com')
    expect(await within(card).findByText('Invoices')).toHaveTextContent('Invoices')
    await waitFor(() => expect(within(card).getByText('Invoices').previousElementSibling).toHaveTextContent('2'))
    await waitFor(() => expect(within(card).getByText('Documents').previousElementSibling).toHaveTextContent('3'))
    await userEvent.unhover(link)
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Example Shop B.V.' })).toBeNull())
  })

  it('remembers that the products tip was closed', async () => {
    server.use(...loaded())
    const { unmount } = renderAt('/11/sales_invoices/901')
    await screen.findByRole('heading', { level: 1, name: '2026-0001' })
    await userEvent.click(within(sidebar()).getByRole('button', { name: 'Close' }))
    expect(within(sidebar()).queryByRole('heading', { name: 'Products' })).toBeNull()
    unmount()
    renderAt('/11/sales_invoices/901')
    await screen.findByRole('heading', { level: 1, name: '2026-0001' })
    expect(within(sidebar()).queryByRole('heading', { name: 'Products' })).toBeNull()
  })

  it('shows the history of the invoice by day, with who did what', async () => {
    const at = (iso: string) => ({ created_at: iso, updated_at: iso, administration_id: '11', link_entity_id: '901', link_entity_type: 'SalesInvoice', user_id: '1', data: {} })
    const paid = invoice({
      state: 'paid', total_unpaid: '0.0', paid_at: '2026-11-10',
      events: [
        { action: 'sales_invoice_created', ...at('2026-11-10T10:15:00Z') },
        { action: 'sales_invoice_state_changed_to_open', ...at('2026-11-10T10:15:01Z') },
        { action: 'sales_invoice_send_manually', ...at('2026-11-10T10:15:02Z') },
        { action: 'payment_registered', ...at('2026-11-10T10:15:03Z') },
        { action: 'sales_invoice_state_changed_to_paid', ...at('2026-11-10T10:15:04Z') },
      ],
      payments: [{ id: '981', administration_id: '11', invoice_type: 'SalesInvoice', invoice_id: '901', price: '242.0', price_base: '242.0', payment_date: '2026-11-10', financial_mutation_id: null, manual_payment_action: 'bank_transfer', user_id: '1', created_at: '2026-11-10T10:15:03Z', updated_at: stamp }],
      notes: [{ id: '971', administration_id: '11', entity_id: '901', entity_type: 'SalesInvoice', note: 'Called about payment', todo: false, created_at: '2026-11-11T09:30:00Z' }],
    })
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(paid)),
      http.get('*/api/v2/11/users', () => HttpResponse.json([{ id: '1', name: 'Ann de Vries', email: 'ann@example.com' }])),
    )
    renderAt('/11/sales_invoices/901')
    expect(await stateLabel('Paid')).toBeInTheDocument()
    const history = within(sidebar()).getByRole('region', { name: 'Notes and tasks' })
    const days = within(history).getAllByRole('heading', { level: 3 }).map((h) => h.textContent)
    expect(days).toEqual(['Today', 'Yesterday'])
    const rows = within(history).getAllByRole('listitem').map((r) => r.querySelector('.history__text')?.textContent)
    expect(rows).toEqual([
      'Called about payment',
      'Invoice has been paid',
      'Received payment of €242.00Payment is not yet linked to a financial mutation',
      'Invoice manually marked as sent',
      'State of invoice changed to open',
      'Invoice created',
    ])
    const created = within(history).getAllByRole('listitem').at(-1)!
    expect(within(created).getByText('11:15')).toBeInTheDocument()
    expect(await within(created).findByTitle('Ann de Vries')).toHaveTextContent('A')
    await userEvent.click(within(created).getByRole('button', { name: 'Show/hide details' }))
    expect(within(created).getByText(/^By/)).toHaveTextContent('By Ann de Vries')
  })

  it('sends a draft by email, after Moneybird', async () => {
    let sent: unknown
    let current = draft
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/workflows', () => HttpResponse.json([workflow({ invoice_email_body: 'Dear {contact_name}, here is invoice {invoice_id}.' })])),
      http.get('*/api/v2/11/sales_invoices/902', () => HttpResponse.json(current)),
      http.patch('*/api/v2/11/sales_invoices/902/send_invoice', async ({ request }) => {
        sent = await request.json()
        current = invoice({ id: '902', invoice_id: '2026-0002', state: 'open' })
        return HttpResponse.json(current)
      }),
    )
    renderAt('/11/sales_invoices/902')
    expect(await screen.findByRole('heading', { level: 1, name: 'Draft #3' })).toBeInTheDocument()
    expect(await stateLabel('Draft')).toBeInTheDocument()
    // What Moneybird offers for a draft: no Edit, as the draft is edited in its document.
    expect(screen.queryByRole('link', { name: 'Edit' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Register payment' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Journal entries' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Download as...' }))
    expect(screen.getByRole('link', { name: 'Invoice (PDF)' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Invoice (UBL)' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Extra' }))
    expect(screen.getByRole('link', { name: 'Duplicate to recurring invoice' })).toHaveAttribute('href', '/11/recurring_sales_invoices/new?from=902')
    expect(screen.getByText('Convert to recurring').closest('.dropdown__item')).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('button', { name: 'Change layout' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Duplicate to credit note' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Delete invoice' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(screen.getByRole('button', { name: 'Send in future' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))

    await choose('Send', 'Send invoice')
    const panel = screen.getByRole('region', { name: 'Send invoice Draft #3' })
    const methods = within(panel).getByRole('radiogroup', { name: 'Delivery method' })
    expect(within(methods).getAllByRole('radio').map((r) => r.closest('label')?.querySelector('.send-methods__title')?.textContent)).toEqual([
      'Send by email', 'Send via Peppol', 'Send manually',
    ])
    expect(within(methods).getByRole('radio', { name: /Send by email/ })).toBeChecked()
    expect(within(methods).getByRole('radio', { name: /Send via Peppol/ })).toBeDisabled()
    expect(within(panel).getByLabelText('Email address')).toHaveValue('billing@example.com')
    // More addresses, with the plus beside the first.
    await userEvent.click(within(panel).getByRole('button', { name: 'Add email address' }))
    await userEvent.type(within(panel).getByLabelText('Email address 2'), 'boss@example.com')
    // The message starts as the workflow's email text; its placeholders are filled in when sent.
    const message = within(panel).getByLabelText('Email message')
    expect(message).toHaveValue('Dear {contact_name}, here is invoice {invoice_id}.')
    await userEvent.type(message, ' Thanks for your order')
    const ubl = within(panel).getByRole('switch', { name: /Attach UBL invoice/ })
    await waitFor(() => expect(ubl).toBeChecked())
    await userEvent.click(ubl)
    await userEvent.click(within(panel).getByRole('button', { name: 'Send invoice' }))
    expect(await screen.findByRole('heading', { level: 1, name: '2026-0002' })).toBeInTheDocument()
    expect(sent).toEqual({
      sales_invoice_sending: {
        delivery_method: 'Email', email_address: 'billing@example.com, boss@example.com',
        email_message: 'Dear {contact_name}, here is invoice {invoice_id}. Thanks for your order', deliver_ubl: false,
      },
    })
    expect(await stateLabel('Open')).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: /Send invoice/ })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(screen.getByRole('button', { name: 'Send invoice again' })).toBeInTheDocument()
  })

  it('starts the message with the default text in the language of the invoice', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/902')
    await choose('Send', 'Send invoice')
    expect(within(screen.getByRole('region', { name: 'Send invoice Draft #3' })).getByLabelText('Email message')).toHaveValue(
      'Beste {contact_name},\n\nIn de bijlage vind je factuur {invoice_id} van {invoice_date} voor {total}. We ontvangen de betaling graag uiterlijk {due_date} onder vermelding van {payment_reference}.\n\nMet vriendelijke groet,\n{company_name}',
    )
  })

  it('marks a draft as sent without sending it', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/sales_invoices/902/send_invoice', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(invoice({ id: '902', invoice_id: '2026-0002', state: 'open' }))
    }))
    renderAt('/11/sales_invoices/902')
    await choose('Send', 'Send invoice')
    const panel = screen.getByRole('region', { name: 'Send invoice Draft #3' })
    await userEvent.click(within(panel).getByRole('radio', { name: /Send manually/ }))
    expect(within(panel).queryByLabelText('Email address')).toBeNull()
    expect(within(panel).queryByLabelText('Email message')).toBeNull()
    await userEvent.click(within(panel).getByRole('button', { name: 'Send invoice' }))
    await waitFor(() => expect(sent).toEqual({ sales_invoice_sending: { delivery_method: 'Manual' } }))
  })

  it('edits a draft in its document, without an editor page of its own', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded(), ...creating(sent, draft))
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #3' })
    const doc = invoicePaper('Invoice Draft #3')
    // Nothing changed yet, so nothing to save.
    expect(screen.queryByRole('button', { name: 'Save invoice' })).toBeNull()
    await userEvent.click(within(doc).getByRole('button', { name: /payment conditions/i }))
    let card = within(doc).getByRole('region', { name: 'Payment conditions' })
    await userEvent.type(within(card).getByLabelText('Payment conditions'), 'Pay within 30 days.')
    // Cancel puts it back as it was.
    await userEvent.click(within(card).getByRole('button', { name: 'Cancel' }))
    expect(within(invoicePaper('Invoice Draft #3')).queryByText('Pay within 30 days.')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Save invoice' })).toBeNull()

    await userEvent.click(within(invoicePaper('Invoice Draft #3')).getByRole('button', { name: /payment conditions/i }))
    card = within(invoicePaper('Invoice Draft #3')).getByRole('region', { name: 'Payment conditions' })
    await userEvent.type(within(card).getByLabelText('Payment conditions'), 'Pay within 30 days.')
    await userEvent.click(within(card).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.patch).toMatchObject({ payment_conditions: 'Pay within 30 days.' }))
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/902$/)
    // The sidebar stays beside it.
    expect(sidebar()).toBeInTheDocument()
  })

  it('changes the layout of a draft from Extra, as Moneybird asks: the styles under a notice and Change layout', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded(), ...creating(sent, draft))
    server.use(http.get('*/api/v2/11/document_styles', () => HttpResponse.json([senderStyle, { ...senderStyle, id: '702', name: 'Modern', default: false, uri: '/11/document_styles/702' }])))
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #3' })
    await choose('Extra', 'Change layout')
    const layouts = await screen.findByRole('region', { name: 'Change layout' })
    expect(layouts).toHaveTextContent('The layout determines the style and sender address on the invoice. You can change the language on the invoice in the workflow.')
    expect(within(layouts).getByRole('radio', { name: 'Standaard' })).toBeChecked()
    await userEvent.click(within(layouts).getByRole('radio', { name: 'Modern' }))
    expect(sent.patch).toBeUndefined()
    await userEvent.click(within(layouts).getByRole('button', { name: 'Change layout' }))
    await waitFor(() => expect(sent.patch).toMatchObject({ document_style_id: '702' }))
    expect(screen.queryByRole('region', { name: 'Change layout' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Save invoice' })).toBeNull()
  })

  it("opens the editor's popup and details from the document of a draft", async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #3' })
    await userEvent.click(within(invoicePaper('Invoice Draft #3')).getByRole('button', { name: 'Contact' }))
    expect(await screen.findByRole('dialog', { name: 'Edit invoice' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/902$/)
  })

  it('opens the details editor from the lines of a draft', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #3' })
    await userEvent.click(within(invoicePaper('Invoice Draft #3')).getByRole('button', { name: /invoice details/ }))
    expect(await screen.findByRole('table', { name: 'Invoice details' })).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('warns that a sent invoice was sent before it is edited', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/901')
    await screen.findByRole('heading', { level: 1, name: '2026-0001' })
    await userEvent.click(within(invoicePaper('Invoice 2026-0001')).getByRole('button', { name: 'Edit invoice details' }))
    const warning = screen.getByRole('alertdialog', { name: 'This invoice has been sent' })
    expect(warning).toHaveTextContent('When you make changes, make sure to inform the recipient. Are you sure you want to edit this invoice?')
    // It points to the regular way first.
    expect(warning).toHaveTextContent('The regular way to correct a sent invoice is to cancel or refund it with a credit note.')
    expect(within(warning).getByRole('button', { name: 'Duplicate to credit note' })).toBeInTheDocument()
    await userEvent.click(within(warning).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()
    await userEvent.click(within(invoicePaper('Invoice 2026-0001')).getByRole('button', { name: 'Edit contact' }))
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'This invoice has been sent' })).getByRole('button', { name: 'Continue editing' }))
    expect(await screen.findByRole('dialog', { name: 'Edit invoice' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/901$/)
  })

  it("prints the details the layout puts beside the title, as Moneybird's document does", async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/document_styles', () => HttpResponse.json([{
      ...senderStyle, invoice_metadata_left: [{ field: 'reference' }, { field: 'customer_id' }],
    }])))
    renderAt('/11/sales_invoices/901')
    const doc = await screen.findByRole('document', { name: 'Invoice 2026-0001' })
    expect(within(doc).getByText('Order 12')).toBeInTheDocument()
    expect(await within(doc).findByText('Klantnummer:')).toBeInTheDocument()
    expect(within(doc).getByText('Klantnummer:').parentElement).toHaveTextContent('Klantnummer: 1')
  })

  it('prints the payment conditions with their tags filled in and their textile styled, as Moneybird does', async () => {
    const conditions = `*Let op:* ${defaultConditions}`
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice({ payment_conditions: conditions, payment_reference: 'RF130000000000000PV96X5KP' }))))
    renderAt('/11/sales_invoices/901')
    const doc = await screen.findByRole('document', { name: 'Invoice 2026-0001' })
    const text = await within(doc).findByText(/We verzoeken u vriendelijk/)
    expect(text).toHaveTextContent('Let op: We verzoeken u vriendelijk het bovenstaande bedrag van € 242,00 voor 15-11-2026 te voldoen op onze bankrekening onder vermelding van de omschrijving RF13PV96X5KP.')
    expect(within(text).getByText('Let op:').tagName).toBe('STRONG')
  })

  it("shows Moneybird's placeholder payment reference on a draft", async () => {
    server.use(...loaded())
    server.use(...creating({}, invoice({ ...draft, payment_conditions: 'Kenmerk {document.payment_reference}' })))
    renderAt('/11/sales_invoices/902')
    const doc = await screen.findByRole('document', { name: 'Invoice Draft #3' })
    expect(within(doc).getByText('Kenmerk RF94CONCEPT3')).toBeInTheDocument()
  })

  it('edits the payment conditions of a sent invoice from its document, after the warning', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/sales_invoices/901', async ({ request }) => {
      sent.patch = ((await request.json()) as { sales_invoice: Record<string, unknown> }).sales_invoice
      return HttpResponse.json(invoice({ payment_conditions: 'Pay {document.total_price}' }))
    }))
    renderAt('/11/sales_invoices/901')
    await screen.findByRole('heading', { level: 1, name: '2026-0001' })
    await userEvent.click(within(invoicePaper('Invoice 2026-0001')).getByRole('button', { name: 'Edit payment conditions' }))
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'This invoice has been sent' })).getByRole('button', { name: 'Continue editing' }))
    const card = await screen.findByRole('region', { name: 'Payment conditions' })
    const area = within(card).getByLabelText('Payment conditions')
    await userEvent.type(area, 'Pay ')
    await userEvent.selectOptions(within(card).getByRole('combobox', { name: 'Insert tag' }), 'Amount')
    expect(area).toHaveValue('Pay {document.total_price}')
    await userEvent.click(within(card).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.patch).toMatchObject({ payment_conditions: 'Pay {document.total_price}' }))
  })

  it('can still be edited and deleted with a payment registered, its payments going along as at Moneybird', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice({
      total_unpaid: '142.0',
      payments: [{ id: '981', administration_id: '11', invoice_type: 'SalesInvoice', invoice_id: '901', price: '100.0', price_base: '100.0', payment_date: '2026-11-05', manual_payment_action: 'cash_payment', created_at: stamp, updated_at: stamp }],
    }))))
    renderAt('/11/sales_invoices/901')
    await screen.findByRole('heading', { level: 1, name: '2026-0001' })
    expect(within(invoicePaper('Invoice 2026-0001')).getByRole('button', { name: 'Edit invoice details' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Register payment' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Extra' }))
    expect(screen.getByRole('button', { name: 'Delete invoice' })).toBeInTheDocument()
  })

  it('warns before a sent invoice is deleted, offering a credit note instead', async () => {
    let deleted = false
    let credited = false
    server.use(...loaded())
    server.use(
      http.delete('*/api/v2/11/sales_invoices/901', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
      http.patch('*/api/v2/11/sales_invoices/901/duplicate_creditinvoice', () => {
        credited = true
        return HttpResponse.json({ ...draft, id: '903', draft_id: 4, original_sales_invoice_id: '901', uri: '/11/sales_invoices/903' })
      }),
      http.get('*/api/v2/11/sales_invoices/903', () => HttpResponse.json({ ...draft, id: '903', draft_id: 4, original_sales_invoice_id: '901', uri: '/11/sales_invoices/903' })),
    )
    renderAt('/11/sales_invoices/901')
    await choose('Extra', 'Delete invoice')
    const warning = screen.getByRole('alertdialog', { name: 'Delete this sent invoice?' })
    expect(warning).toHaveTextContent('The regular way to correct a sent invoice is to cancel or refund it with a credit note.')
    await userEvent.click(within(warning).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()

    await choose('Extra', 'Delete invoice')
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'Delete this sent invoice?' })).getByRole('button', { name: 'Duplicate to credit note' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/903$/))
    expect(credited).toBe(true)
    expect(deleted).toBe(false)
  })

  it('deletes a sent invoice when told to anyway', async () => {
    let deleted = false
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/sales_invoices/901', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/sales_invoices/901')
    await choose('Extra', 'Delete invoice')
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'Delete this sent invoice?' })).getByRole('button', { name: 'Delete anyway' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices$/))
    expect(deleted).toBe(true)
  })

  it('sends a draft in the future, by the way the contact prefers', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/sales_invoices/902/send_invoice', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ ...draft, state: 'scheduled' })
    }))
    renderAt('/11/sales_invoices/902')
    await choose('Send', 'Send in future')
    const panel = screen.getByRole('region', { name: 'Send invoice in the future' })
    expect(panel).toHaveTextContent('The invoice will be sent automatically on the invoice date.')
    expect(panel).toHaveTextContent('The contact prefers to receive the invoice per email at billing@example.com.')
    const date = within(panel).getByLabelText('Send invoice on')
    expect(date).toHaveAttribute('min', (date as HTMLInputElement).value)
    await userEvent.clear(date)
    await userEvent.type(date, '2030-01-15')
    await userEvent.click(within(panel).getByRole('checkbox', { name: /Send one invoice to this contact/ }))
    await userEvent.click(within(panel).getByRole('button', { name: 'Send invoice' }))
    expect(await stateLabel('Scheduled')).toBeInTheDocument()
    expect(sent).toEqual({ sales_invoice_sending: { sending_scheduled: true, invoice_date: '2030-01-15', mergeable: true } })
  })

  it("shows the invoice's journal entries, by document", async () => {
    const seen: URL[] = []
    const paid = invoice({
      state: 'paid', total_unpaid: '0.0',
      payments: [{ id: '981', administration_id: '11', invoice_type: 'SalesInvoice', invoice_id: '901', price: '242.0', price_base: '242.0', payment_date: '2026-11-11', manual_payment_action: 'bank_transfer', created_at: stamp, updated_at: stamp }],
    })
    const entry = (id: string, document_type: 'SalesInvoice' | 'Payment', document_id: string, date: string, ledger_account_id: string, amount: string, tax_rate_id: string | null = null) =>
      ({ id, date, document_type, document_id, ledger_account_id, amount, tax_rate_id, contact_id: '701', project_id: null, financial_account_id: null, description: null })
    const debtors: LedgerAccount = { ...sales, id: '1300', name: 'Accounts receivable', account_type: 'current_assets', uri: '/11/ledger_accounts/1300' }
    const vat: LedgerAccount = { ...sales, id: '1500', name: 'Payable VAT', account_type: 'current_liabilities', uri: '/11/ledger_accounts/1500' }
    const bank: LedgerAccount = { ...sales, id: '1100', name: 'Bank', account_type: 'current_assets', uri: '/11/ledger_accounts/1100' }
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(paid)),
      http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales, debtors, vat, bank])),
      http.get('*/api/v2/11/reports/journal_entries', ({ request }) => {
        const url = new URL(request.url)
        seen.push(url)
        const id = url.searchParams.get('document_id')
        return HttpResponse.json(id === '901'
          ? [entry('1', 'SalesInvoice', '901', '2026-11-01', '1300', '242.0'), entry('2', 'SalesInvoice', '901', '2026-11-01', '8001', '-200.0', '2101'), entry('3', 'SalesInvoice', '901', '2026-11-01', '1500', '-42.0', '2101')]
          : [entry('4', 'Payment', '981', '2026-11-11', '1100', '242.0'), entry('5', 'Payment', '981', '2026-11-11', '1300', '-242.0')])
      }),
    )
    renderAt('/11/sales_invoices/901')
    const button = await screen.findByRole('button', { name: 'Journal entries' })
    await userEvent.click(button)
    expect(button).toHaveAttribute('aria-pressed', 'true')
    const panel = screen.getByRole('region', { name: 'Journal entries' })
    expect(panel).toHaveTextContent('In the background, MoneyHarpy has created the following bookings, also called Journal Entries.')
    const table = await within(panel).findByRole('table', { name: 'Journal entries' })
    expect(within(within(table).getAllByRole('row')[0]!).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Category', 'Debit', 'Credit', 'VAT'])
    await waitFor(() => expect(within(table).getAllByRole('row').map((r) => r.textContent)).toEqual([
      'CategoryDebitCreditVAT',
      'Invoice - 2026-11-01',
      'Accounts receivable (Balance sheet)242.00',
      'Sales (Profit and loss statement)200.0021% btw',
      'Payable VAT (Balance sheet)42.0021% btw',
      'Payment - 2026-11-11',
      'Bank (Balance sheet)242.00',
      'Accounts receivable (Balance sheet)242.00',
    ]))
    expect(seen.map((u) => `${u.searchParams.get('document_type')}:${u.searchParams.get('document_id')}`).sort()).toEqual(['Payment:981', 'SalesInvoice:901'])
    expect(seen.every((u) => !u.searchParams.has('period'))).toBe(true)
    await userEvent.click(button)
    expect(screen.queryByRole('region', { name: 'Journal entries' })).toBeNull()
  })

  it('explains when the email could not be sent', async () => {
    server.use(...loaded(), http.patch('*/api/v2/11/sales_invoices/902/send_invoice', () =>
      HttpResponse.json({ error: { base: ['The invoice is booked but could not be emailed; check the mail settings and send it again'] } }, { status: 422 })))
    renderAt('/11/sales_invoices/902')
    await choose('Send', 'Send invoice')
    await userEvent.click(within(screen.getByRole('region', { name: 'Send invoice Draft #3' })).getByRole('button', { name: 'Send invoice' }))
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
    await choose('Extra', 'Duplicate to credit note')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/sales_invoices/903'))
    expect(called).toBe(true)
    expect(await screen.findByRole('heading', { level: 1, name: 'Draft #4' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'the original invoice' })).toHaveAttribute('href', '/11/sales_invoices/901')
  })

  it('duplicates an invoice into a new draft and opens it in the editor', async () => {
    let posted: Record<string, unknown> | undefined
    server.use(...loaded())
    server.use(
      http.post('*/api/v2/11/sales_invoices', async ({ request }) => {
        posted = ((await request.json()) as { sales_invoice: Record<string, unknown> }).sales_invoice
        return HttpResponse.json({ ...draft, id: '904', draft_id: 5, uri: '/11/sales_invoices/904' }, { status: 201 })
      }),
      http.get('*/api/v2/11/sales_invoices/904', () => HttpResponse.json({ ...draft, id: '904', draft_id: 5, uri: '/11/sales_invoices/904' })),
    )
    renderAt('/11/sales_invoices/901')
    await choose('Extra', 'Duplicate invoice')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/904$/))
    expect(posted).toEqual({
      contact_id: '701', reference: 'Order 12', workflow_id: '601', document_style_id: '701', prices_are_incl_tax: false, payment_conditions: '',
      details_attributes: [{ description: 'Consulting', amount: '2', price: '100.0', tax_rate_id: '2101', ledger_account_id: '8001', row_order: 1 }],
    })
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
    await choose('Extra', 'Pause reminders')
    expect(await screen.findByText('Reminders for this invoice are paused.')).toBeInTheDocument()
    await choose('Extra', 'Resume reminders')
    await waitFor(() => expect(screen.queryByText('Reminders for this invoice are paused.')).toBeNull())
  })

  it('deletes a draft and returns to the list', async () => {
    server.use(...loaded(), http.delete('*/api/v2/11/sales_invoices/902', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/sales_invoices/902')
    expect(await screen.findByRole('heading', { level: 1, name: 'Draft #3' })).toBeInTheDocument()
    await choose('Extra', 'Delete invoice')
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'Delete this draft invoice? This cannot be undone.' })).getByRole('button', { name: 'Delete permanently' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices$/))
  })

  it('warns before a paid invoice is edited, pointing to a refund', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice({ state: 'paid', total_unpaid: '0.0', paid_at: '2026-11-05' }))))
    renderAt('/11/sales_invoices/901')
    expect(await stateLabel('Paid')).toBeInTheDocument()
    const doc = invoicePaper('Invoice 2026-0001')
    await userEvent.click(within(doc).getByRole('button', { name: 'Edit invoice details' }))
    const warning = screen.getByRole('alertdialog', { name: 'This invoice has been paid' })
    expect(warning).toHaveTextContent('A refund is registered as the payment of a credit note.')
    expect(within(warning).getByRole('button', { name: 'Continue editing' })).toBeInTheDocument()
    await userEvent.click(within(warning).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('link', { name: 'Register payment' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Download as...' }))
    expect(screen.getByText('Payment receipt (PDF)').closest('.dropdown__item')).toHaveAttribute('aria-disabled', 'true')
    await userEvent.click(screen.getByRole('button', { name: 'Extra' }))
    expect(screen.queryByRole('button', { name: 'Pause reminders' })).toBeNull()
  })

  it('keeps notes and to-dos', async () => {
    const sent: unknown[] = []
    server.use(
      ...loaded(),
      http.get('*/api/v2/11/users', () => HttpResponse.json([{ id: '1', name: 'Ann de Vries', email: 'ann@example.com' }])),
      http.post('*/api/v2/11/sales_invoices/901/notes', async ({ request }) => {
        sent.push(await request.json())
        return HttpResponse.json({}, { status: 201 })
      }),
    )
    renderAt('/11/sales_invoices/901')
    const notes = await screen.findByRole('region', { name: 'Notes and tasks' })
    const text = within(notes).getByRole('textbox', { name: 'Add a note or to-do' })
    expect(text).toHaveAttribute('placeholder', 'Add')
    expect(within(notes).queryByRole('button', { name: 'Save' })).toBeNull()
    await userEvent.type(text, 'Called about payment')
    await userEvent.selectOptions(within(notes).getByLabelText('Note or to-do'), 'To-do')
    await userEvent.click(within(notes).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual([{ note: { note: 'Called about payment', todo: true } }]))
    // Enter saves too; a to-do can be for someone.
    await userEvent.type(text, 'Check the bank')
    await userEvent.selectOptions(within(notes).getByLabelText('Note or to-do'), 'To-do for Ann de Vries')
    await userEvent.type(text, '{Enter}')
    await waitFor(() => expect(sent).toHaveLength(2))
    expect(sent[1]).toEqual({ note: { note: 'Check the bank', todo: true, assignee_id: '1' } })
  })

  it('removes a note from the history', async () => {
    let removed = false
    const note = { id: '971', administration_id: '11', entity_id: '901', entity_type: 'SalesInvoice', note: 'Called about payment', todo: true, created_at: '2026-11-11T09:30:00Z' }
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice({ notes: removed ? [] : [note] }))),
      http.delete('*/api/v2/11/sales_invoices/901/notes/971', () => {
        removed = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/sales_invoices/901')
    const notes = await screen.findByRole('region', { name: 'Notes and tasks' })
    const row = (await within(notes).findByText('Called about payment')).closest('li')!
    expect(row).toHaveTextContent('To-do')
    await userEvent.click(within(row).getByRole('button', { name: 'Show/hide details' }))
    await userEvent.click(within(row).getByRole('button', { name: 'Delete note' }))
    await userEvent.click(within(row).getByRole('button', { name: 'Confirm delete note' }))
    await waitFor(() => expect(removed).toBe(true))
    await waitFor(() => expect(within(notes).queryByText('Called about payment')).toBeNull())
  })

  it('uploads, lists and removes attachments under the document', async () => {
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
    expect(block).toHaveTextContent('Drag files here to add or select files (Maximum 10 MB total)')
    await userEvent.upload(within(block).getByLabelText('select files'), new File(['%PDF-1.4'], 'order.pdf', { type: 'application/pdf' }))
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

  it('refuses attachments over 10 MB in all', async () => {
    let posted = false
    server.use(...loaded(), http.post('*/api/v2/11/sales_invoices/901/attachments', () => {
      posted = true
      return HttpResponse.json({})
    }))
    renderAt('/11/sales_invoices/901')
    const block = await screen.findByRole('region', { name: 'Attachments' })
    const big = new File(['x'], 'scan.pdf', { type: 'application/pdf' })
    Object.defineProperty(big, 'size', { value: 11 * 1024 * 1024 })
    await userEvent.upload(within(block).getByLabelText('select files'), big)
    expect(await within(block).findByText('This attachment exceeds 10 MB. A maximum of 10 MB is permitted.')).toBeInTheDocument()
    expect(posted).toBe(false)
  })
})
