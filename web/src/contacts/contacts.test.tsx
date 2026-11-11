import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { chooseMore } from '../test/menu'
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
    http.get('*/api/v2/11/contacts/doubles', () => HttpResponse.json(doubles.map((contacts) => ({ contacts })))),
    http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact())),
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
  it('shows its details, people and notes', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact({
      contact_people: [{ id: '911', contact_id: '901', administration_id: '11', firstname: 'Zoë', lastname: null, phone: null, email: 'zoe@example.com', department: 'Finance', created_at: stamp, updated_at: stamp, version: 1 }],
      notes: [{ id: '921', administration_id: '11', entity_id: '901', entity_type: 'Contact', user_id: '1', assignee_id: null, todo: true, note: 'Call back', completed_at: null, completed_by_id: null, todo_type: null, data: null, created_at: stamp, updated_at: stamp }],
    }))))
    renderAt('/11/contacts/901')
    expect(await screen.findByRole('heading', { level: 1, name: 'Example Shop B.V.' })).toBeInTheDocument()
    expect(screen.getByText('Dam 1')).toBeInTheDocument()
    const people = screen.getByRole('region', { name: 'Contact people' })
    expect(within(people).getByText('Zoë')).toBeInTheDocument()
    expect(within(people).getByText('Finance')).toBeInTheDocument()
    const notes = screen.getByRole('region', { name: 'Notes' })
    expect(within(notes).getByText('Call back')).toBeInTheDocument()
    expect(within(notes).getByText('To do')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Edit' })).toHaveAttribute('href', '/11/contacts/901/edit')
  })

  it('adds a contact person and a note', async () => {
    const sent: unknown[] = []
    server.use(
      ...loaded(),
      http.post('*/api/v2/11/contacts/901/contact_people', async ({ request }) => {
        sent.push(await request.json())
        return HttpResponse.json({ id: '912', contact_id: '901', administration_id: '11', firstname: 'Ann', lastname: null, phone: null, email: null, department: null, created_at: stamp, updated_at: stamp, version: 1 }, { status: 201 })
      }),
      http.post('*/api/v2/11/contacts/901/notes', async ({ request }) => {
        sent.push(await request.json())
        return HttpResponse.json({}, { status: 201 })
      }),
    )
    renderAt('/11/contacts/901')
    const people = await screen.findByRole('region', { name: 'Contact people' })
    await userEvent.click(within(people).getByRole('button', { name: 'Add contact person' }))
    await userEvent.type(within(people).getByLabelText('First name'), 'Ann')
    await userEvent.click(within(people).getByRole('button', { name: 'Save contact person' }))
    await waitFor(() => expect(sent[0]).toEqual({ contact_person: { firstname: 'Ann', lastname: '', email: '', phone: '', department: '' } }))

    const notes = screen.getByRole('region', { name: 'Notes' })
    await userEvent.type(within(notes).getByLabelText('New note'), 'Sent the contract')
    await userEvent.click(within(notes).getByRole('checkbox', { name: 'This is a to-do' }))
    await userEvent.click(within(notes).getByRole('button', { name: 'Add note' }))
    await waitFor(() => expect(sent[1]).toEqual({ note: { note: 'Sent the contract', todo: true } }))
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
    await chooseMore('Archive')
    expect(await screen.findByText('This contact is archived. It no longer shows in lists.')).toBeInTheDocument()
    await chooseMore('Bring back')
    await waitFor(() => expect(screen.queryByText('This contact is archived. It no longer shows in lists.')).toBeNull())
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(screen.getByRole('button', { name: 'Archive' })).toBeInTheDocument()
  })

  it('explains why a contact in use cannot be deleted', async () => {
    server.use(...loaded(), http.delete('*/api/v2/11/contacts/901', () => HttpResponse.json({ error: { base: ['Documents refer to this contact; archive it instead'] } }, { status: 422 })))
    renderAt('/11/contacts/901')
    await chooseMore('Delete')
    await userEvent.click(screen.getByRole('button', { name: 'Delete permanently' }))
    expect(await screen.findByText('Documents refer to this contact; archive it instead')).toBeInTheDocument()
  })

  it('deletes a contact and returns to the list', async () => {
    server.use(...loaded(), http.delete('*/api/v2/11/contacts/901', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/contacts/901')
    await chooseMore('Delete')
    const confirm = screen.getByRole('alertdialog', { name: 'Delete this contact? This cannot be undone.' })
    expect(within(confirm).getByRole('button', { name: 'Delete permanently' })).toHaveFocus()
    await userEvent.click(within(confirm).getByRole('button', { name: 'Delete permanently' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/contacts$/))
  })

  it('asks before deleting and keeps the contact on cancel', async () => {
    let deleted = false
    server.use(...loaded(), http.delete('*/api/v2/11/contacts/901', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/contacts/901')
    await chooseMore('Delete')
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
    expect(screen.getByLabelText('Client number')).toHaveAttribute('placeholder', 'Next number')
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
