import { replaceCached } from '../api/cache'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fileBody } from '../invoices/queries'
import { api, unwrap } from '../api/client'
import type { components } from '../api/schema'
import { balanceSheetTypes, type DocumentType, type LedgerAccount, profitAndLossTypes, tree } from '../bookkeeping/queries'

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
    onSuccess: (saved) => replaceCached(client, ['bank-settings', administrationId], saved),
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
/** Imports a statement file into the account of its account number, as the financial accounts page's file field does. */
export function useImportStatementFile(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (file: File) =>
      unwrap(
        api.POST('/api/v2/{administration_id}/financial_statements/import', {
          ...admin(administrationId),
          body: file as unknown as string,
          bodySerializer: (body: unknown) => body as Blob,
          headers: { 'Content-Type': 'application/octet-stream' },
        }),
      ),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['financial-mutations', administrationId] })
      void client.invalidateQueries({ queryKey: ['financial-accounts', administrationId] })
      void client.invalidateQueries({ queryKey: ['overview', administrationId] })
    },
  })
}

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
    onSuccess: (saved: Mutation) => replaceCached(client, ['financial-mutations', administrationId, 'one', id], saved),
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

/** Pays a sales or external invoice with what the transaction has open, or the amount given in its direction. */
export function useLinkInvoice(administrationId: string, id: string) {
  const client = useQueryClient()
  const saved = useMutationSaved(administrationId, id)
  return useMutation({
    mutationFn: (body: { booking_type: 'SalesInvoice' | 'ExternalSalesInvoice'; booking_id: string; price_base?: string }) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/financial_mutations/{id}/link_booking', { ...one(administrationId, id), body })),
    onSuccess: saved.onSuccess,
    onSettled: () => {
      void saved.onSettled()
      for (const key of ['sales-invoices', 'sales-invoice', 'external-invoices', 'external-invoice']) {
        void client.invalidateQueries({ queryKey: [key, administrationId] })
      }
    },
  })
}

export function useUnlinkPayment(administrationId: string, id: string) {
  const client = useQueryClient()
  const saved = useMutationSaved(administrationId, id)
  return useMutation({
    mutationFn: (paymentId: string) =>
      unwrap(api.DELETE('/api/v2/{administration_id}/financial_mutations/{id}/unlink_booking', { ...one(administrationId, id), body: { booking_type: 'Payment', booking_id: paymentId } })),
    onSuccess: saved.onSuccess,
    onSettled: () => {
      void saved.onSettled()
      void client.invalidateQueries({ queryKey: ['sales-invoice', administrationId] })
      void client.invalidateQueries({ queryKey: ['external-invoice', administrationId] })
    },
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
export function bookable(accounts: LedgerAccount[], documentType: DocumentType = 'financial_mutation') {
  // Invoices settle the debtors and creditors; journal entries cannot.
  const settled = documentType === 'general_journal_document' ? ['accounts_receivable', 'accounts_payable'] : []
  const usable = accounts.filter((a) => a.active && a.allowed_document_types?.includes(documentType) && !settled.includes(a.system_role ?? ''))
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

export type LinkSuggestion = components['schemas']['LinkSuggestion']
export type LinkScope = 'all' | 'sales_invoices' | 'documents' | 'ledger_accounts'

/** What the transaction can be linked to, best match first, as Moneybird suggests when linking. */
export function useLinkSuggestions(administrationId: string, id: string, query: string, scope: LinkScope, enabled = true) {
  return useQuery({
    queryKey: ['financial-mutations', administrationId, 'suggestions', id, scope, query],
    enabled,
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/financial_mutations/{id}/link_suggestions', {
      ...one(administrationId, id), params: { ...one(administrationId, id).params, query: { ...(query ? { query } : {}), scope } },
    })),
  })
}

type LinkBody = {
  booking_type: LinkSuggestion['booking_type'] | 'NewPurchaseInvoice' | 'NewReceipt'
  booking_id?: string
  price_base?: string
  description?: string
}

/** Links (part of) the transaction to anything: an invoice, a document, a category or a new purchase invoice or receipt. */
export function useLink(administrationId: string, id: string) {
  const client = useQueryClient()
  const saved = useMutationSaved(administrationId, id)
  return useMutation({
    mutationFn: (body: LinkBody) => unwrap(api.PATCH('/api/v2/{administration_id}/financial_mutations/{id}/link_booking', { ...one(administrationId, id), body })),
    onSuccess: saved.onSuccess,
    onSettled: () => {
      void saved.onSettled()
      for (const key of ['sales-invoices', 'sales-invoice', 'external-invoices', 'external-invoice', 'documents', 'overview']) {
        void client.invalidateQueries({ queryKey: [key, administrationId] })
      }
    },
  })
}

/**
 * Moneybird's Upload a document on a transaction: a new receipt for what
 * is open, paid by the transaction, with the file attached.
 */
export function useUploadForTransaction(administrationId: string, id: string) {
  const client = useQueryClient()
  const saved = useMutationSaved(administrationId, id)
  return useMutation({
    mutationFn: async (file: File) => {
      const before = await unwrap(api.GET('/api/v2/{administration_id}/financial_mutations/{id}', one(administrationId, id)))
      const known = new Set((before.payments ?? []).map((p) => p.id))
      const linked = await unwrap(api.PATCH('/api/v2/{administration_id}/financial_mutations/{id}/link_booking', { ...one(administrationId, id), body: { booking_type: 'NewReceipt' } }))
      const made = (linked.payments ?? []).find((p) => !known.has(p.id))
      if (made?.invoice_id) {
        await unwrap(api.POST('/api/v2/{administration_id}/documents/receipts/{id}/attachments', { ...one(administrationId, made.invoice_id), ...fileBody(file) }))
      }
      return linked
    },
    onSuccess: saved.onSuccess,
    onSettled: () => {
      void saved.onSettled()
      void client.invalidateQueries({ queryKey: ['documents', administrationId] })
    },
  })
}

export type PurchaseTransaction = components['schemas']['PurchaseTransaction']

/** The outgoing payments the filter selects (Moneybird's purchase transactions). */
export function usePurchaseTransactions(administrationId: string, filter: string) {
  return useQuery({
    queryKey: ['purchase-transactions', administrationId, filter],
    placeholderData: keepPreviousData,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/purchase_transactions', { params: { path: { administration_id: administrationId }, query: { filter, per_page: '100' } } })),
  })
}

function usePaymentsChanged(administrationId: string) {
  const client = useQueryClient()
  return () => {
    for (const key of ['purchase-transactions', 'documents', 'incoming']) {
      void client.invalidateQueries({ queryKey: [key, administrationId] })
    }
  }
}

/** Sets up a bank transfer paying a purchase invoice or receipt, from a bank account, to send in a batch. */
export function useCreatePurchaseTransaction(administrationId: string) {
  return useMutation({
    mutationFn: (body: { payable_id: string; financial_account_id: string }) =>
      unwrap(api.POST('/api/v2/{administration_id}/purchase_transactions', { ...admin(administrationId), body: { purchase_transaction: { ...body, payable_type: 'Document' } } })),
    onSettled: usePaymentsChanged(administrationId),
  })
}

/** Groups payments of an account in a batch, to download as one SEPA credit transfer file. */
export function useCreatePaymentBatch(administrationId: string) {
  return useMutation({
    mutationFn: (body: { financial_account_id: string; purchase_transaction_ids: string[] }) =>
      unwrap(api.POST('/api/v2/{administration_id}/purchase_transaction_batches', { ...admin(administrationId), body: { purchase_transaction_batch: body } })),
    onSettled: usePaymentsChanged(administrationId),
  })
}

/** Removes a payment that was not sent yet. */
export function useDeletePurchaseTransaction(administrationId: string) {
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE('/api/v2/{administration_id}/purchase_transactions/{id}', one(administrationId, id))),
    onSettled: usePaymentsChanged(administrationId),
  })
}

