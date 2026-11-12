import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import type { Contact } from '../contacts/queries'
import type { SalesInvoice } from '../invoices/queries'
import { renderAt } from '../test/render'
import { administration, documentStyle, identity, invoiceSettings, listing, server, signedIn, workflow } from '../test/server'
import type { RecurringInvoice } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const vat21: TaxRate = {
  id: '2101', administration_id: '11', name: '21% btw', percentage: '21', tax_rate_type: 'sales_invoice', show_tax: true, active: true,
  country: null, created_at: stamp, updated_at: stamp, uri: '/11/tax_rates/2101', tax_report_boxes: ['NL/1a'], position: 1,
}

const sales: LedgerAccount = {
  id: '8001', administration_id: '11', name: 'Sales', account_type: 'revenue', account_id: '8000', parent_id: null, active: true,
  allowed_document_types: ['sales_invoice'], financial_account_id: null, created_at: stamp, updated_at: stamp,
  taxonomy_item: null, uri: '/11/ledger_accounts/8001', description: null, system_role: null, template: null,
}

const shop = {
  id: '701', administration_id: '11', company_name: 'Example Shop B.V.', firstname: null, lastname: null, customer_id: '1',
  send_invoices_to_email: 'billing@example.com', email_ubl: false, delivery_method: 'Email' as const, country: 'NL',
}

const recurring = (over: Partial<RecurringInvoice> = {}): RecurringInvoice => ({
  id: '941', administration_id: '11', contact_id: '701', contact: shop, contact_person_id: null, workflow_id: '601', document_style_id: '701',
  identity_id: '461', start_date: '2026-11-01', invoice_date: '2026-12-01', last_date: null, active: true, payment_conditions: null,
  reference: 'Contract 7', language: 'en', currency: 'EUR', discount: '0.0', first_due_interval: 14, auto_send: false,
  sending_scheduled_at: null, sending_scheduled_user_id: null, mergeable: false, frequency_type: 'month', frequency: 1, invoices_created: 1,
  created_at: stamp, updated_at: stamp, version: 1, prices_are_incl_tax: false,
  total_price_excl_tax: '100.0', total_price_excl_tax_base: '100.0', total_price_incl_tax: '121.0', total_price_incl_tax_base: '121.0', total_tax: '21.0',
  details: [{ id: '961', description: 'Hosting {period}', amount: '1', price: '100.0', tax_rate_id: '2101', ledger_account_id: '8001', total_price_excl_tax_with_discount: '100.0' }],
  custom_fields: [], notes: [], attachments: [], events: [], subscription: null, uri: '/11/recurring_sales_invoices/941',
  ...over,
})

const yearly = recurring({
  id: '942', reference: 'Licence', frequency_type: 'year', frequency: 2, auto_send: true, invoice_date: '2027-01-15', invoices_created: 0,
  total_price_incl_tax: '60.5', uri: '/11/recurring_sales_invoices/942',
})

const made: SalesInvoice = {
  id: '901', administration_id: '11', contact_id: '701', contact: shop, invoice_id: null, draft_id: 4, state: 'draft', invoice_date: '2026-11-01',
  due_date: '2026-11-15', reference: 'Contract 7', currency: 'EUR', prices_are_incl_tax: false, discount: null, workflow_id: '601',
  document_style_id: '701', identity_id: '461', payment_conditions: '', first_due_interval: 14, paused: false, original_sales_invoice_id: null,
  recurring_sales_invoice_id: '941',
  details: [{ id: '951', description: 'Hosting 2026-11-01 – 2026-11-30', amount: '1', price: '100.0', tax_rate_id: '2101', ledger_account_id: '8001', total_price_excl_tax_with_discount: '100.0' }],
  tax_totals: [], payments: [], notes: [], attachments: [], events: [],
  total_price_excl_tax: '100.0', total_tax: '21.0', total_price_incl_tax: '121.0', total_unpaid: '121.0', total_discount: '0.0',
  version: 1, uri: '/11/sales_invoices/901',
}

