/**
 * Calendar arithmetic for the date picker. Days are ISO strings
 * (YYYY-MM-DD), the form the API takes, so they compare as text.
 */

const pattern = /^(\d{4})-(\d{2})-(\d{2})$/

/** The day an ISO string names, as UTC midnight, or null when it names none. */
function dateOf(day: string): Date | null {
  const m = pattern.exec(day)
  if (!m) {
    return null
  }
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])]
  const date = new Date(Date.UTC(y, mo - 1, d))
  return date.getUTCFullYear() === y && date.getUTCMonth() === mo - 1 && date.getUTCDate() === d ? date : null
}

function isoOf(date: Date): string {
  return date.toISOString().slice(0, 10)
}

/** Whether the text is a whole, existing day in YYYY-MM-DD. */
export function isDay(text: string): boolean {
  return dateOf(text) !== null
}

/** The day `n` days after (or before) the given one. */
export function shiftDay(day: string, n: number): string {
  const date = dateOf(day)!
  date.setUTCDate(date.getUTCDate() + n)
  return isoOf(date)
}

/** The same day `n` months on, kept within the shorter month. */
export function shiftMonth(day: string, n: number): string {
  const date = dateOf(day)!
  const target = new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth() + n, 1))
  const last = new Date(Date.UTC(target.getUTCFullYear(), target.getUTCMonth() + 1, 0)).getUTCDate()
  target.setUTCDate(Math.min(date.getUTCDate(), last))
  return isoOf(target)
}

/** Monday 0 … Sunday 6. */
export function weekdayOf(day: string): number {
  return (dateOf(day)!.getUTCDay() + 6) % 7
}

/** A month as the calendar shows it: one year and month (1-12). */
export type Month = { year: number; month: number }

export function monthOf(day: string): Month {
  const date = dateOf(day)!
  return { year: date.getUTCFullYear(), month: date.getUTCMonth() + 1 }
}

export function addMonths({ year, month }: Month, n: number): Month {
  const date = new Date(Date.UTC(year, month - 1 + n, 1))
  return { year: date.getUTCFullYear(), month: date.getUTCMonth() + 1 }
}

export function firstOf({ year, month }: Month): string {
  return isoOf(new Date(Date.UTC(year, month - 1, 1)))
}

export function lastOf({ year, month }: Month): string {
  return isoOf(new Date(Date.UTC(year, month, 0)))
}

/** The weeks of a month, Monday first, with null where a day belongs to another month. */
export function calendarMonth(year: number, month: number): (string | null)[][] {
  const first = firstOf({ year, month })
  const last = lastOf({ year, month })
  const cells: (string | null)[] = Array.from({ length: weekdayOf(first) }, () => null)
  for (let day = first; day <= last; day = shiftDay(day, 1)) {
    cells.push(day)
  }
  while (cells.length % 7 !== 0) {
    cells.push(null)
  }
  return Array.from({ length: cells.length / 7 }, (_, i) => cells.slice(i * 7, i * 7 + 7))
}

/** The day kept within the allowed range. */
export function clampDay(day: string, min?: string, max?: string): string {
  if (min && day < min) {
    return min
  }
  if (max && day > max) {
    return max
  }
  return day
}

/** A JavaScript date for formatting the day's names, at noon so no time zone moves it. */
export function noonOf(day: string): Date {
  const date = dateOf(day)!
  return new Date(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate(), 12)
}

/** Today in the browser's own time zone. */
export function todayDay(now = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
}
