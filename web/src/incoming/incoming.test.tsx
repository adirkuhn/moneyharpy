import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { PurchaseInvoice } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const supplier = { id: '701', administration_id: '11', company_name: 'Supplier B.V.', firstname: null, lastname: null, country: 'NL' }

const invoice = (over: Partial<PurchaseInvoice> = {}): PurchaseInvoice => ({
  id: '941', administration_id: '11', contact_id: '701', contact: supplier, date: '2026-11-01', due_date: '2026-11-15', state: 'open',
  reference: '2026-01234', entry_number: 1, origin: 'api', currency: 'EUR', prices_are_incl_tax: false, paid_at: null, revenue_invoice: false,
  details: [{ id: '961', description: 'Paper', amount: '1', price: '100.0', tax_rate_id: '2101', ledger_account_id: '4999', total_price_excl_tax_with_discount: '100.0' }],
  fiscal_allocations: [], payments: [], notes: [], events: [],
  attachments: [{ id: '971', administration_id: '11', attachable_id: '941', attachable_type: 'Document', filename: 'bill.pdf', content_type: 'application/pdf', size: 2048, rotation: 0, created_at: stamp, updated_at: stamp }],
  total_price_excl_tax: '100.0', total_price_incl_tax: '121.0', total_paid: '0.0', total_unpaid: '121.0', total_tax: '21.0',
  version: 1, uri: '/11/documents/purchase_invoices/941',
  ...over,
})

function loaded() {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/documents', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
    http.get('*/api/v2/11/documents/purchase_invoices/941', () => HttpResponse.json(invoice())),
    http.get('*/api/v2/11/documents/receipts', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
    http.get('*/api/v2/11/contacts', () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } })),
  ]
}

describe('a purchase invoice', () => {
  it('shows a purchase invoice and deletes it', async () => {
    let deleted = false
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/documents/purchase_invoices/941', () => {
      deleted = true
      return HttpResponse.json(200)
    }))
    renderAt('/11/documents/purchase_invoices/941')
    expect(await screen.findByRole('heading', { level: 1, name: '2026-01234' })).toBeInTheDocument()
    expect(screen.getByRole('table', { name: 'Lines' })).toHaveTextContent('Paper')
    expect(screen.getByRole('link', { name: 'bill.pdf' })).toHaveAttribute('href', '/api/v2/11/documents/purchase_invoices/941/attachments/971/download')
    await chooseMore('Delete')
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/documents$/))
    expect(deleted).toBe(true)
  })
})
