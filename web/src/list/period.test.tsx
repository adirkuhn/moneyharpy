import { describe, expect, it } from 'vitest'
import { describePeriod, monthOf, parsePeriod, periodValue } from './period'

const today = new Date(2026, 10, 11) // 11 November 2026

describe('periods of the list filters', () => {
  it('reads named periods relative to today', () => {
    expect(parsePeriod('this_year', today)).toEqual({ from: monthOf(2026, 0), to: monthOf(2026, 11) })
    expect(parsePeriod('prev_year', today)).toEqual({ from: monthOf(2025, 0), to: monthOf(2025, 11) })
    expect(parsePeriod('this_quarter', today)).toEqual({ from: monthOf(2026, 9), to: monthOf(2026, 11) })
    expect(parsePeriod('prev_quarter', today)).toEqual({ from: monthOf(2026, 6), to: monthOf(2026, 8) })
    expect(parsePeriod('this_month', today)).toEqual({ from: monthOf(2026, 10), to: monthOf(2026, 10) })
    expect(parsePeriod('prev_month', today)).toEqual({ from: monthOf(2026, 9), to: monthOf(2026, 9) })
  })

  it('reads ranges of months and of days', () => {
    expect(parsePeriod('202501..202503', today)).toEqual({ from: monthOf(2025, 0), to: monthOf(2025, 2) })
    expect(parsePeriod('20250115..20250220', today)).toEqual({ from: monthOf(2025, 0), to: monthOf(2025, 1) })
  })

  it('has no range without a period or for one it does not know', () => {
    expect(parsePeriod('', today)).toBeNull()
    expect(parsePeriod('someday', today)).toBeNull()
  })

  it('writes a range as a named period when one fits, as months otherwise', () => {
    expect(periodValue({ from: monthOf(2026, 0), to: monthOf(2026, 11) }, today)).toBe('this_year')
    expect(periodValue({ from: monthOf(2026, 9), to: monthOf(2026, 9) }, today)).toBe('prev_month')
    expect(periodValue({ from: monthOf(2024, 10), to: monthOf(2025, 1) }, today)).toBe('202411..202502')
    expect(periodValue({ from: monthOf(2024, 3), to: monthOf(2024, 3) }, today)).toBe('202404..202404')
    expect(periodValue(null, today)).toBe('')
  })

  it('describes a range in words', () => {
    expect(describePeriod({ from: monthOf(2025, 0), to: monthOf(2025, 11) }, 'en')).toBe('2025')
    expect(describePeriod({ from: monthOf(2025, 3), to: monthOf(2025, 3) }, 'en')).toBe('Apr 2025')
    expect(describePeriod({ from: monthOf(2024, 10), to: monthOf(2025, 1) }, 'en')).toBe('Nov 2024 – Feb 2025')
  })
})
