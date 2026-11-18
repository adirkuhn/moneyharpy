import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type VatReturn = components['schemas']['VatReturn']
export type VatPreview = components['schemas']['VatReturnPreview']
export type VatPeriod = components['schemas']['VatReturnPeriod']
export type VatBox = components['schemas']['VatReturnBox']
export type VatKind = NonNullable<VatReturn['kind']>
export type IcpKind = 'goods' | 'services'

export const vatKinds: VatKind[] = ['vat_return', 'supplement', 'icp_declaration']

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** The returns filed, the latest period first. */
export function useVatReturns(administrationId: string) {
  return useQuery({
    queryKey: ['vat-returns', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/declarations', { params: { path: admin(administrationId) } })),
  })
}

export function useVatReturn(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['vat-return', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/declarations/{id}', one(administrationId, id ?? ''))),
  })
}

/** The periods a return of the kind can be filed for. */
export function useVatPeriods(administrationId: string, kind: VatKind | undefined) {
  return useQuery({
    queryKey: ['vat-periods', administrationId, kind],
    enabled: !!kind,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/declarations/periods', { params: { path: admin(administrationId), query: { kind: kind ?? 'vat_return' } } })),
  })
}

/** The period as the API writes it: 20260701..20260930. */
export const periodParam = (p: Pick<VatPeriod, 'period_from' | 'period_to'>) => `${p.period_from.replaceAll('-', '')}..${p.period_to.replaceAll('-', '')}`

/** The return of the kind for the period as it would be filed, with its checks. */
export function useVatPreview(administrationId: string, kind: VatKind | undefined, period: string | undefined) {
  return useQuery({
    queryKey: ['vat-preview', administrationId, kind, period],
    enabled: !!kind && !!period,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/declarations/preview', {
      params: { path: admin(administrationId), query: { kind: kind ?? 'vat_return', period: period ?? '' } },
    })),
  })
}

function useSettle(administrationId: string) {
  const client = useQueryClient()
  return () => {
    void client.invalidateQueries({ queryKey: ['vat-returns', administrationId] })
    void client.invalidateQueries({ queryKey: ['vat-periods', administrationId] })
    void client.invalidateQueries({ queryKey: ['vat-preview', administrationId] })
    void client.invalidateQueries({ queryKey: ['bookkeeping-settings', administrationId] })
  }
}

/** Records a return filed by hand. */
export function useSubmitVatReturn(administrationId: string) {
  const settle = useSettle(administrationId)
  return useMutation({
    mutationFn: (vat_return: { kind: VatKind; period: string; icp_lines?: { contact_id: string; kind: IcpKind }[] }) =>
      unwrap(api.POST('/api/v2/{administration_id}/declarations', { params: { path: admin(administrationId) }, body: { vat_return } })),
    onSuccess: settle,
  })
}

export function useDeleteVatReturn(administrationId: string, id: string) {
  const settle = useSettle(administrationId)
  const client = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { error, response } = await api.DELETE('/api/v2/{administration_id}/declarations/{id}', one(administrationId, id))
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
    },
    onSuccess: () => {
      client.removeQueries({ queryKey: ['vat-return', administrationId, id] })
      settle()
    },
  })
}
