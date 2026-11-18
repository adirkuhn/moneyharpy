import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'
import type { Granularity } from './period'

export type ProfitLoss = components['schemas']['ProfitLossReport']
export type BalanceSheet = components['schemas']['BalanceSheetReport']
export type TaxReport = components['schemas']['TaxReport']
export type CashFlow = components['schemas']['CashFlowReport']
export type GeneralLedger = components['schemas']['GeneralLedgerReport']
export type AgingItem = components['schemas']['aging_report_item']
export type AssetsReportLine = components['schemas']['AssetsReportLine']

/** The reports that list an amount per contact. */
export type ContactReportKind = 'debtors' | 'creditors' | 'revenue_by_contact' | 'expenses_by_contact'
export type AgingKind = 'debtors_aging' | 'creditors_aging'
/** The reports that list an amount per project. */
export type ProjectReportKind = 'revenue_by_project' | 'expenses_by_project'

/** One row of a report per contact or project: who or what, and its amount. */
export type Subject = { id: string | null; amount: string }
/** A report per contact or project: the page's rows for the whole period and, split up, for each part of it. */
export type SubjectReport = { rows: Subject[]; parts: { period: string; rows: Subject[] }[]; total: number }

/** What a report is asked for: its period and, for a report split into columns, how it is split. */
export type ReportQuery = { period: string; granularity?: Granularity }

/** The rows of a report page. */
export const reportPerPage = 100

const path = (administrationId: string) => ({ administration_id: administrationId })
const query = (q: ReportQuery) => ({ period: q.period, ...(q.granularity ? { granularity: q.granularity } : {}) })

export function useProfitLoss(administrationId: string, q: ReportQuery) {
  return useQuery({
    queryKey: ['report', 'profit_loss', administrationId, q],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/profit_loss', { params: { path: path(administrationId), query: query(q) } })),
  })
}

export function useBalanceSheet(administrationId: string, q: ReportQuery) {
  return useQuery({
    queryKey: ['report', 'balance_sheet', administrationId, q],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/balance_sheet', { params: { path: path(administrationId), query: query(q) } })),
  })
}

export function useTaxReport(administrationId: string, q: ReportQuery) {
  return useQuery({
    queryKey: ['report', 'tax', administrationId, q],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/tax', { params: { path: path(administrationId), query: query(q) } })),
  })
}

export function useCashFlow(administrationId: string, q: ReportQuery) {
  return useQuery({
    queryKey: ['report', 'cash_flow', administrationId, q],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/cash_flow', { params: { path: path(administrationId), query: query(q) } })),
  })
}

export function useGeneralLedger(administrationId: string, q: ReportQuery) {
  return useQuery({
    queryKey: ['report', 'general_ledger', administrationId, q],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/general_ledger', { params: { path: path(administrationId), query: query(q) } })),
  })
}

type ContactItem = { contact_id?: string | number | null; amount: string }
type ProjectItem = { project_id?: string | number | null; amount: string }
const ofContacts = (items: ContactItem[]): Subject[] => items.map((x) => ({ id: x.contact_id == null ? null : String(x.contact_id), amount: x.amount }))
const ofProjects = (items: ProjectItem[]): Subject[] => items.map((x) => ({ id: x.project_id == null ? null : String(x.project_id), amount: x.amount }))

