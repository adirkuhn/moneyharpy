import { useQuery } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'

export const manualPath = (country = 'NL') => `/onboarding/company/manually?country=${country}`
export const createPath = '/onboarding/company/create_administration?origin=onboarding'

export function useOnboardingOptions() {
  return useQuery({ queryKey: ['onboarding-options'], queryFn: () => unwrap(api.GET('/api/onboarding/options')) })
}

export function useCompanySearch(term: string) {
  return useQuery({
    queryKey: ['company-search', term],
    enabled: term.length >= 2,
    queryFn: () => unwrap(api.GET('/api/onboarding/company_search', { params: { query: { country: 'NL', query: term } } })),
  })
}

/** Finds an address, or null when the combination is unknown. */
export async function lookupAddress(zipcode: string, houseNumber: string) {
  try {
    return await unwrap(api.GET('/api/onboarding/address_lookup', { params: { query: { zipcode, house_number: houseNumber } } }))
  } catch (e) {
    if (e instanceof ApiError) {
      return null
    }
    throw e
  }
}
