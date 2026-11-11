/**
 * The period of a list filter as a range of whole months. A month is counted
 * from year 0 (year * 12 + month, January being 0) so ranges compare as numbers.
 */
export type MonthRange = { from: number; to: number }

export const monthOf = (year: number, month: number) => year * 12 + month
export const yearOfMonth = (m: number) => Math.floor(m / 12)

/** The named periods the API knows, in the order they are preferred when writing one. */
const named = ['this_year', 'prev_year', 'this_quarter', 'prev_quarter', 'this_month', 'prev_month'] as const

function namedRange(name: string, today: Date): MonthRange | undefined {
  const now = monthOf(today.getFullYear(), today.getMonth())
  const quarter = now - (now % 3)
  const year = now - (now % 12)
  const ranges: Record<string, MonthRange> = {
    this_month: { from: now, to: now },
    prev_month: { from: now - 1, to: now - 1 },
    this_quarter: { from: quarter, to: quarter + 2 },
    prev_quarter: { from: quarter - 3, to: quarter - 1 },
    this_year: { from: year, to: year + 11 },
    prev_year: { from: year - 12, to: year - 1 },
  }
  return ranges[name]
}

const yearMonth = (s: string) => monthOf(Number(s.slice(0, 4)), Number(s.slice(4, 6)) - 1)

/** The months a period such as this_year, 202401..202403 or 20240101..20240131 covers; null without one. */
export function parsePeriod(period: string, today: Date): MonthRange | null {
  const range = namedRange(period, today)
  if (range) {
    return range
  }
  const found = /^(\d{6})(\d{2})?\.\.(\d{6})(\d{2})?$/.exec(period)
  if (!found?.[1] || !found[3]) {
    return null
  }
  return { from: yearMonth(found[1]), to: yearMonth(found[3]) }
}

const digits = (m: number) => `${yearOfMonth(m)}${String((m % 12) + 1).padStart(2, '0')}`

/** The period to filter on: a named one when it fits, a range of months otherwise; empty for any period. */
export function periodValue(range: MonthRange | null, today: Date): string {
  if (!range) {
    return ''
  }
  const name = named.find((n) => {
    const r = namedRange(n, today)
    return r?.from === range.from && r.to === range.to
  })
  return name ?? `${digits(range.from)}..${digits(range.to)}`
}

/** A range in words, such as "2025", "Apr 2025" or "Nov 2024 – Feb 2025". */
export function describePeriod(range: MonthRange, locale: string): string {
  if (range.from % 12 === 0 && range.to === range.from + 11) {
    return String(yearOfMonth(range.from))
  }
  const format = (m: number) => new Date(yearOfMonth(m), m % 12, 1).toLocaleDateString(locale, { month: 'short', year: 'numeric' })
  return range.from === range.to ? format(range.from) : `${format(range.from)} – ${format(range.to)}`
}

/** Whether a period from an address is one the lists and reports understand. */
export function isPeriod(period: string, today: Date): boolean {
  return parsePeriod(period, today) !== null
}
