import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type TaskList = components['schemas']['TaskList']
export type TaskGroup = components['schemas']['task_list_group_response']
export type Task = components['schemas']['task_list_task_response']

export const perPage = 30

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of task lists, newest first, with the number found in all. */
export function useTaskLists(administrationId: string, page: number) {
  return useQuery({
    queryKey: ['task_lists', administrationId, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/task_lists', {
        params: { path: admin(administrationId), query: { page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { lists: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useTaskList(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['task_list', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/task_lists/{id}', one(administrationId, id ?? ''))),
  })
}

export function useCreateTaskList(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (name: string) =>
      unwrap(api.POST('/api/v2/{administration_id}/task_lists', { params: { path: admin(administrationId) }, body: { task_list: { name } } })),
    onSuccess: (saved) => {
      client.setQueryData(['task_list', administrationId, saved.id], saved)
      void client.invalidateQueries({ queryKey: ['task_lists', administrationId] })
    },
  })
}

/** What a change to a list's tasks needs to refresh. */
function useRefresh(administrationId: string, listId: string) {
  const client = useQueryClient()
  return () => {
    void client.invalidateQueries({ queryKey: ['task_list', administrationId, listId] })
    void client.invalidateQueries({ queryKey: ['task_lists', administrationId] })
  }
}

/** Adds a task at the end of a group. */
export function useAddTask(administrationId: string, listId: string) {
  const refresh = useRefresh(administrationId, listId)
  return useMutation({
    mutationFn: ({ group, name }: { group: string; name: string }) =>
      unwrap(api.POST('/api/v2/{administration_id}/task_list_groups/{id}/tasks', { ...one(administrationId, group), body: { task: { name } } })),
    onSuccess: refresh,
  })
}

/** Completes a task, or reopens it. */
export function useCompleteTask(administrationId: string, listId: string) {
  const refresh = useRefresh(administrationId, listId)
  return useMutation({
    mutationFn: ({ task, done }: { task: string; done: boolean }) =>
      done
        ? unwrap(api.POST('/api/v2/{administration_id}/task_list_tasks/{id}/completion', one(administrationId, task)))
        : unwrap(api.DELETE('/api/v2/{administration_id}/task_list_tasks/{id}/completion', one(administrationId, task))),
    onSuccess: refresh,
  })
}

export function useDeleteTaskList(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { error, response } = await api.DELETE('/api/v2/{administration_id}/task_lists/{id}', one(administrationId, id))
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
    },
    onSuccess: () => {
      client.removeQueries({ queryKey: ['task_list', administrationId, id] })
      void client.invalidateQueries({ queryKey: ['task_lists', administrationId] })
    },
  })
}
