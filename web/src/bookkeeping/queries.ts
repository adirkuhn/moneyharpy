import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type BookkeepingSettings = components['schemas']['BookkeepingSettings']
export type BookkeepingChanges = components['schemas']['BookkeepingSettingsRequest']['bookkeeping_settings']
export type TaxRate = components['schemas']['TaxRate']
export type TaxRateChanges = components['schemas']['TaxRateRequest']['tax_rate']
export type TaxRateType = components['schemas']['TaxRateType']
export type LedgerAccount = components['schemas']['LedgerAccount']
export type LedgerAccountRequest = components['schemas']['LedgerAccountRequest']
export type AccountType = components['schemas']['AccountType']
export type DocumentType = components['schemas']['DocumentType']
export type Template = components['schemas']['LedgerAccountTemplate']
export type TemplateAccount = components['schemas']['LedgerAccountTemplateAccount']
export type TemplateRequest = components['schemas']['LedgerAccountTemplateRequest']
export type TaxonomyItem = components['schemas']['TaxonomyItem']

export const taxRateTypes: TaxRateType[] = ['sales_invoice', 'purchase_invoice', 'general_journal_document']
export const balanceSheetTypes: AccountType[] = ['non_current_assets', 'current_assets', 'equity', 'provisions', 'non_current_liabilities', 'current_liabilities']
export const profitAndLossTypes: AccountType[] = ['revenue', 'direct_costs', 'expenses', 'other_income_expenses']
export const documentTypes: DocumentType[] = ['sales_invoice', 'purchase_invoice', 'general_journal_document', 'financial_mutation', 'payment']

const admin = (administrationId: string) => ({ params: { path: { administration_id: administrationId } } })
const one = (administrationId: string, id: string) => ({ params: { path: { administration_id: administrationId, id } } })

export function useBookkeepingSettings(administrationId: string) {
  return useQuery({
    queryKey: ['bookkeeping-settings', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/bookkeeping_settings', admin(administrationId))),
  })
}

export function useUpdateBookkeepingSettings(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (bookkeeping_settings: BookkeepingChanges) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/bookkeeping_settings', { ...admin(administrationId), body: { bookkeeping_settings } })),
    onSuccess: (saved, changes) => {
      client.setQueryData(['bookkeeping-settings', administrationId], saved)
      if (changes.chart_scheme !== undefined) {
        void client.invalidateQueries({ queryKey: ['ledger-accounts', administrationId] })
      }
    },
  })
}

export function useTaxRates(administrationId: string) {
  return useQuery({
    queryKey: ['tax-rates', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/tax_rates', admin(administrationId))),
  })
}

export function useTaxRate(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['tax-rates', administrationId, id],
    enabled: id !== undefined,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/tax_rates/{id}', one(administrationId, id ?? ''))),
  })
}

/** Creates a tax rate when id is undefined and updates it otherwise. */
export function useSaveTaxRate(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (tax_rate: TaxRateChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/tax_rates/{id}', { ...one(administrationId, id), body: { tax_rate } }))
        : unwrap(api.POST('/api/v2/{administration_id}/tax_rates', { ...admin(administrationId), body: { tax_rate } })),
    onSettled: () => client.invalidateQueries({ queryKey: ['tax-rates', administrationId] }),
  })
}

export function useReorderTaxRates(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (body: { tax_rate_type: TaxRateType; ids: string[] }) =>
      unwrap(api.PUT('/api/v2/{administration_id}/tax_rates/order', { ...admin(administrationId), body })),
    onSettled: () => client.invalidateQueries({ queryKey: ['tax-rates', administrationId] }),
  })
}

export function useLedgerAccounts(administrationId: string) {
  return useQuery({
    queryKey: ['ledger-accounts', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/ledger_accounts', admin(administrationId))),
  })
}

/** Creates a category when id is undefined and updates it otherwise. */
export function useSaveLedgerAccount(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (body: LedgerAccountRequest) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/ledger_accounts/{id}', { ...one(administrationId, id), body }))
        : unwrap(api.POST('/api/v2/{administration_id}/ledger_accounts', { ...admin(administrationId), body })),
    onSettled: () => client.invalidateQueries({ queryKey: ['ledger-accounts', administrationId] }),
  })
}

/** Deactivates an active category and deletes an inactive one. */
export function useDeleteLedgerAccount(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/v2/{administration_id}/ledger_accounts/{id}', one(administrationId, id))),
    onSettled: () => client.invalidateQueries({ queryKey: ['ledger-accounts', administrationId] }),
  })
}

export function useUpdateScheme(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (ledger_accounts: { id: string; rgs_code: string }[]) =>
      unwrap(api.PUT('/api/v2/{administration_id}/ledger_accounts/scheme', { ...admin(administrationId), body: { ledger_accounts } })),
    onSuccess: (saved) => client.setQueryData(['ledger-accounts', administrationId], saved),
  })
}

export function useTemplates(administrationId: string) {
  return useQuery({
    queryKey: ['ledger-account-templates', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/ledger_account_templates', admin(administrationId))),
  })
}

export function useApplyTemplate(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ key, body }: { key: string; body?: TemplateRequest }) =>
      unwrap(api.POST('/api/v2/{administration_id}/ledger_account_templates/{key}', { params: { path: { administration_id: administrationId, key } }, body })),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['ledger-accounts', administrationId] })
      void client.invalidateQueries({ queryKey: ['ledger-account-templates', administrationId] })
    },
  })
}

export function useTaxonomySearch(administrationId: string, query: string, accountType?: AccountType) {
  return useQuery({
    queryKey: ['taxonomy-items', administrationId, query, accountType],
    placeholderData: keepPreviousData,
    queryFn: () =>
      unwrap(api.GET('/api/v2/{administration_id}/taxonomy_items', {
        params: { path: { administration_id: administrationId }, query: { query, account_type: accountType, limit: 20 } },
      })),
  })
}

/** Orders categories so each parent is followed by its children, with their depth. */
export function tree(accounts: LedgerAccount[]): { account: LedgerAccount; depth: number }[] {
  const ids = new Set(accounts.map((a) => a.id))
  const children = new Map<string, LedgerAccount[]>()
  for (const a of accounts) {
    const parent = a.parent_id && ids.has(String(a.parent_id)) ? String(a.parent_id) : ''
    children.set(parent, [...(children.get(parent) ?? []), a])
  }
  const out: { account: LedgerAccount; depth: number }[] = []
  const walk = (parent: string, depth: number) => {
    for (const a of children.get(parent) ?? []) {
      out.push({ account: a, depth })
      walk(a.id ?? '', depth + 1)
    }
  }
  walk('', 0)
  return out
}

/**
 * A category as an option in a select: its name, indented under its parent.
 * Moneybird shows no account number there.
 */
export const categoryOption = ({ account, depth }: { account: LedgerAccount; depth: number }) => `${'\u2003'.repeat(depth)}${account.name}`
