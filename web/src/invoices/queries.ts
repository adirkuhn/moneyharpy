import type { TFunction } from 'i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'

export type SalesInvoice = components['schemas']['SalesInvoice']
export type SalesInvoiceChanges = components['schemas']['SalesInvoiceRequest']['sales_invoice']
export type Sending = NonNullable<components['schemas']['SendSalesInvoiceRequest']['sales_invoice_sending']>
export type InvoiceState = NonNullable<SalesInvoice['state']>
export type Payment = components['schemas']['Payment']
export type PaymentChanges = components['schemas']['PaymentRequest']['payment']
export type Reminder = components['schemas']['SendRemindersRequest']['sales_invoice_reminders'][number]
export type PaymentAction = NonNullable<PaymentChanges['manual_payment_action']>

export const perPage = 50

/** Sales invoices are made in MoneyHarpy; external ones in other software. */
export type InvoiceKind = 'sales' | 'external'

/** What the payment, write-off, note and attachment panels need of an invoice of either kind. */
export type Collectable = Pick<SalesInvoice, 'id' | 'currency' | 'payments' | 'total_unpaid' | 'total_price_incl_tax' | 'notes' | 'attachments' | 'marked_dubious_on'> & {
  state?: string
  original_sales_invoice_id?: SalesInvoice['original_sales_invoice_id']
}

/** The query keys of one invoice and of the lists, per kind. */
export const invoiceKeys = (kind: InvoiceKind) =>
  kind === 'external' ? { one: 'external-invoice', list: 'external-invoices' } : { one: 'sales-invoice', list: 'sales-invoices' }

export const states: InvoiceState[] = ['draft', 'scheduled', 'open', 'pending_payment', 'late', 'reminded', 'paid', 'uncollectible']
export const unpaidStates = 'open|late|reminded|pending_payment'

/** Sent invoices have a number and are booked. */
export const isSent = (state: string | undefined) => state !== 'draft' && state !== 'scheduled'
/** Paid and uncollectible invoices are finished and no longer change. */
export const isClosed = (state: string | undefined) => state === 'paid' || state === 'uncollectible'
/** Sent invoices waiting for their money take payments and reminders. */
export const isPayable = (state: string | undefined) => state === 'open' || state === 'late' || state === 'reminded' || state === 'pending_payment'
/** Overdue invoices, which the overview counts as to remind. */
export const overdueStates = 'late|reminded'

/** The ways to register a payment by hand, as the payment form offers them. */
export const paymentActions: PaymentAction[] = ['bank_transfer', 'cash_payment', 'private_payment', 'payment_without_proof', 'rounding_error']

