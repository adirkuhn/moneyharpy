import type { TFunction } from 'i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'
import { fileBody, perPage, useSaved } from '../invoices/queries'

export type ExternalInvoice = components['schemas']['ExternalSalesInvoice']
export type ExternalChanges = NonNullable<components['schemas']['ExternalSalesInvoiceRequest']['external_sales_invoice']>
export type ExternalState = NonNullable<ExternalInvoice['state']>

export const externalStates: ExternalState[] = ['new', 'open', 'late', 'paid', 'uncollectible']
export const unpaidExternalStates = 'open|late'

/** An external invoice changes until it takes payments or is marked dubious; paid and uncollectible ones are finished. */
export function isEditable(inv: ExternalInvoice) {
  return (inv.state === 'new' || inv.state === 'open' || inv.state === 'late') && (inv.payments ?? []).length === 0 && !inv.marked_dubious_on
}

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of external invoices for a filter such as "state:open,period:this_year". */
export function useExternalInvoices(administrationId: string, filter: string, page: number) {
  return useQuery({
    queryKey: ['external-invoices', administrationId, filter, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/external_sales_invoices', {
        params: { path: admin(administrationId), query: { filter, page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { invoices: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useExternalInvoice(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['external-invoice', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/external_sales_invoices/{id}', one(administrationId, id ?? ''))),
  })
}

/** Creates an invoice when id is undefined and updates it otherwise. */
export function useSaveExternalInvoice(administrationId: string, id?: string) {
  const saved = useSaved(administrationId, 'external')
  return useMutation({
    mutationFn: (external_sales_invoice: ExternalChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/external_sales_invoices/{id}', { ...one(administrationId, id), body: { external_sales_invoice } }))
        : unwrap(api.POST('/api/v2/{administration_id}/external_sales_invoices', { params: { path: admin(administrationId) }, body: { external_sales_invoice } })),
    onSuccess: saved,
  })
}

export function useDeleteExternalInvoice(administrationId: string, id: string) {
  const saved = useSaved(administrationId, 'external')
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/external_sales_invoices/{id}', one(administrationId, id))),
    onSuccess: () => saved(),
  })
}

/** Makes a new invoice of every file, one request each; it answers with those made and the names of files refused. */
export function useUploadExternalInvoices(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (files: File[]) => {
      const made: ExternalInvoice[] = []
      const refused: string[] = []
      for (const file of files) {
        const { data, response } = await api.POST('/api/v2/{administration_id}/external_sales_invoices/attachment', {
          params: { path: admin(administrationId) }, ...fileBody(file),
        })
        if (response.ok && data) {
          made.push(data)
        } else {
          refused.push(file.name)
        }
      }
      return { made, refused }
    },
    onSettled: () => client.invalidateQueries({ queryKey: ['external-invoices', administrationId] }),
  })
}

/** What the invoice is called: its reference, or its entry number when it has none. */
export function externalTitle(inv: Pick<ExternalInvoice, 'reference' | 'entry_number'>, t: TFunction) {
  return inv.reference || t('external.untitled', { n: inv.entry_number ?? '' })
}
