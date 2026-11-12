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
  http.get('*/api/v2/11/identities', () => HttpResponse.json([identity(), identity({ id: '501', company_name: 'Trade Name', default: false })])),
  http.get('*/api/onboarding/options', () =>
    HttpResponse.json({ countries: ['NL'], currencies: ['EUR', 'USD'], popular_currencies: ['EUR'], company_search_countries: [], address_lookup_countries: [] }),
  ),
]
