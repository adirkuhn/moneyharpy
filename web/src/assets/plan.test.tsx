import { describe, expect, it } from 'vitest'
import { planExample } from './plan'

describe('the calculation example', () => {
  it('takes the purchase month pro rata and the rest a month after the lifespan', () => {
    expect(planExample('2025-01-14', '20003.60', 6, '5000')).toEqual({
      start: '2025-01', end: '2031-01', total: 1500360, monthly: 20838, first: 12099, last: 8763,
    })
  })

  it('takes whole months when bought on the first', () => {
    expect(planExample('2026-06-01', '1200', 1, '')).toEqual({ start: '2026-06', end: '2027-05', total: 120000, monthly: 10000 })
  })

  it('has nothing to show without a value above the residual value', () => {
    expect(planExample('2026-06-01', '0', 5, '')).toBeNull()
    expect(planExample('2026-06-01', '100', 5, '100')).toBeNull()
    expect(planExample('2026-06-01', '100', 0, '')).toBeNull()
  })
})
