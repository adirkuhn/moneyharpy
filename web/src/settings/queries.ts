import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type Identity = components['schemas']['Identity']
export type IdentityFields = NonNullable<components['schemas']['IdentityRequest']['identity']>
export type AdministrationChanges = components['schemas']['UpdateAdministrationRequest']['administration']
export type CustomField = components['schemas']['CustomField']
export type CustomFieldSource = components['schemas']['CustomFieldSource']

const path = (administrationId: string) => ({ params: { path: { administration_id: administrationId } } })

export function useIdentities(administrationId: string) {
  return useQuery({
    queryKey: ['identities', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/identities', path(administrationId))),
  })
}

/** Loads the default identity, or the one with identityId when given. */
export function useIdentity(administrationId: string, identityId?: string) {
  return useQuery({
    queryKey: ['identities', administrationId, identityId ?? 'default'],
    queryFn: () =>
      identityId
        ? unwrap(api.GET('/api/v2/{administration_id}/identities/{id}', { params: { path: { administration_id: administrationId, id: identityId } } }))
        : unwrap(api.GET('/api/v2/{administration_id}/identities/default', path(administrationId))),
  })
}

function saveIdentity(administrationId: string, target: string, identity: IdentityFields) {
  const body = { identity }
  if (target === 'default') {
    return unwrap(api.PATCH('/api/v2/{administration_id}/identities/default', { ...path(administrationId), body }))
  }
  if (target === 'new') {
    return unwrap(api.POST('/api/v2/{administration_id}/identities', { ...path(administrationId), body }))
  }
  return unwrap(
    api.PATCH('/api/v2/{administration_id}/identities/{id}', { params: { path: { administration_id: administrationId, id: target } }, body }),
  )
}

/**
 * Saves an identity: the default one, an existing one, or a new one. The
 * company profile, kept on the administration, is saved after the identity.
 */
export function useSaveIdentity(administrationId: string, target: 'default' | 'new' | string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async ({ identity, profile }: { identity: IdentityFields; profile?: AdministrationChanges }) => {
      const saved = await saveIdentity(administrationId, target, identity)
      if (profile) {
        await unwrap(api.PATCH('/api/v2/administrations/{id}', { params: { path: { id: administrationId } }, body: { administration: profile } }))
      }
      return saved
    },
    // Runs after a failed profile save too, since the identity may be saved.
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['identities', administrationId] })
      void client.invalidateQueries({ queryKey: ['administrations'] })
    },
  })
}

export function useCustomFields(administrationId: string) {
  return useQuery({
    queryKey: ['custom-fields', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/custom_fields', path(administrationId))),
  })
}

export function useCreateCustomField(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (customField: { name: string; source: CustomFieldSource }) =>
      unwrap(api.POST('/api/v2/{administration_id}/custom_fields', { ...path(administrationId), body: { custom_field: customField } })),
    onSuccess: () => client.invalidateQueries({ queryKey: ['custom-fields', administrationId] }),
  })
}

export function useDeleteIdentity(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE('/api/v2/{administration_id}/identities/{id}', { params: { path: { administration_id: administrationId, id } } })),
    onSuccess: () => client.invalidateQueries({ queryKey: ['identities', administrationId] }),
  })
}

export function useUpdateAdministration(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (administration: AdministrationChanges) =>
      unwrap(api.PATCH('/api/v2/administrations/{id}', { params: { path: { id: administrationId } }, body: { administration } })),
    onSuccess: () => client.invalidateQueries({ queryKey: ['administrations'] }),
  })
}
