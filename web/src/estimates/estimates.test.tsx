import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import type { Contact } from '../contacts/queries'
import { choose, rates, sales, senderStyle } from '../invoices/testkit'
import type { SalesInvoice } from '../invoices/queries'
import { renderAt } from '../test/render'
import { administration, identity, invoiceSettings, listing, server, signedIn, workflow } from '../test/server'
import { duplicateOf, type Estimate } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const shop = {
  id: '701', administration_id: '11', company_name: 'Example Shop B.V.', firstname: 'Jan', lastname: 'Voorbeeld', customer_id: '1',
  address1: 'Voorbeeldweg 2', address2: null, zipcode: '2804NA', city: 'Gouda', send_invoices_to_email: 'billing@example.com',
  send_estimates_to_email: 'quotes@example.com', email_ubl: false, delivery_method: 'Email' as const, country: 'NL', estimate_workflow_id: null,
}

const event = (action: string, at: string, data: Record<string, unknown> = {}, link?: [string, string]) => ({
  administration_id: '11', user_id: '1', action, link_entity_id: link?.[1] ?? null, link_entity_type: link?.[0] ?? null, data, created_at: at, updated_at: at,
})

const estimate = (over: Partial<Estimate> = {}): Estimate => ({
  id: '971', administration_id: '11', contact_id: '701', contact: shop, contact_person_id: null, workflow_id: '602', document_style_id: '701',
  identity_id: '500', estimate_id: null, estimate_sequence_id: null, draft_id: 3, state: 'draft', estimate_date: null, due_date: '2026-11-25',
  reference: 'Project X', language: 'nl', currency: 'EUR', exchange_rate: '1.0', discount: '0.0', original_estimate_id: null, show_tax: true,
  sign_online: true, sent_at: null, accepted_at: null, rejected_at: null, archived_at: null, created_at: stamp, updated_at: stamp,
  public_view_code: null, public_view_code_expires_at: null, version: 1, pre_text: 'Beste {recipient.name},', post_text: 'Tot ziens', prices_are_incl_tax: false,
  total_price_excl_tax: '300.0', total_price_excl_tax_base: '300.0', total_price_incl_tax: '363.0', total_price_incl_tax_base: '363.0',
  total_discount: '0.0', total_tax: '63.0', url: 'http://localhost/11/estimates/971', first_due_interval: 14,
  details: [{ id: '981', description: 'Project X', amount: '1', price: '300.0', tax_rate_id: '2101', ledger_account_id: '8001', row_order: 1, total_price_excl_tax_with_discount: '300.0' }],
  custom_fields: [], notes: [], attachments: [], events: [event('estimate_created', stamp)], tax_totals: [], uri: '/11/estimates/971',
  ...over,
})

const accepted = estimate({ id: '972', estimate_id: '2026-0001', draft_id: null, state: 'accepted', estimate_date: '2026-11-01', reference: 'Project Y', uri: '/11/estimates/972' })

function loaded(list: Estimate[] = [estimate(), accepted], seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/estimates', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/estimates/next_draft_id', () => HttpResponse.json({ draft_id: 5 })),
    http.get('*/api/v2/11/estimates/971', () => HttpResponse.json(estimate())),
    http.get('*/api/v2/11/estimates/972', () => HttpResponse.json(accepted)),
    http.get('*/api/v2/11/tax_rates', () => HttpResponse.json(rates)),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales])),
    http.get('*/api/v2/11/details/usage', () => HttpResponse.json({ tax_rates: [], ledger_accounts: [] })),
    http.get('*/api/v2/11/workflows', () => HttpResponse.json([workflow(), workflow({ id: '602', type: 'EstimateWorkflow', name: 'Offerte (NL)' })])),
    http.get('*/api/v2/11/document_styles', () => HttpResponse.json([{ ...senderStyle, estimate_sender_address: senderStyle.invoice_sender_address }])),
    http.get('*/api/v2/11/identities', () => HttpResponse.json([identity({ id: '500' })])),
    http.get('*/api/v2/11/invoice_settings', () => HttpResponse.json(invoiceSettings({ estimate_sequence_id: '0002', estimate_id_example: '2026-0002' }))),
    http.get('*/api/v2/11/custom_fields', () => HttpResponse.json([])),
    http.get('*/api/v2/11/users', () => HttpResponse.json([])),
    http.get('*/api/v2/11/products', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
    http.get('*/api/v2/11/contacts', () => HttpResponse.json([shop as unknown as Contact], { headers: { 'X-Total-Count': '1' } })),
    http.get('*/api/v2/11/contacts/701', () => HttpResponse.json(shop)),
    http.get('*/api/v2/11/documents', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
  ]
}

