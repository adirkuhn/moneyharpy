/** Days as YYYY-MM-DD in the browser's time zone, as Moneybird's timesheet week has them. */

const pad = (n: number) => String(n).padStart(2, '0')

/** The day of a date, in local time. */
export function isoDay(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function toDate(day: string): Date {
  const [y, m, d] = day.split('-').map(Number)
  return new Date(y ?? 0, (m ?? 1) - 1, d ?? 1)
}

/** The day asked for, or today when it is not a real date. */
export function parseDay(value: string | null, today: Date = new Date()): string {
  if (value && /^\d{4}-\d{2}-\d{2}$/.test(value) && isoDay(toDate(value)) === value) {
    return value
  }
  return isoDay(today)
}

export function addDays(day: string, days: number): string {
  const d = toDate(day)
  d.setDate(d.getDate() + days)
  return isoDay(d)
}

/** The Monday of the week the day is in. */
export function mondayOf(day: string): string {
  return addDays(day, -((toDate(day).getDay() + 6) % 7))
}

/** The week from its Monday, as a filter period: 20261116..20261122. */
export function periodOf(monday: string): string {
  return `${monday.replaceAll('-', '')}..${addDays(monday, 6).replaceAll('-', '')}`
}

/** The day a moment falls on, in local time. */
export function dayOf(iso: string | null | undefined): string {
  return iso ? isoDay(new Date(iso)) : ''
}

/** The moment at minutes past midnight on the day. */
export function atTime(day: string, minutes: number): Date {
  const d = toDate(day)
  d.setHours(Math.floor(minutes / 60), minutes % 60, 0, 0)
  return d
}

/** Minutes past midnight from 09:30, 9.30, 930 or 9; null when empty or not a time of day. */
export function parseClock(value: string): number | null {
  const m = /^(\d{1,2})(?:[:.]?(\d{2}))?$/.exec(value.trim())
  if (!m) {
    return null
  }
  const hours = Number(m[1])
  const minutes = Number(m[2] ?? 0)
  return hours < 24 && minutes < 60 ? hours * 60 + minutes : null
}

/** Minutes as 02:05, as Moneybird writes hours. */
export function clock(minutes: number): string {
  return `${pad(Math.floor(minutes / 60))}:${pad(minutes % 60)}`
}

/** Seconds as 00:01:03, a running timer. */
export function stopwatch(seconds: number): string {
  return `${clock(Math.floor(seconds / 60))}:${pad(seconds % 60)}`
}