/** Today in the browser's time zone, as yyyy-mm-dd. */
export function today() {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

const admin = (administrationId: string) => ({ administration_id: administrationId })
const one = (administrationId: string, id: string) => ({ params: { path: { ...admin(administrationId), id } } })

/** One page of invoices for a filter such as "state:draft,period:this_year". */
export function useSalesInvoices(administrationId: string, filter: string, page: number) {
  return useQuery({
    queryKey: ['sales-invoices', administrationId, filter, page],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/sales_invoices', {
        params: { path: admin(administrationId), query: { filter, page: String(page), per_page: String(perPage) } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return { invoices: data ?? [], total: Number(response.headers.get('X-Total-Count') ?? data?.length ?? 0) }
    },
  })
}

export function useSalesInvoice(administrationId: string, id?: string) {
  return useQuery({
    queryKey: ['sales-invoice', administrationId, id],
    enabled: !!id,
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/sales_invoices/{id}', one(administrationId, id ?? ''))),
  })
}

/** The draft number the next new invoice gets, shown as "Concept #n" while adding one. */
export function useNextDraftId(administrationId: string, enabled: boolean) {
  return useQuery({
    queryKey: ['sales-invoice-next-draft-id', administrationId],
    enabled,
    // Another invoice may have taken it since, so it is asked for afresh every time.
    gcTime: 0,
    queryFn: async () => (await unwrap(api.GET('/api/v2/{administration_id}/sales_invoices/next_draft_id', { params: { path: admin(administrationId) } }))).draft_id,
  })
}

/** Keeps the invoice the server answered with and has lists load again. */
export function useSaved(administrationId: string, kind: InvoiceKind = 'sales') {
  const client = useQueryClient()
  const keys = invoiceKeys(kind)
  return (saved?: { id?: string }) => {
    if (saved?.id) {
      client.setQueryData([keys.one, administrationId, saved.id], saved)
    }
    void client.invalidateQueries({ queryKey: [keys.list, administrationId] })
  }
}

function useRefetch(administrationId: string, id: string, kind: InvoiceKind = 'sales') {
  const client = useQueryClient()
  const keys = invoiceKeys(kind)
  return () => {
    void client.invalidateQueries({ queryKey: [keys.one, administrationId, id] })
    void client.invalidateQueries({ queryKey: [keys.list, administrationId] })
  }
}

/** Creates a draft when id is undefined and updates the invoice otherwise. */
export function useSaveSalesInvoice(administrationId: string, id?: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: (sales_invoice: SalesInvoiceChanges) =>
      id
        ? unwrap(api.PATCH('/api/v2/{administration_id}/sales_invoices/{id}', { ...one(administrationId, id), body: { sales_invoice } }))
        : unwrap(api.POST('/api/v2/{administration_id}/sales_invoices', { params: { path: admin(administrationId) }, body: { sales_invoice } })),
    onSuccess: saved,
  })
}

export function useDeleteSalesInvoice(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/v2/{administration_id}/sales_invoices/{id}', one(administrationId, id))),
    onSuccess: () => saved(),
  })
}

export function useSendSalesInvoice(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  const refetch = useRefetch(administrationId, id)
  return useMutation({
    mutationFn: (sales_invoice_sending: Sending) =>
      unwrap(api.PATCH('/api/v2/{administration_id}/sales_invoices/{id}/send_invoice', { ...one(administrationId, id), body: { sales_invoice_sending } })),
    onSuccess: saved,
    // A failed email still books the invoice, so it is loaded again.
    onError: refetch,
  })
}

export function useCreditSalesInvoice(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: () => unwrap(api.PATCH('/api/v2/{administration_id}/sales_invoices/{id}/duplicate_creditinvoice', { ...one(administrationId, id), body: {} })),
    onSuccess: saved,
  })
}

/**
 * What a copy of an invoice starts with, as Moneybird's "Duplicate invoice":
 * the contact, settings and lines, as a new draft.
 */
export function duplicateOf(inv: SalesInvoice): SalesInvoiceChanges {
  return {
    contact_id: inv.contact_id ?? '',
    reference: inv.reference ?? '',
    ...(inv.workflow_id ? { workflow_id: inv.workflow_id } : {}),
    ...(inv.document_style_id ? { document_style_id: inv.document_style_id } : {}),
    prices_are_incl_tax: inv.prices_are_incl_tax ?? false,
    ...(Number(inv.discount ?? 0) ? { discount: String(inv.discount) } : {}),
    payment_conditions: inv.payment_conditions ?? '',
    details_attributes: [...(inv.details ?? [])]
      .sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
      .map((d, i) => ({
        description: d.description ?? '',
        amount: d.amount ?? '1',
        price: String(d.price ?? '0'),
        tax_rate_id: d.tax_rate_id ?? '',
        ...(d.ledger_account_id ? { ledger_account_id: d.ledger_account_id } : {}),
        ...(d.product_id ? { product_id: String(d.product_id) } : {}),
        ...(d.period ? { period: d.period } : {}),
        row_order: i + 1,
      })),
  }
}

