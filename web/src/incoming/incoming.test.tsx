import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { DocumentSummary } from './documents'
import type { GeneralDocument, PurchaseInvoice, TypelessDocument } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const supplier = { id: '701', administration_id: '11', company_name: 'Supplier B.V.', firstname: null, lastname: null, country: 'NL' }

const rate = (over: Partial<TaxRate>): TaxRate => ({
  id: '2201', administration_id: '11', name: '21% btw', percentage: '21', tax_rate_type: 'purchase_invoice', show_tax: true, active: true,
  country: null, created_at: stamp, updated_at: stamp, uri: '/11/tax_rates/2201', tax_report_boxes: [], position: 1, ...over,
})
const rates = [
  rate({}),
  rate({ id: '2209', name: '9% btw', percentage: '9', position: 2 }),
  rate({ id: '2101', name: '21% sales', tax_rate_type: 'sales_invoice', position: 3 }),
]
const account = (over: Partial<LedgerAccount>): LedgerAccount => ({
  id: '4001', administration_id: '11', name: 'Other costs', account_type: 'expenses', account_id: '4000', parent_id: null, active: true,
  allowed_document_types: ['purchase_invoice'], financial_account_id: null, created_at: stamp, updated_at: stamp,
  taxonomy_item: null, uri: '/11/ledger_accounts/4001', description: null, system_role: null, template: null, ...over,
})
const accounts = [account({}), account({ id: '8001', name: 'Sales', account_type: 'revenue', allowed_document_types: ['sales_invoice'] })]

const pdf = { id: '971', administration_id: '11', attachable_id: '941', attachable_type: 'Document' as const, filename: 'bill.pdf', content_type: 'application/pdf', size: 2048, rotation: 0, created_at: stamp, updated_at: stamp }

const invoice = (over: Partial<PurchaseInvoice> = {}): PurchaseInvoice => ({
  id: '941', administration_id: '11', contact_id: '701', contact: supplier, date: '2026-11-01', due_date: '2026-11-15', state: 'open',
  reference: 'INV-1', entry_number: 1, origin: 'api', currency: 'EUR', prices_are_incl_tax: false, paid_at: null, revenue_invoice: false,
  details: [{ id: '961', description: 'Paper', amount: '1', price: '100.0', tax_rate_id: '2201', ledger_account_id: '4001', period: null, row_order: 1, total_price_excl_tax_with_discount: '100.0' }],
  fiscal_allocations: [], payments: [], notes: [], events: [], attachments: [pdf],
  total_price_excl_tax: '100.0', total_price_incl_tax: '121.0', total_paid: '0.0', total_unpaid: '121.0', total_tax: '21.0',
  tax_totals: [{ tax_rate_id: '2201', taxable_amount: '100.0', tax_amount: '21.0' }],
  created_at: stamp, version: 1, uri: '/11/documents/purchase_invoices/941',
  ...over,
})

const summary = (over: Partial<DocumentSummary>): DocumentSummary => ({
  id: '941', administration_id: '11', type: 'PurchaseInvoice', reference: 'INV-1', contact_id: '701', contact_name: 'Supplier B.V.',
  date: '2026-11-01', state: 'open', currency: 'EUR', total_price_incl_tax: '121.0', attachments_count: 1, created_at: stamp,
  uri: '/api/v2/11/documents/purchase_invoices/941', ...over,
})

const upload: TypelessDocument = {
  id: '951', administration_id: '11', contact_id: null, contact: null, date: '2026-11-10', reference: 'scan.jpg', state: 'new', origin: 'upload',
  events: [], created_at: stamp, version: 1, uri: '/11/documents/typeless_documents/951',
  attachments: [{ ...pdf, id: '972', attachable_id: '951', filename: 'scan.jpg', content_type: 'image/jpeg' }],
} as TypelessDocument

const general: GeneralDocument = {
  id: '981', administration_id: '11', contact_id: null, contact: null, date: '2026-11-02', due_date: null, reference: 'Insurance policy', state: 'saved',
  entry_number: 3, notes: [], events: [], attachments: [], created_at: stamp, version: 1, uri: '/11/documents/general_documents/981',
}

/** Three new documents wait to be processed: this one and two more. */
const waiting = [summary({ id: '941', state: 'new' }), summary({ id: '942', type: 'Receipt', state: 'new' }), summary({ id: '943', state: 'new' })]

