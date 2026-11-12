import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { expect } from 'vitest'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import type { Contact } from '../contacts/queries'
import { administration, documentStyle, identity, invoiceSettings, listing, signedIn, workflow } from '../test/server'
import type { SalesInvoice } from './queries'

export const harpy = administration({ id: '11', name: 'Harpy B.V.' })
export const stamp = '2026-11-10T09:00:00Z'

export const rate = (over: Partial<TaxRate> = {}): TaxRate => ({
  id: '2101', administration_id: '11', name: '21% btw', percentage: '21', tax_rate_type: 'sales_invoice', show_tax: true, active: true,
  country: null, created_at: stamp, updated_at: stamp, uri: '/11/tax_rates/2101', tax_report_boxes: ['NL/1a'], position: 1, ...over,
})
export const rates = [
  rate(),
  rate({ id: '2109', name: '9% btw', percentage: '9', position: 2, uri: '/11/tax_rates/2109' }),
  rate({ id: '2150', name: 'Old rate', percentage: '6', active: false, position: 3, uri: '/11/tax_rates/2150' }),
  rate({ id: '2201', name: '21% purchase', tax_rate_type: 'purchase_invoice', uri: '/11/tax_rates/2201' }),
]

export const sales: LedgerAccount = {
  id: '8001', administration_id: '11', name: 'Sales', account_type: 'revenue', account_id: '8000', parent_id: null, active: true,
  allowed_document_types: ['sales_invoice'], financial_account_id: null, created_at: stamp, updated_at: stamp,
  taxonomy_item: null, uri: '/11/ledger_accounts/8001', description: null, system_role: null, template: null,
}

export const shop = {
  id: '701', administration_id: '11', company_name: 'Example Shop B.V.', firstname: null, lastname: null, customer_id: '1',
  address1: 'Dam 2', address2: null, zipcode: '1012AB', city: 'Amsterdam',
  send_invoices_to_email: 'billing@example.com', email_ubl: false, delivery_method: 'Email' as const, country: 'NL',
}

export const invoice = (over: Partial<SalesInvoice> = {}): SalesInvoice => ({
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
export const draft = invoice({ id: '902', invoice_id: null, draft_id: 3, state: 'draft', invoice_date: null, uri: '/11/sales_invoices/902' })

/** The sender block of Moneybird's default layout. */
export const senderStyle = documentStyle({
  id: '701',
  invoice_sender_address: [
    { field: 'company_name', bold: true }, { field: 'address' }, { field: 'newline' }, { field: 'email' }, { field: 'phone' },
    { field: 'newline' }, { field: 'chamber_of_commerce', label: true }, { field: 'tax_number', label: true }, { field: 'bank_account_number', label: true },
  ],
})

export function loaded(list: SalesInvoice[] = [invoice(), draft], total = list.length, seen: URL[] = []) {
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


/** The "Add invoice" popup that opens over a new invoice, and again from the document. */
export const popup = () => screen.getByRole('dialog', { name: 'Add invoice' })

/** Chooses the contact in the popup's contact selector by searching for it. */
export async function pickContact() {
  const dialog = await screen.findByRole('dialog', { name: 'Add invoice' })
  await userEvent.click(within(dialog).getByRole('combobox', { name: 'Contact' }))
  const search = within(dialog).getByRole('searchbox', { name: 'Query' })
  expect(search).toHaveAttribute('placeholder', 'Search contacts')
  expect(search).toHaveFocus()
  await userEvent.type(search, 'shop')
  await userEvent.click(await within(dialog).findByRole('option', { name: 'Example Shop B.V.' }))
}

/** The texts of a row's cells, with the spaces money is written with made plain. */
export const cells = (row: HTMLElement) => within(row).getAllByRole('cell').map((c) => c.textContent?.replace(/\s/g, ' '))

/** The editor's document: a preview on a new invoice, the draft itself on its page. */
export const paper = () => screen.getByRole('document', { name: /^Invoice (preview|Draft #\d+)$/ })

/** The details editor and its detail rows (the header row left out). */
export const editor = () => screen.getByRole('table', { name: 'Invoice details' })
export const detailRows = () => within(editor()).getAllByRole('row').slice(1)

/** Opens the details editor of a saved draft, as a click on the lines does. */
export async function editLines() {
  await userEvent.click(within(paper()).getByRole('button', { name: /invoice details/ }))
  return editor()
}

export const empty = invoice({ id: '902', invoice_id: null, draft_id: 12, state: 'draft', invoice_date: null, reference: 'Order 12', details: [], uri: '/11/sales_invoices/902' })

/** The pages a new invoice goes through, with what was posted and patched. */
export function creating(sent: { post?: Record<string, unknown>; patch?: Record<string, unknown> }, created: SalesInvoice = empty) {
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



/** The details editor's card, with its footer and totals. */
export const editorCard = () => screen.getByRole('region', { name: 'Invoice details' })

/** Opens a menu of the invoice's action bar, such as Send or Extra, and chooses one of its entries. */
export async function choose(menu: string, name: string) {
  await userEvent.click(await screen.findByRole('button', { name: menu }))
  const entry = screen.queryByRole('button', { name }) ?? screen.getByRole('link', { name })
  await userEvent.click(entry)
}

/** The state label at the right of the action bar. */
export const stateLabel = (name: string) => screen.findByText(name, { selector: '.actionbar .badge' })
export const sidebar = () => screen.getByRole('complementary', { name: 'Invoice information' })
export const invoicePaper = (name: string) => screen.getByRole('document', { name })

/** What the hover card of a contact asks for: the contact and how many documents it has. */
export const contactCounts = [
  http.get('*/api/v2/11/contacts/701', () => HttpResponse.json({ ...shop, email: 'info@example.com', created_at: '2026-11-11T09:00:00Z' })),
  http.get('*/api/v2/11/external_sales_invoices', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
  http.get('*/api/v2/11/estimates', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
  http.get('*/api/v2/11/documents/purchase_invoices', () => HttpResponse.json([], { headers: { 'X-Total-Count': '2' } })),
  http.get('*/api/v2/11/documents/receipts', () => HttpResponse.json([], { headers: { 'X-Total-Count': '1' } })),
]
