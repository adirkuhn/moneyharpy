import { describe, expect, it } from 'vitest'
import { numberTags, renderNumber } from './numbering'

describe('renderNumber', () => {
  const at = new Date(2026, 10, 10, 9) // Tuesday 10 November 2026, ISO week 46.

  it.each([
    ['{year}-{id}', '1462', 'en', '2026-1462'],
    ['{year_two_digits}{month}{day}-{id}', '0027', 'en', '261110-0027'],
    ['Q{quarter}/W{week}/{id}', '7', 'en', 'Q4/W46/7'],
    ['{month_name}-{id}', '1', 'en', 'November-1'],
    ['{month_name}-{id}', '1', 'nl', 'november-1'],
    ['{customer_id}-{id}', '5', 'en', '1-5'],
    ['{unknown}-{id}', '5', 'en', '{unknown}-5'],
  ])('renders %s with %s in %s', (template, sequence, language, want) => {
    expect(renderNumber(template, sequence, at, language)).toBe(want)
  })

  it('offers the server tags with {id} first', () => {
    expect(numberTags[0]).toBe('id')
    expect(numberTags).toContain('customer_id')
  })
})
