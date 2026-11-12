import { describe, expect, it } from 'vitest'
import { customerFields, customFieldId, hiddenFields, moveField, withCustomFields } from './styleLayout'

const fields = ['company_name', 'address', 'email'] as const
const line = (field: string, bold = false) => ({ field, bold, italic: false, label: false })

describe('document style columns', () => {
  it('lists the fields no column shows, with a blank line to add', () => {
    expect(hiddenFields([[line('address')]], fields, true)).toEqual(['company_name', 'email', 'newline'])
    expect(hiddenFields([[line('address')], [line('email')]], fields, false)).toEqual(['company_name'])
  })

  it('reorders a column', () => {
    const { columns, at } = moveField([[line('company_name'), line('address'), line('email')]], fields, true, { column: 0, index: 0 }, { column: 0, index: 1 })
    expect(columns[0]?.map((l) => l.field)).toEqual(['address', 'company_name', 'email'])
    expect(at).toEqual({ column: 0, index: 1 })
  })

  it('hides a field and shows it again at the end, keeping it plain', () => {
    const hidden = moveField([[line('company_name', true), line('address')]], fields, true, { column: 0, index: 0 }, { column: 1, index: 0 })
    expect(hidden.columns).toEqual([[line('address')]])
    expect(hidden.at).toEqual({ column: 1, index: 0 })
    const shown = moveField(hidden.columns, fields, true, { column: 1, index: 0 }, { column: 0, index: 99 })
    expect(shown.columns).toEqual([[line('address'), line('company_name')]])
    expect(shown.at).toEqual({ column: 0, index: 1 })
  })

  it('copies blank lines out of the hidden column', () => {
    const { columns } = moveField([[line('address')]], fields, true, { column: 1, index: 2 }, { column: 0, index: 0 })
    expect(columns).toEqual([[line('newline'), line('address')]])
    expect(hiddenFields(columns, fields, true)).toContain('newline')
  })

  it('offers the custom fields of the records a block shows', () => {
    const custom = [{ id: '7', source: 'identity' }, { id: '8', source: 'contact' }, { id: '9', source: 'sales_invoice' }]
    expect(withCustomFields(['email'], custom, ['identity'])).toEqual(['email', 'custom_field_7'])
    expect(withCustomFields(customerFields, custom, ['sales_invoice', 'contact'])).toEqual([...customerFields, 'custom_field_8', 'custom_field_9'])
    expect(customFieldId('custom_field_8')).toBe('8')
    expect(customFieldId('customer_id')).toBeUndefined()
  })

  it('moves a field between shown columns', () => {
    const { columns, at } = moveField([[line('address')], [line('email')]], fields, false, { column: 0, index: 0 }, { column: 1, index: 1 })
    expect(columns).toEqual([[], [line('email'), line('address')]])
    expect(at).toEqual({ column: 1, index: 1 })
  })
})
