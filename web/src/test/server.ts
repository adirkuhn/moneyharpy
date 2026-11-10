import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import type { components } from '../api/schema'

export type Administration = components['schemas']['Administration']

export const user: components['schemas']['CurrentUser'] = {
  id: '1',
  email: 'ann@example.com',
  name: 'Ann de Vries',
  instance_admin: false,
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
