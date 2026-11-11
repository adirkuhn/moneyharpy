/**
 * Invoice totals worked out in the browser exactly as the server books them:
 * amounts in cents, quantities in thousandths and percentages in hundredths.
 */

/** Reads a price like the server does: "100,50", "12.5" or "-3"; null when it is not one. */
export function parseCents(text: string): number | null {
  const s = text.trim().replace(',', '.')
  if (s === '') {
    return 0
  }
  const m = /^([+-]?)(\d{0,12})(?:\.(\d{0,2}))?$/.exec(s)
  if (!m || (m[2] === '' && !m[3])) {
    return null
  }
  const cents = Number(m[2] || '0') * 100 + Number((m[3] ?? '').padEnd(2, '0'))
  return m[1] === '-' ? -cents : cents
}

/** Reads the number in front of a quantity such as "2" or "1,5 hours", in thousandths. */
export function parseQuantity(text: string): number | null {
  const m = /^\s*(-?)(\d{1,9})(?:[.,](\d{1,3}))?(?:\s.*)?$/.exec(text)
  if (!m) {
    return null
  }
  const value = Number(m[2]) * 1000 + Number((m[3] ?? '').padEnd(3, '0'))
  return m[1] === '-' ? -value : value
}

/** Reads a percentage such as "10" or "2,5" in hundredths; empty is none. */
export function parsePercentage(text: string): number | null {
  const s = text.trim()
  if (s === '') {
    return 0
  }
  const m = /^(\d{1,3})(?:[.,](\d{1,2}))?$/.exec(s)
  if (!m) {
    return null
  }
  const value = Number(m[1]) * 100 + Number((m[2] ?? '').padEnd(2, '0'))
  return value > 10000 ? null : value
}

/** Divides and rounds half away from zero. */
function divRound(n: number, d: number) {
  const q = Math.floor((Math.abs(n) + Math.floor(d / 2)) / d)
  return n < 0 ? -q : q
}

export type TotalsLine = { price: number; quantity: number; taxRateId: string; rate: number }
export type RateTotal = { taxRateId: string; taxable: number; tax: number }

export type Totals = {
  /** The lines before discount, as typed: including VAT when prices include it. */
  subtotal: number
  discount: number
  exclTax: number
  tax: number
  inclTax: number
  byRate: RateTotal[]
  /** Each line after discount, in the prices' own terms. */
  lines: number[]
}

/** Works out an invoice's totals; VAT is taken per rate on the summed lines. */
export function computeTotals(lines: TotalsLine[], discount: number, inclTax: boolean): Totals {
  const groups = new Map<string, { rate: number; sum: number }>()
  let subtotal = 0
  let taken = 0
  const amounts = lines.map((l) => {
    const gross = divRound(l.price * l.quantity, 1000)
    const net = gross - divRound(gross * discount, 10000)
    subtotal += gross
    taken += gross - net
    const group = groups.get(l.taxRateId) ?? { rate: l.rate, sum: 0 }
    group.sum += net
    groups.set(l.taxRateId, group)
    return net
  })
  const byRate = [...groups].map(([taxRateId, { rate, sum }]): RateTotal => {
    if (inclTax) {
      const taxable = divRound(sum * 10000, 10000 + rate)
      return { taxRateId, taxable, tax: sum - taxable }
    }
    return { taxRateId, taxable: sum, tax: divRound(sum * rate, 10000) }
  })
  const exclTax = byRate.reduce((s, r) => s + r.taxable, 0)
  const tax = byRate.reduce((s, r) => s + r.tax, 0)
  return { subtotal, discount: taken, exclTax, tax, inclTax: exclTax + tax, byRate, lines: amounts }
}
