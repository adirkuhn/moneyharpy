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

export function useLogin() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (body: { email: string; password: string }) => unwrap(api.POST('/api/auth/login', { body })),
    onSuccess: (user) => client.setQueryData(['me'], user),
  })
}

export function useRegister() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (body: { email: string; name: string; password: string }) =>
      unwrap(api.POST('/api/auth/register', { body })),
    onSuccess: (user) => client.setQueryData(['me'], user),
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