/** A page of a report per contact: the balances of debtors or creditors, or the revenue or costs per contact. */
export function useContactReport(administrationId: string, kind: ContactReportKind, q: ReportQuery, page: number) {
  return useQuery({
    queryKey: ['report', kind, administrationId, q, page],
    placeholderData: keepPreviousData,
    queryFn: async (): Promise<SubjectReport> => {
      const params = { params: { path: path(administrationId), query: { ...query(q), page: String(page), per_page: String(reportPerPage) } } }
      const total = (r: Response, n: number) => Number(r.headers.get('X-Total-Count') ?? n)
      switch (kind) {
        case 'debtors': {
          const { data, error, response } = await api.GET('/api/v2/{administration_id}/reports/debtors', params)
          if (!data) throw new ApiError(response.status, error)
          return { rows: ofContacts(data.debtors), parts: (data.periods ?? []).map((p) => ({ period: p.period, rows: ofContacts(p.debtors) })), total: total(response, data.debtors.length) }
        }
        case 'creditors': {
          const { data, error, response } = await api.GET('/api/v2/{administration_id}/reports/creditors', params)
          if (!data) throw new ApiError(response.status, error)
          return { rows: ofContacts(data.creditors), parts: (data.periods ?? []).map((p) => ({ period: p.period, rows: ofContacts(p.creditors) })), total: total(response, data.creditors.length) }
        }
        case 'revenue_by_contact': {
          const { data, error, response } = await api.GET('/api/v2/{administration_id}/reports/revenue_by_contact', params)
          if (!data) throw new ApiError(response.status, error)
          return {
            rows: ofContacts(data.revenue_by_contact),
            parts: (data.periods ?? []).map((p) => ({ period: p.period, rows: ofContacts(p.revenue_by_contact) })),
            total: total(response, data.revenue_by_contact.length),
          }
        }
        case 'expenses_by_contact': {
          const { data, error, response } = await api.GET('/api/v2/{administration_id}/reports/expenses_by_contact', params)
          if (!data) throw new ApiError(response.status, error)
          return {
            rows: ofContacts(data.expenses_by_contact),
            parts: (data.periods ?? []).map((p) => ({ period: p.period, rows: ofContacts(p.expenses_by_contact) })),
            total: total(response, data.expenses_by_contact.length),
          }
        }
      }
    },
  })
}

/** A page of the revenue or the costs booked per project, the bookings without a project first. */
export function useProjectReport(administrationId: string, kind: ProjectReportKind, q: ReportQuery, page: number) {
  return useQuery({
    queryKey: ['report', kind, administrationId, q, page],
    placeholderData: keepPreviousData,
    queryFn: async (): Promise<SubjectReport> => {
      const params = { params: { path: path(administrationId), query: { ...query(q), page: String(page), per_page: String(reportPerPage) } } }
      if (kind === 'revenue_by_project') {
        const { data, error, response } = await api.GET('/api/v2/{administration_id}/reports/revenue_by_project', params)
        if (!data) throw new ApiError(response.status, error)
        return {
          rows: ofProjects(data.revenue_by_project),
          parts: (data.periods ?? []).map((p) => ({ period: p.period, rows: ofProjects(p.revenue_by_project) })),
          total: Number(response.headers.get('X-Total-Count') ?? data.revenue_by_project.length),
        }
      }
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/reports/expenses_by_project', params)
      if (!data) throw new ApiError(response.status, error)
      return {
        rows: ofProjects(data.expenses_by_project),
        parts: (data.periods ?? []).map((p) => ({ period: p.period, rows: ofProjects(p.expenses_by_project) })),
        total: Number(response.headers.get('X-Total-Count') ?? data.expenses_by_project.length),
      }
    },
  })
}

/** Each fixed asset with how its value changed in the period. */
export function useAssetsReport(administrationId: string, period: string) {
  return useQuery({
    queryKey: ['report', 'assets', administrationId, period],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/assets', { params: { path: path(administrationId), query: { period } } })),
  })
}

/** What contacts had open at the end of the month (today for this month), by age. */
export function useAgingReport(administrationId: string, kind: AgingKind, until: string, page: number) {
  return useQuery({
    queryKey: ['report', kind, administrationId, until, page],
    placeholderData: keepPreviousData,
    queryFn: async (): Promise<{ items: AgingItem[]; total: number }> => {
      const params = { params: { path: path(administrationId), query: { period_until: until, page: String(page), per_page: String(reportPerPage) } } }
      const { data, error, response } = kind === 'debtors_aging'
        ? await api.GET('/api/v2/{administration_id}/reports/debtors_aging', params)
        : await api.GET('/api/v2/{administration_id}/reports/creditors_aging', params)
      if (!response.ok || !data) {
        throw new ApiError(response.status, error)
      }
      const items = 'debtors' in data ? data.debtors : data.creditors
      return { items, total: Number(response.headers.get('X-Total-Count') ?? items.length) }
    },
  })
}

