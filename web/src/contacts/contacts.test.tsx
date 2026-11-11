import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn, workflow } from '../test/server'
import type { Contact } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const contact = (over: Partial<Contact> = {}): Contact => ({
  id: '901', administration_id: '11', company_name: 'Example Shop B.V.', firstname: null, lastname: null, address1: 'Dam 1', address2: null,
  zipcode: '1012JS', city: 'Amsterdam', country: 'NL', phone: null, delivery_method: 'Email', customer_id: '1', tax_number: null,
  chamber_of_commerce: null, bank_account: null, is_trusted: false, max_transfer_amount: null, attention: null, email: 'billing@example.com',
  email_ubl: false, send_invoices_to_attention: null, send_invoices_to_email: 'billing@example.com', send_estimates_to_attention: null,
  send_estimates_to_email: null, direct_debit: false, sepa_active: false, sepa_iban: null, sepa_iban_account_name: null, sepa_bic: null,
  sepa_mandate_id: null, sepa_mandate_date: null, sepa_sequence_type: 'RCUR', credit_card_number: null, credit_card_reference: null,
  credit_card_type: null, tax_number_validated_at: null, tax_number_valid: null, invoice_workflow_id: null, estimate_workflow_id: null,
  si_identifier: null, si_identifier_type: null, moneybird_payments_mandate: false, created_at: stamp, updated_at: stamp, version: 1,
  notes: [], custom_fields: [], contact_people: [], archived: false, events: [], uri: '/11/contacts/901',
  ...over,
})
const ann = contact({ id: '902', company_name: '', firstname: 'Ann', lastname: 'Jansen', customer_id: '2', email: null, send_invoices_to_email: null, city: 'Utrecht', uri: '/11/contacts/902' })

type Options = { total?: number; archived?: number; doubles?: Contact[][]; companySearch?: boolean }

function loaded(list: Contact[] = [contact(), ann], seen: URL[] = [], { total = list.length, archived = 0, doubles = [], companySearch = false }: Options = {}) {
  const page = (request: Request, rows: Contact[], count: number) => {
    const url = new URL(request.url)
    seen.push(url)
    const all = url.searchParams.get('include_archived') === 'true' ? count + archived : count
    return HttpResponse.json(url.searchParams.get('per_page') === '1' ? rows.slice(0, 1) : rows, { headers: { 'X-Total-Count': String(all) } })
  }
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/workflows', () => HttpResponse.json([
      workflow({ id: '601', name: 'Standard invoices' }),
      workflow({ id: '602', type: 'EstimateWorkflow', name: 'Standard quotes' }),
    ])),
    http.get('*/api/v2/11/custom_fields', () => HttpResponse.json([])),
    http.get('*/api/onboarding/options', () => HttpResponse.json({
      countries: ['BE', 'DE', 'NL'], currencies: ['EUR'], popular_currencies: ['EUR'], company_search_countries: companySearch ? ['NL'] : [], address_lookup_countries: [],
    })),
    http.get('*/api/v2/11/contacts', ({ request }) => page(request, list, total)),
    http.get('*/api/v2/11/contacts/filter', ({ request }) => page(request, list.slice(0, 1), 1)),
    http.get('*/api/v2/11/contacts/next_customer_id', () => HttpResponse.json({ customer_id: '43' })),
    http.get('*/api/v2/11/contacts/doubles', () => HttpResponse.json(doubles.map((contacts) => ({ contacts })))),
    http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact())),
    // The documents of a contact, for its feed: none unless a test says otherwise.
    ...['sales_invoices', 'external_sales_invoices', 'estimates', 'documents/purchase_invoices', 'documents/receipts'].map((path) =>
      http.get(`*/api/v2/11/${path}`, () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } }))),
  ]
}

/** The text of the cells of each row of the contacts table, under its header. */
function rowsOf(table: HTMLElement) {
  return within(table).getAllByRole('row').slice(1).map((row) => within(row).getAllByRole('cell').map((cell) => cell.textContent))
}

