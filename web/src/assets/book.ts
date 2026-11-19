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

const cents = (v: string | number | null | undefined) => Math.round(Number(v ?? 0) * 100)

/**
 * The asset's value at the end of day (yyyy-mm-dd), as a divestment on that
 * day books it: the value changes up to the day, less the day's month
 * depreciated pro rata up to it when that month has no depreciation yet.
 * The server does the same when it divests.
 */
export function valueOn(asset: Asset, day: string): number {
  const changes = asset.value_changes ?? []
  let value = cents(asset.calculated_purchase_value ?? asset.purchase_value)
  for (const c of changes) {
    if ((c.date ?? '') <= day) {
      value += cents(c.amount)
    }
  }
  const plan = asset.value_change_plan
  const month = day.slice(0, 7)
  const purchase = asset.purchase_date ?? ''
  if (!plan?.active || asset.disposal || day < purchase || changes.some((c) => c.type === 'linear' && (c.date ?? '').startsWith(month))) {
    return value / 100
  }
  const [year, m, d] = day.split('-').map(Number) as [number, number, number]
  const days = new Date(Date.UTC(year, m, 0)).getUTCDate()
  let first = purchase.startsWith(month) ? Number(purchase.slice(8, 10)) : 1
  const restart = changes.filter((c) => c.type === 'arbitrary').map((c) => c.date ?? '').sort().pop()
  if (restart?.startsWith(month)) {
    first = Number(restart.slice(8, 10)) + 1
  }
  const used = d - first + 1
  if (used <= 0) {
    return value / 100
  }
  const share = Math.round((cents(plan.monthly_depreciation) * used) / days)
  return (value - Math.max(0, Math.min(share, value - cents(plan.residual_value)))) / 100
}
