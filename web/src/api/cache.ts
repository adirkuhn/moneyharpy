import type { QueryClient, QueryKey } from '@tanstack/react-query'

/**
 * Puts what the server answered to a save in the cache. A fetch of the same
 * data still under way holds it from before the save, so it is cancelled
 * first: landing later, it would show the old version again.
 */
export function replaceCached<T>(client: QueryClient, queryKey: QueryKey, saved: T) {
  void client.cancelQueries({ queryKey, exact: true })
  client.setQueryData(queryKey, saved)
}
