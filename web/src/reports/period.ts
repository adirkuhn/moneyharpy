import type { TFunction } from 'i18next'
import { describePeriod, type MonthRange, monthOf, parsePeriod, yearOfMonth } from '../list/period'

/** How a report splits its period into columns, after Moneybird's "Show per". */
export type Granularity = 'month' | 'quarter' | 'year'
export const granularities: Granularity[] = ['month', 'quarter', 'year']

const isQuarter = (r: MonthRange) => r.from % 3 === 0 && r.to === r.from + 2
const isYear = (r: MonthRange) => r.from % 12 === 0 && r.to === r.from + 11

const monthName = (m: number, locale: string, month: 'long' | 'short' = 'long') =>
  new Date(yearOfMonth(m), m % 12, 1).toLocaleDateString(locale, { month, year: 'numeric' })

/** A period in Moneybird's words: "November 2026", "quarter 4 2026", "2026" or a range of months. */
export function rangeName(range: MonthRange, t: TFunction, locale: string): string {
  if (range.from === range.to) {
    return monthName(range.from, locale)
  }
  if (isQuarter(range)) {
    return t('reports.quarterName', { n: (range.from % 12) / 3 + 1, year: yearOfMonth(range.from) })
  }
  return describePeriod(range, locale)
}

/** The period as long again before (-1) or after (1). */
export function shiftRange(range: MonthRange, by: number): MonthRange {
  const length = range.to - range.from + 1
  return { from: range.from + by * length, to: range.to + by * length }
}

/** The last day of a month as yyyy-mm-dd. */
export function lastDay(m: number): string {
  const d = new Date(Date.UTC(yearOfMonth(m), (m % 12) + 1, 0))
  return d.toISOString().slice(0, 10)
}

/** A period as the API takes it, in whole months: 202610..202612. */
export function monthsParam(range: MonthRange): string {
  const digits = (m: number) => `${yearOfMonth(m)}${String((m % 12) + 1).padStart(2, '0')}`
  return `${digits(range.from)}..${digits(range.to)}`
}

/** The head of a report's column for one part of its period: "Oct 2026", "Q4 2026" or "2026". */
export function partName(part: string, t: TFunction, locale: string): string {
  const range = parsePeriod(part, new Date())
  if (!range) {
    return part
  }
  if (isYear(range)) {
    return String(yearOfMonth(range.from))
  }
  if (isQuarter(range)) {
    return t('list.quarter', { n: (range.from % 12) / 3 + 1, year: yearOfMonth(range.from) })
  }
  if (range.from === range.to) {
    return monthName(range.from, locale, 'short')
  }
  return describePeriod(range, locale)
}

/** The month a period ends in, as the range of that one month. */
export const lastMonth = (range: MonthRange): MonthRange => ({ from: range.to, to: range.to })

/** This year, as a range of its months. */
export const yearRange = (year: number): MonthRange => ({ from: monthOf(year, 0), to: monthOf(year, 11) })
