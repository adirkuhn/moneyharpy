import { describe, expect, it } from 'vitest'
import { countUntil, frequencyOfPreset, occurrence, presetOf, upcoming } from './schedule'

describe('the schedule of a recurring invoice', () => {
  it('names Moneybird’s frequency presets, anything else being custom', () => {
    expect(presetOf(1, 'month')).toBe('1:month')
    expect(presetOf(1, 'quarter')).toBe('1:quarter')
    expect(presetOf(6, 'month')).toBe('6:month')
    expect(presetOf(1, 'year')).toBe('1:year')
    expect(presetOf(2, 'week')).toBe('custom')
    expect(presetOf(3, 'month')).toBe('custom')
    expect(frequencyOfPreset('6:month')).toEqual({ frequency: 6, frequencyType: 'month' })
    expect(frequencyOfPreset('custom')).toBeNull()
  })

  it('steps through dates as the server does, keeping the day clamped to the month end', () => {
    expect(occurrence('2026-01-31', 'month', 1, 1)).toBe('2026-02-28')
    expect(occurrence('2026-01-31', 'month', 1, 2)).toBe('2026-03-31')
    expect(occurrence('2026-11-13', 'quarter', 1, 1)).toBe('2027-02-13')
    expect(occurrence('2026-11-13', 'week', 2, 1)).toBe('2026-11-27')
    expect(occurrence('2026-11-13', 'day', 1, 3)).toBe('2026-11-16')
    expect(occurrence('2024-02-29', 'year', 1, 1)).toBe('2025-02-28')
  })

  it('lists the next dates, up to the last one', () => {
    expect(upcoming({ invoice_date: '2026-11-13', frequency: 1, frequency_type: 'month', last_date: null }, 3)).toEqual(['2026-11-13', '2026-12-13', '2027-01-13'])
    expect(upcoming({ invoice_date: '2026-11-13', frequency: 1, frequency_type: 'month', last_date: '2026-12-20' }, 3)).toEqual(['2026-11-13', '2026-12-13'])
  })

  it('counts the invoices still to make before the last date', () => {
    expect(countUntil({ invoice_date: '2026-11-13', frequency: 1, frequency_type: 'month', last_date: '2027-01-13' })).toBe(3)
    expect(countUntil({ invoice_date: '2026-11-13', frequency: 1, frequency_type: 'month', last_date: null })).toBeNull()
  })
})
