import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type Contact = components['schemas']['Contact']
export type ContactChanges = NonNullable<components['schemas']['ContactRequest']['contact']>
export type ContactPerson = components['schemas']['contact_person_response']
export type PersonChanges = components['schemas']['ContactPersonFields']

export const perPage = 50

/** The client number a new contact gets, which the add contact form shows filled in. */
export function useNextCustomerId(administrationId: string, enabled: boolean) {
  return useQuery({
    queryKey: ['contact-next-customer-id', administrationId],
    enabled,
    // Another contact may have taken it since, so it is asked for afresh every time.
    gcTime: 0,
    queryFn: async () => (await unwrap(api.GET('/api/v2/{administration_id}/contacts/next_customer_id', { params: { path: admin(administrationId) } }))).customer_id,
  })
}

export type ContactSearch = {
  query: string
  /** The filter terms of the advanced filter, such as contact_type:company; empty for all contacts. */
  filter: string
  archived: boolean
  page: number
}

export type ContactPage = { contacts: Contact[]; total: number }

const admin = (administrationId: string) => ({ administration_id: administrationId })

async function fetchContacts(administrationId: string, search: ContactSearch, size: number): Promise<ContactPage> {
  const query = {
    page: String(search.page),
    per_page: String(size),
    ...(search.query ? { query: search.query } : {}),
    ...(search.archived ? { include_archived: 'true' as const } : {}),
  }
  const { data, error, response } = search.filter
    ? await api.GET('/api/v2/{administration_id}/contacts/filter', { params: { path: admin(administrationId), query: { ...query, filter: search.filter } } })
    : await api.GET('/api/v2/{administration_id}/contacts', { params: { path: admin(administrationId), query } })
  if (!response.ok) {
    throw new ApiError(response.status, error)
  }
  return { contacts: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
}

/** One page of contacts, with the number selected in all. */
export function useContacts(administrationId: string, search: ContactSearch) {
  return useQuery({
    queryKey: ['contacts', administrationId, search],
    placeholderData: keepPreviousData,
    queryFn: () => fetchContacts(administrationId, search, perPage),
  })
}

/** How many contacts the search selects with the archived ones included, to tell how many it leaves out. */
export function useTotalWithArchived(administrationId: string, search: ContactSearch, enabled: boolean) {
  return useQuery({
    queryKey: ['contacts', administrationId, 'with-archived', search.query, search.filter],
    enabled,
    queryFn: async () => (await fetchContacts(administrationId, { ...search, archived: true, page: 1 }, 1)).total,
  })
}

/** The first page of groups of contacts that may be the same. */
export function useContactDoubles(administrationId: string) {
  return useQuery({
    queryKey: ['contacts', administrationId, 'doubles'],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/contacts/doubles', { params: { path: admin(administrationId) } })),
  })
}

export function useContact(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['contact', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/contacts/{id}', { params: { path: { ...admin(administrationId), id: id ?? '' } } })),
  })
}

function useRefresh(administrationId: string) {
  const client = useQueryClient()
  return (id?: string) => {
    void client.invalidateQueries({ queryKey: ['contacts', administrationId] })
    if (id) {
      void client.invalidateQueries({ queryKey: ['contact', administrationId, id] })
    }
  }
}

/** Creates a contact when id is undefined and updates it otherwise. */
export function useSaveContact(administrationId: string, id?: string) {
  const refresh = useRefresh(administrationId)
  return useMutation({
    mutationFn: (contact: ContactChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/contacts/{id}', { params: { path: { ...admin(administrationId), id } }, body: { contact } }))
        : unwrap(api.POST('/api/v2/{administration_id}/contacts', { params: { path: admin(administrationId) }, body: { contact } })),
    onSuccess: (saved) => refresh(saved.id),
  })
}

export function useArchiveContact(administrationId: string, id: string) {
  const refresh = useRefresh(administrationId)
  return useMutation({
    mutationFn: () => unwrap(api.PATCH('/api/v2/{administration_id}/contacts/{id}/archive', { params: { path: { ...admin(administrationId), id } } })),
    onSuccess: () => refresh(id),
  })
}

export function useDeleteContact(administrationId: string, id: string) {
  const refresh = useRefresh(administrationId)
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/contacts/{id}', { params: { path: { ...admin(administrationId), id } } })),
    onSuccess: () => refresh(),
  })
}

export function useSavePerson(administrationId: string, contactId: string, id?: string) {
  const refresh = useRefresh(administrationId)
  const path = { ...admin(administrationId), contact_id: contactId }
  return useMutation({
    mutationFn: (contact_person: PersonChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/contacts/{contact_id}/contact_people/{id}', { params: { path: { ...path, id } }, body: { contact_person } }))
        : unwrap(api.POST('/api/v2/{administration_id}/contacts/{contact_id}/contact_people', { params: { path }, body: { contact_person } })),
    onSuccess: () => refresh(contactId),
  })
}

export function useDeletePerson(administrationId: string, contactId: string) {
  const refresh = useRefresh(administrationId)
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE('/api/v2/{administration_id}/contacts/{contact_id}/contact_people/{id}', { params: { path: { ...admin(administrationId), contact_id: contactId, id } } })),
    onSuccess: () => refresh(contactId),
  })
}

export function useAddNote(administrationId: string, contactId: string) {
  const refresh = useRefresh(administrationId)
  return useMutation({
    mutationFn: (note: { note: string; todo: boolean }) =>
      unwrap(api.POST('/api/v2/{administration_id}/contacts/{contact_id}/notes', { params: { path: { ...admin(administrationId), contact_id: contactId } }, body: { note } })),
    onSuccess: () => refresh(contactId),
  })
}

export function useDeleteNote(administrationId: string, contactId: string) {
  const refresh = useRefresh(administrationId)
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE('/api/v2/{administration_id}/contacts/{contact_id}/notes/{id}', { params: { path: { ...admin(administrationId), contact_id: contactId, id } } })),
    onSuccess: () => refresh(contactId),
  })
}

export type CompanyMatch = components['schemas']['CompanyMatch']

/** Companies in a country's company register whose name matches, for filling in a new contact. */
export function useCompanyRegister(country: string, term: string, enabled: boolean) {
  return useQuery({
    queryKey: ['company-search', country, term],
    enabled: enabled && term.length >= 2,
    queryFn: () => unwrap(api.GET('/api/onboarding/company_search', { params: { query: { country, query: term } } })),
  })
}

/** What lists show for a contact: the company, else the person's name. */
export function contactName(c: Pick<Contact, 'company_name' | 'firstname' | 'lastname'>) {
  const company = typeof c.company_name === 'string' ? c.company_name : ''
  return company || [c.firstname, c.lastname].filter(Boolean).join(' ')
}
