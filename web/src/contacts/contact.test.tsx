import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { ann, contact, loaded, stamp } from './testkit'

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
    expect(rows.map((l) => l.getAttribute('href'))).toEqual(['/11/sales_invoices/501', '/11/documents/601', '/11/documents/602'])
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