/** Where the browser downloads a batch as a SEPA credit transfer file; the session cookie signs the request. */
export function paymentBatchURL(administrationId: string, id: string) {
  return `/api/v2/${administrationId}/purchase_transaction_batches/${id}/download`
}

export type BankConnection = components['schemas']['BankConnection']
export type ConnectableBank = components['schemas']['ConnectableBank']

export function useBankConnections(administrationId: string) {
  return useQuery({
    queryKey: ['bank-connections', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/bank_connections', admin(administrationId))),
  })
}

/** The banks that can be connected; asked only once the user wants to connect one. */
export function useConnectableBanks(administrationId: string, enabled: boolean) {
  return useQuery({
    queryKey: ['connectable-banks', administrationId],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/bank_connections/banks', { params: { path: { administration_id: administrationId }, query: { country: 'NL' } } })),
    enabled,
    retry: false,
    staleTime: 60 * 60 * 1000,
  })
}

/** Sends the browser to the bank, where the user authorises the connection; tests replace it. */
export const bankRedirect = { go: (url: string) => window.location.assign(url) }
const goToBank = (url: string) => bankRedirect.go(url)

export function useConnectBank(administrationId: string) {
  return useMutation({
    mutationFn: (bank: ConnectableBank) =>
      unwrap(api.POST('/api/v2/{administration_id}/bank_connections', { ...admin(administrationId), body: { bank: bank.name, country: bank.country } })),
    onSuccess: (started) => goToBank(started.authorization_url),
  })
}

export function useReconnectBank(administrationId: string) {
  return useMutation({
    mutationFn: (id: string) => unwrap(api.POST('/api/v2/{administration_id}/bank_connections/{id}/reconnect', one(administrationId, id))),
    onSuccess: (started) => goToBank(started.authorization_url),
  })
}

function useConnectionChange<T, V = string>(administrationId: string, fn: (v: V) => Promise<T>) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['bank-connections', administrationId] })
      void client.invalidateQueries({ queryKey: ['financial-accounts', administrationId] })
      void client.invalidateQueries({ queryKey: ['financial-mutations', administrationId] })
      void client.invalidateQueries({ queryKey: ['overview', administrationId] })
    },
  })
}

export function useSyncBankConnection(administrationId: string) {
  return useConnectionChange(administrationId, (id) => unwrap(api.POST('/api/v2/{administration_id}/bank_connections/{id}/sync', one(administrationId, id))))
}

export function useDisconnectBank(administrationId: string) {
  return useConnectionChange(administrationId, (id) => unwrap(api.DELETE('/api/v2/{administration_id}/bank_connections/{id}', one(administrationId, id))))
}

/** Completes the connection the bank sent the user back from. */
export function useCompleteBankConnection() {
  return useMutation({
    mutationFn: (body: { state: string; code: string }) => unwrap(api.POST('/api/bank_connections/complete', { body })),
  })
}

/** Connects Revolut Business with a read-only API key. */
export function useConnectWithKey(administrationId: string) {
  return useConnectionChange(administrationId, (apiKey: string) =>
    unwrap(api.POST('/api/v2/{administration_id}/bank_connections/api_key', { ...admin(administrationId), body: { provider: 'revolut', api_key: apiKey } })),
  )
}

export function useReplaceKey(administrationId: string) {
  return useConnectionChange(administrationId, ({ id, apiKey }: { id: string; apiKey: string }) =>
    unwrap(api.PUT('/api/v2/{administration_id}/bank_connections/{id}/api_key', { ...one(administrationId, id), body: { api_key: apiKey } })),
  )
}
