import type { components } from '../api/schema'
import { type TimeEntry, workedMinutes } from './queries'
import { dayOf } from './week'

/** How the time becomes invoice lines, as Moneybird's "Markup of invoice detail". */
export const groupings = ['combine_all', 'group_by', 'split_all'] as const
export type Grouping = (typeof groupings)[number]

/** The info a line is made of when the time is grouped. */
export const detailParts = ['description', 'project', 'user'] as const
export type DetailPart = (typeof detailParts)[number]

export type Detail = components['schemas']['SalesInvoiceDetailInput']

/** What every line gets: the hourly rate, the VAT rate and the category, if any. */
export type LineSettings = { price: string; taxRateId: string; ledgerAccountId: string }

/** Minutes as hours to two decimals, such as 1.5. */
export function hoursOf(minutes: number): string {
  return String(Math.round((minutes / 60) * 100) / 100)
}

const part = (e: TimeEntry, p: DetailPart) =>
  p === 'description' ? (e.description ?? '') : p === 'project' ? (e.project?.name ?? '') : (e.user?.name ?? '')

const unique = (values: string[]) => [...new Set(values.filter(Boolean))]

/** The line of some time: its hours at the rate, over the days it was worked. */
function lineOf(list: TimeEntry[], description: string, s: LineSettings): Detail {
  const days = list.map((e) => dayOf(e.started_at).replaceAll('-', '')).sort()
  return {
    description,
    amount: hoursOf(list.reduce((sum, e) => sum + workedMinutes(e), 0)),
    price: s.price,
    tax_rate_id: s.taxRateId,
    ...(s.ledgerAccountId ? { ledger_account_id: s.ledgerAccountId } : {}),
    period: `${days[0]}..${days.at(-1)}`,
  }
}

/**
 * The invoice lines of the time, oldest first: all in one line, a line for
 * each mix of the chosen info, or a line each.
 */
export function billDetails(entries: TimeEntry[], grouping: Grouping, parts: DetailPart[], s: LineSettings): Detail[] {
  const time = [...entries].sort((a, b) => (a.started_at ?? '').localeCompare(b.started_at ?? ''))
  if (time.length === 0) {
    return []
  }
  if (grouping === 'combine_all') {
    return [lineOf(time, unique(time.map((e) => e.description ?? '')).join(', '), s)]
  }
  if (grouping === 'split_all') {
    return time.map((e) => lineOf([e], e.description ?? '', s))
  }
  const groups = new Map<string, TimeEntry[]>()
  for (const e of time) {
    const key = unique(parts.map((p) => part(e, p))).join(' · ')
    groups.set(key, [...(groups.get(key) ?? []), e])
  }
  return [...groups].map(([key, list]) => lineOf(list, key, s))
}
