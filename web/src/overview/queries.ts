import { useQuery } from '@tanstack/react-query'
import { api, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type Overview = components['schemas']['Overview']
export type OverviewItem = components['schemas']['OverviewItem']

export function useOverview(administrationId: string) {
  return useQuery({
    queryKey: ['overview', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/overview', { params: { path: { administration_id: administrationId } } })),
  })
}

/** The part of the day at the hour in the time zone, for the greeting. */
export function partOfDay(now: Date, timeZone: string): 'morning' | 'afternoon' | 'evening' {
  let hour = now.getHours()
  try {
    hour = Number(new Intl.DateTimeFormat('en-GB', { hour: 'numeric', hourCycle: 'h23', timeZone }).format(now))
  } catch {
    // An unknown time zone falls back to the browser's.
  }
  if (hour >= 5 && hour < 12) {
    return 'morning'
  }
  return hour >= 12 && hour < 18 ? 'afternoon' : 'evening'
}

const lastKey = 'moneyharpy.lastAdministration'

/** The administration opened last in this browser, if any. */
export function lastAdministration(): string | null {
  try {
    return localStorage.getItem(lastKey)
  } catch {
    return null
  }
}

export function rememberAdministration(id: string) {
  try {
    localStorage.setItem(lastKey, id)
  } catch {
    // Without storage the logo leads to the list of administrations.
  }
}
