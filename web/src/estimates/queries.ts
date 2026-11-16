import type { TFunction } from 'i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'
import { fileBody, type ListFilter, perPage } from '../invoices/queries'

export type Estimate = components['schemas']['Estimate']
export type EstimateChanges = NonNullable<components['schemas']['EstimateRequest']['estimate']>
export type EstimateState = NonNullable<Estimate['state']>
export type EstimateSending = NonNullable<components['schemas']['SendEstimateRequest']['estimate_sending']>

export const estimateStates: EstimateState[] = ['draft', 'open', 'late', 'accepted', 'rejected', 'billed', 'archived']

/** The states change_state moves an estimate to from its state, as the API allows; a draft only leaves draft by being sent. */
export const transitions: Record<EstimateState, EstimateState[]> = {
  draft: [],
  open: ['late', 'accepted', 'rejected', 'billed', 'archived'],
  late: ['open', 'accepted', 'rejected', 'billed', 'archived'],
  accepted: ['open', 'rejected', 'billed', 'archived'],
  rejected: ['open', 'accepted', 'archived'],
  billed: ['open', 'accepted', 'archived'],
  archived: ['open', 'accepted', 'rejected', 'billed'],
}

/** Whether an estimate can be billed: it is open, late or accepted. */
export const isBillable = (state: string | undefined) => state === 'open' || state === 'late' || state === 'accepted'

/** Whether an estimate can still be changed: it is not billed or archived. */
export const isEditable = (state: string | undefined) => state !== 'billed' && state !== 'archived'

/** Moneybird's quick filters of the quotes: this year's, drafts, the ones not accepted yet, and accepted ones. */
export const quickFilters: Record<string, Pick<ListFilter, 'state' | 'period'>> = {
  this_year: { state: '', period: 'this_year' },
  draft: { state: 'draft', period: '' },
  not_accepted: { state: 'open|late', period: '' },
  accepted: { state: 'accepted', period: '' },
}

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of estimates, newest first. */
export function useEstimates(administrationId: string, filter: string, page: number) {
  return useQuery({
    queryKey: ['estimates', administrationId, filter, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/estimates', {
        params: { path: admin(administrationId), query: { filter, page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { estimates: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useEstimate(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['estimate', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/estimates/{id}', one(administrationId, id ?? ''))),
  })
}

/** The draft number a new estimate gets, which the add quote screen shows as "Concept #n". */
export function useNextEstimateDraftId(administrationId: string, enabled: boolean) {
  return useQuery({
    queryKey: ['estimate-next-draft-id', administrationId],
    enabled,
    // Another estimate may have taken it since, so it is asked for afresh every time.
    gcTime: 0,
    queryFn: async () => (await unwrap(api.GET('/api/v2/{administration_id}/estimates/next_draft_id', { params: { path: admin(administrationId) } }))).draft_id,
  })
}

function useSaved(administrationId: string) {
  const client = useQueryClient()
  return (saved?: { id?: string }) => {
    if (saved?.id) {
      client.setQueryData(['estimate', administrationId, saved.id], saved)
    }
    void client.invalidateQueries({ queryKey: ['estimates', administrationId] })
  }
}

function useRefetch(administrationId: string, id: string) {
  const client = useQueryClient()
  return () => {
    void client.invalidateQueries({ queryKey: ['estimate', administrationId, id] })
    void client.invalidateQueries({ queryKey: ['estimates', administrationId] })
  }
}

/** Creates an estimate when id is undefined and updates it otherwise. */
export function useSaveEstimate(administrationId: string, id?: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: (estimate: EstimateChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/estimates/{id}', { ...one(administrationId, id), body: { estimate } }))
        : unwrap(api.POST('/api/v2/{administration_id}/estimates', { params: { path: admin(administrationId) }, body: { estimate } })),
    onSuccess: saved,
  })
}

/** What a copy of an estimate is made with, as Moneybird's Duplicate quote: its contact, settings, texts and lines, naming the original. */
export function duplicateOf(e: Estimate): EstimateChanges {
  return {
    contact_id: e.contact_id ?? '',
    original_estimate_id: e.id ?? '',
    reference: e.reference ?? '',
    ...(e.workflow_id ? { workflow_id: e.workflow_id } : {}),
    ...(e.document_style_id ? { document_style_id: e.document_style_id } : {}),
    prices_are_incl_tax: e.prices_are_incl_tax ?? false,
    ...(Number(e.discount ?? 0) ? { discount: String(e.discount) } : {}),
    pre_text: e.pre_text ?? '',
    post_text: e.post_text ?? '',
    details_attributes: [...(e.details ?? [])]
      .sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
      .map((d, i) => ({
        description: d.description ?? '',
        amount: d.amount ?? '1',
        price: String(d.price ?? '0'),
        tax_rate_id: d.tax_rate_id ?? '',
        ...(d.ledger_account_id ? { ledger_account_id: d.ledger_account_id } : {}),
        ...(d.product_id ? { product_id: String(d.product_id) } : {}),
        ...(d.period ? { period: d.period } : {}),
        row_order: i + 1,
      })),
  }
}

export function useDuplicateEstimate(administrationId: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: (e: Estimate) => unwrap(api.POST('/api/v2/{administration_id}/estimates', { params: { path: admin(administrationId) }, body: { estimate: duplicateOf(e) } })),
    onSuccess: saved,
  })
}

export function useSendEstimate(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    // The sender's own signature goes beside the sending, where Moneybird's form puts it.
    mutationFn: ({ signature_output, ...estimate_sending }: EstimateSending & { signature_output?: string }) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/estimates/{id}/send_estimate', {
        ...one(administrationId, id), body: { estimate_sending, ...(signature_output ? { signature_output } : {}) },
      })),
    onSuccess: saved,
  })
}

