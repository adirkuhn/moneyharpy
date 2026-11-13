import type { TFunction } from 'i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../api/client'
import type { components, operations } from '../api/schema'
import { fileBody } from '../invoices/queries'
import type { JournalDocument } from '../journal/queries'
import { documentsKey, polling } from './documents'
import type { Recognition } from './recognition'

export type PurchaseInvoice = components['schemas']['PurchaseInvoice']
export type Receipt = components['schemas']['Receipt']
export type IncomingDocument = PurchaseInvoice | Receipt
export type GeneralDocument = components['schemas']['GeneralDocument']
export type TypelessDocument = components['schemas']['TypelessDocument']
export type PurchaseChanges = components['schemas']['PurchaseDocumentInput']
export type GeneralDocumentChanges = components['schemas']['GeneralDocumentRequest']['general_document']
type Summary = components['schemas']['IncomingDocumentSummary']

/** The type of an incoming document, as the API names it. */
export type DocumentType = Summary['type']

/** The types a document can be given, as set_type and the filters name them. */
export type TypeChoice = operations['setIncomingDocumentType']['requestBody']['content']['application/json']['document_type']

/** The types of the Type picker, in Moneybird's order. */
export const typeChoices: TypeChoice[] = ['purchase_invoice', 'receipt', 'general_document', 'general_journal_document']

/** The choice of each type; an upload has none yet. */
export const choiceOf: Record<DocumentType, TypeChoice | null> = {
  PurchaseInvoice: 'purchase_invoice',
  Receipt: 'receipt',
  GeneralDocument: 'general_document',
  GeneralJournalDocument: 'general_journal_document',
  TypelessDocument: null,
}

/** Purchase invoices and receipts, as their paths name them. */
export type IncomingKind = 'purchase_invoices' | 'receipts'

/** The path segment of each type, as the API names it. */
export const segments = {
  PurchaseInvoice: 'purchase_invoices',
  Receipt: 'receipts',
  GeneralDocument: 'general_documents',
  GeneralJournalDocument: 'general_journal_documents',
  TypelessDocument: 'typeless_documents',
} as const satisfies Record<DocumentType, string>

/** A document of any type with the type it is, so its fields can be told apart. */
export type TypedDocument =
  | { type: 'PurchaseInvoice'; doc: PurchaseInvoice }
  | { type: 'Receipt'; doc: Receipt }
  | { type: 'GeneralDocument'; doc: GeneralDocument }
  | { type: 'GeneralJournalDocument'; doc: JournalDocument }
  | { type: 'TypelessDocument'; doc: TypelessDocument }

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** The queries of one incoming document, whatever its type. */
export const incomingKey = (administrationId: string, id: string) => ['incoming-document', administrationId, id]

/** One incoming document of a known type. */
export function useTypedDocument(administrationId: string, type: DocumentType | undefined, id: string) {
  return useQuery({
    queryKey: [...incomingKey(administrationId, id), type],
    enabled: type !== undefined,
    // While Scan and Recognise reads the document it is asked for again, until what was read is filled in.
    refetchInterval: (query) => (query.state.data && recognitionOf(query.state.data)?.state === 'pending' ? polling.intervalMs : false),
    queryFn: async (): Promise<TypedDocument> => {
      const target = one(administrationId, id)
      switch (type) {
        case 'PurchaseInvoice':
          return { type, doc: await unwrap(api.GET('/api/v2/{administration_id}/documents/purchase_invoices/{id}', target)) }
        case 'Receipt':
          return { type, doc: await unwrap(api.GET('/api/v2/{administration_id}/documents/receipts/{id}', target)) }
        case 'GeneralDocument':
          return { type, doc: await unwrap(api.GET('/api/v2/{administration_id}/documents/general_documents/{id}', target)) }
        case 'GeneralJournalDocument':
          return { type, doc: await unwrap(api.GET('/api/v2/{administration_id}/documents/general_journal_documents/{id}', target)) }
        default:
          return { type: 'TypelessDocument', doc: await unwrap(api.GET('/api/v2/{administration_id}/documents/typeless_documents/{id}', target)) }
      }
    },
  })
}

/** What Scan and Recognise read of a document, for the types it reads. */
export function recognitionOf({ doc }: TypedDocument): Recognition | undefined {
  return 'recognition' in doc ? doc.recognition : undefined
}

/** Reloads the document and the lists it is on. */
function useRefresh(administrationId: string, id: string) {
  const client = useQueryClient()
  return () =>
    Promise.all([
      client.invalidateQueries({ queryKey: incomingKey(administrationId, id) }),
      client.invalidateQueries({ queryKey: documentsKey(administrationId) }),
    ])
}

/** Reads the document's attachments again with Scan and Recognise, in the background. */
export function useRecognize(administrationId: string, id: string) {
  const refresh = useRefresh(administrationId, id)
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/v2/{administration_id}/documents/{id}/recognize', one(administrationId, id))),
    onSuccess: refresh,
  })
}

