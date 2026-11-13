import { describe, expect, it } from 'vitest'
import { chargedRate, computeTotals, parseCents, parsePercentage, parseQuantity, vatRows } from './totals'

describe('reading what people type', () => {
  it('reads prices like the server, in cents', () => {
    expect(parseCents('100,50')).toBe(10050)
    expect(parseCents('12.5')).toBe(1250)
    expect(parseCents('-3')).toBe(-300)
    expect(parseCents('')).toBe(0)
    expect(parseCents('1.000,50')).toBeNull()
    expect(parseCents('abc')).toBeNull()
  })

  it('reads the number in front of a quantity, in thousandths', () => {
    expect(parseQuantity('2')).toBe(2000)
    expect(parseQuantity('1,5 hours')).toBe(1500)
    expect(parseQuantity('-1')).toBe(-1000)
    expect(parseQuantity('hours')).toBeNull()
  })

  it('reads a discount percentage in hundredths', () => {
    expect(parsePercentage('10')).toBe(1000)
    expect(parsePercentage('2,5')).toBe(250)
    expect(parsePercentage('')).toBe(0)
    expect(parsePercentage('101')).toBeNull()
  })
})

describe('invoice totals', () => {
  const vat21 = { taxRateId: '1', rate: 2100 }
  const vat9 = { taxRateId: '2', rate: 900 }

  it('adds VAT per rate on the summed lines', () => {
    const t = computeTotals([{ price: 10000, quantity: 2000, ...vat21 }, { price: 5000, quantity: 1000, ...vat9 }], 0, false)
    expect(t.subtotal).toBe(25000)
    expect(t.byRate).toEqual([{ taxRateId: '1', taxable: 20000, tax: 4200 }, { taxRateId: '2', taxable: 5000, tax: 450 }])
    expect(t.inclTax).toBe(29650)
    expect(t.lines).toEqual([20000, 5000])
  })

  it('takes the discount off each line before VAT', () => {
    // 3 x 33.33 = 99.99, 10% discount = 89.99, 21% VAT = 18.90.
    const t = computeTotals([{ price: 3333, quantity: 3000, ...vat21 }], 1000, false)
    expect(t.discount).toBe(1000)
    expect(t.exclTax).toBe(8999)
    expect(t.tax).toBe(1890)
    expect(t.inclTax).toBe(10889)
  })

  it('takes VAT out of prices that include it', () => {
    const t = computeTotals([{ price: 6050, quantity: 1000, ...vat21 }, { price: 6050, quantity: 1000, ...vat21 }], 0, true)
    expect(t.byRate).toEqual([{ taxRateId: '1', taxable: 10000, tax: 2100 }])
    expect(t.inclTax).toBe(12100)
  })

  it('rounds credits away from zero', () => {
    const t = computeTotals([{ price: 3333, quantity: -3000, ...vat21 }], 1000, false)
    expect(t.inclTax).toBe(-10889)
  })
})

describe('vatRows', () => {
  const money = (cents: number) => (cents / 100).toFixed(2)
  const rates = [
    { id: 'nl21', name: '21% btw', percentage: '21', show_tax: true },
    { id: 'eu9', name: 'Binnen EU 9% (btw verlegd)', percentage: '9', show_tax: false },
    { id: 'none', name: 'Geen btw', percentage: '0', show_tax: false },
  ]

  it('shows charged VAT once and reverse-charged VAT as owed and deducted, as Moneybird does', () => {
    const rows = vatRows([
      { taxRateId: 'nl21', taxable: 10000, tax: 2100 },
      { taxRateId: 'eu9', taxable: 379531, tax: 0 },
      { taxRateId: 'none', taxable: 500, tax: 0 },
    ], rates, money)
    expect(rows).toEqual([
      { label: '21% btw', amount: '21.00' },
      { label: 'Binnen EU 9% (btw verlegd)', amount: '-341.58' },
      { label: 'Binnen EU 9% (btw verlegd)', amount: '341.58' },
      { label: 'Geen btw', amount: '0.00' },
    ])
  })

  it('leaves reverse-charged VAT out of what the lines add up to', () => {
    const totals = computeTotals([{ price: 379531, quantity: 1000, taxRateId: 'eu9', rate: chargedRate(rates[1]) }], 0, false)
    expect(totals.inclTax).toBe(379531)
    expect(chargedRate(rates[0])).toBe(2100)
  })
})
