import type { TFunction } from 'i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type SalesInvoice = components['schemas']['SalesInvoice']
export type SalesInvoiceChanges = components['schemas']['SalesInvoiceRequest']['sales_invoice']
export type Sending = NonNullable<components['schemas']['SendSalesInvoiceRequest']['sales_invoice_sending']>
export type InvoiceState = NonNullable<SalesInvoice['state']>

export const perPage = 50

export const states: InvoiceState[] = ['draft', 'scheduled', 'open', 'pending_payment', 'late', 'reminded', 'paid', 'uncollectible']
export const unpaidStates = 'open|late|reminded|pending_payment'

/** Sent invoices have a number and are booked. */
export const isSent = (state: InvoiceState | undefined) => state !== 'draft' && state !== 'scheduled'
/** Paid and uncollectible invoices are finished and no longer change. */
export const isClosed = (state: InvoiceState | undefined) => state === 'paid' || state === 'uncollectible'

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of invoices for a filter such as "state:draft,period:this_year". */
export function useSalesInvoices(administrationId: string, filter: string, page: number) {
  return useQuery({
    queryKey: ['sales-invoices', administrationId, filter, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/sales_invoices', {
        params: { path: admin(administrationId), query: { filter, page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { invoices: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useSalesInvoice(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['sales-invoice', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/sales_invoices/{id}', one(administrationId, id ?? ''))),
  })
}

/** Keeps the invoice the server answered with and has lists load again. */
function useSaved(administrationId: string) {
  const client = useQueryClient()
  return (saved?: SalesInvoice) => {
    if (saved?.id) {
      client.setQueryData(['sales-invoice', administrationId, saved.id], saved)
    }
    void client.invalidateQueries({ queryKey: ['sales-invoices', administrationId] })
  }
}

function useRefetch(administrationId: string, id: string) {
  const client = useQueryClient()
  return () => {
    void client.invalidateQueries({ queryKey: ['sales-invoice', administrationId, id] })
    void client.invalidateQueries({ queryKey: ['sales-invoices', administrationId] })
  }
}

/** Creates a draft when id is undefined and updates the invoice otherwise. */
export function useSaveSalesInvoice(administrationId: string, id?: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: (sales_invoice: SalesInvoiceChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/sales_invoices/{id}', { ...one(administrationId, id), body: { sales_invoice } }))
        : unwrap(api.POST('/api/v2/{administration_id}/sales_invoices', { params: { path: admin(administrationId) }, body: { sales_invoice } })),
    onSuccess: saved,
  })
}

export function useDeleteSalesInvoice(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/sales_invoices/{id}', one(administrationId, id))),
    onSuccess: () => saved(),
  })
}

export function useSendSalesInvoice(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  const refetch = useRefetch(administrationId, id)
  return useMutation({
    mutationFn: (sales_invoice_sending: Sending) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/sales_invoices/{id}/send_invoice', { ...one(administrationId, id), body: { sales_invoice_sending } })),
    onSuccess: saved,
    // A failed email still books the invoice, so it is loaded again.
    onError: refetch,
  })
}

export function useCreditSalesInvoice(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: () => unwrap(api.PATCH('/api/v2/{administration_id}/sales_invoices/{id}/duplicate_creditinvoice', { ...one(administrationId, id), body: {} })),
    onSuccess: saved,
  })
}

export function usePauseSalesInvoice(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: (pause: boolean) =>
      pause
        ? unwrap(api.POST('/api/v2/{administration_id}/sales_invoices/{id}/pause', one(administrationId, id)))
        : unwrap(api.POST('/api/v2/{administration_id}/sales_invoices/{id}/resume', one(administrationId, id))),
    onSuccess: saved,
  })
}

export function useAddInvoiceNote(administrationId: string, id: string) {
  const refetch = useRefetch(administrationId, id)
  return useMutation({
    mutationFn: (note: { note: string; todo: boolean }) =>
      unwrap(api.POST('/api/v2/{administration_id}/sales_invoices/{id}/notes', { ...one(administrationId, id), body: { note } })),
    onSuccess: refetch,
  })
}

export function useDeleteInvoiceNote(administrationId: string, id: string) {
  const refetch = useRefetch(administrationId, id)
  return useMutation({
    mutationFn: (noteId: string) =>
      unwrap(api.DELETE('/api/v2/{administration_id}/sales_invoices/{id}/notes/{note_id}', { params: { path: { ...admin(administrationId), id, note_id: noteId } } })),
    onSuccess: refetch,
  })
}

/** Uploads a file as multipart/form-data in the field "file". */
export function useAddInvoiceAttachment(administrationId: string, id: string) {
  const refetch = useRefetch(administrationId, id)
  return useMutation({
    mutationFn: (file: File) => {
      const form = new FormData()
      form.append('file', file)
      return unwrap(
        api.POST('/api/v2/{administration_id}/sales_invoices/{id}/attachments', {
          ...one(administrationId, id),
          body: form as unknown as { file: string },
          bodySerializer: (body: unknown) => body as FormData,
        }),
      )
    },
    onSuccess: refetch,
  })
}

export function useDeleteInvoiceAttachment(administrationId: string, id: string) {
  const refetch = useRefetch(administrationId, id)
  return useMutation({
    mutationFn: (attachmentId: string) =>
      unwrap(api.DELETE('/api/v2/{administration_id}/sales_invoices/{id}/attachments/{attachment_id}', { params: { path: { ...admin(administrationId), id, attachment_id: attachmentId } } })),
    onSuccess: refetch,
  })
}

/** Where the browser downloads one of the invoice's files; the session cookie signs the request. */
export function invoiceFileURL(administrationId: string, id: string, file: 'download_pdf' | 'download_ubl' | 'download_packing_slip_pdf') {
  return `/api/v2/${administrationId}/sales_invoices/${id}/${file}`
}

export function attachmentURL(administrationId: string, id: string, attachmentId: string) {
  return `/api/v2/${administrationId}/sales_invoices/${id}/attachments/${attachmentId}/download`
}

/** What the invoice is called: its number once sent, "Draft N" before. */
export function invoiceTitle(invoice: Pick<SalesInvoice, 'invoice_id' | 'draft_id'>, t: TFunction) {
  return invoice.invoice_id || t('invoices.draftTitle', { id: invoice.draft_id ?? '' })
}

/** The filter of the list kept in the address: a state, a period and other terms as they are. */
export type ListFilter = { state: string; period: string; rest: string[] }

export function parseFilter(filter: string | null): ListFilter {
  if (filter === null) {
    return { state: '', period: 'this_year', rest: [] }
  }
  const out: ListFilter = { state: '', period: '', rest: [] }
  for (const term of filter.split(',').map((s) => s.trim()).filter(Boolean)) {
    const [key, value = ''] = term.split(/:(.*)/s)
    if (key === 'state' && value !== 'all') {
      out.state = value
    } else if (key === 'period') {
      out.period = value
    } else if (key !== 'state') {
      out.rest.push(term)
    }
  }
  return out
}

/** The filter the API takes; without state or period it asks for everything. */
export function filterTerms({ state, period, rest }: ListFilter) {
  const terms = [state && `state:${state}`, period && `period:${period}`, ...rest].filter(Boolean)
  return terms.length > 0 ? terms.join(',') : 'state:all'
}
