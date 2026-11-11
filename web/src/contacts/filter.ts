import type { Contact } from './queries'

/** The advanced filter of Moneybird's contacts list, in the order of its form; "" selects all. */
export type ContactFilter = {
  contact_type: '' | 'company' | 'private_individual'
  trusted_type: '' | 'trusted' | 'not_trusted'
  estimate_workflow_id: string
  invoice_workflow_id: string
  mandate_type: '' | 'manual' | 'adyen' | 'none'
  delivery_method: '' | 'email' | 'peppol' | 'manual' | 'post' | 'simplerinvoicing'
}

export const noFilter: ContactFilter = { contact_type: '', trusted_type: '', estimate_workflow_id: '', invoice_workflow_id: '', mandate_type: '', delivery_method: '' }

const allowed: { [K in keyof ContactFilter]: (value: string) => boolean } = {
  contact_type: (v) => ['company', 'private_individual'].includes(v),
  trusted_type: (v) => ['trusted', 'not_trusted'].includes(v),
  estimate_workflow_id: (v) => /^[0-9]{1,19}$/.test(v),
  invoice_workflow_id: (v) => /^[0-9]{1,19}$/.test(v),
  mandate_type: (v) => ['manual', 'adyen', 'none'].includes(v),
  delivery_method: (v) => ['email', 'peppol', 'manual', 'post', 'simplerinvoicing'].includes(v),
}

/** Reads the filter from the list's address, as key:value terms joined by commas; terms it does not know are left out. */
export function parseContactFilter(terms: string | null): ContactFilter {
  const filter = { ...noFilter }
  for (const term of (terms ?? '').split(',')) {
    const [key = '', value = ''] = term.split(':')
    if (key in allowed && allowed[key as keyof ContactFilter](value)) {
      Object.assign(filter, { [key]: value })
    }
  }
  return filter
}

/** The filter as the contacts API takes it: only what narrows the list, in the order of the form. */
export function contactFilterTerms(filter: ContactFilter): string {
  return (Object.keys(noFilter) as (keyof ContactFilter)[]).filter((k) => filter[k]).map((k) => `${k}:${filter[k]}`).join(',')
}

/** Moneybird's quick filters of the contacts list. */
export const quickFilters = {
  all: noFilter,
  trusted: { ...noFilter, trusted_type: 'trusted' },
  manual: { ...noFilter, delivery_method: 'manual' },
  email: { ...noFilter, delivery_method: 'email' },
} satisfies Record<string, ContactFilter>

export type QuickFilter = keyof typeof quickFilters | 'custom'

export function quickFilterOf(filter: ContactFilter): QuickFilter {
  const terms = contactFilterTerms(filter)
  const found = Object.entries(quickFilters).find(([, f]) => contactFilterTerms(f) === terms)
  return (found?.[0] as QuickFilter | undefined) ?? 'custom'
}

/** The heading a contact is listed under: its first letter without accents, or the digit or sign it starts with. */
export function contactGroup(name: string): string {
  return name.trim().charAt(0).normalize('NFD').charAt(0).toLocaleUpperCase()
}

/** The person a company contact is for, shown after the company's name in lists. */
export function contactPerson(c: Pick<Contact, 'company_name' | 'firstname' | 'lastname'>): string {
  return c.company_name ? [c.firstname, c.lastname].filter(Boolean).join(' ') : ''
}
