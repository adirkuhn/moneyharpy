/** A depreciation plan worked out as MoneyHarpy books it, for Moneybird's calculation example. */
export type PlanExample = {
  /** The first month, as YYYY-MM. */
  start: string
  /** The month the plan reaches the residual value, as YYYY-MM. */
  end: string
  /** What is depreciated in all, in cents. */
  total: number
  monthly: number
  /** The first and last months when the first is pro rata, in cents. */
  first?: number
  last?: number
}

const cents = (amount: string) => Math.round(Number(amount.replace(',', '.')) * 100)

function addMonths(year: number, month: number, add: number) {
  const index = year * 12 + (month - 1) + add
  return `${Math.floor(index / 12)}-${String((index % 12) + 1).padStart(2, '0')}`
}

/**
 * The plan of an asset bought on date (YYYY-MM-DD) for value, written off over
 * years down to residual: the monthly depreciation spread over the lifespan,
 * the purchase month pro rata by its days in use when bought after the first,
 * and the rest of it in one month after the lifespan. Null when nothing is
 * depreciated.
 */
export function planExample(date: string, value: string, years: number, residual: string): PlanExample | null {
  const months = Math.round(years * 12)
  const total = cents(value) - cents(residual || '0')
  const [year, month, day] = date.split('-').map(Number) as [number, number, number]
  if (!date || months <= 0 || !Number.isFinite(total) || total <= 0 || !day) {
    return null
  }
  const monthly = Math.round(total / months)
  const start = addMonths(year, month, 0)
  if (day === 1) {
    return { start, end: addMonths(year, month, months - 1), total, monthly }
  }
  const days = new Date(Date.UTC(year, month, 0)).getUTCDate()
  const first = Math.round((monthly * (days - day + 1)) / days)
  const last = total - first - monthly * (months - 1)
  return { start, end: addMonths(year, month, months), total, monthly, first, last }
}