describe('contacts list', () => {
  it('lists contacts by name, as Moneybird does', async () => {
    server.use(...loaded([contact({ firstname: 'Ann', lastname: 'de Vries' }), ann]))
    renderAt('/11/contacts')
    const table = await screen.findByRole('table', { name: 'Contacts' })
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['', 'Name'])
    expect(within(table).getByRole('link', { name: 'Example Shop B.V. — Ann de Vries' })).toHaveAttribute('href', '/11/contacts/901')
    expect(within(table).getByRole('link', { name: 'Ann Jansen' })).toHaveAttribute('href', '/11/contacts/902')
    expect(screen.getByRole('button', { name: 'Contacts' })).toBeInTheDocument()
    expect(screen.getByRole('searchbox', { name: 'Search for contacts or companies' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Export' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Import' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add' })).toHaveAttribute('href', '/11/contacts/new')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Contacts' })).toHaveAttribute('aria-current', 'page')
  })

  it('shows the person of a company in grey after its name', async () => {
    server.use(...loaded([contact({ firstname: 'Ann', lastname: 'de Vries' })]))
    renderAt('/11/contacts')
    const table = await screen.findByRole('table', { name: 'Contacts' })
    const person = within(table).getByText('Ann de Vries')
    expect(person).toHaveClass('mute')
    expect(person.parentElement).toHaveTextContent(/^Example Shop B\.V\. — Ann de Vries$/)
  })

  it('groups the contacts by their first letter', async () => {
    server.use(...loaded([
      contact({ id: '1', company_name: '- shop' }),
      contact({ id: '2', company_name: '123 Repairs' }),
      contact({ id: '3', company_name: '123 Repairs West' }),
      contact({ id: '4', company_name: '8wraps' }),
      contact({ id: '5', company_name: 'Acme' }),
      contact({ id: '6', company_name: '', firstname: 'Ann', lastname: 'Jansen' }),
      contact({ id: '7', company_name: 'Één Bakery' }),
      contact({ id: '8', company_name: 'example shop' }),
    ]))
    renderAt('/11/contacts')
    const table = await screen.findByRole('table', { name: 'Contacts' })
    expect(rowsOf(table).map((cells) => cells[0])).toEqual(['-', '1', '', '8', 'A', '', 'E', ''])
  })

  it('searches as the user types', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/contacts')
    await userEvent.type(await screen.findByRole('searchbox', { name: 'Search for contacts or companies' }), 'shop')
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('query')).toBe('shop'))
    expect(screen.getByTestId('path')).toHaveTextContent('/11/contacts')
  })

  it("offers Moneybird's quick filters", async () => {
    server.use(...loaded())
    renderAt('/11/contacts?filter=trusted_type:trusted')
    await userEvent.click(await screen.findByRole('button', { name: 'Trusted contacts' }))
    const panel = screen.getByRole('search', { name: 'Contact filters' })
    expect(within(panel).getByRole('link', { name: 'Trusted contacts' })).toHaveAttribute('aria-current', 'true')
    expect(within(panel).getByRole('link', { name: 'All contacts' })).toHaveAttribute('href', '/11/contacts')
    expect(within(panel).getByRole('link', { name: 'Contacts with delivery method manual' })).toHaveAttribute('href', '/11/contacts?filter=delivery_method%3Amanual')
    expect(within(panel).getByRole('link', { name: 'Contacts with delivery method e-mail' })).toHaveAttribute('href', '/11/contacts?filter=delivery_method%3Aemail')
  })

  it('filters on type, trust, workflows, mandate and delivery method', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/contacts')
    await screen.findByRole('table', { name: 'Contacts' })
    await userEvent.click(screen.getByRole('button', { name: 'Contacts' }))
    const panel = screen.getByRole('search', { name: 'Contact filters' })
    const group = (name: string) => within(panel).getByRole('group', { name })
    expect(within(group('Trusted contact')).getAllByRole('radio').map((r) => r.closest('label')?.textContent)).toEqual(['All', 'Trusted', 'Not trusted'])
    await userEvent.click(within(group('Contact type')).getByRole('radio', { name: 'Company' }))
    await userEvent.click(within(group('Mandate')).getByRole('radio', { name: 'Manual mandate' }))
    await userEvent.click(within(group('Delivery method')).getByRole('radio', { name: 'Peppol' }))
    expect(within(panel).getByRole('combobox', { name: 'Estimate workflow' })).toHaveDisplayValue('All workflows')
    await userEvent.selectOptions(within(panel).getByRole('combobox', { name: 'Invoice workflow' }), 'Standard invoices')
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.pathname).toBe('/api/v2/11/contacts/filter'))
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('contact_type:company,invoice_workflow_id:601,mandate_type:manual,delivery_method:peppol')
    expect(screen.getByRole('button', { name: 'Filtered contacts' })).toBeInTheDocument()
  })

  it('counts the deactivated contacts under the list and shows them on request', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen, { archived: 3 }))
    renderAt('/11/contacts')
    expect(await screen.findByText('There are 3 deactivated items.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Show deactivated items.' }))
    await waitFor(() => expect(seen.some((u) => u.searchParams.get('include_archived') === 'true' && u.searchParams.get('per_page') === '50')).toBe(true))
    await userEvent.click(await screen.findByRole('button', { name: 'Hide deactivated items.' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Show deactivated items.' })).toBeInTheDocument())
  })

  it('says nothing about deactivated contacts when there are none', async () => {
    server.use(...loaded())
    renderAt('/11/contacts')
    await screen.findByRole('table', { name: 'Contacts' })
    expect(screen.queryByText(/deactivated/)).toBeNull()
  })

  it('points out possible duplicates and lists them', async () => {
    const twin = contact({ id: '903', uri: '/11/contacts/903' })
    server.use(...loaded(undefined, undefined, { doubles: [[contact(), twin]] }))
    renderAt('/11/contacts')
    const banner = await screen.findByRole('region', { name: '2 possible duplicate contacts found' })
    expect(within(banner).getByText('Merge them to process documents even more accurately.')).toBeInTheDocument()
    await userEvent.click(within(banner).getByRole('link', { name: 'View contacts' }))
    const table = await screen.findByRole('table', { name: 'Contacts' })
    await waitFor(() => expect(within(table).getAllByRole('link', { name: 'Example Shop B.V.' }).map((a) => a.getAttribute('href'))).toEqual(['/11/contacts/901', '/11/contacts/903']))
    expect(screen.getByRole('button', { name: 'Possible duplicates' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: /duplicate contacts found/ })).toBeNull()
  })

  it('opens a contact from anywhere in its row', async () => {
    server.use(...loaded())
    renderAt('/11/contacts')
    const table = await screen.findByRole('table', { name: 'Contacts' })
    await userEvent.click(within(table).getAllByRole('row')[2]!.querySelector('td')!)
    expect(screen.getByTestId('path')).toHaveTextContent('/11/contacts/902')
  })

  it('pages through many contacts', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen, { total: 120 }))
    renderAt('/11/contacts')
    const pages = await screen.findByRole('navigation', { name: 'Pagination' })
    expect(within(pages).getByText('1')).toHaveAttribute('aria-current', 'page')
    await userEvent.click(within(pages).getByRole('link', { name: 'Page 3' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('page')).toBe('3'))
    expect(within(screen.getByRole('navigation', { name: 'Pagination' })).getByText('3')).toHaveAttribute('aria-current', 'page')
  })

  it('invites adding the first contact', async () => {
    server.use(...loaded([]))
    renderAt('/11/contacts')
    expect(await screen.findByText('No contacts yet. Add your first customer or supplier.')).toBeInTheDocument()
  })
})

