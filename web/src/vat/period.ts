import type { TFunction } from 'i18next'

/** A period by its days, as yyyy-mm-dd or yyyymmdd. */
function parts(day: string) {
  const d = day.replaceAll('-', '')
  return { year: Number(d.slice(0, 4)), month: Number(d.slice(4, 6)) }
}

/**
 * A return's period in words, as Moneybird names it: "Quarter 3 2026",
 * "November 2026" or "2026".
 */
export function periodName(from: string, to: string, t: TFunction, language: string): string {
  const a = parts(from)
  const b = parts(to)
  const months = (b.year - a.year) * 12 + b.month - a.month + 1
  if (months === 12) {
    return String(a.year)
  }
  if (months === 3) {
    return t('vat.quarter', { quarter: Math.floor((a.month - 1) / 3) + 1, year: a.year })
  }
  if (months === 1) {
    return new Intl.DateTimeFormat(language, { month: 'long', year: 'numeric' }).format(new Date(a.year, a.month - 1, 1))
  }
  return `${from} – ${to}`
}

/** A period written 20260701..20260930, as bank links name a return. */
export function periodOfParam(param: string, t: TFunction, language: string): string {
  const [from = '', to = ''] = param.split('..')
  return periodName(from, to, t, language)
}
