import type { FrequencyType } from './queries'

/** Moneybird's frequency choices: every month, quarter, six months or year, or "Every n units". */
export type Preset = '1:month' | '1:quarter' | '6:month' | '1:year' | 'custom'
export const presets: Exclude<Preset, 'custom'>[] = ['1:month', '1:quarter', '6:month', '1:year']

export function presetOf(frequency: number, frequencyType: FrequencyType): Preset {
  const key = `${frequency}:${frequencyType}`
  return (presets as string[]).includes(key) ? (key as Preset) : 'custom'
}

export function frequencyOfPreset(preset: Preset): { frequency: number; frequencyType: FrequencyType } | null {
  if (preset === 'custom') {
    return null
  }
  const [n, type] = preset.split(':')
  return { frequency: Number(n), frequencyType: type as FrequencyType }
}

const iso = (d: Date) => `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}-${String(d.getUTCDate()).padStart(2, '0')}`

function addMonths(date: string, months: number) {
  const [y = 0, m = 1, d = 1] = date.split('-').map(Number)
  const first = new Date(Date.UTC(y, m - 1 + months, 1))
  const last = new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0)).getUTCDate()
  first.setUTCDate(Math.min(d, last))
  return iso(first)
}

function addDays(date: string, days: number) {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return iso(d)
}

/** The n-th date after anchor, as the server's Occurrence: months keep the anchor's day, clamped to the month end. */
export function occurrence(anchor: string, frequencyType: FrequencyType, every: number, n: number): string {
  const steps = every * n
  switch (frequencyType) {
    case 'day':
      return addDays(anchor, steps)
    case 'week':
      return addDays(anchor, 7 * steps)
    case 'quarter':
      return addMonths(anchor, 3 * steps)
    case 'year':
      return addMonths(anchor, 12 * steps)
    default:
      return addMonths(anchor, steps)
  }
}

type Schedule = { invoice_date?: string; frequency?: number; frequency_type?: FrequencyType; last_date?: string | null }

/** The dates of the next invoices, at most count of them, none after the last date. */
export function upcoming(s: Schedule, count: number): string[] {
  const start = s.invoice_date ?? ''
  if (!start) {
    return []
  }
  const dates: string[] = []
  for (let i = 0; i < count; i++) {
    const d = occurrence(start, s.frequency_type ?? 'month', s.frequency ?? 1, i)
    if (s.last_date && d > s.last_date) {
      break
    }
    dates.push(d)
  }
  return dates
}

/** How many invoices are still to be made until the last date, or null without one. */
export function countUntil(s: Schedule): number | null {
  if (!s.last_date || !s.invoice_date) {
    return null
  }
  let n = 0
  while (n < 10000 && occurrence(s.invoice_date, s.frequency_type ?? 'month', s.frequency ?? 1, n) <= s.last_date) {
    n++
  }
  return n
}
