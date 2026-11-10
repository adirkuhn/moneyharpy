import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../api/client'
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
