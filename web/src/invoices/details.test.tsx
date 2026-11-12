import { describe, expect, it } from 'vitest'
import { applyBelow, moveRow, needsVatId, numberParts, periodRange, periodText, presetPeriod, quantityLabel, toPeriod } from './details'

describe('reordering details', () => {
  it('moves a row to another place and keeps the others in order', () => {
    expect(moveRow(['a', 'b', 'c', 'd'], 0, 2)).toEqual(['b', 'c', 'a', 'd'])
    expect(moveRow(['a', 'b', 'c', 'd'], 3, 1)).toEqual(['a', 'd', 'b', 'c'])
    expect(moveRow(['a', 'b'], 0, -1)).toEqual(['a', 'b'])
    expect(moveRow(['a', 'b'], 1, 2)).toEqual(['a', 'b'])
  })

  it('applies a value to the rows below one', () => {
    const rows = [{ rate: '1' }, { rate: '2' }, { rate: '3' }]
    expect(applyBelow(rows, 1, { rate: '9' })).toEqual([{ rate: '1' }, { rate: '2' }, { rate: '9' }])
    expect(applyBelow(rows, 0, { rate: '9' })).toEqual([{ rate: '1' }, { rate: '9' }, { rate: '9' }])
  })
})

describe('periods', () => {
  it('works out the presets from today', () => {
    expect(presetPeriod('today', '2026-11-12')).toBe('20261112..20261112')
    expect(presetPeriod('this_month', '2026-11-12')).toBe('20261101..20261130')
    expect(presetPeriod('previous_month', '2026-01-12')).toBe('20251201..20251231')
    expect(presetPeriod('next_month', '2026-01-31')).toBe('20260201..20260228')
  })

  it('reads and writes the stored form', () => {
    expect(periodRange('20261101..20261130')).toEqual({ from: '2026-11-01', to: '2026-11-30' })
    expect(periodRange('')).toBeNull()
    expect(periodRange(null)).toBeNull()
    expect(toPeriod('2026-11-01', '2026-11-30')).toBe('20261101..20261130')
    expect(toPeriod('2026-11-30', '2026-11-01')).toBe('20261101..20261130')
    expect(toPeriod('2026-11-01', '')).toBe('')
  })

  it('shows a period as dates', () => {
    expect(periodText('20261101..20261130', 'en')).toBe('11/01/2026 – 11/30/2026')
    expect(periodText('20261101..20261130', 'nl')).toBe('01-11-2026 – 30-11-2026')
    expect(periodText('', 'en')).toBe('')
  })
})

describe('what the document shows', () => {
  it('writes " x" after a bare quantity only', () => {
    expect(quantityLabel('2')).toBe('2 x')
    expect(quantityLabel('1,5')).toBe('1,5 x')
    expect(quantityLabel('1 x')).toBe('1 x')
    expect(quantityLabel('3 hours')).toBe('3 hours')
    expect(quantityLabel(' ')).toBe('1 x')
  })

  it('splits the next invoice number around its sequence number', () => {
    expect(numberParts('2026-0001', '0001')).toEqual({ prefix: '2026-', suffix: '' })
    expect(numberParts('F0007/26', '0007')).toEqual({ prefix: 'F', suffix: '/26' })
    expect(numberParts('2026-0001', '42')).toEqual({ prefix: '', suffix: '' })
  })

  it('asks a VAT id of companies in another EU country only', () => {
    expect(needsVatId('BE', 'NL')).toBe(true)
    expect(needsVatId('NL', 'NL')).toBe(false)
    expect(needsVatId('US', 'NL')).toBe(false)
    expect(needsVatId('NL', 'BE')).toBe(false)
    expect(needsVatId('XI', 'NL')).toBe(true)
  })
})
