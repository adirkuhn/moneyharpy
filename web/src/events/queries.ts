import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { api, ApiError } from '../api/client'
import type { components } from '../api/schema'

export type Event = components['schemas']['event_response']

export const eventsPerPage = 50

/** A page of the events made in the period, newest first, with the cursor to the older ones. */
export function useEvents(administrationId: string, period: string, cursor: string | undefined) {
  return useQuery({
    queryKey: ['events', administrationId, period, cursor],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/events', {
        params: { path: { administration_id: administrationId }, query: { period, per_page: String(eventsPerPage), ...(cursor ? { cursor } : {}) } },
      })
      if (!response.ok || !data) {
        throw new ApiError(response.status, error)
      }
      return { events: data, next: nextCursor(response.headers.get('Link')) }
    },
  })
}

/** The cursor of the next page from a Link header, if there is one. */
export function nextCursor(link: string | null): string | undefined {
  const url = /<([^>]+)>;\s*rel="next"/.exec(link ?? '')?.[1]
  return url ? new URL(url, 'http://localhost').searchParams.get('cursor') ?? undefined : undefined
}
