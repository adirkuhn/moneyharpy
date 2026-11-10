/**
 * Number templates, after Moneybird. This mirrors invoicing.RenderNumber on
 * the server so the example updates while typing; the server's example is
 * shown once saved.
 */
export const numberTags = ['id', 'year', 'year_two_digits', 'quarter', 'month', 'month_name', 'week', 'day', 'customer_id'] as const
export type NumberTag = (typeof numberTags)[number]

const pad = (n: number) => String(n).padStart(2, '0')

function isoWeek(date: Date): number {
  const d = new Date(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()))
  const day = d.getUTCDay() || 7
  d.setUTCDate(d.getUTCDate() + 4 - day)
  const yearStart = new Date(Date.UTC(d.getUTCFullYear(), 0, 1))
  return Math.ceil(((d.getTime() - yearStart.getTime()) / 86400000 + 1) / 7)
}

/** Fills in a number template for a document dated at. */
export function renderNumber(template: string, sequence: string, at: Date, language: string): string {
  const month = new Intl.DateTimeFormat(language.startsWith('nl') ? 'nl' : 'en', { month: 'long' }).format(at)
  const values: Record<NumberTag, string> = {
    id: sequence,
    year: String(at.getFullYear()),
    year_two_digits: pad(at.getFullYear() % 100),
    quarter: String(Math.floor(at.getMonth() / 3) + 1),
    month: pad(at.getMonth() + 1),
    month_name: month,
    week: pad(isoWeek(at)),
    day: pad(at.getDate()),
    customer_id: '1',
  }
  return template.replace(/\{([a-z_]+)\}/g, (tag, name: string) => (name in values ? values[name as NumberTag] : tag))
}