function loaded(over: { doc?: PurchaseInvoice; summaries?: DocumentSummary[] } = {}) {
  const doc = over.doc ?? invoice()
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/documents', ({ request }) => {
      const list = new URL(request.url).searchParams.get('filter') === 'state:new' ? (over.summaries ?? waiting) : []
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/documents/941', () => HttpResponse.json(summary({ state: doc.state ?? 'open' }))),
    http.get('*/api/v2/11/documents/951', () => HttpResponse.json(summary({ id: '951', type: 'TypelessDocument', reference: 'scan.jpg', contact_id: null, contact_name: '', state: 'new' }))),
    http.get('*/api/v2/11/documents/981', () => HttpResponse.json(summary({ id: '981', type: 'GeneralDocument', reference: 'Insurance policy', contact_id: null, contact_name: '', state: 'saved' }))),
    http.get('*/api/v2/11/documents/purchase_invoices/941', () => HttpResponse.json(doc)),
    http.get('*/api/v2/11/documents/typeless_documents/951', () => HttpResponse.json(upload)),
    http.get('*/api/v2/11/documents/general_documents/981', () => HttpResponse.json(general)),
    http.get('*/api/v2/11/documents/:type/:id/attachments/:attachment/download', () => new HttpResponse('file', { headers: { 'Content-Type': 'application/octet-stream' } })),
    http.get('*/api/v2/11/tax_rates', () => HttpResponse.json(rates)),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json(accounts)),
    http.get('*/api/v2/11/products', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
    http.get('*/api/v2/11/users', () => HttpResponse.json([])),
    http.get('*/api/v2/11/contacts', () => HttpResponse.json([supplier], { headers: { 'X-Total-Count': '1' } })),
  ]
}

const card = () => screen.getByRole('form', { name: /^(Purchase invoice|Receipt|General document|Document)$/ })

beforeEach(() => {
  Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() })
})
afterEach(() => {
  Object.assign(URL, { createObjectURL: undefined, revokeObjectURL: undefined })
})

