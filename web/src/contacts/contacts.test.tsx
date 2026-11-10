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

function loaded(list: Contact[] = [contact(), ann], total = list.length, seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/workflows', () => HttpResponse.json([workflow({ id: '601' })])),
    http.get('*/api/v2/11/custom_fields', () => HttpResponse.json([])),
    http.get('*/api/onboarding/options', () => HttpResponse.json({ countries: ['BE', 'DE', 'NL'], currencies: ['EUR'], popular_currencies: ['EUR'], company_search_countries: [], address_lookup_countries: [] })),
    http.get('*/api/v2/11/contacts', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(total) } })
    }),
    http.get('*/api/v2/11/contacts/filter', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list.slice(0, 1), { headers: { 'X-Total-Count': '1' } })
    }),
    http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact())),
  ]
}

describe('contacts list', () => {
  it('lists contacts with their number, email and city', async () => {
    server.use(...loaded())
    renderAt('/11/contacts')
    const table = await screen.findByRole('table', { name: 'Contacts' })
    expect(within(table).getByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/contacts/901')
    expect(within(table).getByRole('link', { name: 'Ann Jansen' })).toHaveAttribute('href', '/11/contacts/902')
    expect(within(table).getByText('billing@example.com')).toBeInTheDocument()
    expect(within(table).getByText('Utrecht')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'New contact' })).toHaveAttribute('href', '/11/contacts/new')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Contacts' })).toHaveAttribute('aria-current', 'page')
  })

  it('searches as the user types', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, undefined, seen))
    renderAt('/11/contacts')
    await userEvent.type(await screen.findByRole('searchbox', { name: 'Search contacts' }), 'shop')
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('query')).toBe('shop'))
    expect(screen.getByTestId('path')).toHaveTextContent('/11/contacts')
  })

  it('filters on the kind of contact and shows archived ones on request', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, undefined, seen))
    renderAt('/11/contacts')
    await screen.findByRole('table', { name: 'Contacts' })
    await userEvent.selectOptions(screen.getByLabelText('Kind'), 'company')
    await waitFor(() => expect(seen.at(-1)?.pathname).toBe('/api/v2/11/contacts/filter'))
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('contact_type:company')
    await userEvent.click(screen.getByRole('checkbox', { name: 'Show archived contacts' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('include_archived')).toBe('true'))
  })

  it('pages through many contacts', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, 120, seen))
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
    await userEvent.click(await screen.findByRole('button', { name: 'Archive' }))
    expect(await screen.findByText('This contact is archived. It no longer shows in lists.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Bring back' }))
    expect(await screen.findByRole('button', { name: 'Archive' })).toBeInTheDocument()
  })

  it('explains why a contact in use cannot be deleted', async () => {
    server.use(...loaded(), http.delete('*/api/v2/11/contacts/901', () => HttpResponse.json({ error: { base: ['Documents refer to this contact; archive it instead'] } }, { status: 422 })))
    renderAt('/11/contacts/901')
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete permanently' }))
    expect(await screen.findByText('Documents refer to this contact; archive it instead')).toBeInTheDocument()
  })

  it('deletes a contact and returns to the list', async () => {
    server.use(...loaded(), http.delete('*/api/v2/11/contacts/901', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/contacts/901')
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete permanently' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/contacts$/))
  })
})

describe('the contact form', () => {
  it('creates a contact and opens it', async () => {
    let sent: { contact?: Record<string, unknown> } = {}
    server.use(...loaded(), http.post('*/api/v2/11/contacts', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(contact({ id: '903', uri: '/11/contacts/903' }), { status: 201 })
    }))
    renderAt('/11/contacts/new')
    await userEvent.type(await screen.findByLabelText('Company name'), 'Example Shop B.V.')
    await userEvent.type(screen.getByLabelText('Email for invoices'), 'billing@example.com')
    await userEvent.selectOptions(screen.getByLabelText('Send invoices by'), 'Post')
    await userEvent.click(screen.getByRole('button', { name: 'Save contact' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/contacts/903'))
    expect(sent.contact).toMatchObject({ company_name: 'Example Shop B.V.', send_invoices_to_email: 'billing@example.com', delivery_method: 'Post', country: 'NL' })
    expect(sent.contact).not.toHaveProperty('customer_id')
  })

  it('asks for the mandate details once direct debit is on', async () => {
    server.use(...loaded())
    renderAt('/11/contacts/new')
    await screen.findByLabelText('Company name')
    expect(screen.queryByLabelText('IBAN')).toBeNull()
    await userEvent.click(screen.getByRole('checkbox', { name: 'Collect by direct debit (SEPA mandate)' }))
    expect(screen.getByLabelText('IBAN')).toBeInTheDocument()
    expect(screen.getByLabelText('Mandate reference')).toBeInTheDocument()
    expect(screen.getByLabelText('Mandate date')).toBeInTheDocument()
  })

  it('asks for the e-invoicing identifier for Peppol', async () => {
    server.use(...loaded())
    renderAt('/11/contacts/new')
    await screen.findByLabelText('Company name')
    expect(screen.queryByLabelText('E-invoicing identifier')).toBeNull()
    await userEvent.selectOptions(screen.getByLabelText('Send invoices by'), 'Peppol')
    expect(screen.getByLabelText('E-invoicing identifier')).toBeInTheDocument()
  })

  it('shows what is wrong', async () => {
    server.use(...loaded(), http.post('*/api/v2/11/contacts', () => HttpResponse.json({ error: { base: ['Company name, first name or last name can\'t be blank'], send_invoices_to_email: ['is not a valid email address'] } }, { status: 422 })))
    renderAt('/11/contacts/new')
    await userEvent.type(await screen.findByLabelText('Email for invoices'), 'nope')
    await userEvent.click(screen.getByRole('button', { name: 'Save contact' }))
    expect(await screen.findByText("Company name, first name or last name can't be blank")).toBeInTheDocument()
    expect(screen.getByText(/^Email for invoices is not a valid email address/)).toBeInTheDocument()
  })

  it('edits a contact', async () => {
    let sent: { contact?: Record<string, unknown> } = {}
    server.use(...loaded(), http.patch('*/api/v2/11/contacts/901', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(contact({ city: 'Delft' }))
    }))
    renderAt('/11/contacts/901/edit')
    const city = await screen.findByLabelText('City')
    expect(city).toHaveValue('Amsterdam')
    await userEvent.clear(city)
    await userEvent.type(city, 'Delft')
    await userEvent.click(screen.getByRole('button', { name: 'Save contact' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/contacts\/901$/))
    expect(sent.contact).toMatchObject({ city: 'Delft', customer_id: '1' })
  })
})