describe('a contact', () => {
  /** Opens one of the menus of the action bar, such as Extra, and chooses an entry. */
  async function choose(menu: string, entry: string) {
    await userEvent.click(await screen.findByRole('button', { name: menu }))
    await userEvent.click(screen.queryByRole('button', { name: entry }) ?? screen.getByRole('link', { name: entry }))
  }

  const person = { id: '911', contact_id: '901', administration_id: '11', firstname: 'Zoë', lastname: null, phone: null, email: 'zoe@example.com', department: 'Finance', created_at: stamp, updated_at: stamp, version: 1 }

  it("shows a company in Moneybird's contact card", async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact({ chamber_of_commerce: '12345678', tax_number: 'NL000099998B57', phone: null }))))
    renderAt('/11/contacts/901')
    expect(await screen.findByRole('heading', { level: 1, name: 'Contact' })).toBeInTheDocument()
    const card = await screen.findByRole('region', { name: 'Example Shop B.V.' })
    expect(within(card).getByRole('heading', { level: 2, name: 'Example Shop B.V.' })).toBeInTheDocument()
    expect(within(card).getByText('Client number: 1')).toBeInTheDocument()
    expect(within(card).getByRole('link', { name: 'Open address in Google Maps' })).toHaveAttribute('href', expect.stringContaining('google.com/maps/search/?api=1&query=Dam%201'))
    expect(within(card).getByRole('link', { name: 'View the Dutch Chamber of Commerce page of this company' })).toHaveAttribute('href', expect.stringContaining('12345678'))
    expect(within(card).queryByRole('link', { name: 'Call this contact' })).toBeNull()
    const facts = within(card).getAllByRole('term').map((dt) => dt.textContent)
    expect(facts).toEqual(['Email address', 'Address', 'Netherlands Chamber of Commerce KVK number', 'VAT number'])
    expect(within(card).getByText('NL000099998B57')).toBeInTheDocument()
    expect(within(card).getByRole('button', { name: 'Copy VAT number' })).toBeInTheDocument()
    expect(within(card).queryByText(/Additional information may be available/)).toBeNull()
    expect(within(card).getByText('billing@example.com')).toBeInTheDocument()
    expect(within(card).getByText('Dam 1')).toBeInTheDocument()
    expect(within(card).getByText('1012JS Amsterdam')).toBeInTheDocument()
    expect(within(card).getByText('Netherlands')).toBeInTheDocument()
    expect(within(card).getByRole('button', { name: 'Add contact person' })).toBeInTheDocument()

    expect(screen.getByRole('link', { name: 'Edit' })).toHaveAttribute('href', '/11/contacts/901/edit')
    expect(screen.getByRole('button', { name: 'Export' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Extra' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add' })).toBeInTheDocument()
    expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual(['Feed', 'Details', 'Tasks and notes'])
    expect(screen.getByRole('tab', { name: 'Feed' })).toHaveAttribute('aria-selected', 'true')
  })

  it('suggests the Dutch Chamber of Commerce for a Dutch company without a KVK number', async () => {
    server.use(...loaded([contact(), ann], [], { companySearch: true }))
    renderAt('/11/contacts/901')
    const card = await screen.findByRole('region', { name: 'Example Shop B.V.' })
    expect(await within(card).findByText(/Additional information may be available for this contact\./)).toBeInTheDocument()
    expect(within(card).getByRole('link', { name: 'Add information from the Dutch Chamber of Commerce' })).toHaveAttribute('href', '/11/contacts/901/edit')
  })

  it('offers to call, email and find a private person', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/contacts/902', () => HttpResponse.json({ ...ann, phone: '0612345678', email: 'ann@example.com' })))
    renderAt('/11/contacts/902')
    const card = await screen.findByRole('region', { name: 'Ann Jansen' })
    const quick = within(card).getAllByRole('link').filter((l) => l.classList.contains('btn--icon')).map((l) => l.getAttribute('aria-label'))
    expect(quick).toEqual(['Call this contact', 'Email this contact', 'Open address in Google Maps'])
    expect(within(card).getByRole('link', { name: 'Call this contact' })).toHaveAttribute('href', 'tel:0612345678')
    expect(within(card).getByRole('link', { name: 'Email this contact' })).toHaveAttribute('href', 'mailto:ann@example.com')
    expect(within(card).getAllByRole('term').map((dt) => dt.textContent)).toEqual(['Email address', 'Phone number', 'Address'])
    expect(within(card).queryByRole('button', { name: 'Add contact person' })).toBeNull()
  })

  it("lists the contact's documents in its feed and counts them", async () => {
    const seen: URL[] = []
    const invoice = { id: '501', contact_id: '901', contact: contact(), invoice_id: '2026-0001', reference: 'Project X', invoice_date: '2026-10-02', state: 'paid', total_price_incl_tax: '121.0', currency: 'EUR', details: [{ description: 'Design work' }, { description: 'Hosting' }] }
    const draft = { ...invoice, id: '502', invoice_id: null, draft_id: '3', reference: '', invoice_date: '2026-11-05', state: 'draft', details: [{ description: 'Support' }] }
    const receipt = (id: string, date: string) => ({ id, contact_id: '901', contact: contact(), reference: `R-${id}`, date, state: 'paid', total_price_incl_tax: '25.1', currency: 'EUR', details: [{ description: 'Office supplies' }] })
    const page = (rows: unknown[]) => ({ request }: { request: Request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(rows, { headers: { 'X-Total-Count': String(rows.length) } })
    }
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/sales_invoices', page([draft, invoice])),
      http.get('*/api/v2/11/documents/receipts', page([receipt('601', '2026-09-16'), receipt('602', '2025-08-03')])),
    )
    renderAt('/11/contacts/901')
    const card = await screen.findByRole('region', { name: 'Example Shop B.V.' })
    expect(await within(card).findByRole('link', { name: '2 Invoices' })).toHaveAttribute('href', '/11/sales_invoices?filter=contact_id:901')
    expect(within(card).getByRole('link', { name: '2 Receipts' })).toHaveAttribute('href', '/11/documents/receipts?filter=contact_id:901')
    expect(within(card).queryByRole('link', { name: /Quote/ })).toBeNull()
    expect(seen.find((u) => u.pathname.endsWith('/sales_invoices'))?.searchParams.get('filter')).toBe('state:all,contact_id:901')

    const feed = screen.getByRole('tabpanel', { name: 'Feed' })
    const open = within(feed).getByRole('list', { name: 'To process' })
    expect(within(open).getAllByRole('link').map((l) => l.getAttribute('href'))).toEqual(['/11/sales_invoices/502'])
    expect(within(feed).queryByText('Everything has been processed!')).toBeNull()
    const processed = within(feed).getByRole('list', { name: 'Recently processed' })
    const rows = within(processed).getAllByRole('link')
    expect(rows.map((l) => l.getAttribute('href'))).toEqual(['/11/sales_invoices/501', '/11/documents/receipts/601', '/11/documents/receipts/602'])
    expect(rows[0]).toHaveTextContent('Example Shop B.V. · 2026-0001 · Project X')
    expect(rows[0]).toHaveTextContent('Design work · Hosting')
    expect(rows[0]).toHaveTextContent('€+121.00')
    expect(rows[0]).toHaveTextContent('Paid')
    expect(rows[0]).toHaveTextContent('Oct 2')
    expect(rows[1]).toHaveTextContent('€-25.10')
    expect(rows[2]).toHaveTextContent('Aug 3, 2025')
  })

  it('says everything has been processed when nothing waits', async () => {
    server.use(...loaded())
    renderAt('/11/contacts/901')
    const feed = await screen.findByRole('tabpanel', { name: 'Feed' })
    expect(await within(feed).findByRole('heading', { name: 'Everything has been processed!' })).toBeInTheDocument()
    expect(within(feed).getByText('Nothing processed yet.')).toBeInTheDocument()
  })

  it('shows its details by section', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact({
      tax_number: 'NL000099998B57', sepa_iban: 'NL91ABNA0417164300', sepa_iban_account_name: 'Example Shop', sepa_active: true, sepa_mandate_id: 'MANDATE-1',
      sepa_mandate_date: '2026-11-01', invoice_workflow_id: '601', custom_fields: [{ id: '1', name: 'Region', value: 'North' }],
    }))))
    renderAt('/11/contacts/901')
    await userEvent.click(await screen.findByRole('tab', { name: 'Details' }))
    const details = screen.getByRole('tabpanel', { name: 'Details' })
    expect(within(details).getAllByRole('heading', { level: 3 }).map((h) => h.textContent)).toEqual(['Contact', 'Sending', 'Payment', 'Workflow', 'Custom fields'])
    const row = (section: string, label: string) => within(within(details).getByRole('region', { name: section })).getByText(label).nextElementSibling?.textContent
    expect(row('Contact', 'VAT-id')).toBe('NL000099998B57')
    expect(row('Sending', 'Delivery method')).toBe('Email')
    expect(row('Sending', 'Email address for invoices')).toBe('billing@example.com')
    expect(row('Payment', 'IBAN')).toBe('NL91 ABNA 0417 1643 00')
    expect(row('Payment', 'Mandate')).toBe('MANDATE-1 · 2026-11-01')
    expect(row('Workflow', 'For invoices')).toBe('Standard invoices')
    expect(row('Custom fields', 'Region')).toBe('North')
  })

  it('adds, edits and removes contact people', async () => {
    const sent: unknown[] = []
    let removed = ''
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact({ contact_people: [person] }))),
      http.post('*/api/v2/11/contacts/901/contact_people', async ({ request }) => {
        sent.push(await request.json())
        return HttpResponse.json({ ...person, id: '912', firstname: 'Ann' }, { status: 201 })
      }),
      http.patch('*/api/v2/11/contacts/901/contact_people/911', async ({ request }) => {
        sent.push(await request.json())
        return HttpResponse.json(person)
      }),
      http.delete('*/api/v2/11/contacts/901/contact_people/911', () => {
        removed = '911'
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/contacts/901')
    const people = await screen.findByRole('region', { name: 'Contact people' })
    expect(await within(people).findByText('Zoë')).toBeInTheDocument()
    expect(within(people).getByText('Finance')).toBeInTheDocument()

    await userEvent.click(within(people).getByRole('button', { name: 'Add contact person' }))
    await userEvent.type(within(people).getByLabelText('First name'), 'Ann')
    await userEvent.click(within(people).getByRole('button', { name: 'Save contact person' }))
    await waitFor(() => expect(sent[0]).toEqual({ contact_person: { firstname: 'Ann', lastname: '', email: '', phone: '', department: '' } }))

    await userEvent.click(within(people).getByRole('button', { name: 'Edit Zoë' }))
    expect(within(people).getByLabelText('Email')).toHaveValue('zoe@example.com')
    await userEvent.clear(within(people).getByLabelText('Department'))
    await userEvent.type(within(people).getByLabelText('Department'), 'Sales')
    await userEvent.click(within(people).getByRole('button', { name: 'Save contact person' }))
    await waitFor(() => expect(sent[1]).toEqual({ contact_person: { firstname: 'Zoë', lastname: '', email: 'zoe@example.com', phone: '', department: 'Sales' } }))

    await userEvent.click(await within(people).findByRole('button', { name: 'Remove Zoë' }))
    await userEvent.click(within(people).getByRole('button', { name: 'Confirm remove Zoë' }))
    await waitFor(() => expect(removed).toBe('911'))
  })

  it('keeps tasks and notes in their own tab', async () => {
    const sent: unknown[] = []
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact({
        notes: [{ id: '921', administration_id: '11', entity_id: '901', entity_type: 'Contact', user_id: '1', assignee_id: null, todo: true, note: 'Call back', completed_at: null, completed_by_id: null, todo_type: null, data: null, created_at: stamp, updated_at: stamp }],
      }))),
      http.post('*/api/v2/11/contacts/901/notes', async ({ request }) => {
        sent.push(await request.json())
        return HttpResponse.json({}, { status: 201 })
      }),
    )
    renderAt('/11/contacts/901?tab=notes')
    const panel = await screen.findByRole('tabpanel', { name: 'Tasks and notes' })
    expect(screen.getByRole('tab', { name: 'Tasks and notes' })).toHaveAttribute('aria-selected', 'true')
    const notes = within(panel).getByRole('region', { name: 'Notes' })
    expect(await within(notes).findByText('Call back')).toBeInTheDocument()
    expect(within(notes).getByText('To do')).toBeInTheDocument()
    await userEvent.type(within(notes).getByLabelText('New note'), 'Sent the contract')
    await userEvent.click(within(notes).getByRole('checkbox', { name: 'This is a to-do' }))
    await userEvent.click(within(notes).getByRole('button', { name: 'Add note' }))
    await waitFor(() => expect(sent[0]).toEqual({ note: { note: 'Sent the contract', todo: true } }))
  })

  it('moves between the tabs with the arrow keys', async () => {
    server.use(...loaded())
    renderAt('/11/contacts/901')
    const feed = await screen.findByRole('tab', { name: 'Feed' })
    feed.focus()
    await userEvent.keyboard('{ArrowRight}')
    expect(screen.getByRole('tab', { name: 'Details' })).toHaveFocus()
    expect(screen.getByRole('tab', { name: 'Details' })).toHaveAttribute('aria-selected', 'true')
    await userEvent.keyboard('{ArrowLeft}{ArrowLeft}')
    expect(screen.getByRole('tab', { name: 'Tasks and notes' })).toHaveAttribute('aria-selected', 'true')
  })

  it('adds documents for the contact', async () => {
    server.use(...loaded())
    renderAt('/11/contacts/901')
    await userEvent.click(await screen.findByRole('button', { name: 'Add' }))
    expect(screen.getByRole('link', { name: 'Invoice' })).toHaveAttribute('href', '/11/sales_invoices/new?contact_id=901')
    expect(screen.getByRole('link', { name: 'External invoice' })).toHaveAttribute('href', '/11/external_sales_invoices/new?contact_id=901')
    expect(screen.getByRole('link', { name: 'Recurring invoice' })).toHaveAttribute('href', '/11/recurring_sales_invoices/new?contact_id=901')
    expect(screen.getByRole('link', { name: 'Quote' })).toHaveAttribute('href', '/11/estimates/new?contact_id=901')
  })

  it('offers the exports of Moneybird, not available yet', async () => {
    server.use(...loaded())
    renderAt('/11/contacts/901')
    await userEvent.click(await screen.findByRole('button', { name: 'Export' }))
    expect(screen.getByText('Excel').closest('[aria-disabled]')).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByText('vCard').closest('[aria-disabled]')).toHaveAttribute('aria-disabled', 'true')
  })

  it('archives and brings back a contact', async () => {
    let archived = false
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact({ archived }))),
      http.patch('*/api/v2/11/contacts/901/archive', () => {
        archived = true
        return new HttpResponse(null, { status: 204 })
      }),
      http.patch('*/api/v2/11/contacts/901', async ({ request }) => {
        expect(await request.json()).toEqual({ contact: { archived: false } })
        archived = false
        return HttpResponse.json(contact())
      }),
    )
    renderAt('/11/contacts/901')
    await choose('Extra', 'Archive contact')
    const confirm = screen.getByRole('alertdialog', { name: 'Are you sure you want to archive Example Shop B.V.?' })
    expect(confirm).toHaveTextContent('You can undo this at any time. All data will be retained.')
    await userEvent.click(within(confirm).getByRole('button', { name: 'Archive' }))
    expect(await screen.findByText('This contact is archived. It no longer shows in lists.')).toBeInTheDocument()
    await choose('Extra', 'Unarchive contact')
    await waitFor(() => expect(screen.queryByText('This contact is archived. It no longer shows in lists.')).toBeNull())
    await userEvent.click(screen.getByRole('button', { name: 'Extra' }))
    expect(screen.getByRole('button', { name: 'Archive contact' })).toBeInTheDocument()
  })

  it('explains why a contact in use cannot be deleted', async () => {
    server.use(...loaded(), http.delete('*/api/v2/11/contacts/901', () => HttpResponse.json({ error: { base: ['Documents refer to this contact; archive it instead'] } }, { status: 422 })))
    renderAt('/11/contacts/901')
    await choose('Extra', 'Delete contact')
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(await screen.findByText('Documents refer to this contact; archive it instead')).toBeInTheDocument()
  })

  it('deletes a contact and returns to the list', async () => {
    server.use(...loaded(), http.delete('*/api/v2/11/contacts/901', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/contacts/901')
    await choose('Extra', 'Delete contact')
    const confirm = screen.getByRole('alertdialog', { name: 'Are you sure you want to delete Example Shop B.V.?' })
    expect(confirm).toHaveTextContent('You cannot undo this. All data will be deleted.')
    expect(within(confirm).getByRole('button', { name: 'Delete' })).toHaveFocus()
    await userEvent.click(within(confirm).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/contacts$/))
  })

  it('asks before deleting and keeps the contact on cancel', async () => {
    let deleted = false
    server.use(...loaded(), http.delete('*/api/v2/11/contacts/901', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/contacts/901')
    await choose('Extra', 'Delete contact')
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(deleted).toBe(false)
    expect(screen.getByTestId('path')).toHaveTextContent('/11/contacts/901')
  })
})

describe('the contact form', () => {
  type Sent = { contact?: Record<string, unknown> }
  const creating = (sent: Sent) => http.post('*/api/v2/11/contacts', async ({ request }) => {
    Object.assign(sent, await request.json())
    return HttpResponse.json(contact({ id: '903', uri: '/11/contacts/903' }), { status: 201 })
  })

  it("is laid out in Moneybird's sections", async () => {
    server.use(...loaded())
    renderAt('/11/contacts/new')
    expect(await screen.findByRole('heading', { level: 1, name: 'Add contact' })).toBeInTheDocument()
    const headings = within(screen.getByRole('form', { name: 'Add contact' })).getAllByRole('heading', { level: 2 }).map((h) => h.textContent)
    expect(headings).toEqual(['Contact', 'Address', 'Sending', 'Payment', 'Workflow', 'Custom fields'])
    expect(screen.getByRole('button', { name: 'More actions' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/11/contacts')
    expect(screen.getByRole('radio', { name: 'Company' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'Email' })).toBeChecked()
    // The next client number is filled in, as Moneybird does.
    await waitFor(() => expect(screen.getByLabelText('Client number')).toHaveValue('43'))
    expect(screen.getByLabelText('IBAN')).toHaveAttribute('placeholder', 'NL 00 BANK 0123456789')
    expect(screen.getByRole('checkbox', { name: 'Trusting a contact can be done when approving a payment.' })).toBeDisabled()
    expect(screen.getByLabelText('For invoices')).toHaveDisplayValue('No workflow')
    expect(screen.getByText('Add custom fields to store extra information with contacts.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add custom field' })).toHaveAttribute('href', '/11/custom_fields/new?source=contact')
  })

  it('creates a company with its contact person and opens it', async () => {
    const sent: Sent = {}
    server.use(...loaded(), creating(sent))
    renderAt('/11/contacts/new')
    await userEvent.type(await screen.findByLabelText('Company name'), 'Example Shop B.V.')
    await userEvent.type(screen.getByLabelText('Netherlands Chamber of Commerce KVK number'), '12345678')
    await userEvent.type(screen.getByLabelText('VAT-id'), 'NL000099998B57')
    await userEvent.type(screen.getByLabelText('Phone number'), '+31 20 123 4567')
    expect(screen.getByRole('group', { name: 'Contact person' })).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Firstname'), 'Ann')
    await userEvent.type(screen.getByLabelText('Lastname'), 'de Vries')
    await userEvent.type(screen.getByRole('textbox', { name: 'Address' }), 'Dam 1')
    await userEvent.type(screen.getByLabelText('Zipcode'), '1012 JS')
    await userEvent.type(screen.getByLabelText('City'), 'Amsterdam')
    await userEvent.type(screen.getByLabelText('Attn. for invoices'), 'Accounts payable')
    await userEvent.type(screen.getByLabelText('Email address for invoices'), 'billing@example.com')
    await userEvent.click(screen.getByRole('checkbox', { name: 'Automatically add as attachment' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/contacts/903'))
    expect(sent.contact).toMatchObject({
      company_name: 'Example Shop B.V.', chamber_of_commerce: '12345678', tax_number: 'NL000099998B57', phone: '+31 20 123 4567',
      firstname: 'Ann', lastname: 'de Vries', address1: 'Dam 1', zipcode: '1012 JS', city: 'Amsterdam', country: 'NL',
      delivery_method: 'Email', email_ubl: true, send_invoices_to_attention: 'Accounts payable', send_invoices_to_email: 'billing@example.com',
      send_estimates_to_attention: 'Accounts payable', send_estimates_to_email: 'billing@example.com', direct_debit: false,
      invoice_workflow_id: '', estimate_workflow_id: '',
    })
    expect(sent.contact).not.toHaveProperty('customer_id')
  })

  it('asks a private individual for their own name instead of a company', async () => {
    const sent: Sent = {}
    server.use(...loaded(), creating(sent))
    renderAt('/11/contacts/new')
    await userEvent.click(await screen.findByRole('radio', { name: 'Private' }))
    expect(screen.queryByLabelText('Company name')).toBeNull()
    expect(screen.queryByLabelText('Netherlands Chamber of Commerce KVK number')).toBeNull()
    expect(screen.queryByRole('group', { name: 'Contact person' })).toBeNull()
    await userEvent.type(screen.getByLabelText('First name'), 'Ann')
    await userEvent.type(screen.getByLabelText('Last name'), 'Jansen')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.contact).toMatchObject({ company_name: '', firstname: 'Ann', lastname: 'Jansen' }))
  })

  it('finds the company in the register and fills in its details', async () => {
    server.use(...loaded(undefined, undefined, { companySearch: true }), http.get('*/api/onboarding/company_search', ({ request }) => {
      expect(new URL(request.url).searchParams.get('country')).toBe('NL')
      return HttpResponse.json([{ name: 'Example Shop B.V.', chamber_of_commerce: '12345678', address1: 'Dam 1', zipcode: '1012JS', city: 'Amsterdam' }])
    }))
    renderAt('/11/contacts/new')
    expect(await screen.findByRole('combobox', { name: 'Company register country' })).toHaveDisplayValue('NL')
    await userEvent.type(screen.getByLabelText('Company name'), 'Exam')
    await userEvent.click(await screen.findByRole('option', { name: /Example Shop B\.V\./ }))
    expect(screen.getByLabelText('Company name')).toHaveValue('Example Shop B.V.')
    expect(screen.getByLabelText('Netherlands Chamber of Commerce KVK number')).toHaveValue('12345678')
    expect(screen.getByRole('textbox', { name: 'Address' })).toHaveValue('Dam 1')
    expect(screen.getByLabelText('Zipcode')).toHaveValue('1012JS')
    expect(screen.getByLabelText('City')).toHaveValue('Amsterdam')
  })

  it('reveals a second address line on request', async () => {
    server.use(...loaded())
    renderAt('/11/contacts/new')
    await screen.findByRole('textbox', { name: 'Address' })
    expect(screen.queryByLabelText('Additional address details')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Add address line' }))
    expect(screen.getByLabelText('Additional address details')).toHaveFocus()
  })

  it('takes other data for quotes once the same data is no longer used', async () => {
    const sent: Sent = {}
    server.use(...loaded(), creating(sent))
    renderAt('/11/contacts/new')
    await userEvent.type(await screen.findByLabelText('Company name'), 'Example Shop B.V.')
    await userEvent.type(screen.getByLabelText('Email address for invoices'), 'billing@example.com')
    expect(screen.queryByLabelText('Email address for quotes')).toBeNull()
    await userEvent.click(screen.getByRole('checkbox', { name: 'Use the same data for quotes' }))
    await userEvent.type(screen.getByLabelText('Attn. for quotes'), 'Sales')
    await userEvent.type(screen.getByLabelText('Email address for quotes'), 'quotes@example.com')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.contact).toMatchObject({ send_invoices_to_email: 'billing@example.com', send_estimates_to_attention: 'Sales', send_estimates_to_email: 'quotes@example.com' }))
  })

  it('sends manually or by Peppol, asking for the Peppol address', async () => {
    const sent: Sent = {}
    server.use(...loaded(), creating(sent))
    renderAt('/11/contacts/new')
    await userEvent.type(await screen.findByLabelText('Company name'), 'Example Shop B.V.')
    expect(screen.queryByLabelText('Peppol address')).toBeNull()
    await userEvent.click(screen.getByRole('radio', { name: 'Peppol' }))
    await userEvent.type(screen.getByLabelText('Peppol address type'), '0106')
    await userEvent.type(screen.getByLabelText('Peppol address'), '12345678')
    await userEvent.click(screen.getByRole('radio', { name: 'Send manually' }))
    expect(screen.queryByLabelText('Peppol address')).toBeNull()
    expect(screen.queryByRole('checkbox', { name: 'Automatically add as attachment' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.contact).toMatchObject({ delivery_method: 'Manual' }))
  })

  it('takes the payment details and the manual mandate', async () => {
    const sent: Sent = {}
    server.use(...loaded(), creating(sent))
    renderAt('/11/contacts/new')
    await userEvent.type(await screen.findByLabelText('Company name'), 'Example Shop B.V.')
    await userEvent.type(screen.getByLabelText('IBAN'), 'NL91ABNA0417164300')
    expect(screen.getByText('0/70')).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Account name'), 'Example Shop')
    expect(screen.getByText('12/70')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('checkbox', { name: 'Automatically mark future purchase invoices to be paid through direct debit' }))
    expect(screen.queryByLabelText('Mandate reference')).toBeNull()
    await userEvent.click(screen.getByRole('checkbox', { name: 'The contact signed a SEPA direct debit mandate' }))
    await userEvent.type(screen.getByLabelText('Mandate reference'), 'MANDATE-1')
    await userEvent.type(screen.getByLabelText('Mandate date'), '2026-11-01')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.contact).toMatchObject({
      sepa_iban: 'NL91ABNA0417164300', sepa_iban_account_name: 'Example Shop', direct_debit: true,
      sepa_active: true, sepa_mandate_id: 'MANDATE-1', sepa_mandate_date: '2026-11-01', sepa_sequence_type: 'RCUR',
    }))
  })

  it('asks for the BIC of a bank outside the Netherlands', async () => {
    server.use(...loaded())
    renderAt('/11/contacts/new')
    await screen.findByLabelText('IBAN')
    expect(screen.queryByLabelText('BIC')).toBeNull()
    await userEvent.type(screen.getByLabelText('IBAN'), 'DE89370400440532013000')
    expect(screen.getByLabelText('BIC')).toHaveAttribute('placeholder', 'BANKNL0A')
  })

  it("chooses the contact's workflows and fills its custom fields", async () => {
    const sent: Sent = {}
    server.use(...loaded(), creating(sent))
    server.use(http.get('*/api/v2/11/custom_fields', () => HttpResponse.json([
      { id: '801', administration_id: '11', name: 'Branch', source: 'contact', uri: '/11/custom_fields/801' },
      { id: '802', administration_id: '11', name: 'Slogan', source: 'identity', uri: '/11/custom_fields/802' },
    ])))
    renderAt('/11/contacts/new')
    await userEvent.type(await screen.findByLabelText('Company name'), 'Example Shop B.V.')
    await userEvent.selectOptions(screen.getByLabelText('For invoices'), 'Standard invoices')
    await userEvent.selectOptions(screen.getByLabelText('For quotes'), 'Standard quotes')
    await userEvent.type(await screen.findByLabelText('Branch'), 'Retail')
    expect(screen.queryByLabelText('Slogan')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.contact).toMatchObject({
      invoice_workflow_id: '601', estimate_workflow_id: '602', custom_fields_attributes: [{ id: '801', value: 'Retail' }],
    }))
  })

  it('shows what is wrong', async () => {
    server.use(...loaded(), http.post('*/api/v2/11/contacts', () => HttpResponse.json({ error: { base: ['Company name, first name or last name can\'t be blank'], send_invoices_to_email: ['is not a valid email address'] } }, { status: 422 })))
    renderAt('/11/contacts/new')
    await userEvent.type(await screen.findByLabelText('Email address for invoices'), 'nope')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText("Company name, first name or last name can't be blank")).toBeInTheDocument()
    expect(screen.getByText(/^Email address for invoices is not a valid email address/)).toBeInTheDocument()
  })

  it('edits a contact, keeping what it already holds', async () => {
    let sent: Sent = {}
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact({
        firstname: 'Ann', lastname: 'de Vries', address2: 'Floor 2', send_estimates_to_email: 'quotes@example.com',
        sepa_active: true, sepa_iban: 'NL91ABNA0417164300', sepa_mandate_id: 'MANDATE-1', delivery_method: 'Post',
      }))),
      http.patch('*/api/v2/11/contacts/901', async ({ request }) => {
        sent = (await request.json()) as Sent
        return HttpResponse.json(contact({ city: 'Delft' }))
      }),
    )
    renderAt('/11/contacts/901/edit')
    expect(await screen.findByRole('heading', { level: 1, name: 'Edit contact' })).toBeInTheDocument()
    const city = await screen.findByLabelText('City')
    expect(city).toHaveValue('Amsterdam')
    expect(screen.getByLabelText('Firstname')).toHaveValue('Ann')
    expect(screen.getByLabelText('Additional address details')).toHaveValue('Floor 2')
    expect(screen.getByRole('checkbox', { name: 'Use the same data for quotes' })).not.toBeChecked()
    expect(screen.getByLabelText('Email address for quotes')).toHaveValue('quotes@example.com')
    expect(screen.getByRole('radio', { name: 'Post' })).toBeChecked()
    expect(screen.getByLabelText('Mandate reference')).toHaveValue('MANDATE-1')
    expect(screen.getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/11/contacts/901')
    await userEvent.clear(city)
    await userEvent.type(city, 'Delft')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/contacts\/901$/))
    expect(sent.contact).toMatchObject({
      city: 'Delft', customer_id: '1', company_name: 'Example Shop B.V.', firstname: 'Ann', lastname: 'de Vries', address2: 'Floor 2',
      delivery_method: 'Post', send_estimates_to_email: 'quotes@example.com', sepa_active: true, sepa_mandate_id: 'MANDATE-1',
    })
  })
})
