import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type JournalDocument = components['schemas']['GeneralJournalDocument']
export type JournalDocumentChanges = components['schemas']['GeneralJournalDocumentRequest']['general_journal_document']
export type GeneralLedger = components['schemas']['GeneralLedgerReport']
export type JournalEntry = components['schemas']['JournalEntry']

export const reportPeriods = ['this_month', 'prev_month', 'this_quarter', 'prev_quarter', 'this_year', 'prev_year'] as const
export const documentsPerPage = 50

const path = (administrationId: string) => ({ administration_id: administrationId })

export function useGeneralLedger(administrationId: string, period: string) {
  return useQuery({
    queryKey: ['general-ledger', administrationId, period],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/general_ledger', { params: { path: path(administrationId), query: { period } } })),
  })
}

export function useLedgerEntries(administrationId: string, ledgerAccountId: string, period: string, page: number) {
  return useQuery({
    queryKey: ['ledger-entries', administrationId, ledgerAccountId, period, page],
    placeholderData: keepPreviousData,
    enabled: !!ledgerAccountId,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/reports/journal_entries', {
        params: { path: path(administrationId), query: { period, ledger_account_id: ledgerAccountId, page: String(page), per_page: '100' } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { entries: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useJournalDocuments(administrationId: string, period: string, page: number) {
  return useQuery({
    queryKey: ['journal-documents', administrationId, period, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/documents/general_journal_documents', {
        params: { path: path(administrationId), query: { filter: `period:${period}`, page: String(page), per_page: String(documentsPerPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { documents: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useJournalDocument(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['journal-document', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/documents/general_journal_documents/{id}', { params: { path: { ...path(administrationId), id: id ?? '' } } })),
  })
}

function useRefresh(administrationId: string) {
  const client = useQueryClient()
  return () => {
    for (const key of ['journal-documents', 'journal-document', 'general-ledger', 'ledger-entries']) {
      void client.invalidateQueries({ queryKey: [key, administrationId] })
    }
  }
}

/** Creates a journal entry when id is undefined and updates it otherwise. */
export function useSaveJournalDocument(administrationId: string, id?: string) {
  const refresh = useRefresh(administrationId)
  return useMutation({
    mutationFn: (general_journal_document: JournalDocumentChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/documents/general_journal_documents/{id}', { params: { path: { ...path(administrationId), id } }, body: { general_journal_document } }))
        : unwrap(api.POST('/api/v2/{administration_id}/documents/general_journal_documents', { params: { path: path(administrationId) }, body: { general_journal_document } })),
    onSuccess: refresh,
  })
}

export function useDeleteJournalDocument(administrationId: string, id: string) {
  const refresh = useRefresh(administrationId)
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/documents/general_journal_documents/{id}', { params: { path: { ...path(administrationId), id } } })),
    onSuccess: refresh,
  })
}

/** Turns a ledger report's lists into values per category. */
export function byAccount(values: { ledger_accounts: { ledger_account_id: string; value: string }[] } | undefined) {
  return new Map((values?.ledger_accounts ?? []).map((v) => [v.ledger_account_id, Number(v.value)]))
}

/** Reads an amount people type, such as "1.000,50" or "400". */
export function parseAmount(text: string): number | null {
  const s = text.trim()
  if (s === '') {
    return 0
  }
  const normalized = s.includes(',') ? s.replace(/\./g, '').replace(',', '.') : s
  return /^\d+(\.\d{1,2})?$/.test(normalized) ? Number(normalized) : null
}
