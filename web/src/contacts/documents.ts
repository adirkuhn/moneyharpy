import { useQueries } from '@tanstack/react-query'
import { api, ApiError } from '../api/client'

/** The kinds of document a contact's feed and its "Corresponding documents" show, in that order. */
export const documentKinds = ['invoice', 'external', 'quote', 'purchase', 'receipt'] as const
export type DocumentKind = (typeof documentKinds)[number]

/** One document of a contact as its feed shows it, whatever its kind. */
export type FeedEntry = {
  kind: DocumentKind
  id: string
  /** Its number and reference, such as "2026-0001 · Project X". */
  names: string[]
  /** The descriptions of its lines. */
  lines: string[]
  amount: string
  currency: string
  state: string
  date: string
  /** Whether something still has to be done with it, such as being sent or paid. */
  open: boolean
}

export type ContactDocuments = { kind: DocumentKind; total: number; entries: FeedEntry[] }

/** How many of the newest documents of each kind the feed shows. */
const feedSize = 25

type Row = {
  id?: string | null
  reference?: string | null
  invoice_id?: string | null
  estimate_id?: string | null
  draft_id?: string | number | null
  entry_number?: number | null
  invoice_date?: string | null
  estimate_date?: string | null
  date?: string | null
  state?: string | null
  total_price_incl_tax?: string | number | null
  currency?: string | null
  details?: ({ description?: string | null } | null)[] | null
}

const openStates: Record<DocumentKind, string[]> = {
  invoice: ['draft', 'scheduled', 'open', 'pending_payment', 'late', 'reminded'],
  external: ['new', 'open', 'late'],
  quote: ['draft', 'open', 'late'],
  purchase: ['new', 'open', 'late', 'pending_payment'],
  receipt: ['new', 'open', 'late', 'pending_payment'],
}

function entryOf(kind: DocumentKind, d: Row): FeedEntry {
  const number = kind === 'quote' ? d.estimate_id : d.invoice_id
  const state = d.state ?? ''
  return {
    kind,
    id: String(d.id ?? ''),
    names: [number, d.reference].filter((n): n is string => !!n),
    lines: (d.details ?? []).map((l) => l?.description?.split('\n')[0]?.trim() ?? '').filter(Boolean),
    amount: String(d.total_price_incl_tax ?? '0'),
    currency: d.currency ?? '',
    state,
    date: d.invoice_date ?? d.estimate_date ?? d.date ?? '',
    open: openStates[kind].includes(state),
  }
}

async function fetchKind(administrationId: string, contactId: string, kind: DocumentKind): Promise<ContactDocuments> {
  const options = {
    params: {
      path: { administration_id: administrationId },
      query: { filter: `state:all,contact_id:${contactId}`, page: '1', per_page: String(feedSize) },
    },
  }
  const { data, error, response } = await (() => {
    switch (kind) {
      case 'invoice': return api.GET('/api/v2/{administration_id}/sales_invoices', options)
      case 'external': return api.GET('/api/v2/{administration_id}/external_sales_invoices', options)
      case 'quote': return api.GET('/api/v2/{administration_id}/estimates', options)
      case 'purchase': return api.GET('/api/v2/{administration_id}/documents/purchase_invoices', options)
      case 'receipt': return api.GET('/api/v2/{administration_id}/documents/receipts', options)
    }
  })()
  if (!response.ok) {
    throw new ApiError(response.status, error)
  }
  const rows = (data ?? []) as Row[]
  return { kind, total: Number(response.headers.get('X-Total-Count') ?? rows.length), entries: rows.map((r) => entryOf(kind, r)) }
}

/**
 * The newest documents of a contact of every kind, with how many there are of
 * each. A kind the user may not see, or that fails, is left out.
 */
export function useContactDocuments(administrationId: string, contactId: string) {
  return useQueries({
    queries: documentKinds.map((kind) => ({
      queryKey: ['contact-documents', administrationId, contactId, kind],
      queryFn: () => fetchKind(administrationId, contactId, kind),
    })),
    combine: (results) => ({
      documents: results.flatMap((r) => (r.data ? [r.data] : [])),
      pending: results.some((r) => r.isPending && !r.isError),
    }),
  })
}

/** Newest first; documents of the same day keep the order of their kinds. */
export function newestFirst(entries: FeedEntry[]) {
  return [...entries].sort((a, b) => b.date.localeCompare(a.date))
}

/** Where a document opens. */
export function documentPath(administrationId: string, e: Pick<FeedEntry, 'kind' | 'id'>) {
  const paths: Record<DocumentKind, string> = {
    invoice: 'sales_invoices', external: 'external_sales_invoices', quote: 'estimates', purchase: 'documents/purchase_invoices', receipt: 'documents/receipts',
  }
  return `/${administrationId}/${paths[e.kind]}/${e.id}`
}

/** The list of a kind of document filtered on the contact, as "Corresponding documents" links to it. */
export function listPath(administrationId: string, kind: DocumentKind, contactId: string) {
  if (kind === 'quote') {
    return `/${administrationId}/estimates?contact_id=${contactId}`
  }
  const paths: Record<Exclude<DocumentKind, 'quote'>, string> = {
    invoice: 'sales_invoices', external: 'external_sales_invoices', purchase: 'documents/purchase_invoices', receipt: 'documents/receipts',
  }
  return `/${administrationId}/${paths[kind]}?filter=contact_id:${contactId}`
}

/** Sales come in, purchases go out; quotes move no money yet. */
export function amountSign(kind: DocumentKind) {
  return kind === 'invoice' || kind === 'external' ? '+' : kind === 'quote' ? '' : '-'
}
