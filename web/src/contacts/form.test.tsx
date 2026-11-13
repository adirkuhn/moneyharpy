import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { contact, loaded } from './testkit'

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

  it('says when the register cannot be searched and keeps the name typed', async () => {
    server.use(...loaded(undefined, undefined, { companySearch: true }), http.get('*/api/onboarding/company_search', () =>
      HttpResponse.json({ error: 'KVK blocked the search request', code: 'kvk_blocked' }, { status: 503 })))
    renderAt('/11/contacts/new')
    await screen.findByRole('combobox', { name: 'Company register country' })
    await userEvent.type(screen.getByLabelText('Company name'), 'Example Shop')
    expect(await screen.findByRole('status')).toHaveTextContent('KVK blocked the search request. Fill in the details by hand.')
    expect(screen.getByLabelText('Company name')).toHaveValue('Example Shop')
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
