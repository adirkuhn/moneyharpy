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

describe('the value on a day', () => {
  it('takes the day\'s month pro rata, as a divestment books it', async () => {
    const { valueOn } = await import('./book')
    const car = {
      purchase_date: '2025-01-14', purchase_value: '20003.6', calculated_purchase_value: '20003.6', disposal: null,
      value_change_plan: { active: true, lifespan_in_years: 6, residual_value: '5000.0', monthly_depreciation: '208.38' },
      value_changes: [{ type: 'linear', date: '2026-10-31', amount: '-4496.97' }],
    } as unknown as Parameters<typeof valueOn>[0]
    expect(valueOn(car, '2026-11-18')).toBe(15381.6)
    expect(valueOn(car, '2026-10-31')).toBe(15506.63)
  })
})
