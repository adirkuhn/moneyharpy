import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'

export function useCurrentUser() {
  return useQuery({
    queryKey: ['me'],
    queryFn: async () => {
      try {
        return await unwrap(api.GET('/api/auth/me'))
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) {
          return null
        }
        throw e
      }
    },
  })
}

export function useProviders() {
  return useQuery({ queryKey: ['providers'], queryFn: () => unwrap(api.GET('/api/auth/providers')) })
}

/** Signs in; resolves to "two_factor" when a code is needed next. */
export function useLogin() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (body: { email: string; password: string }) => {
      const result = await unwrap(api.POST('/api/auth/login', { body }))
      if ('two_factor_required' in result) {
        return 'two_factor' as const
      }
      client.setQueryData(['me'], result)
      return 'signed_in' as const
    },
  })
}

export function useCompleteSignIn() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (code: string) => unwrap(api.POST('/api/auth/two_factor', { body: { code } })),
    onSuccess: (user) => client.setQueryData(['me'], user),
  })
}

export function useRegister() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (body: { email: string; name: string; password: string; invitation_token?: string }) =>
      unwrap(api.POST('/api/auth/register', { body })),
    onSuccess: (user) => {
      if (!('two_factor_required' in user)) {
        client.setQueryData(['me'], user)
      }
    },
  })
}

export function useLogout() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/auth/logout')),
    onSuccess: () => {
      client.clear()
      client.setQueryData(['me'], null)
    },
  })
}
