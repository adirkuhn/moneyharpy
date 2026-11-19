import { describe, expect, it } from 'vitest'
import { periodLabel, periodRange, previousPeriod } from './period'

describe('task list periods', () => {
  it('runs from the first to the last day of a month, quarter or year', () => {
    expect(periodRange('month', 2024, 2)).toEqual({ begin: '2024-02-01', end: '2024-02-29' })
    expect(periodRange('quarter', 2026, 3)).toEqual({ begin: '2026-07-01', end: '2026-09-30' })
    expect(periodRange('year', 2026, 1)).toEqual({ begin: '2026-01-01', end: '2026-12-31' })
  })

  it('proposes the period before today, as Moneybird does', () => {
    expect(previousPeriod('month', '2026-11-20')).toEqual({ year: 2026, part: 10 })
    expect(previousPeriod('month', '2026-01-05')).toEqual({ year: 2025, part: 12 })
    expect(previousPeriod('quarter', '2026-02-05')).toEqual({ year: 2025, part: 4 })
    expect(previousPeriod('quarter', '2026-11-20')).toEqual({ year: 2026, part: 3 })
    expect(previousPeriod('year', '2026-11-20')).toEqual({ year: 2025, part: 1 })
  })

  it('is written as Moneybird writes it', () => {
    expect(periodLabel('2026-10-01', '2026-10-31', 'en')).toBe('Oct 2026')
    expect(periodLabel('2026-07-01', '2026-09-30', 'en')).toBe('Q3 2026')
    expect(periodLabel('2026-01-01', '2026-12-31', 'en')).toBe('2026')
    expect(periodLabel('2026-10-02', '2026-10-31', 'en')).toBe('2026-10-02 – 2026-10-31')
    expect(periodLabel(null, null, 'en')).toBe('')
  })
})
