import type { TFunction } from 'i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type Product = components['schemas']['Product']
export type ProductChanges = components['schemas']['ProductRequest']['product']
export type ProductFrequencyType = NonNullable<Product['frequency_type']>

export const perPage = 25
export const productFrequencyTypes: ProductFrequencyType[] = ['day', 'week', 'month', 'quarter', 'year']

/** The list's search: text to find, and whether deactivated products show too. */
export type ProductSearch = { query: string; all: boolean; page: number }

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of products, active ones unless all are asked for, with the number found in all. */
export function useProducts(administrationId: string, search: ProductSearch) {
  return useQuery({
    queryKey: ['products', administrationId, search],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/products', {
        params: {
          path: admin(administrationId),
          query: {
            page: String(search.page), per_page: String(perPage),
            ...(search.query ? { query: search.query } : {}),
            ...(search.all ? { active: 'false' as const } : {}),
          },
        },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { products: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useProduct(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['product', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/products/{id}', one(administrationId, id ?? ''))),
  })
}

/** Creates a product when id is undefined and updates it otherwise. */
export function useSaveProduct(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (product: ProductChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/products/{id}', { ...one(administrationId, id), body: { product } }))
        : unwrap(api.POST('/api/v2/{administration_id}/products', { params: { path: admin(administrationId) }, body: { product } })),
    onSuccess: (saved) => {
      if (saved.id) {
        client.setQueryData(['product', administrationId, saved.id], saved)
      }
      void client.invalidateQueries({ queryKey: ['products', administrationId] })
    },
  })
}

/**
 * Deletes a product. One named on invoice lines is deactivated instead; the
 * result is then "deactivated" (the X-Kept-As header), and null when it is gone.
 */
export function useDeleteProduct(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { error, response } = await api.DELETE('/api/v2/{administration_id}/products/{id}', one(administrationId, id))
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return response.headers.get('X-Kept-As')
    },
    onSuccess: (kept) => {
      if (kept) {
        void client.invalidateQueries({ queryKey: ['product', administrationId, id] })
      } else {
        client.removeQueries({ queryKey: ['product', administrationId, id] })
      }
      void client.invalidateQueries({ queryKey: ['products', administrationId] })
    },
  })
}

/** What a product is called: its title, else its description. */
export function productName(p: Pick<Product, 'title' | 'description'>, t: TFunction) {
  return p.title || p.description || t('products.untitled')
}

/** How often a subscription product is invoiced, such as "Every month"; empty for a one-off product. */
export function productFrequency(p: Pick<Product, 'frequency' | 'frequency_type'>, t: TFunction) {
  return p.frequency_type ? t(`recurring.every.${p.frequency_type}`, { count: p.frequency ?? 1 }) : ''
}