export function useDuplicateSalesInvoice(administrationId: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: (inv: SalesInvoice) =>
      unwrap(api.POST('/api/v2/{administration_id}/sales_invoices', { params: { path: admin(administrationId) }, body: { sales_invoice: duplicateOf(inv) } })),
    onSuccess: saved,
  })
}

export function usePauseSalesInvoice(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  return useMutation({
    mutationFn: (pause: boolean) =>
      pause
        ? unwrap(api.POST('/api/v2/{administration_id}/sales_invoices/{id}/pause', one(administrationId, id)))
        : unwrap(api.POST('/api/v2/{administration_id}/sales_invoices/{id}/resume', one(administrationId, id))),
    onSuccess: saved,
  })
}

export function useRegisterPayment(administrationId: string, id: string, kind: InvoiceKind = 'sales') {
  const refetch = useRefetch(administrationId, id, kind)
  return useMutation({
    mutationFn: (payment: PaymentChanges) =>
      kind === 'external'
        ? unwrap(api.POST('/api/v2/{administration_id}/external_sales_invoices/{id}/payments', { ...one(administrationId, id), body: { payment } }))
        : unwrap(api.POST('/api/v2/{administration_id}/sales_invoices/{id}/payments', { ...one(administrationId, id), body: { payment } })),
    onSuccess: refetch,
  })
}

export function useDeletePayment(administrationId: string, id: string, kind: InvoiceKind = 'sales') {
  const refetch = useRefetch(administrationId, id, kind)
  return useMutation({
    mutationFn: (paymentId: string) => {
      const params = { params: { path: { ...admin(administrationId), id, payment_id: paymentId } } }
      return kind === 'external'
        ? unwrap(api.DELETE('/api/v2/{administration_id}/external_sales_invoices/{id}/payments/{payment_id}', params))
        : unwrap(api.DELETE('/api/v2/{administration_id}/sales_invoices/{id}/payments/{payment_id}', params))
    },
    onSuccess: refetch,
  })
}

/** Settles a credit invoice with the invoice it credits. */
export function useSettleCredit(administrationId: string, id: string) {
  const saved = useSaved(administrationId)
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.PATCH('/api/v2/{administration_id}/sales_invoices/{id}/register_payment_creditinvoice', { ...one(administrationId, id), body: {} })),
    onSuccess: (inv) => {
      saved(inv)
      void client.invalidateQueries({ queryKey: ['sales-invoice', administrationId] })
    },
  })
}

export type WriteOff = { kind: 'dubious' | 'uncollectible'; date: string }

/** Marks an invoice dubious or, for good, uncollectible. */
export function useWriteOff(administrationId: string, id: string, invoiceKind: InvoiceKind = 'sales') {
  const saved = useSaved(administrationId, invoiceKind)
  return useMutation({
    mutationFn: ({ kind, date }: WriteOff): Promise<{ id?: string }> => {
      const path = one(administrationId, id)
      if (invoiceKind === 'external') {
        return kind === 'dubious'
          ? unwrap(api.PATCH('/api/v2/{administration_id}/external_sales_invoices/{id}/mark_as_dubious', { ...path, body: { dubious_date: date } }))
          : unwrap(api.PATCH('/api/v2/{administration_id}/external_sales_invoices/{id}/mark_as_uncollectible', { ...path, body: { uncollectible_date: date, book_method: 'revenue' } }))
      }
      return kind === 'dubious'
        ? unwrap(api.PATCH('/api/v2/{administration_id}/sales_invoices/{id}/mark_as_dubious', { ...path, body: { dubious_date: date } }))
        : unwrap(api.PATCH('/api/v2/{administration_id}/sales_invoices/{id}/mark_as_uncollectible', { ...path, body: { uncollectible_date: date, book_method: 'revenue' } }))
    },
    onSuccess: saved,
  })
}

