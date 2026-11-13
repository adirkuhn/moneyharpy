import { ApiError } from '../api/client'

type Problem = 'onboarding.kvkUnavailable' | 'onboarding.kvkBlocked' | 'onboarding.kvkRateLimited' | 'onboarding.searchFailed'

/**
 * Why a company register search failed, as a translation key: the KVK server
 * not answering, KVK blocking the request, too many searches, or otherwise a
 * register that could not be reached. Each tells people to go on by hand.
 */
export function registerProblem(error: unknown): Problem {
  switch (error instanceof ApiError ? error.code : '') {
    case 'kvk_unavailable':
      return 'onboarding.kvkUnavailable'
    case 'kvk_blocked':
      return 'onboarding.kvkBlocked'
    case 'kvk_rate_limited':
      return 'onboarding.kvkRateLimited'
    default:
      return 'onboarding.searchFailed'
  }
}
