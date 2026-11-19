/** Moneybird's period types of a task list: none, a month, a quarter or a year. */
export type PeriodKind = 'none' | 'month' | 'quarter' | 'year'

const pad = (n: number) => String(n).padStart(2, '0')
const lastDay = (year: number, month: number) => new Date(Date.UTC(year, month, 0)).getUTCDate()
const parse = (day: string) => day.split('-').map(Number) as [number, number, number]

/** The first and last day of part (a month 1–12 or a quarter 1–4) of a year, or the whole year. */
export function periodRange(kind: Exclude<PeriodKind, 'none'>, year: number, part: number): { begin: string; end: string } {
  const [first, last] = kind === 'year' ? [1, 12] : kind === 'quarter' ? [part * 3 - 2, part * 3] : [part, part]
  return { begin: `${year}-${pad(first)}-01`, end: `${year}-${pad(last)}-${pad(lastDay(year, last))}` }
}

/** The period before the one today falls in, as Moneybird proposes: its year and its month or quarter. */
export function previousPeriod(kind: PeriodKind, today: string): { year: number; part: number } {
  const [y, m] = parse(today)
  if (kind === 'year') {
    return { year: y - 1, part: 1 }
  }
  if (kind === 'quarter') {
    const q = Math.ceil(m / 3)
    return q === 1 ? { year: y - 1, part: 4 } : { year: y, part: q - 1 }
  }
  return m === 1 ? { year: y - 1, part: 12 } : { year: y, part: m - 1 }
}

/** A list's period as Moneybird writes it: "Oct 2026", "Q3 2026", "2026", or else its first and last day. */
export function periodLabel(begin: string | null | undefined, end: string | null | undefined, language: string): string {
  if (!begin || !end) {
    return ''
  }
  const [by, bm, bd] = parse(begin)
  const [ey, em, ed] = parse(end)
  const whole = by === ey && bd === 1 && ed === lastDay(ey, em)
  if (whole && bm === em) {
    const month = new Date(Date.UTC(by, bm - 1, 1)).toLocaleDateString(language, { month: 'short', timeZone: 'UTC' }).replace('.', '')
    return `${month} ${by}`
  }
  if (whole && bm % 3 === 1 && em === bm + 2) {
    return `Q${(bm + 2) / 3} ${by}`
  }
  if (whole && bm === 1 && em === 12) {
    return String(by)
  }
  return `${begin} – ${end}`
}
