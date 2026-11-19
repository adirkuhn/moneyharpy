import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type TaskList = components['schemas']['TaskList']
export type TaskGroup = components['schemas']['task_list_group_response']
/** A task with MoneyHarpy's related category; the generated list types drop it from nested tasks. */
export type Task = components['schemas']['task_list_task_response'] & { related_entity_id?: string | null; generated?: boolean }
export type TaskChanges = components['schemas']['TaskListTaskRequest']['task']
export type ReportType = components['schemas']['TaskListReportType']
export const reportTypes: ReportType[] = ['balance_sheet', 'profit_loss', 'debtor', 'creditor', 'assets']
export type TaskListChanges = components['schemas']['TaskListRequest']['task_list']
export type Template = components['schemas']['TaskListTemplate']
export type TemplateChanges = components['schemas']['TaskListTemplateRequest']['task_list_template']
/** What holds groups of tasks: a task list or a template. */
export type Owner = { kind: 'list' | 'template'; id: string }

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

/** Creates a task list when id is undefined and updates it otherwise. */
export function useSaveTaskList(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (task_list: TaskListChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/task_lists/{id}', { ...one(administrationId, id), body: { task_list } }))
        : unwrap(api.POST('/api/v2/{administration_id}/task_lists', { params: { path: admin(administrationId) }, body: { task_list } })),
    onSuccess: (saved) => {
      client.setQueryData(['task_list', administrationId, saved.id], saved)
      void client.invalidateQueries({ queryKey: ['task_lists', administrationId] })
    },
  })
}

/** The administration's task list templates. */
export function useTemplates(administrationId: string) {
  return useQuery({
    queryKey: ['task_list_templates', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/task_list_templates', { params: { path: admin(administrationId) } })),
  })
}

export function useTemplate(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['task_list_template', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/task_list_templates/{id}', one(administrationId, id ?? ''))),
  })
}

/** Creates a template when id is undefined and updates it otherwise. */
export function useSaveTemplate(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (task_list_template: TemplateChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/task_list_templates/{id}', { ...one(administrationId, id), body: { task_list_template } }))
        : unwrap(api.POST('/api/v2/{administration_id}/task_list_templates', { params: { path: admin(administrationId) }, body: { task_list_template } })),
    onSuccess: (saved) => {
      client.setQueryData(['task_list_template', administrationId, saved.id], saved)
      void client.invalidateQueries({ queryKey: ['task_list_templates', administrationId] })
    },
  })
}

export function useDeleteTemplate(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { error, response } = await api.DELETE('/api/v2/{administration_id}/task_list_templates/{id}', one(administrationId, id))
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
    },
    onSuccess: () => {
      client.removeQueries({ queryKey: ['task_list_template', administrationId, id] })
      void client.invalidateQueries({ queryKey: ['task_list_templates', administrationId] })
    },
  })
}

/** Makes a task list from a template, for the period before the current one. */
export function useListFromTemplate(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (template: string) =>
      unwrap(api.POST('/api/v2/{administration_id}/task_list_templates/{id}/task_lists', { ...one(administrationId, template), body: { task_list: {} } })),
    onSuccess: (saved) => {
      client.setQueryData(['task_list', administrationId, saved.id], saved)
      void client.invalidateQueries({ queryKey: ['task_lists', administrationId] })
    },
  })
}

/** What a change to a list's tasks needs to refresh. */
function useRefresh(administrationId: string, owner: Owner) {
  const client = useQueryClient()
  return () => {
    if (owner.kind === 'template') {
      void client.invalidateQueries({ queryKey: ['task_list_template', administrationId, owner.id] })
      return
    }
    void client.invalidateQueries({ queryKey: ['task_list', administrationId, owner.id] })
    void client.invalidateQueries({ queryKey: ['task_lists', administrationId] })
  }
}

/** Adds a task at the end of a group when id is undefined, and changes the task otherwise; unassign clears its assignee. */
export function useSaveTask(administrationId: string, owner: Owner) {
  const refresh = useRefresh(administrationId, owner)
  return useMutation({
    mutationFn: async ({ group, id, task, unassign }: { group?: string; id?: string; task: TaskChanges; unassign?: boolean }) => {
      if (!id) {
        return unwrap(api.POST('/api/v2/{administration_id}/task_list_groups/{id}/tasks', { ...one(administrationId, group ?? ''), body: { task } }))
      }
      if (unassign) {
        await unwrap(api.DELETE('/api/v2/{administration_id}/task_list_tasks/{id}/assignment', one(administrationId, id)))
      }
      return unwrap(api.PATCH('/api/v2/{administration_id}/task_list_tasks/{id}', { ...one(administrationId, id), body: { task } }))
    },
    onSuccess: refresh,
  })
}

export function useDeleteTask(administrationId: string, owner: Owner) {
  const refresh = useRefresh(administrationId, owner)
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE('/api/v2/{administration_id}/task_list_tasks/{id}', one(administrationId, id))
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
    },
    onSuccess: refresh,
  })
}

/** Adds a group at the end of the list when id is undefined, and renames the group otherwise. */
export function useSaveGroup(administrationId: string, owner: Owner) {
  const refresh = useRefresh(administrationId, owner)
  return useMutation({
    mutationFn: ({ id, name }: { id?: string; name: string }) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/task_list_groups/{id}', { ...one(administrationId, id), body: { group: { name } } }))
        : owner.kind === 'template'
          ? unwrap(api.POST('/api/v2/{administration_id}/task_list_templates/{id}/groups', { ...one(administrationId, owner.id), body: { group: { name } } }))
          : unwrap(api.POST('/api/v2/{administration_id}/task_lists/{id}/groups', { ...one(administrationId, owner.id), body: { group: { name } } })),
    onSuccess: refresh,
  })
}

export function useDeleteGroup(administrationId: string, owner: Owner) {
  const refresh = useRefresh(administrationId, owner)
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE('/api/v2/{administration_id}/task_list_groups/{id}', one(administrationId, id))
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
    },
    onSuccess: refresh,
  })
}

/** Completes a task, or reopens it. */
export function useCompleteTask(administrationId: string, owner: Owner) {
  const refresh = useRefresh(administrationId, owner)
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
