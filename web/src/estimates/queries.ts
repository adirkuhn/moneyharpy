import type { TFunction } from 'i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'
import { perPage } from '../invoices/queries'

export type Estimate = components['schemas']['Estimate']
export type EstimateChanges = NonNullable<components['schemas']['EstimateRequest']['estimate']>
export type EstimateState = NonNullable<Estimate['state']>
export type EstimateSending = NonNullable<components['schemas']['SendEstimateRequest']['estimate_sending']>

export const estimateStates: EstimateState[] = ['draft', 'open', 'late', 'accepted', 'rejected', 'billed', 'archived']

/** The states an estimate can be moved to by hand from its state, as the API allows. */
export const transitions: Record<EstimateState, EstimateState[]> = {
  draft: [],
  open: ['accepted', 'rejected', 'archived'],
  late: ['accepted', 'rejected', 'archived'],
  accepted: ['open', 'rejected', 'archived'],
  rejected: ['open', 'accepted', 'archived'],
  billed: ['archived'],
  archived: ['open'],
}

/** Whether an estimate can be billed: it is open, late or accepted. */
export const isBillable = (state: string | undefined) => state === 'open' || state === 'late' || state === 'accepted'

/** Whether an estimate can still be changed: it is not billed or archived. */
export const isEditable = (state: string | undefined) => state !== 'billed' && state !== 'archived'

/** The list's filter: a state (empty for all) and a contact. */
export type EstimateFilter = { state: string; contactId: string }

export function estimateFilterOf(params: URLSearchParams): EstimateFilter {
  return { state: params.get('state') ?? '', contactId: params.get('contact_id') ?? '' }
}

/** The address of the list with a filter, leaving out what is empty. */
export function estimateQuery(f: EstimateFilter, params: URLSearchParams) {
  const next = new URLSearchParams(params)
  for (const [key, value] of [['state', f.state], ['contact_id', f.contactId]] as const) {
    if (value) {
      next.set(key, value)
    } else {
      next.delete(key)
    }
  }
  next.delete('page')
  return next
}

/** Moneybird's filter terms for a filter, such as "state:open,contact_id:1"; all estimates of any date without a state. */
export function estimateFilterTerms(f: EstimateFilter) {
  return [`state:${f.state || 'all'}`, f.contactId && `contact_id:${f.contactId}`].filter(Boolean).join(',')
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

function useSaved(administrationId: string) {
  const client = useQueryClient()
  return (saved?: { id?: string }) => {
    if (saved?.id) {
      client.setQueryData(['estimate', administrationId, saved.id], saved)
    }
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

export function useSendEstimate(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: (estimate_sending: EstimateSending) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/estimates/{id}/send_estimate', { ...one(administrationId, id), body: { estimate_sending } })),
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

/** Where the browser downloads the estimate's PDF; the session cookie signs the request. */
export function estimatePdfURL(administrationId: string, id: string) {
  return `/api/v2/${administrationId}/estimates/${id}/download_pdf`
}

/** What the estimate is called: its number once sent, "Draft quote N" before. */
export function estimateTitle(e: Pick<Estimate, 'estimate_id' | 'draft_id'>, t: TFunction) {
  return e.estimate_id || t('estimates.draftTitle', { id: e.draft_id ?? '' })
}
