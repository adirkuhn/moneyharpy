import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'
import { fileBody, perPage } from '../invoices/queries'

export type DocumentSummary = components['schemas']['IncomingDocumentSummary']
export type DocumentType = DocumentSummary['type']
export type DocumentStatistics = components['schemas']['IncomingDocumentStatistics']

/** The types of the Type filter, as the filter names them, in Moneybird's order. */
export const filterTypes = ['purchase_invoice', 'receipt', 'general_journal_document', 'general_document'] as const
export type FilterType = (typeof filterTypes)[number]

/** The states of the Status filter, in Moneybird's order; late is shown as Expired. */
export const documentStates = ['new', 'saved', 'open', 'paid', 'pending_payment', 'late'] as const

/** The path segment of each type, as the API and the app name it. */
const segments: Record<DocumentType, string> = {
  PurchaseInvoice: 'purchase_invoices',
  Receipt: 'receipts',
  GeneralDocument: 'general_documents',
  GeneralJournalDocument: 'general_journal_documents',
  TypelessDocument: 'typeless_documents',
}

/** The types the app has a page of their own for. */
const typedPages: DocumentType[] = ['PurchaseInvoice', 'Receipt', 'GeneralJournalDocument']

/** The short label before a reference in the list, after Moneybird; an upload without a type has a dash. */
export const typeTags: Record<DocumentType, string> = {
  PurchaseInvoice: 'PUR',
  Receipt: 'REC',
  GeneralDocument: 'DOC',
  GeneralJournalDocument: 'MEM',
  TypelessDocument: '—',
}

/** Whether a document of this type books an amount payable, so its total means something. */
export const hasAmount = (type: DocumentType) => type === 'PurchaseInvoice' || type === 'Receipt'

/**
 * The page of a document of this type, or the generic /documents/:id where the
 * app has no page of the type's own yet.
 */
export function documentPath(administrationId: string, d: Pick<DocumentSummary, 'id' | 'type'>) {
  return typedPages.includes(d.type)
    ? `/${administrationId}/documents/${segments[d.type]}/${d.id}`
    : `/${administrationId}/documents/${d.id}`
}

/** Whether the app has a page of the type's own. */
export const hasTypedPage = (type: DocumentType) => typedPages.includes(type)

const admin = (administrationId: string) => ({ administration_id: administrationId })

export const documentsKey = (administrationId: string) => ['documents', administrationId]

/** One page of the incoming documents of every type, for a filter such as "period:this_year,type:receipt". */
export function useDocuments(administrationId: string, filter: string, page: number) {
  return useQuery({
    queryKey: [...documentsKey(administrationId), 'list', filter, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/documents', {
        params: { path: admin(administrationId), query: { filter, page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      const documents = data ?? []
      return { documents, total: Number(response.headers.get('X-Total-Count') ?? documents.length) }
    },
  })
}

/** The costs, the amount payable and the costs per category of the documents a filter selects. */
export function useDocumentStatistics(administrationId: string, filter: string, enabled: boolean) {
  return useQuery({
    queryKey: [...documentsKey(administrationId), 'statistics', filter],
    enabled,
    queryFn: () =>
      unwrap(api.GET('/api/v2/{administration_id}/documents/statistics', { params: { path: admin(administrationId), query: { filter } } })),
  })
}

/** An incoming document of any type, found by its id. */
export function useDocumentSummary(administrationId: string, id: string) {
  return useQuery({
    queryKey: [...documentsKey(administrationId), 'one', id],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/documents/{id}', { params: { path: { ...admin(administrationId), id } } })),
  })
}

const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** Makes a typeless document named after its first file and attaches the files; it answers with the document and the files refused. */
async function uploadAsOne(administrationId: string, files: File[]) {
  const made = await api.POST('/api/v2/{administration_id}/documents/typeless_documents', {
    params: { path: admin(administrationId) }, body: { typeless_document: { reference: files[0]?.name ?? '' } },
  })
  if (!made.response.ok || !made.data?.id) {
    throw new ApiError(made.response.status, made.error)
  }
  const id = made.data.id
  const refused: string[] = []
  for (const file of files) {
    const attached = await api.POST('/api/v2/{administration_id}/documents/typeless_documents/{id}/attachments', {
      ...one(administrationId, id), ...fileBody(file),
    })
    if (!attached.response.ok) {
      refused.push(file.name)
    }
  }
  if (refused.length === files.length) {
    await api.DELETE('/api/v2/{administration_id}/documents/typeless_documents/{id}', one(administrationId, id))
    return { id: undefined, refused }
  }
  return { id, refused }
}

/** Makes one document of every file, as the green bar of the Incoming page does; it answers with the files refused. */
export function useUploadEach(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (files: File[]) => {
      const refused: string[] = []
      for (const file of files) {
        try {
          refused.push(...(await uploadAsOne(administrationId, [file])).refused)
        } catch {
          refused.push(file.name)
        }
      }
      return { refused }
    },
    onSettled: () => client.invalidateQueries({ queryKey: documentsKey(administrationId) }),
  })
}

/** Makes one document of all the files, as the Add page does. */
export function useUploadTogether(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (files: File[]) => uploadAsOne(administrationId, files),
    onSettled: () => client.invalidateQueries({ queryKey: documentsKey(administrationId) }),
  })
}

/** The types that are made empty, without a document, from the Add page. */
export type EmptyType = 'purchase_invoice' | 'receipt' | 'general_document'

/** Makes an empty document of a type, dated today, to fill in on its page. */
export function useCreateEmpty(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (type: EmptyType) => {
      const date = new Date().toISOString().slice(0, 10)
      const path = { params: { path: admin(administrationId) } }
      if (type === 'purchase_invoice') {
        const made = await unwrap(api.POST('/api/v2/{administration_id}/documents/purchase_invoices', { ...path, body: { purchase_invoice: { date } } }))
        return { id: made.id ?? '', type: 'PurchaseInvoice' as const }
      }
      if (type === 'receipt') {
        const made = await unwrap(api.POST('/api/v2/{administration_id}/documents/receipts', { ...path, body: { receipt: { date } } }))
        return { id: made.id ?? '', type: 'Receipt' as const }
      }
      const made = await unwrap(api.POST('/api/v2/{administration_id}/documents/general_documents', { ...path, body: { general_document: { date } } }))
      return { id: made.id ?? '', type: 'GeneralDocument' as const }
    },
    onSuccess: () => client.invalidateQueries({ queryKey: documentsKey(administrationId) }),
  })
}

/** Deletes documents of any type; it answers with those that could not be deleted. */
export function useDeleteDocuments(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (documents: DocumentSummary[]) => {
      const failed: DocumentSummary[] = []
      for (const d of documents) {
        const target = one(administrationId, d.id)
        const call = {
          PurchaseInvoice: () => api.DELETE('/api/v2/{administration_id}/documents/purchase_invoices/{id}', target),
          Receipt: () => api.DELETE('/api/v2/{administration_id}/documents/receipts/{id}', target),
          GeneralDocument: () => api.DELETE('/api/v2/{administration_id}/documents/general_documents/{id}', target),
          GeneralJournalDocument: () => api.DELETE('/api/v2/{administration_id}/documents/general_journal_documents/{id}', target),
          TypelessDocument: () => api.DELETE('/api/v2/{administration_id}/documents/typeless_documents/{id}', target),
        }[d.type]
        const { response } = await call()
        if (!response.ok) {
          failed.push(d)
        }
      }
      return failed
    },
    onSettled: () => client.invalidateQueries({ queryKey: documentsKey(administrationId) }),
  })
}
