import { replaceCached } from '../api/cache'
import { useTranslation } from 'react-i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type Asset = components['schemas']['Asset']
export type AssetInput = components['schemas']['AssetInput']

export const perPage = 25
export type AssetView = 'active' | 'all'

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of assets, the active ones or all, with the number found in all. */
export function useAssets(administrationId: string, view: AssetView, page: number) {
  return useQuery({
    queryKey: ['assets', administrationId, view, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/assets', {
        params: { path: admin(administrationId), query: { active: view === 'active' ? 'true' : 'false', page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { assets: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useAsset(administrationId: string, id: string) {
  return useQuery({
    queryKey: ['asset', administrationId, id],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/assets/{id}', one(administrationId, id))),
  })
}

export function useDeleteAsset(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/assets/{id}', one(administrationId, id))),
    onSuccess: () => client.invalidateQueries({ queryKey: ['assets', administrationId] }),
  })
}

/** The number of assets, the active ones or all, read from one short page. */
export function useAssetCount(administrationId: string, view: AssetView) {
  return useQuery({
    queryKey: ['assets', administrationId, view, 'count'],
    queryFn: async () => {
      const { error, response } = await api.GET('/api/v2/{administration_id}/assets', {
        params: { path: admin(administrationId), query: { active: view === 'active' ? 'true' : 'false', per_page: '1' } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return Number(response.headers.get('X-Total-Count') ?? 0)
    },
  })
}

/** Creates an asset when id is undefined and updates it otherwise. */
export function useSaveAsset(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (asset: AssetInput) => id === undefined
      ? unwrap(api.POST('/api/v2/{administration_id}/assets', { params: { path: admin(administrationId) }, body: { asset } as components['schemas']['AssetRequest'] }))
      : unwrap(api.PATCH('/api/v2/{administration_id}/assets/{id}', { ...one(administrationId, id), body: { asset } })),
    onSuccess: (saved) => {
      void client.invalidateQueries({ queryKey: ['assets', administrationId] })
      replaceCached(client, ['asset', administrationId, String(saved.id)], saved)
    },
  })
}

export type AssetBooking = components['schemas']['AssetBooking']

/** Refreshes the asset and the lists it is in. */
function useRefreshAsset(administrationId: string, id: string) {
  const client = useQueryClient()
  return () => Promise.all([
    client.invalidateQueries({ queryKey: ['asset', administrationId, id] }),
    client.invalidateQueries({ queryKey: ['assets', administrationId] }),
    client.invalidateQueries({ queryKey: ['asset-linkable', administrationId, id] }),
  ])
}

export function useAddAssetNote(administrationId: string, id: string) {
  const refresh = useRefreshAsset(administrationId, id)
  return useMutation({
    mutationFn: (note: { note: string; todo: boolean; assignee_id?: string }) =>
      unwrap(api.POST('/api/v2/{administration_id}/assets/{id}/notes', { ...one(administrationId, id), body: { note } })),
    onSuccess: refresh,
  })
}

export function useDeleteAssetNote(administrationId: string, id: string) {
  const refresh = useRefreshAsset(administrationId, id)
  return useMutation({
    mutationFn: (noteId: string) => unwrap(api.DELETE('/api/v2/{administration_id}/assets/{id}/notes/{note_id}', {
      params: { path: { ...admin(administrationId), id, note_id: noteId } },
    })),
    onSuccess: refresh,
  })
}

/** The bookings on the asset's category that no asset links yet, once asked for. */
export function useLinkableBookings(administrationId: string, id: string, enabled: boolean) {
  return useQuery({
    queryKey: ['asset-linkable', administrationId, id],
    enabled,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/assets/{id}/linkable_bookings', one(administrationId, id))),
  })
}

export function useLinkSource(administrationId: string, id: string) {
  const refresh = useRefreshAsset(administrationId, id)
  return useMutation({
    mutationFn: (b: AssetBooking) => unwrap(api.POST('/api/v2/{administration_id}/assets/{id}/sources', {
      ...one(administrationId, id),
      body: b.detail_id ? { detail_id: b.detail_id } : { general_journal_document_entry_id: b.general_journal_document_entry_id },
    })),
    onSuccess: refresh,
  })
}

export function useUnlinkSource(administrationId: string, id: string) {
  const refresh = useRefreshAsset(administrationId, id)
  return useMutation({
    mutationFn: (sourceId: string) => unwrap(api.DELETE('/api/v2/{administration_id}/assets/{id}/sources/{source_id}', {
      params: { path: { ...admin(administrationId), id, source_id: sourceId } },
    })),
    onSuccess: refresh,
  })
}

/** Adds an arbitrary depreciation or takes the asset out of service (Moneybird's full depreciation). */
export function useAssetChange(administrationId: string, id: string, kind: 'arbitrary' | 'full_depreciation') {
  const refresh = useRefreshAsset(administrationId, id)
  return useMutation({
    mutationFn: (change: { date: string; description: string; amount?: string }) => kind === 'arbitrary'
      ? unwrap(api.POST('/api/v2/{administration_id}/assets/{id}/value_changes/arbitrary', {
        ...one(administrationId, id), body: { date: change.date, description: change.description, amount: change.amount ?? '' },
      }))
      : unwrap(api.POST('/api/v2/{administration_id}/assets/{id}/value_changes/full_depreciation', {
        ...one(administrationId, id), body: { date: change.date, description: change.description },
      })),
    onSuccess: refresh,
  })
}

/** The name of the category a divested asset's sale is booked on, as Moneybird's "Book result company car". */
export const bookResultName = (category: string, prefix: string) => `${prefix} ${category.charAt(0).toLowerCase()}${category.slice(1)}`

type Divestment = { date: string; invoice?: { contactId: string; amount: string } }

/**
 * Divests the asset and, when asked, makes the draft sales invoice of the
 * sale, as Moneybird does: to the contact, for the amount, on the category
 * of the book result of the asset's category (added when missing) at the
 * standard VAT rate. Returns the invoice's id, if one was made.
 */
export function useDivestAsset(administrationId: string, asset: Asset) {
  const { t } = useTranslation()
  const id = String(asset.id)
  const refresh = useRefreshAsset(administrationId, id)
  const client = useQueryClient()
  return useMutation({
    mutationFn: async ({ date, invoice }: Divestment): Promise<string | null> => {
      if (invoice && !invoice.contactId) {
        throw new ApiError(422, { error: { contact_id: ["can't be blank"] } })
      }
      await unwrap(api.POST('/api/v2/{administration_id}/assets/{id}/value_changes/divestment', { ...one(administrationId, id), body: { date } }))
      if (!invoice) {
        return null
      }
      const accounts = await unwrap(api.GET('/api/v2/{administration_id}/ledger_accounts', { params: { path: admin(administrationId) } }))
      const category = accounts.find((a) => a.id === asset.ledger_account_id)?.name ?? asset.name ?? ''
      const name = bookResultName(category, t('assets.forms.divestment.bookResult'))
      const existing = accounts.find((a) => a.name === name && a.active !== false)
      const ledger = existing ?? await unwrap(api.POST('/api/v2/{administration_id}/ledger_accounts', {
        params: { path: admin(administrationId) },
        body: { ledger_account: { name, account_type: 'other_income_expenses', allowed_document_types: ['sales_invoice'] } },
      }))
      const rates = await unwrap(api.GET('/api/v2/{administration_id}/tax_rates', { params: { path: admin(administrationId) } }))
      const standard = rates
        .filter((r) => r.tax_rate_type === 'sales_invoice' && r.active !== false)
        .sort((a, b) => Number(b.percentage ?? 0) - Number(a.percentage ?? 0))[0]
      const made = await unwrap(api.POST('/api/v2/{administration_id}/sales_invoices', {
        params: { path: admin(administrationId) },
        body: { sales_invoice: {
          contact_id: invoice.contactId, invoice_date: date,
          details_attributes: [{ description: asset.name ?? '', price: invoice.amount || '0', amount: '1', ledger_account_id: String(ledger.id), ...(standard ? { tax_rate_id: String(standard.id) } : {}) }],
        } },
      }))
      void client.invalidateQueries({ queryKey: ['ledger-accounts', administrationId] })
      return String(made.id)
    },
    onSuccess: refresh,
    onError: refresh,
  })
}
