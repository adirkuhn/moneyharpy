import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import { type DocumentStatistics, type DocumentSummary, polling } from './documents'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const doc = (over: Partial<DocumentSummary>): DocumentSummary => ({
  id: '941', administration_id: '11', type: 'PurchaseInvoice', reference: '2026-01234', contact_id: '701', contact_name: 'Supplier B.V.',
  date: '2026-11-01', state: 'open', currency: 'EUR', total_price_incl_tax: '121.0', attachments_count: 1, created_at: stamp,
  uri: '/api/v2/11/documents/purchase_invoices/941', recognition_state: 'none', possible_duplicates: [],
  ...over,
})

const rows = [
  doc({}),
  doc({ id: '942', type: 'Receipt', reference: 'Till 0042', contact_name: 'Example Shop B.V.', state: 'saved', total_price_incl_tax: '12.5', uri: '/api/v2/11/documents/receipts/942' }),
  doc({ id: '951', type: 'TypelessDocument', reference: 'scan.pdf', contact_id: null, contact_name: '', state: 'new', total_price_incl_tax: '0.0', attachments_count: 0, uri: '/api/v2/11/documents/typeless_documents/951' }),
  doc({ id: '961', type: 'GeneralJournalDocument', reference: 'Opening balance', contact_id: null, contact_name: '', state: 'saved', total_price_incl_tax: '0.0', attachments_count: 0, uri: '/api/v2/11/documents/general_journal_documents/961' }),
]

const statistics: DocumentStatistics = {
  currency: 'EUR', costs_excl_tax: '1250.0', amount_payable: '302.5',
  categories: [{ ledger_account_id: '4001', name: 'Housing costs', amount: '1000.0' }, { ledger_account_id: null, name: 'Uncategorized expenses', amount: '250.0' }],
}

function loaded(seen: URL[] = [], list: DocumentSummary[] = rows) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/documents', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/documents/statistics', () => HttpResponse.json(statistics)),
    http.get('*/api/v2/11/contacts', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
  ]
}

const filterOf = (seen: URL[]) => seen.at(-1)?.searchParams.get('filter')