export function useChangeEstimateState(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: (state: EstimateState) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/estimates/{id}/change_state', { ...one(administrationId, id), body: { state } })),
    onSuccess: saved,
  })
}

/** Bills the estimate: the API makes a draft sales invoice and answers with it. */
export function useBillEstimate(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.PATCH('/api/v2/{administration_id}/estimates/{id}/bill_estimate', { ...one(administrationId, id), body: {} })),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ['estimate', administrationId, id] })
      void client.invalidateQueries({ queryKey: ['estimates', administrationId] })
      void client.invalidateQueries({ queryKey: ['sales-invoices', administrationId] })
    },
  })
}

export function useDeleteEstimate(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/estimates/{id}', one(administrationId, id))),
    onSuccess: () => {
      client.removeQueries({ queryKey: ['estimate', administrationId, id] })
      void client.invalidateQueries({ queryKey: ['estimates', administrationId] })
    },
  })
}

export function useAddEstimateNote(administrationId: string, id: string) {
  const refetch = useRefetch(administrationId, id)
  return useMutation({
    mutationFn: (note: { note: string; todo: boolean; assignee_id?: string }) =>
      unwrap(api.POST('/api/v2/{administration_id}/estimates/{id}/notes', { ...one(administrationId, id), body: { note } })),
    onSuccess: refetch,
  })
}

export function useDeleteEstimateNote(administrationId: string, id: string) {
  const refetch = useRefetch(administrationId, id)
  return useMutation({
    mutationFn: (noteId: string) =>
      unwrap(api.DELETE('/api/v2/{administration_id}/estimates/{id}/notes/{note_id}', { params: { path: { ...admin(administrationId), id, note_id: noteId } } })),
    onSuccess: refetch,
  })
}

export function useAddEstimateAttachment(administrationId: string, id: string) {
  const refetch = useRefetch(administrationId, id)
  return useMutation({
    mutationFn: (file: File) => unwrap(api.POST('/api/v2/{administration_id}/estimates/{id}/attachments', { ...one(administrationId, id), ...fileBody(file) })),
    onSuccess: refetch,
  })
}

export function useDeleteEstimateAttachment(administrationId: string, id: string) {
  const refetch = useRefetch(administrationId, id)
  return useMutation({
    mutationFn: (attachmentId: string) =>
      unwrap(api.DELETE('/api/v2/{administration_id}/estimates/{id}/attachments/{attachment_id}', {
        params: { path: { ...admin(administrationId), id, attachment_id: attachmentId } },
      })),
    onSuccess: refetch,
  })
}

/** Where the browser downloads the estimate's PDF; the session cookie signs the request. */
export function estimatePdfURL(administrationId: string, id: string) {
  return `/api/v2/${administrationId}/estimates/${id}/download_pdf`
}

/** Where the browser downloads the quote its contact signed online. */
export function signedEstimatePdfURL(administrationId: string, id: string) {
  return `/api/v2/${administrationId}/estimates/${id}/download_signed_pdf`
}

export function estimateAttachmentURL(administrationId: string, id: string, attachmentId: string) {
  return `/api/v2/${administrationId}/estimates/${id}/attachments/${attachmentId}/download`
}

/** What the estimate is called: its number once sent, "Draft quote N" before. */
export function estimateTitle(e: Pick<Estimate, 'estimate_id' | 'draft_id'>, t: TFunction) {
  return e.estimate_id || t('estimates.draftTitle', { id: e.draft_id ?? '' })
}
