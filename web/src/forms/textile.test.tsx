import { describe, expect, it } from 'vitest'
import { parseTextile, renderTags } from './textile'

describe('textile', () => {
  it('styles _italic_, *bold* and +underline+, nested too', () => {
    expect(parseTextile('Pay *now* or _later_ +please+')).toEqual([
      { text: 'Pay ' }, { text: 'now', bold: true }, { text: ' or ' }, { text: 'later', italic: true }, { text: ' ' }, { text: 'please', underline: true },
    ])
    expect(parseTextile('*_both_*')).toEqual([{ text: 'both', bold: true, italic: true }])
  })

  it('leaves markers alone that wrap nothing or sit inside words and tags', () => {
    expect(parseTextile('2 * 3 * 4')).toEqual([{ text: '2 * 3 * 4' }])
    expect(parseTextile('{document.total_price} and snake_case_name')).toEqual([{ text: '{document.total_price} and snake_case_name' }])
    expect(parseTextile('*open')).toEqual([{ text: '*open' }])
  })

  it('fills in the tags it knows and leaves others as written', () => {
    expect(renderTags('{document.total_price} by {document.due_date} {unknown.tag} {company.email}', { 'document.total_price': '€ 10,00', 'document.due_date': '01-12-2026', 'company.email': '' }))
      .toBe('€ 10,00 by 01-12-2026 {unknown.tag} ')
  })
})
