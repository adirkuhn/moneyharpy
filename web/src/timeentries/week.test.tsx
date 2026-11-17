import { describe, expect, it } from 'vitest'
import { addDays, atTime, clock, dayOf, mondayOf, parseClock, parseDay, periodOf, stopwatch } from './week'

describe('the timesheet week', () => {
  it('reads a day, falling back on today', () => {
    const today = new Date(2026, 10, 18, 15, 30)
    expect(parseDay('2026-11-16', today)).toBe('2026-11-16')
    expect(parseDay('2026-13-40', today)).toBe('2026-11-18')
    expect(parseDay(null, today)).toBe('2026-11-18')
  })

  it('starts on Monday', () => {
    expect(mondayOf('2026-11-18')).toBe('2026-11-16')
    expect(mondayOf('2026-11-22')).toBe('2026-11-16')
    expect(mondayOf('2026-11-16')).toBe('2026-11-16')
    expect(addDays('2026-11-30', 1)).toBe('2026-12-01')
    expect(periodOf('2026-11-16')).toBe('20261116..20261122')
  })

  it('puts a time on a day, and reads the day of a moment', () => {
    const at = atTime('2026-11-18', 9 * 60 + 5)
    expect(at.getHours()).toBe(9)
    expect(at.getMinutes()).toBe(5)
    expect(dayOf(at.toISOString())).toBe('2026-11-18')
  })

  it('reads and writes hours and minutes as Moneybird does', () => {
    expect(parseClock('09:30')).toBe(570)
    expect(parseClock('9.30')).toBe(570)
    expect(parseClock('930')).toBe(570)
    expect(parseClock('9')).toBe(540)
    expect(parseClock('')).toBeNull()
    expect(parseClock('25:00')).toBeNull()
    expect(parseClock('9:75')).toBeNull()
    expect(clock(0)).toBe('00:00')
    expect(clock(125)).toBe('02:05')
    expect(clock(61 * 60)).toBe('61:00')
    expect(stopwatch(63)).toBe('00:01:03')
  })
})