function loaded(list: RecurringInvoice[] = [recurring(), yearly], seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/recurring_sales_invoices', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/recurring_sales_invoices/941', () => HttpResponse.json(recurring())),
    http.get('*/api/v2/11/sales_invoices', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json([made], { headers: { 'X-Total-Count': '1' } })
    }),
    http.get('*/api/v2/11/sales_invoices/901', () => HttpResponse.json(made)),
    http.get('*/api/v2/11/tax_rates', () => HttpResponse.json([vat21])),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales])),
    http.get('*/api/v2/11/workflows', () => HttpResponse.json([workflow()])),
    http.get('*/api/v2/11/document_styles', () => HttpResponse.json([documentStyle({ id: '701' })])),
    http.get('*/api/v2/11/contacts', () => HttpResponse.json([shop as unknown as Contact], { headers: { 'X-Total-Count': '1' } })),
  ]
}

describe('the recurring invoice list', () => {
  it('lists recurring invoices with their contact, frequency, next date, auto send and total', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/recurring_sales_invoices')
    const table = await screen.findByRole('table', { name: 'Recurring invoices' })
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('Example Shop B.V.')
    expect(rows[1]).toHaveTextContent('Every month')
    expect(rows[1]).toHaveTextContent('2026-12-01')
    expect(rows[1]).toHaveTextContent('€121.00')
    expect(rows[1]).toHaveTextContent('Contract 7')
    expect(rows[1]).toHaveTextContent('Active')
    expect(within(rows[1] as HTMLElement).queryByRole('img', { name: 'Sent automatically' })).toBeNull()
    expect(rows[2]).toHaveTextContent('Every 2 years')
    expect(within(rows[2] as HTMLElement).getByRole('img', { name: 'Sent automatically' })).toBeInTheDocument()
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/recurring_sales_invoices/941')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:active')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Recurring invoices' })).toHaveAttribute('aria-current', 'page')

    const toggle = screen.getByRole('button', { name: 'Active recurring invoices' })
    await userEvent.click(toggle)
    const panel = screen.getByRole('search', { name: 'Recurring invoice filters' })
    expect(within(panel).getByRole('link', { name: 'Show all active recurring invoices' })).toHaveAttribute('href', '/11/recurring_sales_invoices')
    await userEvent.click(within(panel).getByRole('radio', { name: 'Inactive' }))
    await userEvent.click(within(panel).getByRole('radio', { name: 'Annually' }))
    await userEvent.click(within(within(panel).getByRole('group', { name: 'Send automatically' })).getByRole('radio', { name: 'Yes' }))
    await userEvent.selectOptions(await within(panel).findByRole('combobox', { name: 'Workflow' }), await within(panel).findByRole('option', { name: 'Standaard (NL)' }))
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:inactive,frequency:year,auto_send:true,workflow_id:601'))
    expect(toggle).toHaveTextContent('Filtered recurring invoices')

    await userEvent.click(toggle)
    await userEvent.click(within(screen.getByRole('search', { name: 'Recurring invoice filters' })).getByRole('link', { name: 'Show all active recurring invoices' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:active'))
    expect(toggle).toHaveTextContent('Active recurring invoices')
  })

  it('says when there are none yet', async () => {
    server.use(...loaded([]))
    renderAt('/11/recurring_sales_invoices')
    expect(await screen.findByText(/No recurring invoices yet/)).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Send periodic invoices' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add recurring invoice' })).toHaveAttribute('href', '/11/recurring_sales_invoices/new')
    expect(screen.getByRole('button', { name: 'Export' })).toHaveAttribute('aria-disabled', 'true')
  })
})

