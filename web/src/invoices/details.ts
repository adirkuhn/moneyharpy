/**
 * The rules of Moneybird's invoice details editor and "Add invoice" popup that
 * do not depend on the screen: reordering, periods, quantities and numbers.
 */
import { documentDate } from './paper'

/** The rows with the one at `from` moved to `to`; out of range leaves them as they are. */
export function moveRow<T>(rows: T[], from: number, to: number): T[] {
  if (from === to || from < 0 || to < 0 || from >= rows.length || to >= rows.length) {
    return rows
  }
  const next = [...rows]
  const [row] = next.splice(from, 1)
  next.splice(to, 0, row as T)
  return next
}

/** "Apply to details below": the change given to every row after `index`. */
export function applyBelow<T extends object>(rows: T[], index: number, change: Partial<T>): T[] {
  return rows.map((r, i) => (i > index ? { ...r, ...change } : r))
}

export type PeriodPreset = 'today' | 'this_month' | 'previous_month' | 'next_month'
export const periodPresets: PeriodPreset[] = ['today', 'this_month', 'previous_month', 'next_month']

const compact = (iso: string) => iso.replaceAll('-', '')
const iso = (y: number, m: number, d: number) => `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
/** The first and last day of the month `offset` months from the given yyyy-mm-dd day. */
function month(day: string, offset: number): [string, string] {
  const first = new Date(Date.UTC(Number(day.slice(0, 4)), Number(day.slice(5, 7)) - 1 + offset, 1))
  const last = new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0))
  return [iso(first.getUTCFullYear(), first.getUTCMonth() + 1, 1), iso(last.getUTCFullYear(), last.getUTCMonth() + 1, last.getUTCDate())]
}

/** A preset of the period selector as the detail stores it, yyyymmdd..yyyymmdd. */
export function presetPeriod(preset: PeriodPreset, today: string): string {
  const offsets: Record<Exclude<PeriodPreset, 'today'>, number> = { this_month: 0, previous_month: -1, next_month: 1 }
  const [from, to] = preset === 'today' ? [today, today] : month(today, offsets[preset])
  return toPeriod(from, to)
}

/** A stored period as two yyyy-mm-dd days; null when there is none. */
export function periodRange(period: string | null | undefined): { from: string; to: string } | null {
  const m = /^(\d{4})(\d{2})(\d{2})\.\.(\d{4})(\d{2})(\d{2})$/.exec(period ?? '')
  return m ? { from: iso(Number(m[1]), Number(m[2]), Number(m[3])), to: iso(Number(m[4]), Number(m[5]), Number(m[6])) } : null
}

/** Two yyyy-mm-dd days as the detail stores them, earliest first; empty unless both are given. */
export function toPeriod(from: string, to: string): string {
  if (!from || !to) {
    return ''
  }
  const [a, b] = from <= to ? [from, to] : [to, from]
  return `${compact(a)}..${compact(b)}`
}

/** A stored period as the user reads it, such as 01-11-2026 – 30-11-2026. */
export function periodText(period: string | null | undefined, language: string): string {
  const range = periodRange(period)
  return range ? `${documentDate(range.from, language)} – ${documentDate(range.to, language)}` : ''
}

/** A quantity as the document prints it: a bare number gets " x", as Moneybird writes "2 x". */
export function quantityLabel(amount: string): string {
  const a = amount.trim()
  if (a === '') {
    return '1 x'
  }
  return /^-?\d+(?:[.,]\d+)?$/.test(a) ? `${a} x` : a
}

/** The next invoice number around its sequence number, as the popup shows "2026-" before "0001". */
export function numberParts(example: string, sequence: string): { prefix: string; suffix: string } {
  const at = sequence ? example.lastIndexOf(sequence) : -1
  return at < 0 ? { prefix: '', suffix: '' } : { prefix: example.slice(0, at), suffix: example.slice(at + sequence.length) }
}

/** The EU countries a company needs a VAT id in, when invoiced from another country, as Moneybird lists them. */
export const euVatCountries = ['BE', 'BG', 'CZ', 'DK', 'DE', 'EE', 'IE', 'GR', 'ES', 'FR', 'HR', 'IT', 'CY', 'LV', 'LT', 'LU', 'HU', 'MT', 'AT', 'PL', 'PT', 'RO', 'SI', 'SK', 'FI', 'SE', 'XI']

/** Whether the quick contact form asks a company's VAT id: only for another EU country. */
export function needsVatId(country: string, home: string): boolean {
  return country !== home && euVatCountries.includes(country)
}
