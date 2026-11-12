import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { documentStyle, invoiceSettings, server, workflow } from '../test/server'
import { cells, creating, loaded, paper, pickContact, popup, senderStyle, shop } from './testkit'

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
    // Add custom field is among the settings behind Edit, as on Moneybird.
    expect(within(dialog).queryByRole('link', { name: 'Add custom field' })).toBeNull()
    await userEvent.click(within(dialog).getByRole('button', { name: 'Edit' }))
    expect(within(dialog).getByRole('link', { name: 'Add custom field' })).toHaveAttribute(
      'href', `/11/custom_fields/new?source=sales_invoice&return_to=${encodeURIComponent('/11/sales_invoices/new')}`,
    )
    await userEvent.click(within(dialog).getByRole('button', { name: 'Close', expanded: true }))

    // Closing it, like Cancel, goes back to the invoices: nothing was made yet.
    await userEvent.click(within(dialog).getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices$/))
    expect(screen.queryByRole('dialog', { name: 'Add invoice' })).toBeNull()
  })

  it('sets the invoice id, date, workflow, term and layout behind Edit', async () => {
    const sent: { post?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(
      ...creating(sent),
      http.get('*/api/v2/11/invoice_settings', () => HttpResponse.json(invoiceSettings({ invoice_sequence_id: '0007', invoice_id_example: '2026-0007' }))),
      http.get('*/api/v2/11/document_styles', () => HttpResponse.json([senderStyle, documentStyle({ id: '702', name: 'Modern', default: false, uri: '/11/document_styles/702' })])),
      http.get('*/api/v2/11/workflows', () => HttpResponse.json([workflow(), workflow({ id: '603', name: 'English', default: false, language: 'en', currency: 'GBP', prices_are_incl_tax: true, uri: '/11/workflows/603' })])),
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

    // Whether prices include VAT is set above the details, as on Moneybird.
    expect(within(dialog).queryByRole('checkbox', { name: 'Prices include VAT' })).toBeNull()

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
      // The currency and prices follow the workflow chosen.
      document_style_id: '702', prices_are_incl_tax: true, currency: 'GBP', details_attributes: [],
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