/** What the document and its popup ask for besides the recurring invoices. */
function documentHandlers(current: () => RecurringInvoice = () => recurring()) {
  // The first handler that matches answers, so these come before the list's.
  return [
    http.get('*/api/v2/11/recurring_sales_invoices/941', () => HttpResponse.json(current())),
    http.get('*/api/v2/11/identities', () => HttpResponse.json([identity({ id: '500' })])),
    http.get('*/api/v2/11/invoice_settings', () => HttpResponse.json(invoiceSettings())),
    http.get('*/api/v2/11/custom_fields', () => HttpResponse.json([])),
    http.get('*/api/v2/11/users', () => HttpResponse.json([])),
    http.get('*/api/v2/11/products', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
    http.get('*/api/v2/11/sales_invoices/next_draft_id', () => HttpResponse.json({ draft_id: 12 })),
    http.get('*/api/v2/11/document_styles', () => HttpResponse.json([documentStyle({ id: '701' }), documentStyle({ id: '702', name: 'Modern', default: false })])),
    ...loaded(),
  ]
}

const scheduleCard = () => screen.getByRole('region', { name: 'Schedule' })

describe('adding a recurring invoice', () => {
  it('sets the schedule first, as Moneybird asks, then the contact in the popup, whose Save makes it', async () => {
    let sent: Record<string, unknown> | undefined
    server.use(...documentHandlers(), http.post('*/api/v2/11/recurring_sales_invoices', async ({ request }) => {
      sent = ((await request.json()) as { recurring_sales_invoice: Record<string, unknown> }).recurring_sales_invoice
      return HttpResponse.json(recurring(), { status: 201 })
    }))
    renderAt('/11/recurring_sales_invoices/new')
    expect(await screen.findByRole('heading', { level: 1, name: 'Add recurring invoice' })).toBeInTheDocument()
    const form = within(scheduleCard()).getByRole('form', { name: 'Recurring invoice schedule' })
    expect(within(form).getByLabelText('Start date')).toHaveValue('2026-11-11')
    const frequency = within(form).getByRole('radiogroup', { name: 'Frequency' })
    expect(within(frequency).getAllByRole('radio').map((r) => r.closest('label')?.querySelector('span')?.textContent)).toEqual(['Every month', 'Every quarter', 'Every six months', 'Every year', 'Every'])
    expect(within(frequency).getByRole('radio', { name: 'Every month' })).toBeChecked()
    // The document is shown as it will look, not yet to be changed.
    expect(await screen.findByRole('document', { name: 'Invoice preview' })).toBeInTheDocument()

    // The layout is chosen above the schedule, as Moneybird's Change layout shows.
    await userEvent.click(screen.getByRole('button', { name: 'Change layout' }))
    await userEvent.click(within(screen.getByRole('region', { name: 'Select layout' })).getByRole('button', { name: 'Modern' }))

    await userEvent.click(within(frequency).getByRole('radio', { name: 'Every six months' }))
    await userEvent.click(within(form).getByRole('checkbox', { name: 'Add another' }))
    await userEvent.type(within(form).getByRole('textbox', { name: 'Number of invoices' }), '4')
    await userEvent.click(within(form).getByRole('checkbox', { name: 'Automatically send invoice on scheduled dates.' }))
    expect(within(form).getByRole('checkbox', { name: /^Send one invoice to this contact/ })).not.toBeChecked()
    await userEvent.click(within(form).getByRole('button', { name: 'Continue' }))

    // The schedule is told in a sentence and the popup asks for the contact.
    expect(scheduleCard()).toHaveTextContent('This recurring invoice will be created every 6 months, 4 more times.')
    const dialog = await screen.findByRole('dialog', { name: 'Add recurring invoice' })
    expect(within(dialog).queryByText('Invoice id')).toBeNull()
    await userEvent.click(within(dialog).getByRole('combobox', { name: 'Contact' }))
    await userEvent.type(within(dialog).getByRole('searchbox', { name: 'Query' }), 'shop')
    await userEvent.click(await within(dialog).findByRole('option', { name: 'Example Shop B.V.' }))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toMatchObject({
      contact_id: '701', invoice_date: '2026-11-11', frequency: 6, frequency_type: 'month', has_desired_count: true, desired_count: 4,
      auto_send: true, mergeable: false, document_style_id: '702',
    }))
    expect(sent).not.toHaveProperty('invoice_sequence_id')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/recurring_sales_invoices/941'))
  })

  it('goes back to the schedule when the popup is cancelled', async () => {
    server.use(...documentHandlers())
    renderAt('/11/recurring_sales_invoices/new')
    await userEvent.click(await screen.findByRole('button', { name: 'Continue' }))
    const dialog = await screen.findByRole('dialog', { name: 'Add recurring invoice' })
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    expect(within(scheduleCard()).getByRole('button', { name: 'Continue' })).toBeInTheDocument()
  })

  it('starts from a sales invoice, as its "Duplicate to recurring invoice" does', async () => {
    let sent: Record<string, unknown> | undefined
    server.use(...documentHandlers(), http.post('*/api/v2/11/recurring_sales_invoices', async ({ request }) => {
      sent = ((await request.json()) as { recurring_sales_invoice: Record<string, unknown> }).recurring_sales_invoice
      return HttpResponse.json(recurring(), { status: 201 })
    }))
    renderAt('/11/sales_invoices/901')
    await userEvent.click(await screen.findByRole('button', { name: 'Extra' }))
    await userEvent.click(screen.getByRole('link', { name: 'Duplicate to recurring invoice' }))
    await userEvent.click(await screen.findByRole('button', { name: 'Continue' }))
    const dialog = await screen.findByRole('dialog', { name: 'Add recurring invoice' })
    expect(within(dialog).getByLabelText('Reference')).toHaveValue('Contract 7')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toMatchObject({
      contact_id: '701', reference: 'Contract 7', frequency: 1, frequency_type: 'month', has_desired_count: false,
      details_attributes: [{ description: 'Hosting 2026-11-01 – 2026-11-30', amount: '1', price: '100', tax_rate_id: '2101', ledger_account_id: '8001', row_order: 1 }],
    }))
    expect((sent?.details_attributes as Record<string, unknown>[])[0]).not.toHaveProperty('id')
  })
})

