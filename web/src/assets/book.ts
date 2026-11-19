import type { HistoryItem } from '../invoices/history'
import type { Asset } from './queries'

/** One value change as Moneybird's table shows it: what it took off and the book value after it. */
export type BookRow = { key: string; date: string; type: string; depreciation: number; bookValue: number }

/** The rows of one year; a year before the current one has its total, as at the end of that year. */
export type BookYear = { year: string; rows: BookRow[]; total?: { date: string; depreciation: number; bookValue: number } }

/**
 * The asset's value changes newest first, grouped by year: the book value
 * runs from the calculated purchase value, and each year before thisYear is
 * summed up to its last day.
 */
export function bookRows(asset: Asset, thisYear: number): BookYear[] {
  let value = Number(asset.calculated_purchase_value ?? asset.purchase_value ?? 0)
  const ordered = (asset.value_changes ?? [])
    .map((c, i) => ({ c, i }))
    .sort((a, b) => (a.c.date ?? '').localeCompare(b.c.date ?? '') || a.i - b.i)
    .map(({ c, i }) => {
      value += Number(c.amount ?? 0)
      return { key: `${c.date}-${i}`, date: c.date ?? '', type: c.type ?? 'manual', depreciation: -Number(c.amount ?? 0), bookValue: value }
    })
  const years: BookYear[] = []
  for (const r of [...ordered].reverse()) {
    const year = r.date.slice(0, 4)
    let group = years.find((g) => g.year === year)
    if (!group) {
      group = { year, rows: [] }
      years.push(group)
    }
    group.rows.push(r)
  }
  for (const g of years) {
    if (Number(g.year) < thisYear) {
      g.total = { date: `${g.year}-12-31`, depreciation: g.rows.reduce((sum, r) => sum + r.depreciation, 0), bookValue: g.rows[0]?.bookValue ?? 0 }
    }
  }
  return years
}

/** Whole months from the commissioning date (yyyy-mm-dd) to now. */
export function monthsInUse(date: string, now: Date): number {
  const [year, month, day] = date.split('-').map(Number)
  if (!year || !month || !day) {
    return 0
  }
  const months = (now.getFullYear() - year) * 12 + (now.getMonth() + 1 - month) - (now.getDate() < day ? 1 : 0)
  return Math.max(0, months)
}

/** What happened to the asset and its notes, newest first, for the history beside it. */
export function assetHistory(asset: Asset): HistoryItem[] {
  const events: HistoryItem[] = (asset.events ?? []).map((e, i) => ({
    kind: 'event', id: `event-${i}`, action: e.action ?? '', at: e.created_at ?? '', userId: e.user_id ?? undefined, timed: true, data: e.data ?? {},
  }))
  const notes: HistoryItem[] = (asset.notes ?? []).map((n) => ({
    kind: 'note', id: `note-${n.id}`, at: n.created_at ?? '', userId: n.user_id ?? undefined, timed: true, note: n,
  }))
  const ms = (iso: string) => new Date(iso).getTime()
  return [...events, ...notes].map((h, i) => ({ h, i })).sort((a, b) => ms(b.h.at) - ms(a.h.at) || b.i - a.i).map(({ h }) => h)
}