type Values = { ledger_accounts?: { ledger_account_id?: string; value?: string }[] }

/** Turns a ledger_accounts list into amounts by category id. */
export function amounts(values?: Values): Map<string, number> {
  return new Map((values?.ledger_accounts ?? []).map((v) => [v.ledger_account_id ?? '', Number(v.value ?? 0)]))
}

type BalanceLines = { ledger_account_id?: string; value?: string; children?: { ledger_account_id?: string; value?: string }[] }[] | undefined

/** The categories of a balance sheet section with their balances, the nesting left to the chart of accounts. */
export function balances(lines: BalanceLines): Map<string, number> {
  const out = new Map<string, number>()
  for (const l of lines ?? []) {
    for (const x of [l, ...(l.children ?? [])]) {
      const id = x.ledger_account_id ?? ''
      out.set(id, (out.get(id) ?? 0) + Number(x.value ?? 0))
    }
  }
  return out
}

/** The VAT report's formats, after Moneybird's: the Dutch return's boxes, the Dutch ICP declaration, or a summary per VAT rate. */
export type VatFormat = 'dutch' | 'dutch_icp' | 'overview'
export const vatFormats: VatFormat[] = ['dutch', 'dutch_icp', 'overview']

/** What a return or ICP declaration of the period would hold as the books are now. Period as YYYYMMDD..YYYYMMDD. */
export function useVatOverview(administrationId: string, kind: 'vat_return' | 'icp_declaration', period: string, enabled: boolean) {
  return useQuery({
    queryKey: ['report', 'vat_overview', administrationId, kind, period],
    placeholderData: keepPreviousData,
    enabled,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/declarations/overview', { params: { path: path(administrationId), query: { kind, period } } })),
  })
}

/** Current assets less current liabilities at the end of the period and of each part. */
export function useWorkingCapital(administrationId: string, q: ReportQuery) {
  return useQuery({
    queryKey: ['report', 'working_capital', administrationId, q],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/working_capital', { params: { path: path(administrationId), query: query(q) } })),
  })
}

/** The financial accounts' balance at the end of the period and of each part. */
export function useBankBalance(administrationId: string, q: ReportQuery) {
  return useQuery({
    queryKey: ['report', 'bank_balance', administrationId, q],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/bank_balance', { params: { path: path(administrationId), query: query(q) } })),
  })
}

export type AuditReport = components['schemas']['AuditReport']
export type AuditLine = components['schemas']['AuditLine']

/** Which categories were audited for the period, by whom, and whether they changed since. */
export function useAuditReport(administrationId: string, period: string) {
  return useQuery({
    queryKey: ['report', 'audit', administrationId, period],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/audit', { params: { path: path(administrationId), query: { period } } })),
  })
}

/** Marks a category as audited for the period, or takes the audit back. */
export function useToggleAudit(administrationId: string, period: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async ({ ledgerAccountId, audited }: { ledgerAccountId: string; audited: boolean }) => {
      const params = { params: { path: { ...path(administrationId), ledger_account_id: ledgerAccountId }, query: { period } } }
      if (audited) {
        const { error, response } = await api.DELETE('/api/v2/{administration_id}/reports/audit/{ledger_account_id}', params)
        if (!response.ok) throw new ApiError(response.status, error)
        return
      }
      await unwrap(api.POST('/api/v2/{administration_id}/reports/audit/{ledger_account_id}', params))
    },
    onSuccess: () => client.invalidateQueries({ queryKey: ['report', 'audit', administrationId, period] }),
  })
}
