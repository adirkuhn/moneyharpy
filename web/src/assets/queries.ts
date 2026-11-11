import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type Asset = components['schemas']['Asset']

export const perPage = 25
export const assetViews = ['active', 'all'] as const
export type AssetView = (typeof assetViews)[number]

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
