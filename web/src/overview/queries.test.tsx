import { describe, expect, it } from 'vitest'
import { partOfDay } from './queries'

describe('partOfDay', () => {
  it('follows the administration time zone', () => {
    const at = new Date('2026-11-10T10:30:00Z')
    expect(partOfDay(at, 'Europe/Amsterdam')).toBe('morning')
    expect(partOfDay(at, 'Asia/Tokyo')).toBe('evening')
    expect(partOfDay(new Date('2026-11-10T13:00:00Z'), 'Europe/Amsterdam')).toBe('afternoon')
    expect(partOfDay(new Date('2026-11-10T02:00:00Z'), 'Europe/Amsterdam')).toBe('evening')
  })
})
