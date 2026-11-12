import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { contactCounts, creating, detailRows, draft, editLines, editorCard, empty, invoice, invoicePaper, loaded, paper, pickContact, popup, sales } from './testkit'

describe('the invoice details editor', () => {
  it('adds details with their totals, discount and VAT, after Moneybird', async () => {
    const sent: { post?: Record<string, unknown>; patch?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(...creating(sent))
    renderAt('/11/sales_invoices/new')
    await pickContact()
    await userEvent.type(within(popup()).getByLabelText('Reference'), 'Order 12')
    // "Invoice details" saves the draft and opens its details.
    await userEvent.click(within(popup()).getByRole('button', { name: 'Invoice details' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/902$/))
    expect(sent.post).toMatchObject({ contact_id: '701', reference: 'Order 12' })

    const table = await screen.findByRole('table', { name: 'Invoice details' })
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['', 'Amount', 'Description', 'Amount', 'Total', 'VAT rate and category', ''])
    let rows = detailRows()
    expect(rows).toHaveLength(1)
    const amount = within(rows[0]!).getByLabelText('Amount')
    expect(amount).toHaveValue('1 x')
    expect(amount).toHaveAttribute('placeholder', '#')
    await userEvent.clear(amount)
    await userEvent.type(amount, '2 x')
    await userEvent.type(within(rows[0]!).getByLabelText('Description'), 'Consulting')
    expect(within(rows[0]!).getByLabelText('Price')).toHaveAttribute('placeholder', '0.00')
    await userEvent.type(within(rows[0]!).getByLabelText('Price'), '100')
    expect(within(rows[0]!).getByLabelText('VAT rate')).toHaveValue('2101')
    expect(within(within(rows[0]!).getByLabelText('VAT rate')).queryByRole('option', { name: 'Old rate' })).toBeNull()
    expect(within(rows[0]!).getByTestId('detail-total')).toHaveTextContent('€200.00')

    await userEvent.click(screen.getByRole('button', { name: 'Add detail' }))
    rows = detailRows()
    await userEvent.type(within(rows[1]!).getByLabelText('Description'), 'Book')
    await userEvent.type(within(rows[1]!).getByLabelText('Price'), '50')
    await userEvent.selectOptions(within(rows[1]!).getByLabelText('VAT rate'), '2109')
    await userEvent.selectOptions(within(rows[1]!).getByLabelText('Category'), '8001')

    await userEvent.click(screen.getByRole('button', { name: 'Discount' }))
    await userEvent.click(screen.getByRole('button', { name: 'Discount over total price' }))
    await userEvent.type(screen.getByRole('textbox', { name: 'Discount' }), '10')

    const totals = within(editorCard()).getByRole('table', { name: 'Totals' })
    expect(within(totals).getByRole('row', { name: /^Subtotal/ })).toHaveTextContent('€250.00')
    expect(within(totals).getByRole('row', { name: /Discount/ })).toHaveTextContent('-€25.00')
    expect(within(totals).getByRole('row', { name: /21% btw/ })).toHaveTextContent('€37.80')
    expect(within(totals).getByRole('row', { name: /9% btw/ })).toHaveTextContent('€4.05')
    expect(within(totals).getByRole('row', { name: /^Total/ })).toHaveTextContent('€266.85')

    await userEvent.click(screen.getByRole('button', { name: 'Save invoice' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/902$/))
    expect(sent.patch).toMatchObject({ contact_id: '701', reference: 'Order 12', discount: '10', prices_are_incl_tax: false })
    expect(sent.patch?.details_attributes).toEqual([
      { description: 'Consulting', amount: '2 x', price: '100', tax_rate_id: '2101', row_order: 1 },
      { description: 'Book', amount: '1 x', price: '50', tax_rate_id: '2109', ledger_account_id: '8001', row_order: 2 },
    ])
  })

  it('fills a detail from a product, sets its period and applies it to the details below', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    const product = {
      id: '301', administration_id: '11', title: 'Advice', description: 'Advice per hour', price: '80.0', currency: 'EUR',
      tax_rate_id: '2109', ledger_account_id: '8001', active: true, uri: '/11/products/301',
    }
    server.use(...loaded())
    server.use(
      ...creating(sent),
      http.get('*/api/v2/11/products', ({ request }) =>
        HttpResponse.json(new URL(request.url).searchParams.get('query') === 'adv' ? [product] : [], { headers: { 'X-Total-Count': '1' } })),
    )
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #12' })
    await editLines()
    await userEvent.click(screen.getByRole('button', { name: 'Add detail' }))
    await userEvent.click(screen.getByRole('button', { name: 'Add detail' }))
    let rows = detailRows()
    const description = within(rows[0]!).getByRole('combobox', { name: 'Description' })
    await userEvent.type(description, 'adv')
    await userEvent.click(await screen.findByRole('option', { name: /Advice per hour/ }))
    expect(description).toHaveValue('Advice per hour')
    expect(within(rows[0]!).getByLabelText('Price')).toHaveValue('80')
    expect(within(rows[0]!).getByLabelText('VAT rate')).toHaveValue('2109')
    expect(within(rows[0]!).getByLabelText('Category')).toHaveValue('8001')

    // The period, from a preset.
    const period = within(rows[0]!).getByRole('button', { name: 'Period: Not selected' })
    await userEvent.click(period)
    await userEvent.click(screen.getByRole('button', { name: 'This month' }))
    expect(within(rows[0]!).getByRole('button', { name: 'Period: 11/01/2026 – 11/30/2026' })).toBeInTheDocument()

    // A changed period, VAT rate or category can be given to the rows below.
    await userEvent.click(within(rows[0]!).getByRole('button', { name: 'Apply to details below' }))
    rows = detailRows()
    expect(within(rows[2]!).getByRole('button', { name: 'Period: 11/01/2026 – 11/30/2026' })).toBeInTheDocument()
    expect(within(rows[0]!).queryByRole('button', { name: 'Apply to details below' })).toBeNull()
    expect(within(rows[1]!).getByLabelText('VAT rate')).toHaveValue('2101')
    await userEvent.selectOptions(within(rows[0]!).getByLabelText('VAT rate'), '2101')
    await userEvent.selectOptions(within(rows[0]!).getByLabelText('VAT rate'), '2109')
    await userEvent.click(within(rows[0]!).getByRole('button', { name: 'Apply to details below' }))
    expect(within(rows[1]!).getByLabelText('VAT rate')).toHaveValue('2109')
    expect(within(rows[2]!).getByLabelText('VAT rate')).toHaveValue('2109')

    // A discount detail, then moved to the top with the keyboard.
    await userEvent.click(screen.getByRole('button', { name: 'Discount' }))
    await userEvent.click(screen.getByRole('button', { name: 'Add discount detail' }))
    rows = detailRows()
    expect(within(rows[3]!).getByLabelText('Description')).toHaveValue('Discount')
    await userEvent.type(within(rows[3]!).getByLabelText('Price'), '-10')
    const grip = within(rows[3]!).getByRole('button', { name: 'Move row 4' })
    grip.focus()
    await userEvent.keyboard('{ArrowUp}{ArrowUp}{ArrowUp}')
    rows = detailRows()
    expect(within(rows[0]!).getByLabelText('Description')).toHaveValue('Discount')
    expect(within(rows[0]!).getByRole('button', { name: 'Move row 1' })).toHaveFocus()
    await userEvent.click(within(rows[3]!).getByRole('button', { name: 'Remove row 4' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save invoice' }))
    await waitFor(() => expect(sent.patch).toBeDefined())
    expect(sent.patch?.details_attributes).toEqual([
      { description: 'Discount', amount: '1 x', price: '-10', tax_rate_id: '2109', row_order: 1 },
      { description: 'Advice per hour', amount: '1 x', price: '80', tax_rate_id: '2109', ledger_account_id: '8001', product_id: '301', period: '20261101..20261130', row_order: 2 },
    ])
  })

  it("starts a new detail with the VAT rate and category of the one above, and names categories without their code", async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: /Draft/ })
    await editLines()
    const first = detailRows()[0]!
    await userEvent.selectOptions(within(first).getByLabelText('VAT rate'), '2109')
    await userEvent.selectOptions(within(first).getByLabelText('Category'), '8001')
    expect(within(within(first).getByLabelText('Category')).getByRole('option', { name: 'Sales' })).toBeInTheDocument()
    expect(within(within(first).getByLabelText('Category')).queryByRole('option', { name: /8000/ })).toBeNull()

    await userEvent.click(screen.getByRole('button', { name: 'Add detail' }))
    const added = detailRows().at(-1)!
    expect(within(added).getByLabelText('VAT rate')).toHaveValue('2109')
    expect(within(added).getByLabelText('Category')).toHaveValue('8001')
  })

  it('cancels the changes to the details and closes the editor, as Moneybird does', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: /Draft/ })
    await editLines()
    const description = within(detailRows()[0]!).getByLabelText('Description')
    await userEvent.clear(description)
    await userEvent.type(description, 'Something else')
    await userEvent.click(screen.getByRole('button', { name: 'Add detail' }))
    await userEvent.click(within(editorCard()).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('table', { name: 'Invoice details' })).toBeNull()
    expect(within(paper()).getByText('Consulting')).toBeInTheDocument()
    expect(within(paper()).queryByText('Something else')).toBeNull()
  })

  it('shows what is wrong beside the fields', async () => {
    server.use(...loaded())
    server.use(...creating({}))
    server.use(http.patch('*/api/v2/11/sales_invoices/902', () => HttpResponse.json({ error: { 'details_attributes.0.price': ['is not an amount'] } }, { status: 422 })))
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #12' })
    await editLines()
    await userEvent.type(within(detailRows()[0]!).getByLabelText('Description'), 'Consulting')
    await userEvent.click(screen.getByRole('button', { name: 'Save invoice' }))
    expect(await screen.findByText('Price on line 1 is not an amount')).toBeInTheDocument()
    expect(within(detailRows()[0]!).getByLabelText('Price')).toHaveAttribute('aria-invalid', 'true')
  })

  it('edits the dates and the payment conditions in place', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(...creating(sent))
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #12' })

    await userEvent.click(within(paper()).getByRole('button', { name: /Factuurdatum/ }))
    const date = within(paper()).getByLabelText('Invoice date')
    expect(date).toHaveFocus()
    await userEvent.type(date, '2026-12-01')
    await userEvent.keyboard('{Enter}')
    expect(within(paper()).getByRole('button', { name: /Factuurdatum/ })).toHaveTextContent('01-12-2026')
    expect(within(paper()).getByRole('button', { name: /Vervaldatum/ })).toHaveTextContent('15-12-2026')

    await userEvent.click(within(paper()).getByRole('button', { name: /Vervaldatum/ }))
    const due = within(paper()).getByLabelText('Due date')
    await userEvent.clear(due)
    await userEvent.type(due, '2026-12-31')
    await userEvent.keyboard('{Enter}')
    expect(within(paper()).getByRole('button', { name: /Vervaldatum/ })).toHaveTextContent('31-12-2026')

    // As on Moneybird, the payment conditions open a card with the textile editor, its workflow and Save.
    await userEvent.click(within(paper()).getByRole('button', { name: /payment conditions/i }))
    const card = within(paper()).getByRole('region', { name: 'Payment conditions' })
    expect(within(card).getByRole('link', { name: 'Edit in workflow Standaard (NL)' })).toHaveAttribute('href', '/11/workflows/601?return_to=%2F11%2Fsales_invoices%2F902')
    await userEvent.type(within(card).getByLabelText('Payment conditions'), 'Pay within 30 days.')
    const save = within(card).getByRole('button', { name: 'Save' })
    expect(save).toHaveClass('btn--primary')
    await userEvent.click(save)
    await waitFor(() => expect(sent.patch).toMatchObject({ invoice_date: '2026-12-01', first_due_interval: 30, payment_conditions: 'Pay within 30 days.' }))
    await waitFor(() => expect(within(paper()).queryByRole('region', { name: 'Payment conditions' })).toBeNull())
  })

  it("starts a new invoice with its workflow's payment conditions, filled in on the preview", async () => {
    const sent: { post?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(...creating(sent))
    renderAt('/11/sales_invoices/new')
    await pickContact()
    const text = within(paper()).getByText(/We verzoeken u vriendelijk/)
    expect(text).toHaveTextContent(/het bovenstaande bedrag van € 0,00 voor \d{2}-\d{2}-\d{4} te voldoen .* omschrijving RF\d{2}CONCEPT12\./)
    await userEvent.click(within(popup()).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.post).toBeDefined())
    // The server gives the draft its workflow's text.
    expect(sent.post).not.toHaveProperty('payment_conditions')
  })

  it('saves the invoice with the blue Save of the details editor and closes it', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(...creating(sent))
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #12' })
    await editLines()
    await userEvent.type(within(detailRows()[0]!).getByLabelText('Description'), 'Consulting')
    await userEvent.type(within(detailRows()[0]!).getByLabelText('Price'), '100')
    expect(within(editorCard()).queryByRole('button', { name: 'Done' })).toBeNull()
    const save = within(editorCard()).getByRole('button', { name: 'Save' })
    expect(save).toHaveClass('btn--primary')
    expect(within(editorCard()).getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
    await userEvent.click(save)
    await waitFor(() => expect(sent.patch?.details_attributes).toEqual([expect.objectContaining({ description: 'Consulting', price: '100' })]))
    await waitFor(() => expect(screen.queryByRole('table', { name: 'Invoice details' })).toBeNull())
  })

  it('takes VAT out of prices that include it', async () => {
    server.use(...loaded())
    server.use(...creating({}, { ...empty, prices_are_incl_tax: true }))
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #12' })
    await editLines()
    await userEvent.type(within(detailRows()[0]!).getByLabelText('Price'), '121')
    const totals = within(editorCard()).getByRole('table', { name: 'Totals' })
    expect(within(totals).getByRole('row', { name: /21% btw/ })).toHaveTextContent('€21.00')
    expect(within(totals).getByRole('row', { name: /^Total/ })).toHaveTextContent('€121.00')
  })

  it('sets whether prices include VAT and the currency above the details, after Moneybird', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(...creating(sent))
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #12' })
    await editLines()
    await userEvent.type(within(detailRows()[0]!).getByLabelText('Description'), 'Consulting')
    await userEvent.type(within(detailRows()[0]!).getByLabelText('Price'), '121')
    const totals = () => within(editorCard()).getByRole('table', { name: 'Totals' })
    expect(within(totals()).getByRole('row', { name: /^Total/ })).toHaveTextContent('€146.41')

    const prices = within(editorCard()).getByRole('radiogroup', { name: 'Prices are' })
    expect(within(prices).getByRole('radio', { name: 'Excluding VAT' })).toBeChecked()
    await userEvent.click(within(prices).getByRole('radio', { name: 'Including VAT' }))
    expect(within(prices).getByRole('radio', { name: 'Including VAT' })).toBeChecked()
    expect(within(totals()).getByRole('row', { name: /21% btw/ })).toHaveTextContent('€21.00')
    expect(within(totals()).getByRole('row', { name: /^Total/ })).toHaveTextContent('€121.00')

    const currency = within(editorCard()).getByRole('combobox', { name: 'Currency' })
    expect(currency).toHaveValue('EUR')
    const options = within(currency).getAllByRole('option')
    expect(options.slice(0, 5).map((o) => o.textContent)).toEqual(['EUR - Euro', 'USD - United States Dollar', 'GBP - British Pound', '----------', 'AED - United Arab Emirates Dirham'])
    expect(options[3]).toBeDisabled()
    expect(options.at(-1)).toHaveTextContent('ZWL - Zimbabwean Dollar')
    expect(within(currency).getByRole('option', { name: 'XAU - Gold (Troy Ounce)' })).toBeInTheDocument()
    // The popular ones are not listed twice.
    expect(within(currency).getAllByRole('option', { name: 'EUR - Euro' })).toHaveLength(1)
    await userEvent.selectOptions(currency, 'USD')
    expect(within(detailRows()[0]!).getByTestId('detail-total')).toHaveTextContent('$121.00')
    expect(within(totals()).getByRole('row', { name: /^Total/ })).toHaveTextContent('$121.00')

    await userEvent.click(screen.getByRole('button', { name: 'Save invoice' }))
    await waitFor(() => expect(sent.patch).toMatchObject({ currency: 'USD', prices_are_incl_tax: true }))
  })

  it("prints the document's amounts in the invoice's currency", async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/sales_invoices/902', () => HttpResponse.json({ ...draft, currency: 'USD' })))
    renderAt('/11/sales_invoices/902')
    const doc = await screen.findByRole('document', { name: 'Invoice Draft #3' })
    const totals = within(doc).getByRole('table', { name: 'Totals' })
    expect(within(totals).getByRole('row', { name: /^Totaal/ })).toHaveTextContent('US$ 242,00')
    await editLines()
    expect(within(editorCard()).getByRole('combobox', { name: 'Currency' })).toHaveValue('USD')
  })

  it('keeps the currency of a sent invoice, as Moneybird does', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/901')
    await screen.findByRole('heading', { level: 1, name: '2026-0001' })
    await userEvent.click(within(invoicePaper('Invoice 2026-0001')).getByRole('button', { name: 'Edit invoice details' }))
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'This invoice has been sent' })).getByRole('button', { name: 'Continue editing' }))
    expect(await within(editorCard()).findByRole('combobox', { name: 'Currency' })).toBeDisabled()
    expect(within(editorCard()).getByRole('radio', { name: 'Including VAT' })).toBeEnabled()
  })

  it('reopens the popup as "Edit invoice" from the document of a draft, and saves it', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded(), ...contactCounts)
    server.use(...creating(sent))
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #12' })
    expect(screen.queryByRole('dialog')).toBeNull()
    await userEvent.click(within(paper()).getByRole('button', { name: 'Contact' }))
    const dialog = screen.getByRole('dialog', { name: 'Edit invoice' })
    expect(within(dialog).queryByText('Add the basic details to your invoice. Select the contact you are invoicing.')).toBeNull()
    expect(within(dialog).getByRole('note')).toHaveTextContent('The invoice uses workflow Standaard (NL) and expires after 14 days.')
    // The contact shows its hover card.
    const contact = within(dialog).getByRole('combobox', { name: 'Contact' })
    expect(contact).toHaveTextContent('Example Shop B.V.')
    await userEvent.hover(contact)
    expect(await screen.findByRole('dialog', { name: 'Example Shop B.V.' })).toBeInTheDocument()
    await userEvent.unhover(contact)
    // No layouts nor prices including VAT: the details editor and Change layout set those.
    await userEvent.click(within(dialog).getByRole('button', { name: 'Edit' }))
    expect(within(dialog).getByLabelText('Workflow')).toHaveValue('601')
    expect(within(dialog).queryByRole('radiogroup', { name: 'Layout' })).toBeNull()
    expect(within(dialog).queryByRole('checkbox', { name: 'Prices include VAT' })).toBeNull()
    expect(within(dialog).getByLabelText('Reference')).toHaveValue('Order 12')
    await userEvent.clear(within(dialog).getByLabelText('Reference'))
    await userEvent.type(within(dialog).getByLabelText('Reference'), 'Order 13')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.patch).toMatchObject({ reference: 'Order 13' }))
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/902$/)
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    // The heading opens it too.
    await userEvent.click(within(paper()).getByRole('button', { name: 'Invoice settings' }))
    expect(popup()).toBeInTheDocument()
    await userEvent.click(within(popup()).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('dialog')).toBeNull()
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
    // The old address of the editor leads to the invoice, which is edited in place after the warning.
    renderAt('/11/sales_invoices/901/edit')
    expect(await screen.findByRole('heading', { level: 1, name: '2026-0001' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/901$/)
    const doc = () => invoicePaper('Invoice 2026-0001')
    await userEvent.click(within(doc()).getByRole('button', { name: 'Edit invoice details' }))
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'This invoice has been sent' })).getByRole('button', { name: 'Continue editing' }))
    expect(await within(doc()).findByRole('button', { name: 'Contact' })).toHaveTextContent('Example Shop B.V.')
    expect(within(doc()).getByRole('heading', { level: 2 })).toHaveTextContent('Factuur 2026-0001')
    const price = within(detailRows()[0]!).getByLabelText('Price')
    expect(price).toHaveValue('100')
    await userEvent.clear(price)
    await userEvent.type(price, '120')
    await userEvent.click(screen.getByRole('button', { name: 'Remove row 2' }))
    expect(within(within(editorCard()).getByRole('table', { name: 'Totals' })).getByRole('row', { name: /^Total/ })).toHaveTextContent('€290.40')
    await userEvent.click(screen.getByRole('button', { name: 'Save invoice' }))
    // Saved, the document is read again.
    expect(await within(doc()).findByRole('button', { name: 'Edit contact' })).toBeInTheDocument()
    expect(screen.queryByRole('table', { name: 'Invoice details' })).toBeNull()
    expect(sent.sales_invoice).toMatchObject({ contact_id: '701', reference: 'Order 12', workflow_id: '601', first_due_interval: 14 })
    expect(sent.sales_invoice?.details_attributes).toEqual([
      { id: '951', description: 'Consulting', amount: '2', price: '120', tax_rate_id: '2101', ledger_account_id: '8001', row_order: 1 },
      { id: '952', _destroy: true },
    ])
  })

  it('saves the draft before "Add Category..." opens Add category in this tab, which comes back to the invoice', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(
      ...creating(sent, draft),
      http.get('*/api/v2/11/bookkeeping_settings', () => HttpResponse.json({ chart_scheme: '', chart_schemes: [] })),
      http.get('*/api/v2/11/ledger_account_templates', () => HttpResponse.json([])),
    )
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: /^Invoice Draft #3$/ })
    await editLines()
    const row = detailRows()[0]!
    await userEvent.clear(within(row).getByLabelText('Description'))
    await userEvent.type(within(row).getByLabelText('Description'), 'Coaching')
    await userEvent.selectOptions(within(row).getByLabelText('Category'), 'Add Category...')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/ledger_accounts\/blueprints$/))
    // Moneybird's new_entity_field names the field that asked, so the new category lands there.
    const query = new URLSearchParams(screen.getByTestId('search').textContent ?? '')
    expect(query.get('new_entity_field')).toBe('sales_invoice[details_attributes][0][ledger_account_id]')
    expect(query.get('return_to')).toBe('/11/sales_invoices/902')
    expect(sent.patch?.details_attributes).toEqual([expect.objectContaining({ description: 'Coaching' })])
    expect(await screen.findByRole('heading', { name: 'Add category for Invoice', level: 1 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'To invoice' })).toHaveAttribute('href', '/11/sales_invoices/902')
  })

  it('saves the draft before "Add VAT rate..." opens Add VAT rate in this tab, as Moneybird does', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(...creating(sent, draft), http.get('*/api/v2/11/bookkeeping_settings', () => HttpResponse.json({ chart_scheme: '', chart_schemes: [], tax_report_boxes: [] })))
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: /^Invoice Draft #3$/ })
    await editLines()
    const row = detailRows()[0]!
    await userEvent.clear(within(row).getByLabelText('Description'))
    await userEvent.type(within(row).getByLabelText('Description'), 'Coaching')
    await userEvent.selectOptions(within(row).getByLabelText('VAT rate'), 'Add VAT rate...')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/tax_rates\/new$/))
    const query = new URLSearchParams(screen.getByTestId('search').textContent ?? '')
    expect(query.get('new_entity_field')).toBe('sales_invoice[details_attributes][0][tax_rate_id]')
    expect(query.get('return_to')).toBe('/11/sales_invoices/902')
    expect(sent.patch?.details_attributes).toEqual([expect.objectContaining({ description: 'Coaching' })])
  })

  it('puts the category added for a row on that row when it comes back, as Moneybird does', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales, { ...sales, id: '8002', name: 'Coaching', account_id: '8010', uri: '/11/ledger_accounts/8002' }])))
    renderAt({ pathname: '/11/sales_invoices/902', state: { details: true, newEntity: { field: 'sales_invoice[details_attributes][0][ledger_account_id]', id: '8002' } } })
    await waitFor(() => expect(within(detailRows()[0]!).getByLabelText('Category')).toHaveValue('8002'))
  })

  it('puts the VAT rate added for a row on that row when it comes back', async () => {
    server.use(...loaded())
    renderAt({ pathname: '/11/sales_invoices/902', state: { details: true, newEntity: { field: 'sales_invoice[details_attributes][0][tax_rate_id]', id: '2109' } } })
    await waitFor(() => expect(within(detailRows()[0]!).getByLabelText('VAT rate')).toHaveValue('2109'))
  })

  it('opens the details of a sent invoice again when it comes back from adding a category', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales, { ...sales, id: '8002', name: 'Coaching', uri: '/11/ledger_accounts/8002' }])))
    renderAt({ pathname: '/11/sales_invoices/901', state: { details: true, newEntity: { field: 'sales_invoice[details_attributes][0][ledger_account_id]', id: '8002' } } })
    await waitFor(() => expect(within(detailRows()[0]!).getByLabelText('Category')).toHaveValue('8002'))
  })

  it('goes to Add category straight away when nothing changed', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(
      ...creating(sent, draft),
      http.get('*/api/v2/11/bookkeeping_settings', () => HttpResponse.json({ chart_scheme: '', chart_schemes: [] })),
      http.get('*/api/v2/11/ledger_account_templates', () => HttpResponse.json([])),
    )
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: /^Invoice Draft #3$/ })
    await editLines()
    await userEvent.selectOptions(within(detailRows()[0]!).getByLabelText('Category'), 'Add Category...')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/ledger_accounts\/blueprints$/))
    expect(sent.patch).toBeUndefined()
  })
})
