import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type ProfitLoss = components['schemas']['ProfitLossReport']
export type BalanceSheet = components['schemas']['BalanceSheetReport']
export type TaxReport = components['schemas']['TaxReport']
export type CashFlow = components['schemas']['CashFlowReport']
export type AgingItem = components['schemas']['aging_report_item']

/** The reports that list an amount per contact. */
export type ContactReportKind = 'debtors' | 'creditors' | 'revenue_by_contact' | 'expenses_by_contact'
export type AgingKind = 'debtors_aging' | 'creditors_aging'

export type ContactAmount = { contact_id?: string | number | null; amount: string }

const path = (administrationId: string) => ({ administration_id: administrationId })

export function useProfitLoss(administrationId: string, period: string) {
  return useQuery({
    queryKey: ['report', 'profit_loss', administrationId, period],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/profit_loss', { params: { path: path(administrationId), query: { period } } })),
  })
}

export function useBalanceSheet(administrationId: string, period: string) {
  return useQuery({
    queryKey: ['report', 'balance_sheet', administrationId, period],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/balance_sheet', { params: { path: path(administrationId), query: { period } } })),
  })
}

export function useTaxReport(administrationId: string, period: string) {
  return useQuery({
    queryKey: ['report', 'tax', administrationId, period],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/tax', { params: { path: path(administrationId), query: { period } } })),
  })
}

export function useCashFlow(administrationId: string, period: string) {
  return useQuery({
    queryKey: ['report', 'cash_flow', administrationId, period],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/cash_flow', { params: { path: path(administrationId), query: { period } } })),
  })
}

/** The first 100 contacts of a report per contact, the largest first. */
export function useContactReport(administrationId: string, kind: ContactReportKind, period: string) {
  return useQuery({
    queryKey: ['report', kind, administrationId, period],
    placeholderData: keepPreviousData,
    queryFn: async (): Promise<ContactAmount[]> => {
      const params = { params: { path: path(administrationId), query: { period, per_page: '100' } } }
      switch (kind) {
        case 'debtors':
          return (await unwrap(api.GET('/api/v2/{administration_id}/reports/debtors', params))).debtors
        case 'creditors':
          return (await unwrap(api.GET('/api/v2/{administration_id}/reports/creditors', params))).creditors
        case 'revenue_by_contact':
          return (await unwrap(api.GET('/api/v2/{administration_id}/reports/revenue_by_contact', params))).revenue_by_contact
        case 'expenses_by_contact':
          return (await unwrap(api.GET('/api/v2/{administration_id}/reports/expenses_by_contact', params))).expenses_by_contact
      }
    },
  })
}

/** What contacts had open at the end of the month (today for this month), by age. */
export function useAgingReport(administrationId: string, kind: AgingKind, until: string) {
  return useQuery({
    queryKey: ['report', kind, administrationId, until],
    placeholderData: keepPreviousData,
    queryFn: async (): Promise<AgingItem[]> => {
      const params = { params: { path: path(administrationId), query: { period_until: until, per_page: '100' } } }
      const { data, error, response } = kind === 'debtors_aging'
        ? await api.GET('/api/v2/{administration_id}/reports/debtors_aging', params)
        : await api.GET('/api/v2/{administration_id}/reports/creditors_aging', params)
      if (!response.ok || !data) {
        throw new ApiError(response.status, error)
      }
      return 'debtors' in data ? data.debtors : data.creditors
    },
  })
}

/** Turns a ledger_accounts list into amounts by category id. */
export function amounts(values?: { ledger_accounts?: { ledger_account_id?: string; value?: string }[] }): [string, number][] {
  return (values?.ledger_accounts ?? []).map((v) => [v.ledger_account_id ?? '', Number(v.value ?? 0)])
}
