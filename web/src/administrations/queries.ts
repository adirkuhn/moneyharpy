import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type Administration, api, type Language, unwrap } from '../api/client'

export function useAdministrations() {
  return useQuery({ queryKey: ['administrations'], queryFn: () => unwrap(api.GET('/api/v2/administrations')) })
}

export function useAdministration(id: string) {
  return useQuery({
    queryKey: ['administrations', id],
    queryFn: () => unwrap(api.GET('/api/v2/administrations/{id}', { params: { path: { id } } })),
  })
}

export function useSetFavorite() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ id, favorite }: Pick<Administration, 'id' | 'favorite'>) => {
      const options = { params: { path: { id } } }
      return unwrap(
        favorite
          ? api.PUT('/api/v2/administrations/{id}/favorite', options)
          : api.DELETE('/api/v2/administrations/{id}/favorite', options),
      )
    },
    onSettled: () => client.invalidateQueries({ queryKey: ['administrations'] }),
  })
}

export type NewAdministration = {
  name: string
  language: Language
  currency: string
  country: string
  time_zone: string
}

export function useCreateAdministration() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (administration: NewAdministration) =>
      unwrap(api.POST('/api/v2/administrations', { body: { administration } })),
    onSuccess: () => client.invalidateQueries({ queryKey: ['administrations'] }),
  })
}