const quotePaper = (name: RegExp) => screen.findByRole('document', { name })

describe('the quote list', () => {
  it("shows this year's quotes in Moneybird's columns, with the contact's card", async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/estimates')
    const table = await screen.findByRole('table', { name: 'Quotes' })
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Quote id', 'Reference', '', 'Quote date', 'Contact', 'Amount', 'Status'])
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('Draft #3')
    expect(within(rows[1] as HTMLElement).getByText('Draft #3').tagName).toBe('EM')
    expect(rows[1]).toHaveTextContent('Project X')
    expect(rows[1]).toHaveTextContent('€363.00')
    expect(within(rows[1] as HTMLElement).getByText('Draft', { selector: '.badge' })).toBeInTheDocument()
    expect(rows[2]).toHaveTextContent('2026-0001')
    expect(rows[2]).toHaveTextContent('2026-11-01')
    expect(within(rows[2] as HTMLElement).getByText('Accepted', { selector: '.badge' })).toBeInTheDocument()
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Draft #3' })).toHaveAttribute('href', '/11/estimates/971')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/contacts/701')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('period:this_year')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Quotes' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Add quote' })).toHaveAttribute('href', '/11/estimates/new')
  })

  it("filters on Moneybird's quick filters and the advanced filter", async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/estimates')
    await userEvent.click(await screen.findByRole('button', { name: 'Quotes from this year' }))
    const panel = screen.getByRole('search', { name: 'Quote filters' })
    expect(within(panel).getByRole('link', { name: 'Non-accepted quotes' })).toHaveAttribute('href', `/11/estimates?filter=${encodeURIComponent('state:open|late')}`)
    expect(within(panel).getByRole('link', { name: 'Draft quotes' })).toHaveAttribute('href', '/11/estimates?filter=state%3Adraft')
    await userEvent.click(within(panel).getByRole('radio', { name: 'Rejected' }))
    expect(within(panel).getByRole('combobox', { name: 'Workflow' })).toHaveTextContent('Offerte (NL)')
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:rejected,period:this_year'))
    expect(await screen.findByRole('button', { name: 'Filtered quotes' })).toBeInTheDocument()
  })

  it('says when there are none yet, with what quotes are for', async () => {
    server.use(...loaded([]))
    renderAt('/11/estimates')
    expect(await screen.findByText(/No quotes yet/)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Send your quotes' })).toBeInTheDocument()
  })
})

