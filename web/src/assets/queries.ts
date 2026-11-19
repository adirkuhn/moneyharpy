import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type Asset = components['schemas']['Asset']
export type AssetInput = components['schemas']['AssetInput']

export const perPage = 25
export type AssetView = 'active' | 'all'

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of assets, the active ones or all, with the number found in all. */
export function useAssets(administrationId: string, view: AssetView, page: number) {
  return useQuery({
    queryKey: ['assets', administrationId, view, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/assets', {
        params: { path: admin(administrationId), query: { active: view === 'active' ? 'true' : 'false', page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { assets: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useAsset(administrationId: string, id: string) {
  return useQuery({
    queryKey: ['asset', administrationId, id],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/assets/{id}', one(administrationId, id))),
  })
}

export function useDeleteAsset(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/assets/{id}', one(administrationId, id))),
    onSuccess: () => client.invalidateQueries({ queryKey: ['assets', administrationId] }),
  })
}

/** The number of assets, the active ones or all, read from one short page. */
export function useAssetCount(administrationId: string, view: AssetView) {
  return useQuery({
    queryKey: ['assets', administrationId, view, 'count'],
    queryFn: async () => {
      const { error, response } = await api.GET('/api/v2/{administration_id}/assets', {
        params: { path: admin(administrationId), query: { active: view === 'active' ? 'true' : 'false', per_page: '1' } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return Number(response.headers.get('X-Total-Count') ?? 0)
    },
  })
}

/** Creates an asset when id is undefined and updates it otherwise. */
export function useSaveAsset(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (asset: AssetInput) => id === undefined
      ? unwrap(api.POST('/api/v2/{administration_id}/assets', { params: { path: admin(administrationId) }, body: { asset } as components['schemas']['AssetRequest'] }))
      : unwrap(api.PATCH('/api/v2/{administration_id}/assets/{id}', { ...one(administrationId, id), body: { asset } })),
    onSuccess: (saved) => {
      void client.invalidateQueries({ queryKey: ['assets', administrationId] })
      client.setQueryData(['asset', administrationId, String(saved.id)], saved)
    },
  })
}

export type AssetBooking = components['schemas']['AssetBooking']

/** Refreshes the asset and the lists it is in. */
function useRefreshAsset(administrationId: string, id: string) {
  const client = useQueryClient()
  return () => Promise.all([
    client.invalidateQueries({ queryKey: ['asset', administrationId, id] }),
    client.invalidateQueries({ queryKey: ['assets', administrationId] }),
    client.invalidateQueries({ queryKey: ['asset-linkable', administrationId, id] }),
  ])
}

export function useAddAssetNote(administrationId: string, id: string) {
  const refresh = useRefreshAsset(administrationId, id)
  return useMutation({
    mutationFn: (note: { note: string; todo: boolean; assignee_id?: string }) =>
      unwrap(api.POST('/api/v2/{administration_id}/assets/{id}/notes', { ...one(administrationId, id), body: { note } })),
    onSuccess: refresh,
  })
}

export function useDeleteAssetNote(administrationId: string, id: string) {
  const refresh = useRefreshAsset(administrationId, id)
  return useMutation({
    mutationFn: (noteId: string) => unwrap(api.DELETE('/api/v2/{administration_id}/assets/{id}/notes/{note_id}', {
      params: { path: { ...admin(administrationId), id, note_id: noteId } },
    })),
    onSuccess: refresh,
  })
}

/** The bookings on the asset's category that no asset links yet, once asked for. */
export function useLinkableBookings(administrationId: string, id: string, enabled: boolean) {
  return useQuery({
    queryKey: ['asset-linkable', administrationId, id],
    enabled,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/assets/{id}/linkable_bookings', one(administrationId, id))),
  })
}

export function useLinkSource(administrationId: string, id: string) {
  const refresh = useRefreshAsset(administrationId, id)
  return useMutation({
    mutationFn: (b: AssetBooking) => unwrap(api.POST('/api/v2/{administration_id}/assets/{id}/sources', {
      ...one(administrationId, id),
      body: b.detail_id ? { detail_id: b.detail_id } : { general_journal_document_entry_id: b.general_journal_document_entry_id },
    })),
    onSuccess: refresh,
  })
}

export function useUnlinkSource(administrationId: string, id: string) {
  const refresh = useRefreshAsset(administrationId, id)
  return useMutation({
    mutationFn: (sourceId: string) => unwrap(api.DELETE('/api/v2/{administration_id}/assets/{id}/sources/{source_id}', {
      params: { path: { ...admin(administrationId), id, source_id: sourceId } },
    })),
    onSuccess: refresh,
  })
}
