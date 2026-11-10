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
    ...over,
  }
}

export const signedIn = (u = user) => http.get('*/api/auth/me', () => HttpResponse.json(u))
export const signedOut = () =>
  http.get('*/api/auth/me', () => HttpResponse.json({ error: 'Authorization required' }, { status: 401 }))
export const listing = (rows: Administration[]) =>
  http.get('*/api/v2/administrations', () => HttpResponse.json(rows))
export const providers = (google = false, registration_open = true, lm2 = false) =>
  http.get('*/api/auth/providers', () => HttpResponse.json({ google, registration_open, lm2 }))

export const server = setupServer()
