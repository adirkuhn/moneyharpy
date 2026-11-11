import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import type { Contact } from '../contacts/queries'
import type { SalesInvoice } from '../invoices/queries'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, documentStyle, listing, server, signedIn, workflow } from '../test/server'
import { estimateFilterTerms, type Estimate } from './queries'

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

const estimate = (over: Partial<Estimate> = {}): Estimate => ({
  id: '971', administration_id: '11', contact_id: '701', contact: shop, contact_person_id: null, workflow_id: '602', document_style_id: '701',
  identity_id: '461', estimate_id: null, estimate_sequence_id: null, draft_id: 3, state: 'draft', estimate_date: null, due_date: '2026-12-10',
  reference: 'Project X', language: 'en', currency: 'EUR', exchange_rate: '1.0', discount: '0.0', original_estimate_id: null, show_tax: true,
  sign_online: true, sent_at: null, accepted_at: null, rejected_at: null, archived_at: null, created_at: stamp, updated_at: stamp,
  public_view_code: null, public_view_code_expires_at: null, version: 1, pre_text: 'Hello', post_text: null, prices_are_incl_tax: false,
  total_price_excl_tax: '300.0', total_price_excl_tax_base: '300.0', total_price_incl_tax: '363.0', total_price_incl_tax_base: '363.0',
  total_discount: '0.0', total_tax: '63.0', url: 'http://localhost/11/estimates/971', first_due_interval: 30,
  details: [{ id: '981', description: 'Project X', amount: '1', price: '300.0', tax_rate_id: '2101', ledger_account_id: '8001', total_price_excl_tax_with_discount: '300.0' }],
  custom_fields: [], notes: [], attachments: [], events: [], tax_totals: [], uri: '/11/estimates/971',
  ...over,
})

const accepted = estimate({ id: '972', estimate_id: 'OF-0001', draft_id: null, state: 'accepted', estimate_date: '2026-11-01', reference: 'Project Y', uri: '/11/estimates/972' })

function loaded(list: Estimate[] = [estimate(), accepted], seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/estimates', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/estimates/971', () => HttpResponse.json(estimate())),
    http.get('*/api/v2/11/estimates/972', () => HttpResponse.json(accepted)),
    http.get('*/api/v2/11/tax_rates', () => HttpResponse.json([vat21])),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales])),
    http.get('*/api/v2/11/workflows', () => HttpResponse.json([workflow(), workflow({ id: '602', type: 'EstimateWorkflow', name: 'Offerte (NL)' })])),
    http.get('*/api/v2/11/document_styles', () => HttpResponse.json([documentStyle({ id: '701' })])),
    http.get('*/api/v2/11/contacts', () => HttpResponse.json([shop as unknown as Contact], { headers: { 'X-Total-Count': '1' } })),
  ]
}

describe('the quote filter', () => {
  it('asks for every date, and a state when chosen', () => {
    expect(estimateFilterTerms({ state: '', contactId: '' })).toBe('state:all')
    expect(estimateFilterTerms({ state: 'open', contactId: '701' })).toBe('state:open,contact_id:701')
  })
})

describe('the quote list', () => {
  it('lists quotes with their contact, number, total and state, and filters them', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/estimates')
    const table = await screen.findByRole('table', { name: 'Quotes' })
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('Example Shop B.V.')
    expect(rows[1]).toHaveTextContent('Draft quote 3')
    expect(rows[1]).toHaveTextContent('€363.00')
    expect(rows[1]).toHaveTextContent('Draft')
    expect(rows[2]).toHaveTextContent('OF-0001')
    expect(rows[2]).toHaveTextContent('Accepted')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/estimates/971')
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:all')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Quotes' })).toHaveAttribute('aria-current', 'page')

    await userEvent.click(screen.getByRole('button', { name: 'All quotes' }))
    const panel = screen.getByRole('search', { name: 'Quote filters' })
    await userEvent.click(within(panel).getByRole('radio', { name: 'Rejected' }))
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:rejected'))
  })

  it('says when there are none yet', async () => {
    server.use(...loaded([]))
    renderAt('/11/estimates')
    expect(await screen.findByText(/No quotes yet/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'New quote' })).toHaveAttribute('href', '/11/estimates/new')
  })
})

describe('a quote', () => {
  it('is made with the line editor', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/estimates', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(estimate(), { status: 201 })
    }))
    renderAt('/11/estimates/new')
    const form = await screen.findByRole('form', { name: 'Quote' })
    await userEvent.type(within(form).getByRole('combobox', { name: 'Contact' }), 'Example')
    await userEvent.click(await screen.findByRole('option', { name: /Example Shop B.V./ }))
    await userEvent.type(within(form).getByLabelText('Reference'), 'Project X')
    await userEvent.type(within(form).getByLabelText('Text above the lines'), 'Hello')
    await userEvent.type(within(form).getByLabelText('Description'), 'Project X')
    await userEvent.type(within(form).getByLabelText('Price'), '300')
    await userEvent.click(within(form).getByRole('button', { name: 'Save quote' }))
    await waitFor(() => expect(sent).toMatchObject({
      estimate: {
        contact_id: '701', reference: 'Project X', pre_text: 'Hello', estimate_date: '',
        details_attributes: [{ description: 'Project X', amount: '1', price: '300', tax_rate_id: '2101' }],
      },
    }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/estimates/971'))
  })

  it('is sent, accepted and billed', async () => {
    let current = estimate()
    let sending: unknown
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/estimates/971', () => HttpResponse.json(current)),
      http.patch('*/api/v2/11/estimates/971/send_estimate', async ({ request }) => {
        sending = await request.json()
        current = estimate({ state: 'open', estimate_id: 'OF-0002', draft_id: null, estimate_date: '2026-11-11', updated_at: '2026-11-11T09:00:00Z' })
        return HttpResponse.json(current)
      }),
      http.patch('*/api/v2/11/estimates/971/change_state', async ({ request }) => {
        const { state } = (await request.json()) as { state: Estimate['state'] }
        current = { ...current, state, updated_at: '2026-11-11T09:01:00Z' }
        return HttpResponse.json(current)
      }),
      http.patch('*/api/v2/11/estimates/971/bill_estimate', () => HttpResponse.json({ id: '991', state: 'draft' } as SalesInvoice)),
      http.get('*/api/v2/11/sales_invoices/991', () => HttpResponse.json({ id: '991' })),
    )
    renderAt('/11/estimates/971')
    expect(await screen.findByRole('heading', { name: 'Draft quote 3' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Bill' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Send by email' }))
    await waitFor(() => expect(sending).toEqual({ estimate_sending: { delivery_method: 'Email' } }))
    expect(await screen.findByRole('heading', { name: 'OF-0002' })).toBeInTheDocument()
    await chooseMore('Mark as accepted')
    expect(await screen.findByText('Accepted', { selector: '.badge' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Bill' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/sales_invoices/991'))
  })

  it('cannot change once billed', async () => {
    server.use(...loaded())
    server.use(http.get('*/api/v2/11/estimates/971', () => HttpResponse.json(estimate({ state: 'billed', estimate_id: 'OF-0003' }))))
    renderAt('/11/estimates/971')
    expect(await screen.findByText('This quote is billed or archived and cannot change.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save quote' })).toBeNull()
    expect(within(screen.getByRole('form', { name: 'Quote' })).getByLabelText('Reference')).toBeDisabled()
  })
})