/** The reminders that cover invoices: one per contact, workflow, document style and identity, as Moneybird groups them. */
export function remindersFor(invoices: SalesInvoice[], extra: Partial<Reminder> = {}): Reminder[] {
  const groups = new Map<string, Reminder>()
  for (const inv of invoices) {
    const key = [inv.contact_id, inv.workflow_id, inv.document_style_id, inv.identity_id].join('/')
    const group = groups.get(key) ?? {
      contact_id: inv.contact_id ?? '', workflow_id: inv.workflow_id ?? '', document_style_id: inv.document_style_id ?? '', identity_id: inv.identity_id ?? '',
      sales_invoice_ids: [], ...extra,
    }
    group.sales_invoice_ids.push(inv.id ?? '')
    groups.set(key, group)
  }
  return [...groups.values()]
}

export function useSendReminders(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (sales_invoice_reminders: Reminder[]) =>
      unwrap(api.POST('/api/v2/{administration_id}/sales_invoices/send_reminders', { params: { path: admin(administrationId) }, body: { sales_invoice_reminders } })),
    onSuccess: (list) => {
      for (const inv of list) {
        client.setQueryData(['sales-invoice', administrationId, inv.id], inv)
      }
      void client.invalidateQueries({ queryKey: ['sales-invoices', administrationId] })
    },
  })
}

/** Registers full payments of several invoices dated today, one request each; it answers with the ones that failed. */
export function useRegisterFullPayments(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (invoices: SalesInvoice[]) => {
      const failed: SalesInvoice[] = []
      for (const inv of invoices) {
        const { response } = await api.POST('/api/v2/{administration_id}/sales_invoices/{id}/payments', {
          ...one(administrationId, inv.id ?? ''), body: { payment: { payment_date: today(), price: inv.total_unpaid ?? '0' } },
        })
        if (!response.ok) {
          failed.push(inv)
        }
      }
      return failed
    },
    onSettled: () => client.invalidateQueries({ queryKey: ['sales-invoices', administrationId] }),
  })
}

/** Deletes several drafts, one request each; it answers with the ones that failed. */
export function useDeleteDrafts(administrationId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (invoices: SalesInvoice[]) => {
      const failed: SalesInvoice[] = []
      for (const inv of invoices) {
        const { response } = await api.DELETE('/api/v2/{administration_id}/sales_invoices/{id}', one(administrationId, inv.id ?? ''))
        if (!response.ok) {
          failed.push(inv)
        }
      }
      return failed
    },
    onSettled: () => client.invalidateQueries({ queryKey: ['sales-invoices', administrationId] }),
  })
}

export function useAddInvoiceNote(administrationId: string, id: string, kind: InvoiceKind = 'sales') {
  const refetch = useRefetch(administrationId, id, kind)
  return useMutation({
    mutationFn: (note: { note: string; todo: boolean; assignee_id?: string }) =>
      kind === 'external'
        ? unwrap(api.POST('/api/v2/{administration_id}/external_sales_invoices/{id}/notes', { ...one(administrationId, id), body: { note } }))
        : unwrap(api.POST('/api/v2/{administration_id}/sales_invoices/{id}/notes', { ...one(administrationId, id), body: { note } })),
    onSuccess: refetch,
  })
}

export function useDeleteInvoiceNote(administrationId: string, id: string, kind: InvoiceKind = 'sales') {
  const refetch = useRefetch(administrationId, id, kind)
  return useMutation({
    mutationFn: (noteId: string) => {
      const params = { params: { path: { ...admin(administrationId), id, note_id: noteId } } }
      return kind === 'external'
        ? unwrap(api.DELETE('/api/v2/{administration_id}/external_sales_invoices/{id}/notes/{note_id}', params))
        : unwrap(api.DELETE('/api/v2/{administration_id}/sales_invoices/{id}/notes/{note_id}', params))
    },
    onSuccess: refetch,
  })
}

/** A file as the multipart/form-data body the API takes, in the field "file". */
export function fileBody(file: File) {
  const form = new FormData()
  form.append('file', file)
  return { body: form as unknown as { file: string }, bodySerializer: (body: unknown) => body as FormData }
}

