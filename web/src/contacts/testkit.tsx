import { within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { administration, listing, signedIn, workflow } from '../test/server'
import type { Contact } from './queries'

export const harpy = administration({ id: '11', name: 'Harpy B.V.' })
export const stamp = '2026-11-10T09:00:00Z'

export const contact = (over: Partial<Contact> = {}): Contact => ({
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
export const ann = contact({ id: '902', company_name: '', firstname: 'Ann', lastname: 'Jansen', customer_id: '2', email: null, send_invoices_to_email: null, city: 'Utrecht', uri: '/11/contacts/902' })

export type Options = { total?: number; archived?: number; doubles?: Contact[][]; companySearch?: boolean }

export function loaded(list: Contact[] = [contact(), ann], seen: URL[] = [], { total = list.length, archived = 0, doubles = [], companySearch = false }: Options = {}) {
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
    http.get('*/api/v2/11/contacts/next_customer_id', () => HttpResponse.json({ customer_id: '43' })),
    http.get('*/api/v2/11/contacts/doubles', () => HttpResponse.json(doubles.map((contacts) => ({ contacts })))),
    http.get('*/api/v2/11/contacts/901', () => HttpResponse.json(contact())),
    // The documents of a contact, for its feed: none unless a test says otherwise.
    ...['sales_invoices', 'external_sales_invoices', 'estimates', 'documents/purchase_invoices', 'documents/receipts'].map((path) =>
      http.get(`*/api/v2/11/${path}`, () => HttpResponse.json([], { headers: { 'X-Total-Count': '0' } }))),
  ]
}

/** The text of the cells of each row of the contacts table, under its header. */
export function rowsOf(table: HTMLElement) {
  return within(table).getAllByRole('row').slice(1).map((row) => within(row).getAllByRole('cell').map((cell) => cell.textContent))
}
