import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type Connection = components['schemas']['Connection']
export type CreatedConnection = components['schemas']['CreatedConnection']
export type Scope = components['schemas']['Scope']
export type Webhook = components['schemas']['Webhook']

export const scopes: Scope[] = ['sales_invoices', 'documents', 'estimates', 'bank', 'time_entries', 'settings']

const admin = (administrationId: string) => ({ params: { path: { administration_id: administrationId } } })
const one = (administrationId: string, id: string) => ({ params: { path: { administration_id: administrationId, id } } })

export function useConnectionSettings(administrationId: string) {
  return useQuery({
    queryKey: ['connection-settings', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/connection_settings', admin(administrationId))),
  })
}

export function useUpdateConnectionSettings(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (users_may_connect: boolean) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/connection_settings', { ...admin(administrationId), body: { users_may_connect } })),
    onSuccess: (saved) => client.setQueryData(['connection-settings', administrationId], saved),
  })
}

export function useConnections(administrationId: string) {
  return useQuery({
    queryKey: ['connections', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/connections', admin(administrationId))),
  })
}

export function useCreateConnection(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (connection: components['schemas']['ConnectionRequest']['connection']) =>
      unwrap(api.POST('/api/v2/{administration_id}/connections', { ...admin(administrationId), body: { connection } })),
    onSettled: () => client.invalidateQueries({ queryKey: ['connections', administrationId] }),
  })
}

export function useRevokeConnection(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/v2/{administration_id}/connections/{id}', one(administrationId, id))),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['connections', administrationId] })
      void client.invalidateQueries({ queryKey: ['webhooks', administrationId] })
    },
  })
}

export function useWebhooks(administrationId: string, enabled = true) {
  return useQuery({
    queryKey: ['webhooks', administrationId],
    enabled,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/webhooks', admin(administrationId))),
  })
}

export function useWebhookEvents(administrationId: string) {
  return useQuery({
    queryKey: ['webhook-events', administrationId],
    staleTime: Infinity,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/webhooks/events', admin(administrationId))),
  })
}

export function useCreateWebhook(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (body: { url: string; enabled_events: string[] }) =>
      unwrap(api.POST('/api/v2/{administration_id}/webhooks', { ...admin(administrationId), body })),
    onSettled: () => client.invalidateQueries({ queryKey: ['webhooks', administrationId] }),
  })
}

export function useSetWebhookActive(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (on: boolean) =>
      on
        ? unwrap(api.PATCH('/api/v2/{administration_id}/webhooks/{id}/activate', one(administrationId, id)))
        : unwrap(api.PATCH('/api/v2/{administration_id}/webhooks/{id}/deactivate', one(administrationId, id))),
    onSettled: () => client.invalidateQueries({ queryKey: ['webhooks', administrationId] }),
  })
}

export function useDeleteWebhook(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/v2/{administration_id}/webhooks/{id}', one(administrationId, id))),
    onSettled: () => client.invalidateQueries({ queryKey: ['webhooks', administrationId] }),
  })
}

export type MoneybirdImport = components['schemas']['MoneybirdImport']
export type MoneybirdAdministration = components['schemas']['MoneybirdAdministration']

/** The import from Moneybird, null before one was started. While it runs it is asked for again every few seconds. */
export function useMoneybirdImport(administrationId: string) {
  return useQuery({
    queryKey: ['moneybird-import', administrationId],
    queryFn: async () => {
      try {
        return await unwrap(api.GET('/api/v2/{administration_id}/moneybird_import', admin(administrationId)))
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) {
          return null
        }
        throw e
      }
    },
    refetchInterval: (query) => (query.state.data?.state === 'running' ? 3000 : false),
  })
}

/** Checks a Moneybird API token and lists the administrations it reaches. */
export function useMoneybirdAdministrations(administrationId: string) {
  return useMutation({
    mutationFn: (token: string) => unwrap(api.POST('/api/v2/{administration_id}/moneybird_import/administrations', { ...admin(administrationId), body: { token } })),
  })
}

export function useStartMoneybirdImport(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    // Without a token, the one the last import kept imports the same administration again.
    mutationFn: (body: { token?: string; administration_id?: string }) =>
      unwrap(api.POST('/api/v2/{administration_id}/moneybird_import', { ...admin(administrationId), body: { moneybird_import: body } })),
    onSuccess: (started) => client.setQueryData(['moneybird-import', administrationId], started),
  })
}

/** Forgets the Moneybird token the import keeps; importing again then asks for one. */
export function useForgetMoneybirdImportToken(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/moneybird_import/token', admin(administrationId))),
    onSuccess: (job) => client.setQueryData(['moneybird-import', administrationId], job),
  })
}

/** Brings an import queued after a failure forward, to try again now. */
export function useTryMoneybirdImportNow(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/v2/{administration_id}/moneybird_import/try_now', admin(administrationId))),
    onSuccess: (job) => client.setQueryData(['moneybird-import', administrationId], job),
  })
}

export type MoneybirdImportProblem = components['schemas']['MoneybirdImportProblem']

/** One page of what the import could not bring in, with how many in all. */
export function useMoneybirdImportProblems(administrationId: string, page: number, enabled: boolean) {
  return useQuery({
    queryKey: ['moneybird-import-problems', administrationId, page],
    enabled,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/moneybird_import/problems', {
        params: { path: { administration_id: administrationId }, query: { page: String(page), per_page: '50' } },
      })
      if (error || !data) {
        throw new ApiError(response.status, error)
      }
      return { problems: data, total: Number(response.headers.get('X-Total-Count') ?? data.length) }
    },
  })
}
