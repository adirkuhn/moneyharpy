import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type InvoiceSettings = components['schemas']['InvoiceSettings']
export type InvoiceSettingsChanges = components['schemas']['InvoiceSettingsRequest']['invoice_settings']
export type Workflow = components['schemas']['Workflow']
export type WorkflowChanges = components['schemas']['WorkflowRequest']['workflow']
export type DocumentStyle = components['schemas']['DocumentStyle']
export type DocumentStyleChanges = components['schemas']['DocumentStyleRequest']['document_style']
export type Line = components['schemas']['DocumentStyleLine']
export type FileKind = 'logo' | 'background'

const admin = (administrationId: string) => ({ params: { path: { administration_id: administrationId } } })
const one = (administrationId: string, id: string) => ({ params: { path: { administration_id: administrationId, id } } })

export function useInvoiceSettings(administrationId: string) {
  return useQuery({
    queryKey: ['invoice-settings', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/invoice_settings', admin(administrationId))),
  })
}

export function useUpdateInvoiceSettings(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (invoice_settings: InvoiceSettingsChanges) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/invoice_settings', { ...admin(administrationId), body: { invoice_settings } })),
    onSuccess: (saved) => client.setQueryData(['invoice-settings', administrationId], saved),
  })
}

export function useWorkflows(administrationId: string) {
  return useQuery({
    queryKey: ['workflows', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/workflows', admin(administrationId))),
  })
}

export function useWorkflow(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['workflows', administrationId, id],
    enabled: id !== undefined,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/workflows/{id}', one(administrationId, id ?? ''))),
  })
}

/** Creates a workflow when id is undefined and updates it otherwise. */
export function useSaveWorkflow(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (workflow: WorkflowChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/workflows/{id}', { ...one(administrationId, id), body: { workflow } }))
        : unwrap(api.POST('/api/v2/{administration_id}/workflows', { ...admin(administrationId), body: { workflow } })),
    onSuccess: () => client.invalidateQueries({ queryKey: ['workflows', administrationId] }),
  })
}

export function useDeleteWorkflow(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/v2/{administration_id}/workflows/{id}', one(administrationId, id))),
    onSuccess: () => client.invalidateQueries({ queryKey: ['workflows', administrationId] }),
  })
}

export function useDocumentStyles(administrationId: string) {
  return useQuery({
    queryKey: ['document-styles', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/document_styles', admin(administrationId))),
  })
}

export function useDocumentStyle(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['document-styles', administrationId, id],
    enabled: id !== undefined,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/document_styles/{id}', one(administrationId, id ?? ''))),
  })
}

/** Creates a document style when id is undefined and updates it otherwise. */
export function useSaveDocumentStyle(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (document_style: DocumentStyleChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/document_styles/{id}', { ...one(administrationId, id), body: { document_style } }))
        : unwrap(api.POST('/api/v2/{administration_id}/document_styles', { ...admin(administrationId), body: { document_style } })),
    onSuccess: () => client.invalidateQueries({ queryKey: ['document-styles', administrationId] }),
  })
}

export function useDeleteDocumentStyle(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/v2/{administration_id}/document_styles/{id}', one(administrationId, id))),
    onSuccess: () => client.invalidateQueries({ queryKey: ['document-styles', administrationId] }),
  })
}

const filePath = '/api/v2/{administration_id}/document_styles/{id}/{file}'

/** Where the browser can show a document style's file; the hash busts caches. */
export function styleFileURL(administrationId: string, id: string, file: FileKind, hash: string) {
  return `/api/v2/${administrationId}/document_styles/${id}/${file}?v=${hash.slice(0, 12)}`
}

/** Uploads a logo or background, sending the file itself as the body. */
export function useUploadStyleFile(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ file, data }: { file: FileKind; data: File }) =>
      unwrap(
        api.PUT(filePath, {
          params: { path: { administration_id: administrationId, id, file } },
          body: data as unknown as string,
          bodySerializer: (body: unknown) => body as Blob,
          headers: { 'Content-Type': data.type || 'application/octet-stream' },
        }),
      ),
    onSuccess: (saved) => {
      client.setQueryData(['document-styles', administrationId, id], saved)
      void client.invalidateQueries({ queryKey: ['document-styles', administrationId] })
    },
  })
}

export function useRemoveStyleFile(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (file: FileKind) => unwrap(api.DELETE(filePath, { params: { path: { administration_id: administrationId, id, file } } })),
    onSuccess: (saved) => {
      client.setQueryData(['document-styles', administrationId, id], saved)
      void client.invalidateQueries({ queryKey: ['document-styles', administrationId] })
    },
  })
}