/** Gives the document another type, as Moneybird's type picker; it keeps its id, files and notes. */
export function useSetDocumentType(administrationId: string, id: string) {
  const refresh = useRefresh(administrationId, id)
  return useMutation({
    mutationFn: (document_type: TypeChoice) =>
      unwrap(api.POST('/api/v2/{administration_id}/documents/{id}/set_type', { ...one(administrationId, id), body: { document_type } })),
    onSuccess: refresh,
  })
}

/** Saves a purchase invoice or receipt; one with its lines (and a supplier, for an invoice) is booked. */
export function useSavePurchase(administrationId: string, kind: IncomingKind, id: string) {
  const refresh = useRefresh(administrationId, id)
  return useMutation({
    mutationFn: (changes: PurchaseChanges) =>
      kind === 'receipts'
        ? unwrap(api.PATCH('/api/v2/{administration_id}/documents/receipts/{id}', { ...one(administrationId, id), body: { receipt: changes } }))
        : unwrap(api.PATCH('/api/v2/{administration_id}/documents/purchase_invoices/{id}', { ...one(administrationId, id), body: { purchase_invoice: changes } })),
    onSuccess: refresh,
  })
}

/** Saves a general document; without a contact the contact it had is removed. */
export function useSaveGeneralDocument(administrationId: string, id: string) {
  const refresh = useRefresh(administrationId, id)
  return useMutation({
    mutationFn: ({ removeContact, ...general_document }: GeneralDocumentChanges & { removeContact?: boolean }) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/documents/general_documents/{id}', {
        ...one(administrationId, id), body: { general_document, ...(removeContact ? { remove_contact: true } : {}) },
      })),
    onSuccess: refresh,
  })
}

/**
 * The paths every type has at the same place, with the same body: they are
 * written as those of receipts for the types and filled in with the type's own
 * segment, which openapi-fetch keeps as it is.
 */
const typed = <P extends string>(type: DocumentType, path: P) => path.replace('/receipts/', `/${segments[type]}/`) as P

export function useDeleteDocument(administrationId: string, type: DocumentType, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE(typed(type, '/api/v2/{administration_id}/documents/receipts/{id}'), one(administrationId, id))),
    onSuccess: () => {
      // The document is gone: forget it rather than ask for it again, which would show the not found page.
      const gone = [...documentsKey(administrationId), 'one', id]
      client.removeQueries({ queryKey: gone })
      client.removeQueries({ queryKey: incomingKey(administrationId, id) })
      return client.invalidateQueries({ queryKey: documentsKey(administrationId) })
    },
  })
}

/** Adds a file to the document. */
export function useAddAttachment(administrationId: string, type: DocumentType, id: string) {
  const refresh = useRefresh(administrationId, id)
  return useMutation({
    mutationFn: (file: File) =>
      unwrap(api.POST(typed(type, '/api/v2/{administration_id}/documents/receipts/{id}/attachments'), { ...one(administrationId, id), ...fileBody(file) })),
    onSettled: refresh,
  })
}

export function useDeleteAttachment(administrationId: string, type: DocumentType, id: string) {
  const refresh = useRefresh(administrationId, id)
  return useMutation({
    mutationFn: (attachmentId: string) =>
      unwrap(api.DELETE(typed(type, '/api/v2/{administration_id}/documents/receipts/{id}/attachments/{attachment_id}'), {
        params: { path: { ...admin(administrationId), id, attachment_id: attachmentId } },
      })),
    onSuccess: refresh,
  })
}

type NoteChange = { note: string; todo: boolean; assignee_id?: string }

/** Adds a note or to-do to a document of a type that keeps them: every type but an upload without one. */
export function useAddDocumentNote(administrationId: string, type: DocumentType, id: string) {
  const refresh = useRefresh(administrationId, id)
  return useMutation({
    mutationFn: (note: NoteChange) =>
      unwrap(api.POST(typed(type, '/api/v2/{administration_id}/documents/receipts/{id}/notes'), { ...one(administrationId, id), body: { note } })),
    onSuccess: refresh,
  })
}

export function useDeleteDocumentNote(administrationId: string, type: DocumentType, id: string) {
  const refresh = useRefresh(administrationId, id)
  return useMutation({
    mutationFn: (noteId: string) =>
      unwrap(api.DELETE(typed(type, '/api/v2/{administration_id}/documents/receipts/{id}/notes/{note_id}'), {
        params: { path: { ...admin(administrationId), id, note_id: noteId } },
      })),
    onSuccess: refresh,
  })
}

/** Where an attachment of an incoming document downloads from. */
export function attachmentURL(administrationId: string, type: DocumentType, id: string, attachmentId: string) {
  return `/api/v2/${administrationId}/documents/${segments[type]}/${id}/attachments/${attachmentId}/download`
}

/** Where a page of an incoming document's attachment previews from, as an image. */
export function previewURL(administrationId: string, id: string, attachmentId: string, page: number) {
  return `/api/v2/${administrationId}/documents/${id}/attachments/${attachmentId}/preview?page=${page}`
}

/** What a purchase invoice or receipt is called: its reference, or its entry number when it has none. */
export function incomingTitle(d: Pick<IncomingDocument, 'reference' | 'entry_number'>, kind: IncomingKind, t: TFunction) {
  return d.reference || t(`purchases.untitled.${kind}`, { n: d.entry_number ?? '' })
}
