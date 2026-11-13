import type { TFunction } from 'i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type PurchaseInvoice = components['schemas']['PurchaseInvoice']
export type Receipt = components['schemas']['Receipt']
export type IncomingDocument = PurchaseInvoice | Receipt

/** Purchase invoices and receipts, as their paths name them. */
export type IncomingKind = 'purchase_invoices' | 'receipts'

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

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
    onSuccess: () => client.invalidateQueries({ queryKey: ['documents', administrationId] }),
  })
}

/** Where an attachment of an incoming document downloads from. */
export function incomingAttachmentURL(administrationId: string, kind: IncomingKind, id: string, attachmentId: string) {
  return `/api/v2/${administrationId}/documents/${kind}/${id}/attachments/${attachmentId}/download`
}

/** What the document is called: its reference, or its entry number when it has none. */
export function incomingTitle(d: Pick<IncomingDocument, 'reference' | 'entry_number'>, kind: IncomingKind, t: TFunction) {
  return d.reference || t(`purchases.untitled.${kind}`, { n: d.entry_number ?? '' })
}
