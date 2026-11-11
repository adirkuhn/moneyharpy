import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type Project = components['schemas']['Project']
export type ProjectChanges = components['schemas']['ProjectRequest']['project']

export const perPage = 25
export const projectStates = ['active', 'archived', 'all'] as const
export type ProjectState = (typeof projectStates)[number]

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of projects in a state, with the number found in all. */
export function useProjects(administrationId: string, state: ProjectState, page: number) {
  return useQuery({
    queryKey: ['projects', administrationId, state, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/projects', {
        params: { path: admin(administrationId), query: { filter: `state:${state}`, page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { projects: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useProject(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['project', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/projects/{id}', one(administrationId, id ?? ''))),
  })
}

/** Creates a project when id is undefined and updates it otherwise, its state included. */
export function useSaveProject(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (project: ProjectChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/projects/{id}', { ...one(administrationId, id), body: { project } }))
        : unwrap(api.POST('/api/v2/{administration_id}/projects', { params: { path: admin(administrationId) }, body: { project } })),
    onSuccess: (saved) => {
      if (saved.id) {
        client.setQueryData(['project', administrationId, saved.id], saved)
      }
      void client.invalidateQueries({ queryKey: ['projects', administrationId] })
    },
  })
}

/**
 * Deletes a project. One with something booked on it is archived instead; the
 * result is then "archived" (the X-Kept-As header), and null when it is gone.
 */
export function useDeleteProject(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { error, response } = await api.DELETE('/api/v2/{administration_id}/projects/{id}', one(administrationId, id))
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return response.headers.get('X-Kept-As')
    },
    onSuccess: (kept) => {
      if (kept) {
        void client.invalidateQueries({ queryKey: ['project', administrationId, id] })
      } else {
        client.removeQueries({ queryKey: ['project', administrationId, id] })
      }
      void client.invalidateQueries({ queryKey: ['projects', administrationId] })
    },
  })
}
