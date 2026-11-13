import type { HistoryItem } from '../invoices/history'
import type { Payment, SalesInvoice } from '../invoices/queries'

type Note = NonNullable<SalesInvoice['notes']>[number]
type Event = NonNullable<SalesInvoice['events']>[number]

/** What the history of an incoming document of any type is told from. */
export type IncomingHistorySource = { created_at?: string; events?: Event[]; notes?: Note[]; payments?: unknown[] }

const ms = (iso: string) => new Date(iso).getTime()

/**
 * Everything that happened to an incoming document, newest first: its events,
 * or "Document saved" when it was made, as Moneybird's history starts; then
 * its payments and notes.
 */
export function incomingHistory(doc: IncomingHistorySource): HistoryItem[] {
  const events: HistoryItem[] = (doc.events ?? []).length
    ? (doc.events ?? []).map((e, i) => ({
        kind: 'event', id: `event-${i}`, action: e.action ?? '', at: e.created_at ?? '', userId: e.user_id ?? undefined, timed: true, data: e.data ?? {},
      }))
    : doc.created_at
      ? [{ kind: 'event', id: 'created', action: 'document_saved', at: doc.created_at, timed: true, data: {} }]
      : []
  const paid: HistoryItem[] = ((doc.payments ?? []) as Payment[]).map((p) => ({
    kind: 'payment', id: `payment-${p.id}`, at: p.created_at ?? '', userId: p.user_id ?? undefined, timed: true, payment: p,
  }))
  const notes: HistoryItem[] = (doc.notes ?? []).map((n) => ({
    kind: 'note', id: `note-${n.id}`, at: n.created_at ?? '', userId: n.user_id ?? undefined, timed: true, note: n,
  }))
  return [...events, ...paid, ...notes].map((h, i) => ({ h, i })).sort((a, b) => ms(b.h.at) - ms(a.h.at) || b.i - a.i).map(({ h }) => h)
}