describe('adding a quote', () => {
  it('draws the new quote as its document, with its texts, and saves it from the Add quote popup', async () => {
    let sent: { estimate: Record<string, unknown> } | undefined
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/estimates', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(estimate(), { status: 201 })
    }))
    renderAt('/11/estimates/new')
    expect(await screen.findByRole('heading', { level: 1, name: 'Add quote' })).toBeInTheDocument()
    const paper = await quotePaper(/^Quote preview$/)
    expect(await within(paper).findByRole('heading', { level: 2, name: 'Offerte Concept #5' })).toBeInTheDocument()
    expect(within(paper).getByRole('button', { name: /Offertedatum/ })).toHaveTextContent('Offertedatum:11-11-2026')
    expect(within(paper).getByRole('button', { name: /Vervaldatum/ })).toHaveTextContent('Vervaldatum:25-11-2026')
    // As on Moneybird, no contact line under the title.
    expect(paper.querySelector('.invoice-paper__contact')).toBeNull()
    // The default texts, in the workflow's language, greet the placeholder until a contact is chosen.
    expect(within(paper).getByRole('button', { name: 'Edit the text above the lines' })).toHaveTextContent('Beste First name Last name,')
    expect(within(paper).getByRole('button', { name: 'Edit the text above the lines' })).toHaveTextContent('onze offerte Concept #5')
    expect(within(paper).getByRole('button', { name: 'Edit the text below the lines' })).toHaveTextContent('Met vriendelijke groet,Harpy B.V.')
    expect(within(paper).getByRole('button', { name: '2. Add quote details' })).toBeInTheDocument()
    // Unlike a new invoice, no popup opens by itself: the contact opens it.
    expect(screen.queryByRole('dialog')).toBeNull()
    await userEvent.click(within(paper).getByRole('button', { name: 'Contact' }))
    const dialog = screen.getByRole('dialog', { name: 'Add quote' })
    expect(within(dialog).getByText('Add the basic details to your quote. Select the contact you are making the quote for.')).toBeInTheDocument()
    expect(within(dialog).getByRole('note')).toHaveTextContent('The quote is created using workflow Offerte (NL) and document style Standaard. It is valid for 14 days after the quote date.')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Edit' }))
    expect(within(dialog).getByLabelText('Quote id')).toHaveValue('0002')
    expect(within(dialog).getByLabelText('Quote date')).toBeDisabled()
    expect(within(dialog).getByLabelText('Workflow')).toHaveValue('602')
    await userEvent.click(within(dialog).getByRole('combobox', { name: 'Contact' }))
    await userEvent.type(within(dialog).getByRole('searchbox', { name: 'Query' }), 'shop')
    await userEvent.click(await within(dialog).findByRole('option', { name: 'Example Shop B.V.' }))
    await userEvent.type(within(dialog).getByLabelText('Reference'), 'Project X')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent?.estimate).toMatchObject({
      contact_id: '701', reference: 'Project X', estimate_date: '',
      pre_text: expect.stringContaining('Beste {recipient.name},') as unknown, post_text: expect.stringContaining('{company.company_name}') as unknown,
      details_attributes: [],
    }))
    expect(sent?.estimate).not.toHaveProperty('payment_conditions')
    expect(sent?.estimate).not.toHaveProperty('invoice_date')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/estimates/971'))
  })
})

