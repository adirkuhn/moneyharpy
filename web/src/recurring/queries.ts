import type { TFunction } from 'i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'
import type { HistoryItem } from '../invoices/history'
import { perPage } from '../invoices/queries'

export type RecurringInvoice = components['schemas']['RecurringSalesInvoice']
export type RecurringChanges = NonNullable<components['schemas']['RecurringSalesInvoiceRequest']['recurring_sales_invoice']>
export type FrequencyType = NonNullable<RecurringChanges['frequency_type']>

export const frequencyTypes: FrequencyType[] = ['day', 'week', 'month', 'quarter', 'year']

/** The list's filter: state is active, inactive or all; the others are left out when empty. */
export type RecurringFilter = { state: string; frequency: string; autoSend: string; contactId: string; workflowId: string }

/** The filter kept in the address, active ones unless it says otherwise. */
export function recurringFilterOf(params: URLSearchParams): RecurringFilter {
  return {
    state: params.get('state') || 'active', frequency: params.get('frequency') ?? '', autoSend: params.get('auto_send') ?? '',
    contactId: params.get('contact_id') ?? '', workflowId: params.get('workflow_id') ?? '',
  }
}

/** The address of the list with a filter, leaving out what is the default. */
export function recurringQuery(f: RecurringFilter, params: URLSearchParams) {
  const next = new URLSearchParams(params)
  const values: [string, string][] = [
    ['state', f.state === 'active' ? '' : f.state], ['frequency', f.frequency], ['auto_send', f.autoSend], ['contact_id', f.contactId], ['workflow_id', f.workflowId],
  ]
  for (const [key, value] of values) {
    if (value) {
      next.set(key, value)
    } else {
      next.delete(key)
    }
  }
  next.delete('page')
  return next
}

/** Moneybird's filter terms for a filter, such as "state:active,frequency:month". */
export function recurringFilterTerms(f: RecurringFilter) {
  return [
    `state:${f.state || 'active'}`, f.frequency && `frequency:${f.frequency}`, f.autoSend && `auto_send:${f.autoSend}`,
    f.contactId && `contact_id:${f.contactId}`, f.workflowId && `workflow_id:${f.workflowId}`,
  ].filter(Boolean).join(',')
}

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of recurring invoices, the next to make first. */
export function useRecurringInvoices(administrationId: string, filter: string, page: number) {
  return useQuery({
    queryKey: ['recurring-invoices', administrationId, filter, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/recurring_sales_invoices', {
        params: { path: admin(administrationId), query: { filter, page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { invoices: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useRecurringInvoice(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['recurring-invoice', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/recurring_sales_invoices/{id}', one(administrationId, id ?? ''))),
  })
}

/** The sales invoices a recurring invoice made, newest first. */
export function useInvoicesOf(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['sales-invoices', administrationId, 'recurring', id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/sales_invoices', {
      params: { path: admin(administrationId), query: { filter: `recurring_sales_invoice_id:${id}`, per_page: '100' } },
    })),
  })
}

function useSaved(administrationId: string) {
  const client = useQueryClient()
  return (saved?: { id?: string }) => {
    if (saved?.id) {
      client.setQueryData(['recurring-invoice', administrationId, saved.id], saved)
    }
    void client.invalidateQueries({ queryKey: ['recurring-invoices', administrationId] })
  }
}

/** Creates a recurring invoice when id is undefined and updates it otherwise. */
export function useSaveRecurringInvoice(administrationId: string, id?: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: (recurring_sales_invoice: RecurringChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/recurring_sales_invoices/{id}', { ...one(administrationId, id), body: { recurring_sales_invoice } }))
        : unwrap(api.POST('/api/v2/{administration_id}/recurring_sales_invoices', { params: { path: admin(administrationId) }, body: { recurring_sales_invoice } })),
    onSuccess: saved,
  })
}

export function useDeactivateRecurringInvoice(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: () => unwrap(api.PATCH('/api/v2/{administration_id}/recurring_sales_invoices/{id}/deactivate', one(administrationId, id))),
    onSuccess: saved,
  })
}

/** Deletes a recurring invoice; one that made invoices is deactivated instead. */
export function useDeleteRecurringInvoice(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/recurring_sales_invoices/{id}', one(administrationId, id))),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ['recurring-invoice', administrationId, id] })
      void client.invalidateQueries({ queryKey: ['recurring-invoices', administrationId] })
    },
  })
}

/** How often it makes an invoice, such as "Every month" or "Every 2 weeks". */
export function frequencyText(r: Pick<RecurringInvoice, 'frequency' | 'frequency_type'>, t: TFunction) {
  return t(`recurring.every.${r.frequency_type ?? 'month'}`, { count: r.frequency ?? 1 })
}

/** The name of a recurring invoice, as Moneybird's title: "Every month to Example Shop B.V.". */
export function recurringTitle(r: Pick<RecurringInvoice, 'frequency' | 'frequency_type'>, contact: string, t: TFunction) {
  return contact ? t('recurring.titleTo', { frequency: frequencyText(r, t), contact }) : frequencyText(r, t)
}

function useRefetchRecurring(administrationId: string, id: string) {
  const client = useQueryClient()
  return () => client.invalidateQueries({ queryKey: ['recurring-invoice', administrationId, id] })
}

export function useAddRecurringNote(administrationId: string, id: string) {
  const refetch = useRefetchRecurring(administrationId, id)
  return useMutation({
    mutationFn: (note: { note: string; todo: boolean; assignee_id?: string }) =>
      unwrap(api.POST('/api/v2/{administration_id}/recurring_sales_invoices/{id}/notes', { ...one(administrationId, id), body: { note } })),
    onSuccess: refetch,
  })
}

export function useDeleteRecurringNote(administrationId: string, id: string) {
  const refetch = useRefetchRecurring(administrationId, id)
  return useMutation({
    mutationFn: (noteId: string) =>
      unwrap(api.DELETE('/api/v2/{administration_id}/recurring_sales_invoices/{id}/notes/{note_id}', { params: { path: { ...admin(administrationId), id, note_id: noteId } } })),
    onSuccess: refetch,
  })
}

/**
 * What the sidebar tells of a recurring invoice: when it was made, as
 * Moneybird's "Recurring invoice created", and its notes, newest first.
 */
export function recurringHistory(r: RecurringInvoice): HistoryItem[] {
  const created: HistoryItem[] = r.created_at
    ? [{ kind: 'event', id: 'created', action: 'recurring_sales_invoice_created', at: r.created_at, timed: true, data: {} }]
    : []
  const notes: HistoryItem[] = (r.notes ?? []).map((n) => ({ kind: 'note', id: `note-${n.id}`, at: n.created_at ?? '', userId: n.user_id ?? undefined, timed: true, note: n }))
  return [...created, ...notes].sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime())
}