describe('a recurring invoice', () => {
  it('is named by its schedule and contact, tells its schedule over the document, and shows its contact, workflow and history', async () => {
    server.use(...documentHandlers())
    renderAt('/11/recurring_sales_invoices/941')
    expect(await screen.findByRole('heading', { level: 1, name: 'Every month to Example Shop B.V.' })).toBeInTheDocument()
    expect(scheduleCard()).toHaveTextContent('This recurring invoice will be created every month indefinitely.')
    const timeline = within(scheduleCard()).getByRole('list', { name: 'Planned invoices' })
    // The invoice it made leads, linked; then its next dates, numbered after it.
    const made = await within(timeline).findByRole('link', { name: 'Draft #4' })
    expect(made).toHaveAttribute('href', '/11/sales_invoices/901')
    expect(timeline).toHaveTextContent('2026-12-01')
    expect(timeline).toHaveTextContent('2027-01-01')
    expect(timeline).toHaveTextContent('2027-02-01')
    expect(within(timeline).getByLabelText('And more')).toBeInTheDocument()
    // The document is the invoice it makes next, in the workflow's language.
    const paper = await screen.findByRole('document', { name: 'Recurring invoice' })
    expect(paper).toHaveTextContent('Periodieke factuur #2')
    expect(paper).toHaveTextContent('Hosting {period}')
    const side = screen.getByRole('complementary', { name: 'Recurring invoice information' })
    expect(within(side).getByRole('link', { name: 'Standaard (NL)' })).toHaveAttribute('href', '/11/workflows/601')
    expect(within(side).getByText('Example Shop B.V.')).toBeInTheDocument()
    expect(within(side).getByText('Recurring invoice created')).toBeInTheDocument()
  })

  it('changes its schedule behind Edit', async () => {
    let sent: Record<string, unknown> | undefined
    server.use(...documentHandlers(), http.patch('*/api/v2/11/recurring_sales_invoices/941', async ({ request }) => {
      sent = ((await request.json()) as { recurring_sales_invoice: Record<string, unknown> }).recurring_sales_invoice
      return HttpResponse.json(recurring({ frequency_type: 'quarter', last_date: '2027-06-01', updated_at: '2026-11-11T09:00:00Z', version: 2 }))
    }))
    renderAt('/11/recurring_sales_invoices/941')
    await userEvent.click(await within(await screen.findByRole('region', { name: 'Schedule' })).findByRole('button', { name: 'Edit' }))
    const form = within(scheduleCard()).getByRole('form', { name: 'Recurring invoice schedule' })
    expect(within(form).getByLabelText('Start date')).toHaveValue('2026-12-01')
    await userEvent.click(within(form).getByRole('radio', { name: /^Every$/ }))
    await userEvent.clear(within(form).getByRole('textbox', { name: 'Every how many' }))
    await userEvent.type(within(form).getByRole('textbox', { name: 'Every how many' }), '1')
    await userEvent.selectOptions(within(form).getByRole('combobox', { name: 'Frequency' }), 'quarters')
    await userEvent.click(within(form).getByRole('checkbox', { name: 'Add another' }))
    await userEvent.type(within(form).getByRole('textbox', { name: 'Number of invoices' }), '3')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    // "Add another" counts from now on; the server counts the invoice made already too.
    await waitFor(() => expect(sent).toEqual({ frequency: 1, frequency_type: 'quarter', has_desired_count: true, desired_count: 4, auto_send: false, mergeable: false }))
    expect(await within(scheduleCard()).findByText(/every quarter, 3 more times\./)).toBeInTheDocument()
  })

  it('saves its details from the document, as a draft invoice does', async () => {
    let sent: Record<string, unknown> | undefined
    server.use(...documentHandlers(), http.patch('*/api/v2/11/recurring_sales_invoices/941', async ({ request }) => {
      sent = ((await request.json()) as { recurring_sales_invoice: Record<string, unknown> }).recurring_sales_invoice
      return HttpResponse.json(recurring({ version: 2 }))
    }))
    renderAt('/11/recurring_sales_invoices/941')
    const paper = await screen.findByRole('document', { name: 'Recurring invoice' })
    await userEvent.click(within(paper).getByRole('button', { name: /invoice details/ }))
    const editor = screen.getByRole('table', { name: 'Invoice details' })
    const description = within(editor).getAllByRole('combobox', { name: 'Description' })[0] as HTMLElement
    await userEvent.clear(description)
    await userEvent.type(description, 'Hosting')
    await userEvent.click(within(screen.getByRole('region', { name: 'Invoice details' })).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toMatchObject({ contact_id: '701', details_attributes: [{ id: '961', description: 'Hosting' }] }))
    expect(sent).not.toHaveProperty('invoice_date')
  })

  it('changes its layout from Extra', async () => {
    let sent: Record<string, unknown> | undefined
    server.use(...documentHandlers(), http.patch('*/api/v2/11/recurring_sales_invoices/941', async ({ request }) => {
      sent = ((await request.json()) as { recurring_sales_invoice: Record<string, unknown> }).recurring_sales_invoice
      return HttpResponse.json(recurring({ document_style_id: '702', version: 2 }))
    }))
    renderAt('/11/recurring_sales_invoices/941')
    await userEvent.click(await screen.findByRole('button', { name: 'Extra' }))
    expect(screen.getByRole('link', { name: 'Duplicate to recurring invoice' })).toHaveAttribute('href', '/11/recurring_sales_invoices/new?recurring=941')
    await userEvent.click(screen.getByRole('button', { name: 'Change layout' }))
    const layouts = await screen.findByRole('region', { name: 'Change layout' })
    expect(layouts).toHaveTextContent('The layout determines the style and sender address on the recurring invoice.')
    await userEvent.click(within(layouts).getByRole('radio', { name: 'Modern' }))
    await userEvent.click(within(layouts).getByRole('button', { name: 'Change layout' }))
    await waitFor(() => expect(sent).toMatchObject({ document_style_id: '702' }))
  })

  it('stops, after which it makes no more invoices', async () => {
    let current = recurring()
    server.use(...documentHandlers(() => current), http.patch('*/api/v2/11/recurring_sales_invoices/941/deactivate', () => {
      current = recurring({ active: false, updated_at: '2026-11-11T09:00:00Z', version: 2 })
      return HttpResponse.json(current)
    }))
    renderAt('/11/recurring_sales_invoices/941')
    await userEvent.click(await screen.findByRole('button', { name: 'Stop' }))
    const ask = screen.getByRole('alertdialog', { name: 'Are you sure you want to stop this recurring invoice?' })
    await userEvent.click(within(ask).getByRole('button', { name: 'Stop' }))
    expect(await screen.findAllByText('This recurring invoice has been stopped and creates no more invoices.')).not.toHaveLength(0)
    expect(screen.queryByRole('button', { name: 'Stop' })).toBeNull()
    expect(within(scheduleCard()).queryByRole('button', { name: 'Edit' })).toBeNull()
  })

  it('is deleted after Moneybird’s question', async () => {
    let deleted = false
    server.use(...documentHandlers(() => recurring({ invoices_created: 0 })), http.delete('*/api/v2/11/recurring_sales_invoices/941', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/recurring_sales_invoices/941')
    await userEvent.click(await screen.findByRole('button', { name: 'Delete recurring invoice' }))
    const ask = screen.getByRole('dialog', { name: 'Are you sure you want to delete this recurring invoice? This cannot be undone.' })
    await userEvent.click(within(ask).getByRole('button', { name: 'Cancel' }))
    expect(deleted).toBe(false)
    await userEvent.click(screen.getByRole('button', { name: 'Delete recurring invoice' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/recurring_sales_invoices$/))
    expect(deleted).toBe(true)
  })
})