describe('a quote', () => {
  it('is viewed and edited on its own page: its text above the lines is changed in place', async () => {
    let patched: { estimate: Record<string, unknown> } | undefined
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/estimates/971', async ({ request }) => {
      patched = (await request.json()) as typeof patched
      return HttpResponse.json(estimate({ pre_text: 'Hallo', version: 2 }))
    }))
    renderAt('/11/estimates/971')
    const paper = await quotePaper(/^Quote Draft #3$/)
    expect(within(paper).getByRole('heading', { level: 2, name: 'Offerte Concept #3' })).toBeInTheDocument()
    expect(within(paper).getByRole('button', { name: 'Edit the text above the lines' })).toHaveTextContent('Beste Example Shop B.V.,')
    await userEvent.click(within(paper).getByRole('button', { name: 'Edit the text above the lines' }))
    const card = screen.getByRole('region', { name: 'Text above the lines' })
    const text = within(card).getByRole('textbox', { name: 'Text above the lines' })
    await userEvent.clear(text)
    await userEvent.type(text, 'Hallo')
    await userEvent.click(within(card).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(patched?.estimate).toMatchObject({ pre_text: 'Hallo', post_text: 'Tot ziens', contact_id: '701' }))
    expect(screen.getByRole('complementary', { name: 'Quote information' })).toHaveTextContent('Quote created')
  })

  it('is sent, marked accepted and billed to an invoice', async () => {
    let current = estimate()
    let sending: unknown
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/estimates/971', () => HttpResponse.json(current)),
      http.patch('*/api/v2/11/estimates/971/send_estimate', async ({ request }) => {
        sending = await request.json()
        current = estimate({
          state: 'open', estimate_id: '2026-0002', draft_id: null, estimate_date: '2026-11-11', updated_at: '2026-11-11T09:00:00Z', version: 2,
          events: [event('estimate_created', stamp), event('estimate_send_email', '2026-11-11T09:00:00Z')],
        })
        return HttpResponse.json(current)
      }),
      http.patch('*/api/v2/11/estimates/971/change_state', async ({ request }) => {
        const { state } = (await request.json()) as { state: Estimate['state'] }
        current = { ...current, state, version: 3, events: [...(current.events ?? []), event(`estimate_mark_${state}`, '2026-11-11T10:00:00Z')] }
        return HttpResponse.json(current)
      }),
      http.patch('*/api/v2/11/estimates/971/bill_estimate', () => HttpResponse.json({ id: '991', state: 'draft' } as SalesInvoice)),
      http.get('*/api/v2/11/sales_invoices/991', () => HttpResponse.json({ id: '991' })),
    )
    renderAt('/11/estimates/971')
    expect(await screen.findByText('Draft', { selector: '.actionbar .badge' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Bill quote' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Change state' })).toBeNull()
    expect(screen.getByRole('link', { name: 'Download' })).toHaveAttribute('href', '/api/v2/11/estimates/971/download_pdf')

    // As on Moneybird, Send quote opens its form above the quote: by email, or manually.
    const toggle = screen.getByRole('button', { name: 'Send quote' })
    await userEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    const form = screen.getByRole('region', { name: /^Send quote / })
    expect(within(form).getByRole('radio', { name: /Send quote by email/ })).toBeChecked()
    expect(within(form).getByText('The quote will be added as a PDF attachment to the email message.')).toBeInTheDocument()
    expect(within(form).getByRole('radio', { name: /Send quote manually/ })).not.toBeChecked()
    expect(within(form).getByText('The quote will be marked as sent.')).toBeInTheDocument()
    const address = within(form).getByRole('textbox', { name: 'Email address' })
    expect(address).toHaveValue('quotes@example.com')
    // Its + adds a comma for one more address, and says so.
    await userEvent.click(within(form).getByRole('button', { name: 'Add an email address' }))
    expect(address).toHaveValue('quotes@example.com, ')
    expect(within(form).getByText('You can add multiple values separated with a comma.')).toBeInTheDocument()
    await userEvent.type(address, 'boss@example.com')
    // The message starts as the workflow's, else the default in the quote's language, with its tags.
    expect(within(form).getByRole('textbox', { name: 'Email message' })).toHaveValue(
      'Beste {contact_name},\n\nIn de bijlage vind je offerte {estimate_id} van {estimate_date} voor {total}. De offerte is geldig tot {due_date}.\n\nMet vriendelijke groet,\n{company_name}',
    )
    await userEvent.click(within(form).getByRole('button', { name: 'Send quote' }))
    await waitFor(() => expect(sending).toEqual({ estimate_sending: {
      delivery_method: 'Email', email_address: 'quotes@example.com, boss@example.com', sign_online: true,
      email_message: 'Beste {contact_name},\n\nIn de bijlage vind je offerte {estimate_id} van {estimate_date} voor {total}. De offerte is geldig tot {due_date}.\n\nMet vriendelijke groet,\n{company_name}',
    } }))
    expect(screen.queryByRole('region', { name: /^Send quote / })).toBeNull()
    expect(await screen.findByText('Open', { selector: '.actionbar .badge' })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { level: 2, name: 'Offerte 2026-0002' })).toBeInTheDocument()
    expect(screen.getByRole('complementary', { name: 'Quote information' })).toHaveTextContent('Quote sent by email')

    await choose('Change state', 'Accepted')
    expect(await screen.findByText('Accepted', { selector: '.actionbar .badge' })).toBeInTheDocument()
    expect(screen.getByRole('complementary', { name: 'Quote information' })).toHaveTextContent('Quote marked as accepted')
    await userEvent.click(screen.getByRole('button', { name: 'Bill quote' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/sales_invoices/991'))
  })

  it('is signed by its sender before it goes to be signed online', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
    vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue('data:image/png;base64,MINE')
    let sending: unknown
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/estimates/971', () => HttpResponse.json(estimate())),
      http.patch('*/api/v2/11/estimates/971/send_estimate', async ({ request }) => {
        sending = await request.json()
        return HttpResponse.json(estimate({ state: 'open', estimate_id: '2026-0002', draft_id: null, version: 2 }))
      }),
    )
    renderAt('/11/estimates/971')
    await userEvent.click(await screen.findByRole('button', { name: 'Send quote' }))
    const form = screen.getByRole('region', { name: /^Send quote / })
    expect(within(form).getByText(/Online accepting enables the recipient/)).toBeInTheDocument()
    expect(within(form).getByRole('checkbox', { name: 'The recipient must sign and accept this quote online.' })).toBeChecked()
    await userEvent.click(within(form).getByRole('checkbox', { name: 'I want to sign this quote myself before it is being sent.' }))
    // Not signed yet: it says so and does not send.
    await userEvent.click(within(form).getByRole('button', { name: 'Send quote' }))
    expect(within(form).getByText('Sign in the field above.')).toBeInTheDocument()
    expect(sending).toBeUndefined()
    const pad = within(form).getByRole('img', { name: 'Signature' })
    fireEvent.pointerDown(pad, { clientX: 5, clientY: 5, pointerId: 1 })
    fireEvent.pointerMove(pad, { clientX: 30, clientY: 15, pointerId: 1 })
    fireEvent.pointerUp(pad, { clientX: 30, clientY: 15, pointerId: 1 })
    await userEvent.click(within(form).getByRole('button', { name: 'Send quote' }))
    await waitFor(() => expect(sending).toMatchObject({ estimate_sending: { delivery_method: 'Email', sign_online: true, sender: true }, signature_output: 'data:image/png;base64,MINE' }))
    vi.restoreAllMocks()
  })

  it('shows who signed it online, and their signed quote', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/estimates/971', () => HttpResponse.json(estimate({
      state: 'accepted', estimate_id: '2026-0002', draft_id: null,
      online_signature: { sender_signed: false, signed: true, signer_name: 'Jan Voorbeeld', signer_email: 'jan@example.com', signer_ip: '203.0.113.7', signed_at: '2026-11-16T14:30:00Z', online_url: 'https://books.example.com/online/estimates/971?code=1' },
      events: [event('estimate_created', stamp), event('estimate_signed_online', '2026-11-16T14:30:00Z', { name: 'Jan Voorbeeld' })],
    }))))
    renderAt('/11/estimates/971')
    const side = await screen.findByRole('complementary', { name: 'Quote information' })
    expect(await within(side).findByText('Jan Voorbeeld', { selector: 'dd' })).toBeInTheDocument()
    expect(within(side).getByText('203.0.113.7')).toBeInTheDocument()
    expect(within(side).getByRole('link', { name: 'Download signed PDF' })).toHaveAttribute('href', '/api/v2/11/estimates/971/download_signed_pdf')
    expect(side).toHaveTextContent('Quote accepted and signed online by Jan Voorbeeld')
    await userEvent.click(screen.getByRole('button', { name: 'Extra' }))
    expect(screen.getByRole('link', { name: 'View quote online' })).toHaveAttribute('href', 'https://books.example.com/online/estimates/971?code=1')
  })

  it('is marked as sent from the same form, without an email', async () => {
    let sending: unknown
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/estimates/971', () => HttpResponse.json(estimate())),
      http.patch('*/api/v2/11/estimates/971/send_estimate', async ({ request }) => {
        sending = await request.json()
        return HttpResponse.json(estimate({ state: 'open', estimate_id: '2026-0002', draft_id: null, version: 2 }))
      }),
    )
    renderAt('/11/estimates/971')
    await userEvent.click(await screen.findByRole('button', { name: 'Send quote' }))
    const form = screen.getByRole('region', { name: /^Send quote / })
    await userEvent.click(within(form).getByRole('radio', { name: /Send quote manually/ }))
    expect(within(form).queryByRole('textbox', { name: 'Email address' })).toBeNull()
    expect(within(form).queryByRole('textbox', { name: 'Email message' })).toBeNull()
    await userEvent.click(within(form).getByRole('button', { name: 'Send quote' }))
    await waitFor(() => expect(sending).toEqual({ estimate_sending: { delivery_method: 'Manual' } }))
  })

  it('names the invoice it was billed to in its sidebar, and cannot change once billed', async () => {
    const billed = estimate({
      state: 'billed', estimate_id: '2026-0003', draft_id: null, estimate_date: '2026-11-01',
      events: [event('estimate_created', stamp), event('estimate_billed', '2026-11-10T10:00:00Z', { draft_id: 4 }, ['SalesInvoice', '991'])],
    })
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/estimates/971', () => HttpResponse.json(billed)),
      http.get('*/api/v2/11/sales_invoices/991', () => HttpResponse.json({ id: '991', invoice_id: '2026-0042', draft_id: null, total_price_incl_tax: '363.0' })),
    )
    renderAt('/11/estimates/971')
    expect(await screen.findByText('This quote is billed or archived and cannot change.')).toBeInTheDocument()
    const sidebar = screen.getByRole('complementary', { name: 'Quote information' })
    expect(await within(sidebar).findByRole('link', { name: '2026-0042' })).toHaveAttribute('href', '/11/sales_invoices/991')
    expect(sidebar).toHaveTextContent('Invoiced€363.00 of €363.00')
    expect(within(sidebar).getByRole('link', { name: 'Offerte (NL)' })).toHaveAttribute('href', '/11/workflows/602')
    expect(sidebar).toHaveTextContent('Quote billed')
    expect(screen.queryByRole('button', { name: 'Bill quote' })).toBeNull()
  })

  it('is duplicated, naming its original, and deleted after asking', async () => {
    let copied: { estimate: Record<string, unknown> } | undefined
    let deleted = false
    server.use(...loaded())
    server.use(
      http.post('*/api/v2/11/estimates', async ({ request }) => {
        copied = (await request.json()) as typeof copied
        return HttpResponse.json(accepted, { status: 201 })
      }),
      http.delete('*/api/v2/11/estimates/972', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/estimates/971')
    await choose('Extra', 'Duplicate quote')
    await waitFor(() => expect(copied?.estimate).toMatchObject({ original_estimate_id: '971', contact_id: '701', pre_text: 'Beste {recipient.name},' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/estimates/972'))
    await choose('Extra', 'Delete quote')
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Delete quote' }))
    await waitFor(() => expect(deleted).toBe(true))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/estimates$/))
  })
})

describe('a copy of a quote', () => {
  it('keeps the lines in their order, without their ids', () => {
    const copy = duplicateOf(estimate({ details: [
      { id: '2', description: 'B', amount: '1', price: '2.0', tax_rate_id: '2101', row_order: 2 },
      { id: '1', description: 'A', amount: '1', price: '1.0', tax_rate_id: '2101', row_order: 1 },
    ] }))
    expect(copy.details_attributes).toEqual([
      { description: 'A', amount: '1', price: '1.0', tax_rate_id: '2101', row_order: 1 },
      { description: 'B', amount: '1', price: '2.0', tax_rate_id: '2101', row_order: 2 },
    ])
  })
})
