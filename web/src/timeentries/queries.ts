import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type TimeEntry = components['schemas']['TimeEntry']
export type TimeEntryChanges = NonNullable<components['schemas']['TimeEntryRequest']['time_entry']>

export const perPage = 25
export const timeStates = ['all', 'open', 'non_billable'] as const
export type TimeState = (typeof timeStates)[number]

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of time entries this year, running timers included, with the number found in all. */
export function useTimeEntries(administrationId: string, state: TimeState, page: number) {
  return useQuery({
    queryKey: ['time_entries', administrationId, state, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/time_entries', {
        params: { path: admin(administrationId), query: { filter: `period:this_year,include_active:true,state:${state}`, page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { entries: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

/** A member's time in the week from its Monday, running timers included: Moneybird's timesheet register. */
export function useWeekEntries(administrationId: string, userId: string, period: string) {
  return useQuery({
    queryKey: ['time_entries', administrationId, 'week', userId, period],
    enabled: !!userId,
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/time_entries', {
      params: { path: admin(administrationId), query: { filter: `period:${period},include_active:true,user_id:${userId}`, per_page: '100' } },
    })),
  })
}

/** How many time entries a report reads at most, a page of a hundred at a time. */
export const reportLimit = 2000

/** Every time entry the filter and search select, for the hours and projects reports. */
export function useReportEntries(administrationId: string, filter: string, query = '', enabled = true) {
  return useQuery({
    queryKey: ['time_entries', administrationId, 'report', filter, query],
    enabled,
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const all: TimeEntry[] = []
      for (let page = 1; all.length < reportLimit; page++) {
        const { data, error, response } = await api.GET('/api/v2/{administration_id}/time_entries', {
          params: { path: admin(administrationId), query: { filter, ...(query ? { query } : {}), page: String(page), per_page: '100' } },
        })
        if (!response.ok) {
          throw new ApiError(response.status, error)
        }
        all.push(...(data ?? []))
        const total = Number(response.headers.get('X-Total-Count') ?? all.length)
        if ((data ?? []).length < 100 || all.length >= total) {
          break
        }
      }
      return all
    },
  })
}

export function useTimeEntry(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['time_entry', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/time_entries/{id}', one(administrationId, id ?? ''))),
  })
}

function useSettle(administrationId: string) {
  const client = useQueryClient()
  return (saved: TimeEntry) => {
    if (saved.id) {
      client.setQueryData(['time_entry', administrationId, saved.id], saved)
    }
    void client.invalidateQueries({ queryKey: ['time_entries', administrationId] })
  }
}

/** Records time. */
export function useCreateTimeEntry(administrationId: string) {
  const settle = useSettle(administrationId)
  return useMutation({
    mutationFn: (time_entry: TimeEntryChanges) =>
      unwrap(api.POST('/api/v2/{administration_id}/time_entries', { params: { path: admin(administrationId) }, body: { time_entry } })),
    onSuccess: settle,
  })
}

/** Stops a running timer, or resumes a stopped one. */
export function useTimer(administrationId: string, id: string) {
  const settle = useSettle(administrationId)
  return useMutation({
    mutationFn: (action: 'stop' | 'resume') =>
      action === 'stop'
        ? unwrap(api.PATCH('/api/v2/{administration_id}/time_entries/{id}/stop', one(administrationId, id)))
        : unwrap(api.PATCH('/api/v2/{administration_id}/time_entries/{id}/resume', one(administrationId, id))),
    onSuccess: settle,
  })
}

export function useDeleteTimeEntry(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { error, response } = await api.DELETE('/api/v2/{administration_id}/time_entries/{id}', one(administrationId, id))
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
    },
    onSuccess: () => {
      client.removeQueries({ queryKey: ['time_entry', administrationId, id] })
      void client.invalidateQueries({ queryKey: ['time_entries', administrationId] })
    },
  })
}

/** The minutes worked: from start to end, or to now while running, less the pauses. */
export function workedMinutes(entry: TimeEntry, now: Date = new Date()): number {
  const start = new Date(entry.started_at ?? '').getTime()
  const end = entry.ended_at ? new Date(entry.ended_at).getTime() : now.getTime()
  return Math.max(0, Math.floor((end - start) / 60000) - Math.floor((entry.paused_duration ?? 0) / 60))
}

/** Hours and minutes, such as 2:05. */
export function duration(minutes: number): string {
  return `${Math.floor(minutes / 60)}:${String(minutes % 60).padStart(2, '0')}`
}
