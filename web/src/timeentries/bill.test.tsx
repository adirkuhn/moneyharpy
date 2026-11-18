import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { TimeEntry } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const shop = { id: '5', company_name: 'Example Shop B.V.', firstname: null, lastname: null }

const entry = (over: Partial<TimeEntry> = {}): TimeEntry => ({
  id: '701', administration_id: '11', user_id: '1', description: 'Designing the website', started_at: '2026-11-17T09:00:00Z',
  ended_at: '2026-11-17T11:30:00Z', paused_duration: 1800, billable: true, contact_id: '5', project_id: null, sales_invoice_id: null,
  contact: shop, project: null, sales_invoice: null, user: { id: '1', name: 'Ann de Vries' }, notes: [], events: [], uri: '/11/time_entries/701',
  ...over,
})

const open = [entry(), entry({ id: '702', description: 'Support call', started_at: '2026-11-16T13:00:00Z', ended_at: '2026-11-16T14:00:00Z', paused_duration: 0, contact: null, contact_id: null })]

function loaded(seen: URL[] = [], sent: { method: string; body: { sales_invoice?: Record<string, unknown> } }[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/contacts', () => HttpResponse.json([shop])),
    http.get('*/api/v2/11/time_entries', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(open, { headers: { 'X-Total-Count': '2' } })
    }),
    http.get('*/api/v2/11/tax_rates', () => HttpResponse.json([
      { id: '91', name: '21%', percentage: '21.0', tax_rate_type: 'purchase_invoice', active: true },
      { id: '92', name: '21%', percentage: '21.0', tax_rate_type: 'sales_invoice', active: true },
    ])),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([
      { id: '81', name: 'Revenue', account_type: 'revenue' }, { id: '82', name: 'Bank', account_type: 'current_assets' },
    ])),
    http.get('*/api/v2/11/sales_invoices', () => HttpResponse.json([
      { id: '301', invoice_id: '2026-0001', state: 'paid', invoice_date: '2026-11-01', total_price_incl_tax: '100.0', currency: 'EUR' },
      { id: '302', invoice_id: null, state: 'draft', invoice_date: null, total_price_incl_tax: '0.0', currency: 'EUR' },
    ], { headers: { 'X-Total-Count': '2' } })),
    http.post('*/api/v2/11/sales_invoices', async ({ request }) => {
      sent.push({ method: 'POST', body: await request.json() as { sales_invoice?: Record<string, unknown> } })
      return HttpResponse.json({ id: '303' }, { status: 201 })
    }),
    http.patch('*/api/v2/11/sales_invoices/:id', async ({ request, params }) => {
      sent.push({ method: `PATCH ${String(params.id)}`, body: await request.json() as { sales_invoice?: Record<string, unknown> } })
      return HttpResponse.json({ id: params.id })
    }),
  ]
}

async function pickShopAndTime() {
  const first = await screen.findByRole('form', { name: 'Contact to bill' })
  expect(screen.getByText('Step 1 of 3')).toBeInTheDocument()
  expect(within(first).getByRole('button', { name: 'Next step' })).toBeDisabled()
  await userEvent.click(within(first).getByRole('combobox', { name: 'Contact' }))
  await userEvent.click(await screen.findByRole('option', { name: 'Example Shop B.V.' }))
  await userEvent.click(within(first).getByRole('button', { name: 'Next step' }))
  const second = await screen.findByRole('form', { name: 'Time entries to bill' })
  expect(await within(second).findByText('Week 47')).toBeInTheDocument()
  expect(within(second).getByRole('status')).toHaveTextContent('0 items selected')
  return second
}

describe('billing time entries', () => {
  it('bills the chosen open time of a contact on a new invoice, merged into one line', async () => {
    const seen: URL[] = []
    const sent: { method: string; body: { sales_invoice?: Record<string, unknown> } }[] = []
    server.use(...loaded(seen, sent))
    renderAt('/11/time_entries_wizard')
    const second = await pickShopAndTime()
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:open,period:this_year,include_active:false,contact_id:5,include_nil_contacts:true')
    await userEvent.click(within(second).getByRole('checkbox', { name: 'Select all' }))
    expect(within(second).getByRole('status')).toHaveTextContent('2 items selected')
    await userEvent.click(within(second).getByRole('button', { name: 'Next step' }))

    const third = await screen.findByRole('form', { name: 'Invoice settings' })
    await userEvent.type(within(third).getByLabelText('Hourly rate'), '80')
    await userEvent.selectOptions(within(third).getByLabelText('Category'), 'Revenue')
    await waitFor(() => expect(within(third).getByRole('button', { name: 'Create invoice' })).toBeEnabled())
    await userEvent.click(within(third).getByRole('button', { name: 'Create invoice' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/sales_invoices/303'))
    expect(sent[0]?.method).toBe('POST')
    expect(sent[0]?.body.sales_invoice).toEqual({
      contact_id: '5', time_entry_ids: ['701', '702'],
      details_attributes: [{ description: 'Support call, Designing the website', amount: '3', price: '80', tax_rate_id: '92', ledger_account_id: '81', period: '20261116..20261117' }],
    })
  })

  it('links time to a paid invoice without adding lines to it', async () => {
    const sent: { method: string; body: { sales_invoice?: Record<string, unknown> } }[] = []
    server.use(...loaded([], sent))
    renderAt('/11/time_entries_wizard')
    const second = await pickShopAndTime()
    await userEvent.click(within(second).getByRole('checkbox', { name: 'Hours for 2026-11-17 - Designing the website' }))
    await userEvent.click(within(second).getByRole('button', { name: 'Next step' }))
    const third = await screen.findByRole('form', { name: 'Invoice settings' })
    await userEvent.click(within(third).getByRole('radio', { name: 'Link existing sales invoice' }))
    expect(within(third).getByRole('checkbox', { name: 'Add details to sales invoice?' })).toBeDisabled()
    await userEvent.click(within(third).getByRole('button', { name: 'Create invoice' }))
    await waitFor(() => expect(sent[0]?.method).toBe('PATCH 301'))
    expect(sent[0]?.body.sales_invoice).toEqual({ time_entry_ids: ['701'] })
  })

  it('goes back a step, and the hours report leads here', async () => {
    server.use(...loaded())
    renderAt('/11/time_entries_report')
    await userEvent.click(await screen.findByRole('link', { name: 'Bill time entries' }))
    const second = await pickShopAndTime()
    await userEvent.click(within(second).getByRole('button', { name: 'Previous step' }))
    expect(await screen.findByRole('form', { name: 'Contact to bill' })).toBeInTheDocument()
  })
})
