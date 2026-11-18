import { describe, expect, it } from 'vitest'
import { amountOf, fileName, reportSheet } from './exportFile'

describe('report export', () => {
  it('reads the amounts as the report writes them', () => {
    expect(amountOf('19,502.05', 'en')).toBe(19502.05)
    expect(amountOf('-4,288.59', 'en')).toBe(-4288.59)
    expect(amountOf('€716', 'en')).toBe(716)
    expect(amountOf('1.234,56', 'nl')).toBe(1234.56)
    expect(amountOf('Oct 2026', 'en')).toBeNull()
    expect(amountOf('2026-09-30', 'en')).toBeNull()
  })

  it('turns the report tables into a sheet with numbers', () => {
    document.body.innerHTML = `
      <table aria-label="Debit"><thead><tr><th><button>Collapse all</button></th><th>Oct 2026</th></tr></thead>
      <tbody><tr><th colspan="2">Revenue</th></tr><tr><th>Rent</th><td>1,700.00</td></tr></tbody></table>`
    const sheet = reportSheet('Profit and loss statement for quarter 4 2026', Array.from(document.querySelectorAll('table')), 'en')
    expect(sheet[0]).toEqual([{ value: 'Profit and loss statement for quarter 4 2026', fontWeight: 'bold' }])
    expect(sheet[2]).toEqual([null, { value: 'Oct 2026', fontWeight: 'bold' }])
    expect(sheet[3]).toEqual([{ value: 'Revenue' }, null])
    expect(sheet[4]).toEqual([{ value: 'Rent' }, { value: 1700, type: Number, format: '#,##0.00' }])
    expect(fileName('Assets in 2026: a/b')).toBe('Assets in 2026 a b.xlsx')
  })
})
