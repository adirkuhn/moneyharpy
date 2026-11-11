import type { TFunction } from 'i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'
import { fileBody, perPage } from '../invoices/queries'

export type PurchaseInvoice = components['schemas']['PurchaseInvoice']
export type Receipt = components['schemas']['Receipt']
export type IncomingDocument = PurchaseInvoice | Receipt
export type TypelessDocument = components['schemas']['TypelessDocument']

/** Purchase invoices and receipts, as their paths name them. */
export type IncomingKind = 'purchase_invoices' | 'receipts'

export const incomingStates = ['new', 'open', 'late', 'pending_payment', 'paid'] as const
export const unpaidStates = 'open|late|pending_payment'

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of purchase invoices or receipts for a filter such as "state:open,period:this_year". */
export function useIncomingDocuments(administrationId: string, kind: IncomingKind, filter: string, page: number) {
  return useQuery({
    queryKey: ['incoming', kind, administrationId, filter, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const options = { params: { path: admin(administrationId), query: { filter, page: String(page), per_page: String(perPage) } } }
      const { data, error, response } = kind === 'receipts'
        ? await api.GET('/api/v2/{administration_id}/documents/receipts', options)
        : await api.GET('/api/v2/{administration_id}/documents/purchase_invoices', options)
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      const documents: IncomingDocument[] = data ?? []
      return { documents, total: Number(response.headers.get('X-Total-Count') ?? documents.length) }
    },
  })
}

export function useIncomingDocument(administrationId: string, kind: IncomingKind, id: string) {
  return useQuery({
    queryKey: ['incoming-document', kind, administrationId, id],
    queryFn: (): Promise<IncomingDocument> =>
      kind === 'receipts'
        ? unwrap(api.GET('/api/v2/{administration_id}/documents/receipts/{id}', one(administrationId, id)))
        : unwrap(api.GET('/api/v2/{administration_id}/documents/purchase_invoices/{id}', one(administrationId, id))),
  })
}

export function useDeleteIncomingDocument(administrationId: string, kind: IncomingKind, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () =>
      kind === 'receipts'
        ? unwrap(api.DELETE('/api/v2/{administration_id}/documents/receipts/{id}', one(administrationId, id)))
        : unwrap(api.DELETE('/api/v2/{administration_id}/documents/purchase_invoices/{id}', one(administrationId, id))),
    onSuccess: () => client.invalidateQueries({ queryKey: ['incoming', kind, administrationId] }),
  })
}

/** The uploads still to process, newest first. */
export function useTypelessDocuments(administrationId: string, page: number) {
  return useQuery({
    queryKey: ['typeless', administrationId, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/documents/typeless_documents', {
        params: { path: admin(administrationId), query: { filter: 'state:all', page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { documents: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

/** Makes a typeless document of every file and attaches the file; it answers with the names of files refused. */
export function useUploadDocuments(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (files: File[]) => {
      const refused: string[] = []
      for (const file of files) {
        const made = await api.POST('/api/v2/{administration_id}/documents/typeless_documents', {
          params: { path: admin(administrationId) }, body: { typeless_document: { reference: file.name } },
        })
        const id = made.data?.id
        if (!made.response.ok || !id) {
          refused.push(file.name)
          continue
        }
        const attached = await api.POST('/api/v2/{administration_id}/documents/typeless_documents/{id}/attachments', {
          ...one(administrationId, id), ...fileBody(file),
        })
        if (!attached.response.ok) {
          refused.push(file.name)
          await api.DELETE('/api/v2/{administration_id}/documents/typeless_documents/{id}', one(administrationId, id))
        }
      }
      return { refused }
    },
    onSettled: () => client.invalidateQueries({ queryKey: ['typeless', administrationId] }),
  })
}

export function useDeleteTypeless(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/v2/{administration_id}/documents/typeless_documents/{id}', one(administrationId, id))),
    onSuccess: () => client.invalidateQueries({ queryKey: ['typeless', administrationId] }),
  })
}

/** Where an attachment of an incoming document downloads from. */
export function incomingAttachmentURL(administrationId: string, kind: IncomingKind | 'typeless_documents', id: string, attachmentId: string) {
  return `/api/v2/${administrationId}/documents/${kind}/${id}/attachments/${attachmentId}/download`
}

/** What the document is called: its reference, or its entry number when it has none. */
export function incomingTitle(d: Pick<IncomingDocument, 'reference' | 'entry_number'>, kind: IncomingKind, t: TFunction) {
  return d.reference || t(`purchases.untitled.${kind}`, { n: d.entry_number ?? '' })
}
