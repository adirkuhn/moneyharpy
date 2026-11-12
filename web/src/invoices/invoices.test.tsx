import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import type { Contact } from '../contacts/queries'
import { renderAt } from '../test/render'
import { administration, documentStyle, identity, invoiceSettings, listing, server, signedIn, workflow } from '../test/server'
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
  address1: 'Dam 2', address2: null, zipcode: '1012AB', city: 'Amsterdam',
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

/** The sender block of Moneybird's default layout. */
const senderStyle = documentStyle({
  id: '701',
  invoice_sender_address: [
    { field: 'company_name', bold: true }, { field: 'address' }, { field: 'newline' }, { field: 'email' }, { field: 'phone' },
    { field: 'newline' }, { field: 'chamber_of_commerce', label: true }, { field: 'tax_number', label: true }, { field: 'bank_account_number', label: true },
  ],
})

function loaded(list: SalesInvoice[] = [invoice(), draft], total = list.length, seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/sales_invoices', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(total) } })
    }),
    http.get('*/api/v2/11/sales_invoices/next_draft_id', () => HttpResponse.json({ draft_id: 12 })),
    http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice())),
    http.get('*/api/v2/11/sales_invoices/902', () => HttpResponse.json(draft)),
    http.get('*/api/v2/11/tax_rates', () => HttpResponse.json(rates)),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales])),
    http.get('*/api/v2/11/workflows', () => HttpResponse.json([workflow(), workflow({ id: '602', type: 'EstimateWorkflow', name: 'Quotes' })])),
    http.get('*/api/v2/11/document_styles', () => HttpResponse.json([senderStyle])),
    http.get('*/api/v2/11/identities', () => HttpResponse.json([identity({ id: '500' })])),
    http.get('*/api/v2/11/invoice_settings', () => HttpResponse.json(invoiceSettings())),
    http.get('*/api/v2/11/custom_fields', () => HttpResponse.json([])),
    http.get('*/api/v2/11/users', () => HttpResponse.json([])),
    http.get('*/api/v2/11/products', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
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

/** The "Add invoice" popup that opens over a new invoice, and again from the document. */
const popup = () => screen.getByRole('dialog', { name: 'Add invoice' })

/** Chooses the contact in the popup's contact selector by searching for it. */
async function pickContact() {
  const dialog = await screen.findByRole('dialog', { name: 'Add invoice' })
  await userEvent.click(within(dialog).getByRole('combobox', { name: 'Contact' }))
  const search = within(dialog).getByRole('searchbox', { name: 'Query' })
  expect(search).toHaveAttribute('placeholder', 'Search contacts')
  expect(search).toHaveFocus()
  await userEvent.type(search, 'shop')
  await userEvent.click(await within(dialog).findByRole('option', { name: 'Example Shop B.V.' }))
}

/** The texts of a row's cells, with the spaces money is written with made plain. */
const cells = (row: HTMLElement) => within(row).getAllByRole('cell').map((c) => c.textContent?.replace(/\s/g, ' '))

/** The editor's document: a preview on a new invoice, the draft itself on its page. */
const paper = () => screen.getByRole('document', { name: /^Invoice (preview|Draft #\d+)$/ })

/** The details editor and its detail rows (the header row left out). */
const editor = () => screen.getByRole('table', { name: 'Invoice details' })
const detailRows = () => within(editor()).getAllByRole('row').slice(1)

/** Opens the details editor of a saved draft, as a click on the lines does. */
async function editLines() {
  await userEvent.click(within(paper()).getByRole('button', { name: /invoice details/ }))
  return editor()
}

const empty = invoice({ id: '902', invoice_id: null, draft_id: 12, state: 'draft', invoice_date: null, reference: 'Order 12', details: [], uri: '/11/sales_invoices/902' })

/** The pages a new invoice goes through, with what was posted and patched. */
function creating(sent: { post?: Record<string, unknown>; patch?: Record<string, unknown> }, created: SalesInvoice = empty) {
  return [
    http.post('*/api/v2/11/sales_invoices', async ({ request }) => {
      sent.post = ((await request.json()) as { sales_invoice: Record<string, unknown> }).sales_invoice
      return HttpResponse.json(created, { status: 201 })
    }),
    http.get('*/api/v2/11/sales_invoices/902', () => HttpResponse.json(created)),
    http.patch('*/api/v2/11/sales_invoices/902', async ({ request }) => {
      sent.patch = ((await request.json()) as { sales_invoice: Record<string, unknown> }).sales_invoice
      return HttpResponse.json(created)
    }),
  ]
}

describe('adding an invoice', () => {
  it('opens the Add invoice popup over the empty document, after Moneybird', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/new')
    expect(await screen.findByRole('heading', { level: 1, name: 'Add invoice' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Change layout' })).toHaveAttribute('aria-expanded', 'false')

    // The document behind it: the sender from the document style and its identity.
    const sender = await within(paper()).findByRole('list', { name: 'Afzenderadres' })
    expect(within(sender).getAllByRole('listitem').map((li) => li.textContent)).toEqual([
      'Harpy B.V.', 'Dam 11012JS Amsterdam', '', 'books@example.com', '', 'KVK: 12345678', 'Bank: NL91ABNA0417164300',
    ])
    const recipient = within(paper()).getByRole('button', { name: 'Contact' })
    expect(recipient).toHaveTextContent('1. Select contact')
    expect(recipient).toHaveTextContent('First name Last nameAddress 1231234AB City')
    expect(await within(paper()).findByRole('heading', { level: 2, name: 'Factuur Concept #12' })).toBeInTheDocument()
    expect(within(paper()).getByRole('button', { name: /Factuurdatum/ })).toHaveTextContent('Factuurdatum:11-11-2026')
    expect(within(paper()).getByRole('button', { name: /Vervaldatum/ })).toHaveTextContent('Vervaldatum:25-11-2026')
    const lines = within(paper()).getByRole('table', { name: 'Factuurregels' })
    expect(cells(within(lines).getAllByRole('row')[1]!)).toEqual(['1 x', "You didn't add any details yet", '€ 0,00', '€ 0,00', '21%'])
    const totals = within(paper()).getByRole('table', { name: 'Totals' })
    expect(within(totals).getAllByRole('row').map((r) => r.textContent?.replace(/\s/g, ' '))).toEqual(['Subtotaal€ 0,00', 'Totaal€ 0,00'])

    // The popup asks for the contact first.
    const dialog = popup()
    expect(dialog).toHaveAttribute('aria-modal', 'true')
    expect(within(dialog).getByText('Add the basic details to your invoice. Select the contact you are invoicing.')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Close' })).toBeInTheDocument()
    expect(within(dialog).getByRole('combobox', { name: 'Contact' })).toHaveTextContent('Select a contact')
    expect(within(dialog).getByLabelText('Reference')).toBeInTheDocument()
    expect(within(dialog).getByRole('note')).toHaveTextContent(
      'The invoice is created using workflow Standaard (NL) and document style Standaard. The due date is 14 days after the invoice date.',
    )
    // The settings are behind Edit, as on Moneybird.
    expect(within(dialog).queryByLabelText('Workflow')).toBeNull()
    expect(within(dialog).getByRole('button', { name: 'Save' })).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Invoice details' })).toBeInTheDocument()
    expect(within(dialog).getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/11/sales_invoices')
    expect(within(dialog).getByRole('link', { name: 'Add custom field' })).toHaveAttribute(
      'href', `/11/custom_fields/new?source=sales_invoice&return_to=${encodeURIComponent('/11/sales_invoices/new')}`,
    )

    // Closing it leaves the document; the recipient opens it again.
    await userEvent.click(within(dialog).getByRole('button', { name: 'Close' }))
    expect(screen.queryByRole('dialog', { name: 'Add invoice' })).toBeNull()
    await userEvent.click(within(paper()).getByRole('button', { name: 'Contact' }))
    expect(popup()).toBeInTheDocument()
  })

  it('sets the invoice id, date, workflow, term and layout behind Edit', async () => {
    const sent: { post?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(
      ...creating(sent),
      http.get('*/api/v2/11/invoice_settings', () => HttpResponse.json(invoiceSettings({ invoice_sequence_id: '0007', invoice_id_example: '2026-0007' }))),
      http.get('*/api/v2/11/document_styles', () => HttpResponse.json([senderStyle, documentStyle({ id: '702', name: 'Modern', default: false, uri: '/11/document_styles/702' })])),
      http.get('*/api/v2/11/workflows', () => HttpResponse.json([workflow(), workflow({ id: '603', name: 'English', default: false, language: 'en', uri: '/11/workflows/603' })])),
    )
    renderAt('/11/sales_invoices/new')
    await pickContact()
    const dialog = popup()
    const edit = within(dialog).getByRole('button', { name: 'Edit' })
    await userEvent.click(edit)
    expect(edit).toHaveAttribute('aria-expanded', 'true')
    expect(edit).toHaveTextContent('Close')

    // The invoice id is the next number, picked when sending, unless filled in yourself.
    const number = within(dialog).getByLabelText('Invoice id')
    expect(number).toBeDisabled()
    expect(await within(dialog).findByText('2026-')).toBeInTheDocument()
    expect(number).toHaveValue('0007')
    expect(within(dialog).getByRole('link', { name: 'company settings' })).toHaveAttribute('href', '/11/settings/invoices_and_estimates')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Unlock invoice number' }))
    const confirm = screen.getByRole('alertdialog', { name: 'Set the invoice number yourself?' })
    expect(confirm).toHaveTextContent("MoneyHarpy picks the next number automatically when you send the invoice. It won't do that for this invoice.")
    await userEvent.click(within(confirm).getByRole('button', { name: 'Fill it in myself' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(number).toBeEnabled()
    await userEvent.clear(number)
    await userEvent.type(number, '0100')

    const date = within(dialog).getByLabelText('Invoice date')
    expect(date).toBeDisabled()
    expect(date).toHaveAttribute('placeholder', 'YYYY-MM-DD')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Unlock invoice date' }))
    const confirmDate = screen.getByRole('alertdialog', { name: 'Set the invoice date yourself?' })
    expect(confirmDate).toHaveTextContent("MoneyHarpy normally uses the date on which you send the invoice. It won't do that for this invoice.")
    await userEvent.click(within(confirmDate).getByRole('button', { name: 'Fill it in myself' }))
    const unlocked = within(dialog).getByLabelText('Invoice date')
    expect(unlocked).toBeEnabled()
    await userEvent.type(unlocked, '2026-12-01')

    expect(within(dialog).getByLabelText('Workflow')).toHaveValue('601')
    expect(within(dialog).getByRole('link', { name: 'Edit this workflow' })).toHaveAttribute('href', '/11/workflows/601')
    await userEvent.selectOptions(within(dialog).getByLabelText('Workflow'), '603')
    expect(within(dialog).getByRole('link', { name: 'Edit this workflow' })).toHaveAttribute('href', '/11/workflows/603')

    const days = within(dialog).getByRole('spinbutton', { name: 'Number of days after expiration' })
    expect(days).toHaveValue(14)
    await userEvent.click(within(dialog).getByRole('button', { name: 'Increase value (plus)' }))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Increase value (plus)' }))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Decrease value (min)' }))
    expect(days).toHaveValue(15)

    await userEvent.click(within(dialog).getByRole('checkbox', { name: 'Prices include VAT' }))

    const layout = within(dialog).getByRole('radiogroup', { name: 'Layout' })
    expect(within(layout).getByRole('radio', { name: 'Standaard' })).toBeChecked()
    await userEvent.click(within(layout).getByRole('radio', { name: 'Modern' }))
    expect(within(dialog).getByRole('note')).toHaveTextContent('The invoice is created using workflow English and document style Modern. The due date is 15 days after the invoice date.')
    // The document behind follows: the workflow is English.
    expect(within(paper()).getByRole('button', { name: /Invoice date/ })).toHaveTextContent('Invoice date:12/01/2026')

    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/902$/))
    expect(sent.post).toEqual({
      contact_id: '701', reference: '', invoice_date: '2026-12-01', invoice_sequence_id: '0100', workflow_id: '603', first_due_interval: 15,
      document_style_id: '702', prices_are_incl_tax: true, details_attributes: [],
    })
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('asks before an invoice without a contact is saved', async () => {
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/sales_invoices', () => HttpResponse.json({ error: { contact_id: ["can't be blank"] } }, { status: 422 })))
    renderAt('/11/sales_invoices/new')
    const dialog = await screen.findByRole('dialog', { name: 'Add invoice' })
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect(await within(dialog).findByText("Contact can't be blank")).toBeInTheDocument()
    expect(within(dialog).getByRole('combobox', { name: 'Contact' })).toHaveAttribute('aria-invalid', 'true')
  })

  it('adds a contact from the popup and selects it', async () => {
    let posted: Record<string, unknown> = {}
    server.use(
      ...loaded(),
      http.get('*/api/onboarding/options', () => HttpResponse.json({ countries: ['NL', 'BE', 'DE'], company_search_countries: ['NL', 'BE'] })),
      http.get('*/api/onboarding/company_search', () => HttpResponse.json([])),
      http.post('*/api/v2/11/contacts', async ({ request }) => {
        posted = ((await request.json()) as { contact: Record<string, unknown> }).contact
        return HttpResponse.json({ ...shop, id: '705', company_name: 'Example Trade B.V.', address1: 'Markt 1', zipcode: '2000', city: 'Antwerpen', country: 'BE' }, { status: 201 })
      }),
    )
    renderAt('/11/sales_invoices/new')
    const dialog = await screen.findByRole('dialog', { name: 'Add invoice' })
    await userEvent.click(within(dialog).getByRole('combobox', { name: 'Contact' }))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Add contact' }))
    const form = within(dialog).getByRole('region', { name: 'Contact' })
    expect(within(form).getByRole('radio', { name: 'Company' })).toBeChecked()
    await userEvent.click(within(form).getByRole('radio', { name: 'Private' }))
    expect(within(form).getByLabelText('First name')).toBeInTheDocument()
    expect(within(form).getByLabelText('Last name')).toBeInTheDocument()
    await userEvent.click(within(form).getByRole('radio', { name: 'Company' }))
    expect(within(form).queryByLabelText('VAT-id')).toBeNull()
    expect(await within(form).findByRole('option', { name: 'Belgium' })).toBeInTheDocument()
    await userEvent.selectOptions(within(form).getByLabelText('Country'), 'BE')
    await userEvent.type(within(form).getByLabelText('Company name'), 'Example Trade B.V.')
    await userEvent.type(within(form).getByLabelText('Address'), 'Markt 1')
    await userEvent.type(within(form).getByLabelText('Zipcode'), '2000')
    await userEvent.type(within(form).getByLabelText('City'), 'Antwerpen')
    await userEvent.type(within(form).getByLabelText('Email address for invoices'), 'billing@example.com')
    await userEvent.type(within(form).getByLabelText('VAT-id'), 'BE0123456789')
    expect(within(form).getByRole('button', { name: 'Complete contact' })).toBeInTheDocument()
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(within(dialog).getByRole('combobox', { name: 'Contact' })).toHaveTextContent('Example Trade B.V.'))
    expect(posted).toEqual({
      company_name: 'Example Trade B.V.', country: 'BE', address1: 'Markt 1', zipcode: '2000', city: 'Antwerpen',
      send_invoices_to_email: 'billing@example.com', tax_number: 'BE0123456789',
    })
    expect(within(paper()).getByRole('button', { name: 'Contact' })).toHaveTextContent('Example Trade B.V.Markt 12000 Antwerpen')
  })

  it('completes a contact on the Add contact page and comes back to the new invoice', async () => {
    server.use(
      ...loaded(),
      http.get('*/api/onboarding/options', () => HttpResponse.json({ countries: ['NL', 'BE'], company_search_countries: [] })),
      http.get('*/api/v2/11/contacts/next_customer_id', () => HttpResponse.json({ customer_id: '2' })),
      http.get('*/api/v2/11/custom_fields', () => HttpResponse.json([])),
      http.post('*/api/v2/11/contacts', () => HttpResponse.json({ ...shop, id: '701' }, { status: 201 })),
      http.get('*/api/v2/11/contacts/701', () => HttpResponse.json(shop)),
    )
    renderAt('/11/sales_invoices/new')
    const dialog = await screen.findByRole('dialog', { name: 'Add invoice' })
    await userEvent.click(within(dialog).getByRole('combobox', { name: 'Contact' }))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Add contact' }))
    const form = within(dialog).getByRole('region', { name: 'Contact' })
    await userEvent.type(within(form).getByLabelText('Company name'), 'Example Shop B.V.')
    await userEvent.type(within(form).getByLabelText('City'), 'Amsterdam')
    await userEvent.click(within(form).getByRole('button', { name: 'Complete contact' }))

    expect(await screen.findByRole('heading', { level: 1, name: 'Add contact' })).toBeInTheDocument()
    expect(screen.getByLabelText('Company name')).toHaveValue('Example Shop B.V.')
    expect(screen.getByLabelText('City')).toHaveValue('Amsterdam')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/new$/))
    const back = await screen.findByRole('dialog', { name: 'Add invoice' })
    expect(within(back).getByRole('combobox', { name: 'Contact' })).toHaveTextContent('Example Shop B.V.')
  })

  it("starts with the contact it was added from, as a contact's Add menu does", async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/contacts/701', () => HttpResponse.json(shop)))
    renderAt('/11/sales_invoices/new?contact_id=701')
    const dialog = await screen.findByRole('dialog', { name: 'Add invoice' })
    expect(within(dialog).getByRole('combobox', { name: 'Contact' })).toHaveTextContent('Example Shop B.V.')
    expect(within(paper()).getByRole('button', { name: 'Contact' })).toHaveTextContent('Example Shop B.V.Dam 21012AB Amsterdam')
  })
})

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
    // "Add Category..." opens Moneybird's Add category in a new tab, which comes back to this invoice.
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    await userEvent.selectOptions(within(rows[1]!).getByLabelText('Category'), 'Add Category...')
    expect(open).toHaveBeenCalledWith(`/11/ledger_accounts/blueprints?return_to=${encodeURIComponent('/11/sales_invoices/902')}&source=sales_invoice`, '_blank', 'noopener')
    expect(within(rows[1]!).getByLabelText('Category')).toHaveValue('8001')
    open.mockRestore()

    await userEvent.click(screen.getByRole('button', { name: 'Discount' }))
    await userEvent.click(screen.getByRole('button', { name: 'Discount over total price' }))
    await userEvent.type(screen.getByRole('textbox', { name: 'Discount' }), '10')

    const totals = within(editorCard()).getByRole('table', { name: 'Totals' })
    expect(within(totals).getByRole('row', { name: /^Subtotal/ })).toHaveTextContent('€250.00')
    expect(within(totals).getByRole('row', { name: /Discount/ })).toHaveTextContent('-€25.00')
    expect(within(totals).getByRole('row', { name: /21% btw/ })).toHaveTextContent('€37.80')
    expect(within(totals).getByRole('row', { name: /9% btw/ })).toHaveTextContent('€4.05')
    expect(within(totals).getByRole('row', { name: /^Total/ })).toHaveTextContent('€266.85')
    expect(within(paper()).getByText(/Graag ontvangen we € 266,85/)).toBeInTheDocument()

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
      { description: 'Discount', amount: '1 x', price: '-10', tax_rate_id: '2101', row_order: 1 },
      { description: 'Advice per hour', amount: '1 x', price: '80', tax_rate_id: '2109', ledger_account_id: '8001', product_id: '301', period: '20261101..20261130', row_order: 2 },
    ])
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

    await userEvent.click(within(paper()).getByRole('button', { name: /payment conditions/i }))
    await userEvent.type(within(paper()).getByLabelText('Payment conditions'), 'Pay within 30 days.')
    await userEvent.tab()
    expect(within(paper()).getByText('Pay within 30 days.')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Save invoice' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/902$/))
    expect(sent.patch).toMatchObject({ invoice_date: '2026-12-01', first_due_interval: 30, payment_conditions: 'Pay within 30 days.' })
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

  it('reopens the Add invoice popup from the document of a draft, and saves it', async () => {
    const sent: { patch?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(...creating(sent))
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #12' })
    expect(screen.queryByRole('dialog')).toBeNull()
    await userEvent.click(within(paper()).getByRole('button', { name: 'Contact' }))
    const dialog = popup()
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
})

/** The details editor's card, with its footer and totals. */
const editorCard = () => screen.getByRole('region', { name: 'Invoice details' })

/** Opens a menu of the invoice's action bar, such as Send or Extra, and chooses one of its entries. */
async function choose(menu: string, name: string) {
  await userEvent.click(await screen.findByRole('button', { name: menu }))
  const entry = screen.queryByRole('button', { name }) ?? screen.getByRole('link', { name })
  await userEvent.click(entry)
}

/** The state label at the right of the action bar. */
const stateLabel = (name: string) => screen.findByText(name, { selector: '.actionbar .badge' })
const sidebar = () => screen.getByRole('complementary', { name: 'Invoice information' })
const invoicePaper = (name: string) => screen.getByRole('document', { name })

/** What the hover card of a contact asks for: the contact and how many documents it has. */
const contactCounts = [
  http.get('*/api/v2/11/contacts/701', () => HttpResponse.json({ ...shop, email: 'info@example.com', created_at: '2026-11-11T09:00:00Z' })),
  http.get('*/api/v2/11/external_sales_invoices', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
  http.get('*/api/v2/11/estimates', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
  http.get('*/api/v2/11/documents/purchase_invoices', () => HttpResponse.json([], { headers: { 'X-Total-Count': '2' } })),
  http.get('*/api/v2/11/documents/receipts', () => HttpResponse.json([], { headers: { 'X-Total-Count': '1' } })),
]

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
    // A sent invoice is credited, not deleted.
    expect(screen.queryByRole('button', { name: 'Delete invoice' })).toBeNull()

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
      'Send by email', 'Send via Peppol', 'Send manually', 'Send by post',
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
    await userEvent.type(within(doc).getByLabelText('Payment conditions'), 'Pay within 30 days.')
    await userEvent.tab()
    // Cancel puts it back as it was.
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(within(invoicePaper('Invoice Draft #3')).queryByText('Pay within 30 days.')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Save invoice' })).toBeNull()

    await userEvent.click(within(invoicePaper('Invoice Draft #3')).getByRole('button', { name: /payment conditions/i }))
    await userEvent.type(within(invoicePaper('Invoice Draft #3')).getByLabelText('Payment conditions'), 'Pay within 30 days.')
    await userEvent.click(screen.getByRole('button', { name: 'Save invoice' }))
    await waitFor(() => expect(sent.patch).toMatchObject({ payment_conditions: 'Pay within 30 days.' }))
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/902$/)
    // The sidebar stays beside it.
    expect(sidebar()).toBeInTheDocument()
  })

  it('changes the layout of a draft from Extra', async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #3' })
    await choose('Extra', 'Change layout')
    expect(await screen.findByRole('region', { name: 'Select layout' })).toBeInTheDocument()
  })

  it("opens the editor's popup and details from the document of a draft", async () => {
    server.use(...loaded())
    renderAt('/11/sales_invoices/902')
    await screen.findByRole('document', { name: 'Invoice Draft #3' })
    await userEvent.click(within(invoicePaper('Invoice Draft #3')).getByRole('button', { name: 'Contact' }))
    expect(await screen.findByRole('dialog', { name: 'Add invoice' })).toBeInTheDocument()
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
    await userEvent.click(within(warning).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()
    await userEvent.click(within(invoicePaper('Invoice 2026-0001')).getByRole('button', { name: 'Edit contact' }))
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'This invoice has been sent' })).getByRole('button', { name: 'Continue editing' }))
    expect(await screen.findByRole('dialog', { name: 'Add invoice' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/901$/)
  })

  it('cannot be edited once a payment is registered', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice({
      total_unpaid: '142.0',
      payments: [{ id: '981', administration_id: '11', invoice_type: 'SalesInvoice', invoice_id: '901', price: '100.0', price_base: '100.0', payment_date: '2026-11-05', manual_payment_action: 'cash_payment', created_at: stamp, updated_at: stamp }],
    }))))
    renderAt('/11/sales_invoices/901')
    await screen.findByRole('heading', { level: 1, name: '2026-0001' })
    expect(within(invoicePaper('Invoice 2026-0001')).queryByRole('button', { name: 'Edit invoice details' })).toBeNull()
    expect(screen.getByRole('link', { name: 'Register payment' })).toBeInTheDocument()
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

  it('cannot be edited once paid', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(invoice({ state: 'paid', total_unpaid: '0.0', paid_at: '2026-11-05' }))))
    renderAt('/11/sales_invoices/901')
    expect(await stateLabel('Paid')).toBeInTheDocument()
    const doc = invoicePaper('Invoice 2026-0001')
    expect(within(doc).queryByRole('link', { name: 'Edit contact' })).toBeNull()
    expect(within(doc).queryByRole('button', { name: 'Edit invoice details' })).toBeNull()
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
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select Draft #3' }))
    let bar = screen.getByRole('group', { name: 'Selected invoices' })
    expect(within(bar).getByText('2 invoices selected:')).toBeInTheDocument()
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

    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select Draft #3' }))
    bar = screen.getByRole('group', { name: 'Selected invoices' })
    expect(within(bar).getByRole('button', { name: 'Register full payment (0)' })).toBeDisabled()
    await userEvent.click(within(bar).getByRole('button', { name: 'Delete drafts (1)' }))
    await userEvent.click(within(bar).getByRole('button', { name: 'Delete 1 draft permanently' }))
    await waitFor(() => expect(deleted).toBe(true))
    expect(await screen.findByText('Deleted 1 draft.')).toBeInTheDocument()
  })
})
