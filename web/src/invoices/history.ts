/**
 * The history of a sales invoice as Moneybird's sidebar shows it: its events,
 * payments and notes, newest first, grouped by day.
 */
import type { Payment, SalesInvoice } from './queries'

type Note = NonNullable<SalesInvoice['notes']>[number]

type Base = { id: string; at: string; userId?: string; timed: boolean }
export type HistoryItem =
  | (Base & { kind: 'event'; action: string; data: Record<string, unknown> })
  | (Base & { kind: 'payment'; payment: Payment })
  | (Base & { kind: 'note'; note: Note })

/** Events whose story the invoice's payments already tell. */
const shownByPayments = new Set(['payment_registered', 'payment_linked_to_financial_mutation'])

const ms = (iso: string) => new Date(iso).getTime()
const later = (iso: string, than: number) => new Date(Math.max(ms(iso), than + 1)).toISOString()
/** Midnight in the browser's time zone, of a yyyy-mm-dd date. */
const dayStart = (date: string) => new Date(`${date}T00:00:00`).toISOString()

/**
 * What an invoice without events of its own can tell from its dates: when it
 * was created, sent and paid, in that order. Only invoices made before the
 * server kept events need this.
 */
function derived(inv: SalesInvoice, payments: Payment[]): HistoryItem[] {
  const items: HistoryItem[] = []
  const event = (action: string, at: string, timed: boolean) => items.push({ kind: 'event', id: `derived-${action}`, action, at, timed, data: {} })
  const created = inv.created_at ?? ''
  if (created) {
    event('sales_invoice_created', created, true)
  }
  let last = created ? ms(created) : 0
  if (inv.sent_at) {
    const how = { Email: 'sales_invoice_send_email', Post: 'sales_invoice_send_post' }[inv.delivery_method ?? ''] ?? 'sales_invoice_send_manually'
    const at = later(dayStart(inv.sent_at), last)
    event(how, at, false)
    last = ms(at)
  }
  if (inv.state === 'paid' && inv.paid_at) {
    const paidLast = Math.max(last, ...payments.map((p) => ms(p.created_at ?? '') || 0))
    event('sales_invoice_state_changed_to_paid', later(dayStart(inv.paid_at), paidLast), false)
  }
  return items
}

/** Everything that happened to the invoice, newest first. */
export function historyOf(inv: SalesInvoice): HistoryItem[] {
  const payments = (inv.payments ?? []) as Payment[]
  const events: HistoryItem[] = (inv.events ?? []).length
    ? (inv.events ?? [])
        .filter((e) => !shownByPayments.has(e.action ?? ''))
        .map((e, i) => ({
          kind: 'event', id: `event-${i}`, action: e.action ?? '', at: e.created_at ?? '', userId: e.user_id ?? undefined, timed: true, data: e.data ?? {},
        }))
    : derived(inv, payments)
  const paid: HistoryItem[] = payments.map((p) => ({ kind: 'payment', id: `payment-${p.id}`, at: p.created_at ?? '', userId: p.user_id ?? undefined, timed: true, payment: p }))
  const notes: HistoryItem[] = (inv.notes ?? []).map((n) => ({
    kind: 'note', id: `note-${n.id}`, at: n.created_at ?? '', userId: n.user_id ?? undefined, timed: true, note: n,
  }))
  // A stable sort keeps the order of things that happened at the same moment, newest first.
  return [...events, ...paid, ...notes].map((h, i) => ({ h, i })).sort((a, b) => ms(b.h.at) - ms(a.h.at) || b.i - a.i).map(({ h }) => h)
}

/** yyyy-mm-dd of a moment, in the browser's time zone. */
export function localDay(at: Date) {
  return `${at.getFullYear()}-${String(at.getMonth() + 1).padStart(2, '0')}-${String(at.getDate()).padStart(2, '0')}`
}

/** Items grouped by the day they happened; today and yesterday are named, other days are their date. */
export function byDay<T extends { at: string }>(items: T[], now: Date) {
  const today = localDay(now)
  const before = new Date(now)
  before.setDate(before.getDate() - 1)
  const yesterday = localDay(before)
  const days: { label: string; items: T[] }[] = []
  for (const item of items) {
    const day = localDay(new Date(item.at))
    const label = day === today ? 'today' : day === yesterday ? 'yesterday' : day
    const group = days.at(-1)
    if (group?.label === label) {
      group.items.push(item)
    } else {
      days.push({ label, items: [item] })
    }
  }
  return days
}

const sentences: Record<string, string> = {
  sales_invoice_created: 'created',
  sales_invoice_created_based_on_estimate: 'createdFromQuote',
  sales_invoice_created_based_on_recurring: 'createdFromRecurring',
  sales_invoice_created_based_on_subscription: 'createdFromSubscription',
  sales_invoice_created_from_original: 'createdFromCopy',
  credit_invoice_created_from_original: 'createdFromOriginal',
  sales_invoice_updated: 'updated',
  sales_invoice_state_changed_to_paid: 'paid',
  sales_invoice_send_email: 'sentEmail',
  sales_invoice_send_post: 'sentPost',
  sales_invoice_send_manually: 'sentManually',
  sales_invoice_send_reminder_email: 'reminderEmail',
  sales_invoice_send_reminder_manually: 'reminderManually',
  sales_invoice_paused: 'paused',
  sales_invoice_unpaused: 'resumed',
  sales_invoice_marked_as_dubious: 'dubious',
  sales_invoice_marked_as_uncollectible: 'uncollectible',
  payment_destroyed: 'paymentDeleted',
  recurring_sales_invoice_created: 'recurringCreated',
}

/** Which sentence tells an event, as a key under invoices.view.events. */
export function eventText(action: string): { key: string; state?: string; action?: string } {
  const key = sentences[action]
  if (key) {
    return { key }
  }
  const state = /^sales_invoice_state_changed_to_(.+)$/.exec(action)?.[1]
  if (state) {
    return { key: 'stateChanged', state }
  }
  return { key: 'other', action: action.replace(/_/g, ' ') }
}
