import { http, HttpResponse } from 'msw'
import { administration, documentStyle, identity, invoiceSettings, listing, signedIn, workflow } from '../test/server'

export const harpy = administration({ id: '11', name: 'Harpy B.V.' })
export const invoiceFlow = workflow()
export const quoteFlow = workflow({ id: '602', type: 'EstimateWorkflow', steps: [] })
export const exportFlow = workflow({ id: '603', name: 'Export', default: false })
export const defaultStyle = documentStyle()
export const otherStyle = documentStyle({ id: '702', name: 'The Best', default: false })
export const slogan = { id: '801', administration_id: '11', name: 'Slogan', source: 'identity' as const, uri: '/11/custom_fields/801' }
export const motto = { id: '802', administration_id: '11', name: 'Motto', source: 'identity' as const, uri: '/11/custom_fields/802' }

const stamp = '2026-11-10T09:00:00Z'
export const salesRate = {
  id: '2101', administration_id: '11', name: '21% btw', percentage: '21', tax_rate_type: 'sales_invoice' as const, show_tax: true, active: true,
  country: null, created_at: stamp, updated_at: stamp, uri: '/11/tax_rates/2101', tax_report_boxes: ['NL/1a'], position: 1,
}
/** A sent invoice the previews can show instead of example data. */
export const sentInvoice = {
  id: '901', administration_id: '11', contact_id: '701', invoice_id: '2026-0042', draft_id: 1, state: 'open' as const,
  contact: { id: '701', company_name: 'Example Shop B.V.', address1: 'Dam 2', zipcode: '1012AB', city: 'Amsterdam', country: 'NL', tax_number: 'NL000099998B57' },
  invoice_date: '2026-11-01', due_date: '2026-11-15', reference: 'Order 12', currency: 'EUR', prices_are_incl_tax: false, payment_conditions: '',
  details: [{ id: '951', description: 'Consulting', amount: '2', price: '100.0', tax_rate_id: '2101' }],
  tax_totals: [{ tax_rate_id: '2101', taxable_amount: '200.0', tax_amount: '42.0' }],
  total_price_excl_tax: '200.0', total_price_incl_tax: '242.0', uri: '/11/sales_invoices/901',
}

export const loaded = () => [
  signedIn(),
  listing([harpy]),
  http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
  http.get('*/api/v2/11/invoice_settings', () => HttpResponse.json(invoiceSettings())),
  http.get('*/api/v2/11/workflows', () => HttpResponse.json([quoteFlow, exportFlow, invoiceFlow])),
  http.get('*/api/v2/11/workflows/601', () => HttpResponse.json(invoiceFlow)),
  http.get('*/api/v2/11/workflows/603', () => HttpResponse.json(exportFlow)),
  http.get('*/api/v2/11/document_styles', () => HttpResponse.json([defaultStyle, otherStyle])),
  http.get('*/api/v2/11/document_styles/702', () => HttpResponse.json(otherStyle)),
  http.get('*/api/v2/11/custom_fields', () => HttpResponse.json([slogan, motto])),
  http.get('*/api/v2/11/tax_rates', () => HttpResponse.json([salesRate])),
  http.get('*/api/v2/11/sales_invoices', () => HttpResponse.json([])),
  http.get('*/api/v2/11/products', () => HttpResponse.json([])),
  http.get('*/api/v2/11/identities', () => HttpResponse.json([identity(), identity({ id: '501', company_name: 'Trade Name', default: false })])),
  http.get('*/api/onboarding/options', () =>
    HttpResponse.json({ countries: ['NL'], currencies: ['EUR', 'USD'], popular_currencies: ['EUR'], company_search_countries: [], address_lookup_countries: [] }),
  ),
]