describe('the page of an incoming document', () => {
  it('opens every type at /documents/:id, and the old pages of a type lead there', async () => {
    server.use(...loaded())
    renderAt('/11/documents/purchase_invoices/941')
    expect(await screen.findByRole('heading', { level: 1, name: 'Purchase invoice INV-1' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/documents\/941$/)
  })

  it('shows the attachment beside the document, with its tiles, download, delete and add', async () => {
    const deleted: string[] = []
    const added: string[] = []
    server.use(...loaded())
    server.use(
      http.delete('*/api/v2/11/documents/purchase_invoices/941/attachments/971', () => { deleted.push('971'); return new HttpResponse(null, { status: 204 }) }),
      http.post('*/api/v2/11/documents/purchase_invoices/941/attachments', () => { added.push('new'); return HttpResponse.json({}, { status: 201 }) }),
    )
    renderAt('/11/documents/941')
    const viewer = await screen.findByRole('region', { name: 'Attachments' })
    expect(await within(viewer).findByTitle('bill.pdf')).toHaveAttribute('src', 'blob:preview')
    const tile = within(viewer).getByRole('button', { name: /bill\.pdf/ })
    expect(tile).toHaveAttribute('aria-pressed', 'true')
    expect(tile).toHaveTextContent('PDF')
    expect(within(viewer).getByRole('link', { name: 'Download attachment' })).toHaveAttribute('href', '/api/v2/11/documents/purchase_invoices/941/attachments/971/download')
    await userEvent.upload(within(viewer).getByLabelText('Add'), new File(['x'], 'page-2.pdf', { type: 'application/pdf' }))
    await waitFor(() => expect(added).toEqual(['new']))
    await userEvent.click(within(viewer).getByRole('button', { name: 'Delete attachment' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(deleted).toEqual(['971']))
  })

  it('shows a saved purchase invoice with its values, and Edit turns them into the form', async () => {
    server.use(...loaded())
    renderAt('/11/documents/941')
    const form = await screen.findByRole('region', { name: 'Purchase invoice' })
    expect(within(form).getByText('Supplier B.V.')).toBeInTheDocument()
    expect(within(form).getByText('2026-11-15')).toBeInTheDocument()
    expect(within(form).getByRole('table', { name: 'Purchase invoice lines' })).toHaveTextContent('Paper')
    expect(within(form).getByRole('table', { name: 'Totals' })).toHaveTextContent('€121.00')
    await userEvent.click(within(form).getByRole('button', { name: 'Edit' }))
    expect(within(card()).getByRole('textbox', { name: 'Invoice id' })).toHaveValue('INV-1')
    expect(within(card()).getByRole('textbox', { name: 'Due date' })).toHaveValue('2026-11-15')
    await userEvent.click(within(card()).getByRole('link', { name: 'Cancel' }))
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/documents$/)
  })

  it('books a new purchase invoice with its lines, and Next opens the next document to process', async () => {
    let sent: Record<string, unknown> | undefined
    server.use(...loaded({ doc: invoice({ state: 'new', reference: 'INV-1', details: [], due_date: null }) }))
    server.use(
      http.patch('*/api/v2/11/documents/purchase_invoices/941', async ({ request }) => {
        sent = ((await request.json()) as { purchase_invoice: Record<string, unknown> }).purchase_invoice
        return HttpResponse.json(invoice())
      }),
      http.get('*/api/v2/11/documents/942', () => HttpResponse.json(summary({ id: '942', type: 'Receipt', reference: 'Till 42', state: 'new' }))),
      http.get('*/api/v2/11/documents/receipts/942', () => HttpResponse.json(invoice({ id: '942', state: 'new', reference: 'Till 42' }))),
    )
    renderAt('/11/documents/941')
    const form = await screen.findByRole('form', { name: 'Purchase invoice' })
    expect(within(form).getByText('2 documents left to process')).toBeInTheDocument()
    expect(within(form).getByRole('combobox', { name: 'Type' })).toHaveTextContent('Purchase invoice')
    const lines = within(form).getByRole('table', { name: 'Purchase invoice lines' })
    await userEvent.type(within(lines).getByRole('combobox', { name: 'Description' }), 'Paper')
    await userEvent.type(within(lines).getByRole('textbox', { name: 'Price' }), '100')
    expect(within(lines).getByRole('combobox', { name: 'VAT rate' })).toHaveDisplayValue('21% btw')
    expect(within(within(lines).getByRole('combobox', { name: 'VAT rate' })).queryByRole('option', { name: '21% sales' })).toBeNull()
    await userEvent.selectOptions(within(lines).getByRole('combobox', { name: 'Category' }), 'Other costs')
    expect(within(form).getByRole('table', { name: 'Totals' })).toHaveTextContent('€121.00')
    await userEvent.type(within(form).getByRole('textbox', { name: 'Days' }), '14')
    expect(within(form).getByRole('textbox', { name: 'Due date' })).toHaveValue('2026-11-15')
    await userEvent.click(within(form).getByRole('button', { name: 'Next' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Receipt Till 42' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/documents\/942$/)
    expect(sent).toMatchObject({
      contact_id: '701', reference: 'INV-1', date: '2026-11-01', due_date: '2026-11-15', currency: 'EUR', prices_are_incl_tax: false,
      details_attributes: [{ description: 'Paper', amount: '1', price: '100', tax_rate_id: '2201', ledger_account_id: '4001', row_order: 1 }],
    })
  })

  it('shows the problems of the server on the fields', async () => {
    server.use(...loaded({ doc: invoice({ state: 'new' }) }))
    server.use(http.patch('*/api/v2/11/documents/purchase_invoices/941', () => HttpResponse.json({ error: { reference: ['is too long'] } }, { status: 422 })))
    renderAt('/11/documents/941')
    const form = await screen.findByRole('form', { name: 'Purchase invoice' })
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    const field = await within(form).findByRole('textbox', { name: 'Invoice id' })
    await waitFor(() => expect(field).toHaveAttribute('aria-invalid', 'true'))
    expect(within(form).getByText('Invoice id is too long')).toBeInTheDocument()
  })

  it('gives an upload a type with the type picker and opens it as that type', async () => {
    let asked: unknown
    let typed = false
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/documents/951', () => HttpResponse.json(typed
        ? summary({ id: '951', type: 'Receipt', reference: 'scan.jpg', contact_id: null, contact_name: '', state: 'new' })
        : summary({ id: '951', type: 'TypelessDocument', reference: 'scan.jpg', contact_id: null, contact_name: '', state: 'new' }))),
      http.post('*/api/v2/11/documents/951/set_type', async ({ request }) => {
        asked = await request.json()
        typed = true
        return HttpResponse.json(summary({ id: '951', type: 'Receipt', reference: 'scan.jpg', state: 'new' }))
      }),
      http.get('*/api/v2/11/documents/receipts/951', () => HttpResponse.json({ ...invoice({ id: '951', state: 'new', reference: 'scan.jpg', details: [], contact: null, contact_id: null }), uri: '/11/documents/receipts/951' })),
    )
    renderAt('/11/documents/951')
    expect(await screen.findByRole('heading', { level: 1, name: 'scan.jpg' })).toBeInTheDocument()
    expect(await screen.findByRole('img', { name: 'scan.jpg' })).toHaveAttribute('src', 'blob:preview')
    const form = card()
    expect(within(form).queryByRole('textbox', { name: 'Reference' })).toBeNull()
    await userEvent.click(within(form).getByRole('combobox', { name: 'Type' }))
    const types = screen.getByRole('listbox', { name: 'Type' })
    expect(within(types).getAllByRole('option').map((o) => o.textContent)).toEqual([
      'Purchase invoiceInvoices for goods or services delivered', 'ReceiptReceipts for shopping, fuel, meals',
      'General documentFor the storage of tax returns, insurance policies, contracts', 'Manual journalFor the recording of opening balances, year-end, payroll costs',
    ])
    await userEvent.click(within(types).getByRole('option', { name: /Receipt/ }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Receipt scan.jpg' })).toBeInTheDocument()
    expect(asked).toEqual({ document_type: 'receipt' })
    await screen.findByRole('form', { name: 'Receipt' })
    expect(within(card()).getByRole('textbox', { name: 'Reference' })).toHaveValue('scan.jpg')
    expect(within(card()).getByRole('table', { name: 'Receipt lines' })).toBeInTheDocument()
    expect(within(card()).queryByRole('textbox', { name: 'Due date' })).toBeNull()
  })

  it('says why a document cannot change its type', async () => {
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/documents/981/set_type', () => HttpResponse.json({ error: 'A booked document cannot change its type' }, { status: 422 })))
    renderAt('/11/documents/981')
    await userEvent.click(await screen.findByRole('combobox', { name: 'Type' }))
    await userEvent.click(within(screen.getByRole('listbox', { name: 'Type' })).getByRole('option', { name: /Purchase invoice/ }))
    expect(await screen.findByText('A booked document cannot change its type')).toBeInTheDocument()
  })

  it('saves a general document', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/documents/general_documents/981', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ ...general, reference: 'Insurance 2027' })
    }))
    renderAt('/11/documents/981')
    expect(await screen.findByRole('heading', { level: 1, name: 'General document Insurance policy' })).toBeInTheDocument()
    await userEvent.click(within(screen.getByRole('region', { name: 'General document' })).getByRole('button', { name: 'Edit' }))
    const reference = within(card()).getByRole('textbox', { name: 'Reference' })
    await userEvent.clear(reference)
    await userEvent.type(reference, 'Insurance 2027')
    await userEvent.click(within(card()).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ general_document: { reference: 'Insurance 2027', date: '2026-11-02', due_date: '' }, remove_contact: true }))
  })

  it('keeps notes and shows the history of the document', async () => {
    let note: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/documents/purchase_invoices/941/notes', async ({ request }) => {
      note = await request.json()
      return HttpResponse.json({}, { status: 201 })
    }))
    renderAt('/11/documents/941')
    const notes = await screen.findByRole('region', { name: 'Notes and tasks' })
    expect(within(notes).getByText('Document saved')).toBeInTheDocument()
    await userEvent.type(within(notes).getByRole('textbox', { name: 'Add a note or to-do' }), 'Check the amount{Enter}')
    await waitFor(() => expect(note).toEqual({ note: { note: 'Check the amount', todo: false } }))
  })

  it('deletes the document after asking', async () => {
    let deleted = false
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/documents/purchase_invoices/941', () => { deleted = true; return new HttpResponse(null, { status: 204 }) }))
    renderAt('/11/documents/941')
    await userEvent.click(await screen.findByRole('button', { name: 'Delete document' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/documents$/))
    expect(deleted).toBe(true)
  })

  it('edits a manual journal in the same place', async () => {
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/documents/961', () => HttpResponse.json(summary({ id: '961', type: 'GeneralJournalDocument', reference: 'Opening balance', state: 'saved' }))),
      http.get('*/api/v2/11/documents/general_journal_documents/961', () => HttpResponse.json({
        id: '961', administration_id: '11', reference: 'Opening balance', date: '2026-01-01', journal_type: null, attachments: [], notes: [], events: [],
        general_journal_document_entries: [
          { id: '1', ledger_account_id: '4001', debit: '10.0', credit: '0.0', description: '' },
          { id: '2', ledger_account_id: '8001', debit: '0.0', credit: '10.0', description: '' },
        ],
        created_at: stamp, version: 1, uri: '/11/documents/general_journal_documents/961',
      })),
    )
    renderAt('/11/documents/961')
    expect(await screen.findByRole('heading', { level: 1, name: 'Manual journal Opening balance' })).toBeInTheDocument()
    const entry = screen.getByRole('form', { name: 'Entry' })
    expect(within(entry).getByRole('textbox', { name: /Reference/ })).toHaveValue('Opening balance')
    expect(screen.getByRole('combobox', { name: 'Type' })).toBeDisabled()
  })
})
