import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type Administration, api, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type User = components['schemas']['User']
export type Invitation = components['schemas']['UserInvitation']
export type UserType = components['schemas']['UserType']
export type Permission = components['schemas']['Permission']

export const userTypes: UserType[] = ['owner', 'employee', 'accountant']
export const permissions: Permission[] = ['sales_invoices', 'documents', 'estimates', 'bank', 'settings', 'ownership', 'time_entries']

/** Whether the signed-in user holds one of the permissions in the administration. */
export function can(a: Administration | undefined | null, ...wanted: Permission[]) {
  if (!a) {
    return false
  }
  return wanted.length === 0 || wanted.some((p) => a.permissions.includes(p))
}

/** The permissions a new user of the type starts with, as on the server. */
export function defaultPermissions(type: UserType): Permission[] {
  if (type === 'owner') {
    return [...permissions]
  }
  if (type === 'accountant') {
    return permissions.filter((p) => p !== 'ownership')
  }
  return ['sales_invoices', 'documents', 'estimates', 'time_entries']
}

const admin = (administrationId: string) => ({ params: { path: { administration_id: administrationId } } })
const one = (administrationId: string, id: string) => ({ params: { path: { administration_id: administrationId, id } } })

export function useUsers(administrationId: string) {
  return useQuery({
    queryKey: ['users', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/users', { params: { path: { administration_id: administrationId }, query: { include_inactive: true, include_accountants: true } } })),
  })
}

export function useUpdateUser(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (user: components['schemas']['UserRequest']['user']) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/users/{id}', { ...one(administrationId, id), body: { user } })),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['users', administrationId] })
      void client.invalidateQueries({ queryKey: ['administrations'] })
    },
  })
}

export function useRemoveUser(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/v2/{administration_id}/users/{id}', one(administrationId, id))),
    onSettled: () => client.invalidateQueries({ queryKey: ['users', administrationId] }),
  })
}

export function useLeave(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/membership', admin(administrationId))),
    onSuccess: () => client.invalidateQueries({ queryKey: ['administrations'] }),
  })
}

export function useInvitations(administrationId: string, enabled: boolean) {
  return useQuery({
    queryKey: ['invitations', administrationId],
    enabled,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/user_invitations', admin(administrationId))),
  })
}

export function useInvite(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (user_invitation: components['schemas']['UserInvitationRequest']['user_invitation']) =>
      unwrap(api.POST('/api/v2/{administration_id}/user_invitations', { ...admin(administrationId), body: { user_invitation } })),
    onSettled: () => client.invalidateQueries({ queryKey: ['invitations', administrationId] }),
  })
}

export function useRevokeInvitation(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/v2/{administration_id}/user_invitations/{id}', one(administrationId, id))),
    onSettled: () => client.invalidateQueries({ queryKey: ['invitations', administrationId] }),
  })
}

export function useRequireTwoFactor(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.PUT('/api/v2/{administration_id}/require_two_factor', admin(administrationId))),
    onSettled: () => client.invalidateQueries({ queryKey: ['administrations'] }),
  })
}

export function useSupportAccess(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (until: string | null) => unwrap(api.PUT('/api/v2/{administration_id}/support_access', { ...admin(administrationId), body: { until } })),
    onSettled: () => client.invalidateQueries({ queryKey: ['administrations'] }),
  })
}

export function useDeleteAdministration(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (name: string) =>
      unwrap(api.POST('/api/v2/administrations/{id}/delete', { params: { path: { id: administrationId } }, body: { name } })),
    // Only the list loads again: the deleted administration itself would answer 404 and show "Page not found".
    onSuccess: () => {
      client.removeQueries({ queryKey: ['administrations', administrationId] })
      return client.invalidateQueries({ queryKey: ['administrations'], exact: true })
    },
  })
}

export function useInvitationPreview(token: string) {
  return useQuery({
    queryKey: ['invitation', token],
    retry: false,
    queryFn: () => unwrap(api.GET('/api/invitations/{token}', { params: { path: { token } } })),
  })
}

export function useAcceptInvitation(token: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/invitations/{token}/accept', { params: { path: { token } } })),
    onSuccess: () => client.invalidateQueries({ queryKey: ['administrations'] }),
  })
}

export function useSupportGrants() {
  return useQuery({ queryKey: ['support-grants'], queryFn: () => unwrap(api.GET('/api/instance/support_access')) })
}

export function useTwoFactor() {
  return useQuery({ queryKey: ['two-factor'], queryFn: () => unwrap(api.GET('/api/user/two_factor')) })
}

export function useStartTwoFactor() {
  return useMutation({ mutationFn: () => unwrap(api.POST('/api/user/two_factor')) })
}

function useTwoFactorChanged() {
  const client = useQueryClient()
  return () => {
    void client.invalidateQueries({ queryKey: ['two-factor'] })
    void client.invalidateQueries({ queryKey: ['me'] })
  }
}

export function useConfirmTwoFactor() {
  const changed = useTwoFactorChanged()
  return useMutation({ mutationFn: (code: string) => unwrap(api.POST('/api/user/two_factor/confirm', { body: { code } })), onSettled: changed })
}

export function useDisableTwoFactor() {
  const changed = useTwoFactorChanged()
  return useMutation({ mutationFn: (code: string) => unwrap(api.DELETE('/api/user/two_factor', { body: { code } })), onSettled: changed })
}

export function useNewRecoveryCodes() {
  const changed = useTwoFactorChanged()
  return useMutation({ mutationFn: (code: string) => unwrap(api.POST('/api/user/two_factor/recovery_codes', { body: { code } })), onSettled: changed })
}
