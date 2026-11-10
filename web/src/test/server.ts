import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import type { components } from '../api/schema'

export type Administration = components['schemas']['Administration']

export const user: components['schemas']['CurrentUser'] = {
  id: '1',
  email: 'ann@example.com',
  name: 'Ann de Vries',
  instance_admin: false,
  two_factor_enabled: false,
}

export function administration(over: Partial<Administration> = {}): Administration {
  const id = over.id ?? '416072101932303822'
  return {
    id,
    name: 'Harpy B.V.',
    language: 'en',
    currency: 'EUR',
    country: 'NL',
    time_zone: 'Europe/Amsterdam',
    access: 'user',
    suspended: false,
    period_locked_until: null,
    period_start_date: '2026-01-01',
    favorite: false,
    uri: `/${id}`,
    small_business_scheme: false,
    company_activities_description: '',
    company_website_url: '',
    company_website_absent: false,
    user_type: 'owner',
    permissions: ['sales_invoices', 'documents', 'estimates', 'bank', 'settings', 'ownership', 'time_entries'],
    require_two_factor: false,
    support_access_until: null,
    ...over,
  }
}

export type Identity = components['schemas']['Identity']

export function identity(over: Partial<Identity> = {}): Identity {
  const id = over.id ?? '500'
  return {
    id,
    administration_id: '11',
    company_name: 'Harpy B.V.',
    country: 'NL',
    city: 'Amsterdam',
    zipcode: '1012JS',
    address1: 'Dam 1',
    address2: null,
    email: 'books@example.com',
    phone: null,
    bank_account_name: null,
    bank_account_number: 'NL91ABNA0417164300',
    bank_account_bic: 'ABNANL2A',
    chamber_of_commerce: '12345678',
    tax_number: null,
    custom_fields: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    legal_entity_type: 'BV',
    tax_number_absence_reason: null,
    default: true,
    uri: `/11/identities/${id}`,
    ...over,
  }
}

export const signedIn = (u = user) => http.get('*/api/auth/me', () => HttpResponse.json(u))
export const signedOut = () =>
  http.get('*/api/auth/me', () => HttpResponse.json({ error: 'Authorization required' }, { status: 401 }))
export const listing = (rows: Administration[]) =>
  http.get('*/api/v2/administrations', () => HttpResponse.json(rows))
export const providers = (google = false, registration_open = true, lm2 = false, password_reset = false) =>
  http.get('*/api/auth/providers', () => HttpResponse.json({ google, registration_open, lm2, password_reset }))

export const server = setupServer()

export type Workflow = components['schemas']['Workflow']
export type DocumentStyle = components['schemas']['DocumentStyle']
export type InvoiceSettings = components['schemas']['InvoiceSettings']

export function workflow(over: Partial<Workflow> = {}): Workflow {
  const id = over.id ?? '601'
  return {
    id, administration_id: '11', type: 'InvoiceWorkflow', name: 'Standaard (NL)', default: true, currency: 'EUR', language: 'nl',
    active: true, prices_are_incl_tax: false, payment_methods: null, collection_method: null, number_of_collections: 0, show_qr_code: true,
    steps: [
      { id: '1', administration_id: '11', step_order: 1, due_interval: 14, reminder_delay: 0, reminder_text: null, reminder_auto_send: false, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
      { id: '2', administration_id: '11', step_order: 2, due_interval: 0, reminder_delay: 7, reminder_text: 'Please pay', reminder_auto_send: true, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
    ],
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', uri: `/11/workflows/${id}`,
    ...over,
  }
}

export function documentStyle(over: Partial<DocumentStyle> = {}): DocumentStyle {
  const id = over.id ?? '701'
  return {
    id, administration_id: '11', name: 'Standaard', identity_id: '500', default: true, logo_hash: null, logo_container_full_width: false,
    logo_display_width: 50, logo_position: 'left', background_hash: null, paper_size: 'A4', address_position: 'left', font_size: 10,
    font_family: 'lucida_grande', print_on_stationery: false, custom_css: null,
    invoice_sender_address: [{ field: 'company_name', bold: true }, { field: 'address' }],
    invoice_metadata_left: [{ field: 'reference' }], invoice_metadata_right: [{ field: 'invoice_date' }, { field: 'due_date' }],
    estimate_sender_address: [{ field: 'company_name', bold: true }], estimate_metadata_left: [], estimate_metadata_right: [{ field: 'estimate_date' }],
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', uri: `/11/document_styles/${id}`,
    ...over,
  }
}

export function invoiceSettings(over: Partial<InvoiceSettings> = {}): InvoiceSettings {
  return {
    invoice_id_template: '{year}-{id}', invoice_sequence_id: '0001', invoice_id_example: '2026-0001',
    estimate_id_template: '{year}-{id}', estimate_sequence_id: '0001', estimate_id_example: '2026-0001',
    invoice_bcc_email_address: '', invoice_reminders_bcc_enabled: false, estimate_bcc_email_address: '', order_bcc_email_address: '',
    default_email_ubl: true, payments_without_proof: true,
    ...over,
  }
}
