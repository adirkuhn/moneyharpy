import { describe, expect, it } from 'vitest'
import { addDays, addressLines, daysBetween, documentDate, percentText, senderEntries } from './paper'

const sender = {
  company_name: 'Example Shop B.V.', address1: 'Dam 1', address2: null, zipcode: '1012JS', city: 'Amsterdam', email: 'books@example.com',
  phone: '', chamber_of_commerce: '12345678', tax_number: null, bank_account_number: 'NL91ABNA0417164300',
}

describe('the sender block', () => {
  it("follows Moneybird's default layout when the style has none, leaving out empty fields", () => {
    expect(senderEntries(undefined, sender)).toEqual([
      { field: 'company_name', bold: true, label: false, text: ['Example Shop B.V.'] },
      { field: 'address', bold: false, label: false, text: ['Dam 1', '1012JS Amsterdam'] },
      { field: 'newline' },
      { field: 'email', bold: false, label: false, text: ['books@example.com'] },
      { field: 'newline' },
      { field: 'chamber_of_commerce', bold: false, label: true, text: ['12345678'] },
      { field: 'bank_account_number', bold: false, label: true, text: ['NL91ABNA0417164300'] },
    ])
  })

  it("follows the style's own layout", () => {
    expect(senderEntries([{ field: 'tax_number', label: true }, { field: 'company_name' }], { ...sender, tax_number: 'NL000099998B57' })).toEqual([
      { field: 'tax_number', bold: false, label: true, text: ['NL000099998B57'] },
      { field: 'company_name', bold: false, label: false, text: ['Example Shop B.V.'] },
    ])
  })

  it('shows nothing but blank lines before the sender is known', () => {
    expect(senderEntries([{ field: 'company_name' }, { field: 'newline' }], undefined)).toEqual([{ field: 'newline' }])
  })
})

describe('document helpers', () => {
  it('writes addresses, dates and percentages as the document does', () => {
    expect(addressLines({ address1: 'Dam 2', address2: 'Floor 3', zipcode: '1012AB', city: 'Amsterdam' })).toEqual(['Dam 2', 'Floor 3', '1012AB Amsterdam'])
    expect(addressLines({ address1: null, zipcode: null, city: null })).toEqual([])
    expect(documentDate('2026-11-25', 'nl')).toBe('25-11-2026')
    expect(percentText('21.0')).toBe('21%')
    expect(percentText('5.5')).toBe('5.5%')
  })

  it('counts days across months and clock changes', () => {
    expect(addDays('2026-11-11', 14)).toBe('2026-11-25')
    expect(addDays('2026-10-20', 14)).toBe('2026-11-03')
    expect(daysBetween('2026-12-01', '2026-12-31')).toBe(30)
    expect(daysBetween('2026-03-20', '2026-04-03')).toBe(14)
  })
})
