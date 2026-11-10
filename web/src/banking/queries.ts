import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../api/client'
import type { components } from '../api/schema'
import { balanceSheetTypes, type LedgerAccount, profitAndLossTypes, tree } from '../bookkeeping/queries'

export type FinancialAccount = components['schemas']['FinancialAccount']
export type FinancialAccountChanges = components['schemas']['FinancialAccountRequest']['financial_account']
export type AccountType = components['schemas']['FinancialAccountType']
export type Mutation = components['schemas']['FinancialMutation']
export type Booking = components['schemas']['LedgerAccountBooking']
export type Statement = components['schemas']['FinancialStatement']
export type MutationInput = components['schemas']['FinancialMutationInput']
export type ImportResult = components['schemas']['ImportResult']
export type BookingRule = components['schemas']['BookingRule']
export type BookingRuleChanges = components['schemas']['BookingRuleRequest']['booking_rule']
export type RuleCondition = components['schemas']['RuleCondition']

export const accountTypes: AccountType[] = ['BankAccount', 'CashAccount', 'CreditCardAccount', 'PaymentServiceAccount', 'CustomAccount']

const admin = (administrationId: string) => ({ params: { path: { administration_id: administrationId } } })
const one = (administrationId: string, id: string) => ({ params: { path: { administration_id: administrationId, id } } })

export function useBankSettings(administrationId: string) {
  return useQuery({
    queryKey: ['bank-settings', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/bank_settings', admin(administrationId))),
  })
}

export function useUpdateBankSettings(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (automatic_booking: boolean) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/bank_settings', { ...admin(administrationId), body: { bank_settings: { automatic_booking } } })),
    onSuccess: (saved) => client.setQueryData(['bank-settings', administrationId], saved),
  })
}

export function useFinancialAccounts(administrationId: string) {
  return useQuery({
    queryKey: ['financial-accounts', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/financial_accounts', admin(administrationId))),
  })
}

/** Creates an account when id is undefined and updates it otherwise. */
export function useSaveFinancialAccount(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (financial_account: FinancialAccountChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/financial_accounts/{id}', { ...one(administrationId, id), body: { financial_account } }))
        : unwrap(api.POST('/api/v2/{administration_id}/financial_accounts', { ...admin(administrationId), body: { financial_account } })),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['financial-accounts', administrationId] })
      void client.invalidateQueries({ queryKey: ['ledger-accounts', administrationId] })
    },
  })
}

export function useReorderFinancialAccounts(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (ids: string[]) => unwrap(api.PUT('/api/v2/{administration_id}/financial_accounts/order', { ...admin(administrationId), body: { ids } })),
    onSettled: () => client.invalidateQueries({ queryKey: ['financial-accounts', administrationId] }),
  })
}

/** Sends a statement file as the body, the way the API takes it. */
export function useImportStatement(administrationId: string, id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (file: File) =>
      unwrap(
        api.POST('/api/v2/{administration_id}/financial_accounts/{id}/import', {
          ...one(administrationId, id),
          body: file as unknown as string,
          bodySerializer: (body: unknown) => body as Blob,
          headers: { 'Content-Type': 'application/octet-stream' },
        }),
      ),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['financial-mutations', administrationId] })
      void client.invalidateQueries({ queryKey: ['financial-accounts', administrationId] })
    },
  })
}

/** Adds transactions typed in by hand, as one statement. */
export function useAddTransactions(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (body: { financial_account_id: string; reference: string; financial_mutations_attributes: MutationInput[] }) =>
      unwrap(api.POST('/api/v2/{administration_id}/financial_statements', { ...admin(administrationId), body: { financial_statement: { ...body, manual: true } } })),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['financial-mutations', administrationId] })
      void client.invalidateQueries({ queryKey: ['financial-accounts', administrationId] })
    },
  })
}

export function useMutations(administrationId: string, filter: string) {
  return useQuery({
    queryKey: ['financial-mutations', administrationId, filter],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/financial_mutations', { params: { path: { administration_id: administrationId }, query: { filter } } })),
  })
}

export function useMutationOne(administrationId: string, id: string) {
  return useQuery({
    queryKey: ['financial-mutations', administrationId, 'one', id],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/financial_mutations/{id}', one(administrationId, id))),
  })
}

function useMutationSaved(administrationId: string, id: string) {
  const client = useQueryClient()
  return {
    onSuccess: (saved: Mutation) => client.setQueryData(['financial-mutations', administrationId, 'one', id], saved),
    onSettled: () => client.invalidateQueries({ queryKey: ['financial-mutations', administrationId] }),
  }
}

export function useLinkBooking(administrationId: string, id: string) {
  return useMutation({
    mutationFn: (body: { booking_id: string; price_base?: string; description?: string }) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/financial_mutations/{id}/link_booking', { ...one(administrationId, id), body: { booking_type: 'LedgerAccount', ...body } })),
    ...useMutationSaved(administrationId, id),
  })
}

export function useUnlinkBooking(administrationId: string, id: string) {
  return useMutation({
    mutationFn: (bookingId: string) =>
      unwrap(api.DELETE('/api/v2/{administration_id}/financial_mutations/{id}/unlink_booking', { ...one(administrationId, id), body: { booking_type: 'LedgerAccountBooking', booking_id: bookingId } })),
    ...useMutationSaved(administrationId, id),
  })
}

export function useBookingRules(administrationId: string) {
  return useQuery({
    queryKey: ['booking-rules', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/booking_rules', admin(administrationId))),
  })
}

/** Creates a rule when id is undefined and updates it otherwise. */
export function useSaveBookingRule(administrationId: string, id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (booking_rule: BookingRuleChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/booking_rules/{id}', { ...one(administrationId, id), body: { booking_rule } }))
        : unwrap(api.POST('/api/v2/{administration_id}/booking_rules', { ...admin(administrationId), body: { booking_rule } })),
    onSettled: () => client.invalidateQueries({ queryKey: ['booking-rules', administrationId] }),
  })
}

export function useDeleteBookingRule(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/v2/{administration_id}/booking_rules/{id}', one(administrationId, id))),
    onSettled: () => client.invalidateQueries({ queryKey: ['booking-rules', administrationId] }),
  })
}

/** The categories transactions can be booked on, grouped by type and nested. */
export function bookable(accounts: LedgerAccount[]) {
  const usable = accounts.filter((a) => a.active && a.allowed_document_types?.includes('financial_mutation'))
  return [...profitAndLossTypes, ...balanceSheetTypes]
    .map((type) => ({ type, rows: tree(usable.filter((a) => a.account_type === type)) }))
    .filter((g) => g.rows.length > 0)
}

/** Formats an amount such as "-25.5" in the administration's language and currency. */
export function formatAmount(amount: string | number | null | undefined, currency: string, language: string) {
  const value = Number(amount ?? 0)
  try {
    return new Intl.NumberFormat(language, { style: 'currency', currency }).format(value)
  } catch {
    return value.toFixed(2)
  }
}