/** Uploads a file to the invoice. */
export function useAddInvoiceAttachment(administrationId: string, id: string, kind: InvoiceKind = 'sales') {
  const refetch = useRefetch(administrationId, id, kind)
  return useMutation({
    mutationFn: (file: File) =>
      kind === 'external'
        ? unwrap(api.POST('/api/v2/{administration_id}/external_sales_invoices/{id}/attachment', { ...one(administrationId, id), ...fileBody(file) }))
        : unwrap(api.POST('/api/v2/{administration_id}/sales_invoices/{id}/attachments', { ...one(administrationId, id), ...fileBody(file) })),
    onSuccess: refetch,
  })
}

export function useDeleteInvoiceAttachment(administrationId: string, id: string, kind: InvoiceKind = 'sales') {
  const refetch = useRefetch(administrationId, id, kind)
  return useMutation({
    mutationFn: (attachmentId: string) => {
      const params = { params: { path: { ...admin(administrationId), id, attachment_id: attachmentId } } }
      return kind === 'external'
        ? unwrap(api.DELETE('/api/v2/{administration_id}/external_sales_invoices/{id}/attachments/{attachment_id}', params))
        : unwrap(api.DELETE('/api/v2/{administration_id}/sales_invoices/{id}/attachments/{attachment_id}', params))
    },
    onSuccess: refetch,
  })
}

/** Where the browser downloads one of the invoice's files; the session cookie signs the request. */
export function invoiceFileURL(administrationId: string, id: string, file: 'download_pdf' | 'download_ubl' | 'download_packing_slip_pdf') {
  return `/api/v2/${administrationId}/sales_invoices/${id}/${file}`
}

export function attachmentURL(administrationId: string, id: string, attachmentId: string, kind: InvoiceKind = 'sales') {
  const path = kind === 'external' ? 'external_sales_invoices' : 'sales_invoices'
  return `/api/v2/${administrationId}/${path}/${id}/attachments/${attachmentId}/download`
}

/** What the invoice is called: its number once sent, "Draft N" before. */
export function invoiceTitle(invoice: Pick<SalesInvoice, 'invoice_id' | 'draft_id'>, t: TFunction) {
  return invoice.invoice_id || t('invoices.draftTitle', { id: invoice.draft_id ?? '' })
}

/** The filter of the list kept in the address: a state, a period and other terms as they are. */
export type ListFilter = { state: string; period: string; rest: string[] }

export function parseFilter(filter: string | null): ListFilter {
  if (filter === null) {
    return { state: '', period: 'this_year', rest: [] }
  }
  const out: ListFilter = { state: '', period: '', rest: [] }
  for (const term of filter.split(',').map((s) => s.trim()).filter(Boolean)) {
    const [key, value = ''] = term.split(/:(.*)/s)
    if (key === 'state' && value !== 'all') {
      out.state = value
    } else if (key === 'period') {
      out.period = value
    } else if (key !== 'state') {
      out.rest.push(term)
    }
  }
  return out
}

/** The filter the API takes; without state or period it asks for everything. */
export function filterTerms({ state, period, rest }: ListFilter) {
  const terms = [state && `state:${state}`, period && `period:${period}`, ...rest].filter(Boolean)
  return terms.length > 0 ? terms.join(',') : 'state:all'
}

/** The value of a term other than state and period, such as contact_id; empty without it. */
export function termOf(rest: string[], key: string) {
  const term = rest.find((r) => r.startsWith(`${key}:`))
  return term ? term.slice(key.length + 1) : ''
}

/** The other terms with one set to a value, or left out when the value is empty. */
export function withTerm(rest: string[], key: string, value: string) {
  const others = rest.filter((r) => !r.startsWith(`${key}:`))
  return value ? [...others, `${key}:${value}`] : others
}