describe('the Incoming page', () => {
  it('has one Incoming entry in the menu that opens every incoming document', async () => {
    server.use(...loaded())
    renderAt('/11/feed')
    const nav = await screen.findByRole('navigation', { name: 'Administration menu' })
    const incoming = await within(nav).findByRole('link', { name: 'Incoming' })
    expect(incoming).toHaveAttribute('href', '/11/documents')
    expect(within(nav).queryByRole('button', { name: 'Incoming' })).toBeNull()
  })

  it('lists documents of every type with their type, contact, date, amount and status', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/documents')
    const table = await screen.findByRole('table', { name: 'Incoming' })
    expect(filterOf(seen)).toBe('period:this_year')
    expect(within(table).getByRole('link', { name: 'PUR 2026-01234' })).toHaveAttribute('href', '/11/documents/941')
    expect(within(table).getByRole('link', { name: 'REC Till 0042' })).toHaveAttribute('href', '/11/documents/942')
    expect(within(table).getByRole('link', { name: '— scan.pdf' })).toHaveAttribute('href', '/11/documents/951')
    expect(within(table).getByRole('link', { name: 'MAN Opening balance' })).toHaveAttribute('href', '/11/documents/961')
    expect(within(table).getAllByText('No contact')).toHaveLength(2)
    expect(within(table).getByText('€121.00')).toBeInTheDocument()
    expect(within(table).getAllByText('2026-11-01', { selector: 'td' })).toHaveLength(4)
    expect(within(table).getAllByText('Saved')[0]).toHaveClass('badge')
    expect(within(table).getAllByRole('img', { name: 'Has attachments' })).toHaveLength(2)
    expect(screen.getByRole('button', { name: 'Documents from this year' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add' })).toHaveAttribute('href', '/11/documents/new')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Incoming' })).toHaveAttribute('aria-current', 'page')
  })

  it('shows the card of a contact when its name is hovered', async () => {
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/contacts/701', () => HttpResponse.json({ id: '701', administration_id: '11', company_name: 'Supplier B.V.', firstname: null, lastname: null, country: 'NL', created_at: stamp })),
      http.get('*/api/v2/11/sales_invoices', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
    )
    renderAt('/11/documents')
    const table = await screen.findByRole('table', { name: 'Incoming' })
    const contact = within(table).getByRole('link', { name: 'Supplier B.V.' })
    expect(contact).toHaveAttribute('href', '/11/contacts/701')
    await userEvent.hover(contact)
    const card = await screen.findByRole('dialog', { name: 'Supplier B.V.' })
    await waitFor(() => expect(within(card).getByText('Documents').previousElementSibling).toHaveTextContent('4'))
  })

  it('marks the rows of documents that may be a copy of another', async () => {
    const copy = { id: '941', type: 'PurchaseInvoice' as const, reference: '2026-01234', date: '2026-11-01', total_price_incl_tax: '121.0', reasons: ['same_file' as const] }
    server.use(...loaded([], [doc({ id: '940', possible_duplicates: [copy] }), doc({})]))
    renderAt('/11/documents')
    const table = await screen.findByRole('table', { name: 'Incoming' })
    expect(within(table).getByRole('link', { name: 'PUR 2026-01234 Possible duplicate' })).toHaveAttribute('href', '/11/documents/940')
    expect(within(table).getByRole('link', { name: 'PUR 2026-01234' })).toHaveAttribute('href', '/11/documents/941')
    // An icon with a tooltip, not words, so a long reference keeps its width.
    expect(within(table).queryByText('Possible duplicate')).not.toBeInTheDocument()
    expect(within(table).getAllByRole('img', { name: 'Possible duplicate' })).toHaveLength(1)
    expect(within(table).getByRole('img', { name: 'Possible duplicate' })).toHaveAttribute('title', 'Possible duplicate')
  })

  it('filters on type, status and attachment through the filter in the address', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/documents')
    await userEvent.click(await screen.findByRole('button', { name: 'Documents from this year' }))
    const panel = screen.getByRole('search', { name: 'Filters' })
    expect(within(panel).getByRole('link', { name: 'Manual journals' })).toHaveAttribute('href', '/11/documents?filter=type%3Ageneral_journal_document')
    expect(within(panel).getByRole('link', { name: 'Unpaid documents' })).toHaveAttribute('href', '/11/documents?filter=state%3Aopen%7Clate')
    const status = within(panel).getByRole('group', { name: 'Status' })
    expect(within(status).getAllByRole('radio').map((r) => r.closest('label')?.textContent)).toEqual(
      ['All', 'New', 'Saved', 'Open', 'Paid', 'Pending payment', 'Expired'])
    await userEvent.click(within(panel).getByRole('radio', { name: 'Receipt' }))
    await userEvent.click(within(within(panel).getByRole('group', { name: 'Attachment' })).getByRole('radio', { name: 'With attachment' }))
    expect(within(panel).getByRole('group', { name: 'Recurring invoice' })).toBeInTheDocument()
    expect(within(panel).getByRole('group', { name: 'Automatic saving' })).toBeInTheDocument()
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(filterOf(seen)).toBe('period:this_year,type:receipt,attachment:with'))
    expect(screen.getByTestId('search')).toHaveTextContent('filter=period%3Athis_year%2Ctype%3Areceipt%2Cattachment%3Awith')
    expect(screen.getByRole('button', { name: 'Filtered documents' })).toBeInTheDocument()
  })

  it('shows the statistics of the selection', async () => {
    const asked: URL[] = []
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/documents/statistics', ({ request }) => {
      asked.push(new URL(request.url))
      return HttpResponse.json(statistics)
    }))
    renderAt('/11/documents?filter=type%3Areceipt')
    await screen.findByRole('table', { name: 'Incoming' })
    expect(asked).toHaveLength(0)
    await userEvent.click(screen.getByRole('button', { name: 'Show statistics' }))
    const panel = await screen.findByRole('region', { name: 'Statistics' })
    expect(await within(panel).findByText('€1,250.00')).toBeInTheDocument()
    expect(within(panel).getByText('€302.50')).toBeInTheDocument()
    expect(within(panel).getByRole('link', { name: 'Show profit and loss statement' })).toHaveAttribute('href', '/11/report/profit_loss')
    expect(within(panel).getByRole('link', { name: 'Show aged payables report' })).toHaveAttribute('href', '/11/report/creditors')
    expect(within(panel).getByText('Housing costs')).toBeInTheDocument()
    expect(within(panel).getByText('Uncategorized expenses')).toBeInTheDocument()
    expect(asked[0]?.searchParams.get('filter')).toBe('type:receipt')
  })

  it('makes a document of every file dropped on the bar', async () => {
    const made: unknown[] = []
    const attached: string[] = []
    const seen: URL[] = []
    server.use(...loaded(seen))
    server.use(
      http.post('*/api/v2/11/documents/typeless_documents', async ({ request }) => {
        made.push(await request.json())
        return HttpResponse.json({ id: `95${made.length}` }, { status: 201 })
      }),
      http.post('*/api/v2/11/documents/typeless_documents/:id/attachments', ({ params }) => {
        attached.push(String(params.id))
        return HttpResponse.json({}, { status: 201 })
      }),
    )
    renderAt('/11/documents')
    await screen.findByRole('table', { name: 'Incoming' })
    const before = seen.length
    await userEvent.upload(screen.getByLabelText('Drag your documents here or click to add them'), [
      new File(['a'], 'bill-1.pdf', { type: 'application/pdf' }),
      new File(['b'], 'bill-2.pdf', { type: 'application/pdf' }),
    ])
    await waitFor(() => expect(attached).toEqual(['951', '952']))
    expect(made).toEqual([{ typeless_document: { reference: 'bill-1.pdf' } }, { typeless_document: { reference: 'bill-2.pdf' } }])
    await waitFor(() => expect(seen.length).toBeGreaterThan(before))
  })

  it('shows the uploads being read as Recognising, and reloads until they are read', async () => {
    const quick = polling.intervalMs
    polling.intervalMs = 20
    try {
      let reading = true
      const seen: URL[] = []
      server.use(...loaded())
      server.use(http.get('*/api/v2/11/documents', ({ request }) => {
        seen.push(new URL(request.url))
        const list = reading
          ? [rows[0] as DocumentSummary, doc({ id: '953', type: 'TypelessDocument', reference: 'till.jpg', contact_id: null, contact_name: '', state: 'new', recognition_state: 'pending' })]
          : [rows[0] as DocumentSummary, doc({ id: '953', type: 'Receipt', reference: 'Till 0077', state: 'new', recognition_state: 'done' })]
        return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
      }))
      renderAt('/11/documents')
      const table = await screen.findByRole('table', { name: 'Incoming' })
      expect(within(table).getByRole('link', { name: /^— till\.jpg\s*Recognising…$/ })).toHaveAttribute('href', '/11/documents/953')
      // Only a spinner shows, so the reference keeps its one line; the words are for screen readers and the tooltip.
      expect(within(table).queryByText('Recognising…')).not.toBeInTheDocument()
      expect(within(table).getByRole('img', { name: 'Recognising…' })).toHaveAttribute('title', 'Recognising…')
      reading = false
      expect(await within(table).findByRole('link', { name: 'REC Till 0077' })).toBeInTheDocument()
      const count = seen.length
      await new Promise((r) => setTimeout(r, 100))
      expect(seen.length).toBe(count)
    } finally {
      polling.intervalMs = quick
    }
  })

  it('introduces the page while there are no documents yet', async () => {
    server.use(...loaded([], []))
    renderAt('/11/documents')
    expect(await screen.findByRole('heading', { name: 'Purchase invoices and receipts' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Upload' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Checking' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Pay' })).toBeInTheDocument()
  })

  it('deletes the documents ticked', async () => {
    const deleted: string[] = []
    server.use(...loaded())
    server.use(
      http.delete('*/api/v2/11/documents/receipts/942', () => { deleted.push('receipt'); return HttpResponse.json(200) }),
      http.delete('*/api/v2/11/documents/typeless_documents/951', () => { deleted.push('upload'); return HttpResponse.json(200) }),
    )
    renderAt('/11/documents')
    const table = await screen.findByRole('table', { name: 'Incoming' })
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select REC Till 0042' }))
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select — scan.pdf' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete documents' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete 2 documents permanently' }))
    await waitFor(() => expect(deleted).toEqual(['receipt', 'upload']))
    expect(await screen.findByText('2 documents deleted.')).toBeInTheDocument()
  })

  it('sends the old lists to Incoming with their type', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/documents/receipts?filter=contact_id%3A901')
    await screen.findByRole('table', { name: 'Incoming' })
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/documents$/)
    expect(filterOf(seen)).toBe('contact_id:901,type:receipt')
  })

  it('sends the old purchase invoice list to this year\'s purchase invoices', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/documents/purchase_invoices')
    await screen.findByRole('table', { name: 'Incoming' })
    expect(filterOf(seen)).toBe('period:this_year,type:purchase_invoice')
  })
})

describe('adding an incoming document', () => {
  it('makes one document of all the files and opens it', async () => {
    const made: unknown[] = []
    const attached: string[] = []
    server.use(...loaded())
    server.use(
      http.post('*/api/v2/11/documents/typeless_documents', async ({ request }) => {
        made.push(await request.json())
        return HttpResponse.json({ id: '957' }, { status: 201 })
      }),
      http.post('*/api/v2/11/documents/typeless_documents/957/attachments', () => {
        attached.push('957')
        return HttpResponse.json({}, { status: 201 })
      }),
      http.get('*/api/v2/11/documents/957', () => HttpResponse.json(rows[2])),
    )
    renderAt('/11/documents/new')
    expect(await screen.findByText('And leave the heavy lifting to us.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'To Incoming' })).toHaveAttribute('href', '/11/documents')
    await userEvent.upload(screen.getByLabelText('Drag files here or click to select'), [
      new File(['a'], 'page-1.pdf', { type: 'application/pdf' }),
      new File(['b'], 'page-2.pdf', { type: 'application/pdf' }),
    ])
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/documents/957'))
    expect(made).toEqual([{ typeless_document: { reference: 'page-1.pdf' } }])
    expect(attached).toEqual(['957', '957'])
  })

  it('continues without a document to the form of a type', async () => {
    server.use(...loaded())
    renderAt('/11/documents/new')
    await userEvent.click(await screen.findByRole('button', { name: 'Or continue without document' }))
    const types = screen.getByRole('list', { name: 'Type' })
    expect(within(types).getByRole('button', { name: /Purchase invoice\s*Invoices for goods or services delivered/ })).toBeInTheDocument()
    expect(within(types).getByRole('button', { name: /General document/ })).toBeInTheDocument()
    expect(within(types).getByRole('link', { name: /Manual journal\s*For the recording of opening balances, year-end, payroll costs/ }))
      .toHaveAttribute('href', '/11/documents/general_journal_documents/new')
  })

  it('makes an empty receipt and opens it', async () => {
    let body: unknown
    server.use(...loaded())
    server.use(
      http.post('*/api/v2/11/documents/receipts', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ id: '944' }, { status: 201 })
      }),
      http.get('*/api/v2/11/documents/944', () => HttpResponse.json(doc({ id: '944', type: 'Receipt', state: 'new' }))),
      http.get('*/api/v2/11/documents/receipts/944', () => HttpResponse.json({})),
    )
    renderAt('/11/documents/new')
    await userEvent.click(await screen.findByRole('button', { name: 'Or continue without document' }))
    await userEvent.click(screen.getByRole('button', { name: /Receipt\s*Receipts for shopping, fuel, meals/ }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/documents\/944$/))
    expect(body).toEqual({ receipt: { date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/) } })
  })
})
