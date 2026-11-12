import { describe, expect, it } from 'vitest'
import { containsPaymentReference, draftPaymentReference, rfCheckDigits, shortPaymentReference } from './paymentReference'

describe('payment references', () => {
  it("computes ISO 11649 check digits as in Moneybird's references", () => {
    expect(rfCheckDigits('SKE5TSYN')).toBe('18')
    expect(rfCheckDigits('PV96X5KP')).toBe('13')
    expect(rfCheckDigits('0000000000000PV96X5KP')).toBe('13')
  })

  it("shows a draft's placeholder as Moneybird does", () => {
    expect(draftPaymentReference(42)).toBe('RF95CONCEPT42')
    expect(draftPaymentReference(null)).toBe('RF89CONCEPT')
  })

  it('prints the short form of a padded reference', () => {
    expect(shortPaymentReference('RF130000000000000PV96X5KP')).toBe('RF13PV96X5KP')
    expect(shortPaymentReference('RF13PV96X5KP')).toBe('RF13PV96X5KP')
    expect(shortPaymentReference('2026-0001')).toBe('2026-0001')
  })

  it('finds a reference in a transaction however it is spaced or cased', () => {
    const ref = 'RF130000000000000PV96X5KP'
    expect(containsPaymentReference('Invoice rf13 pv96 x5kp thanks', ref)).toBe(true)
    expect(containsPaymentReference('RF13 0000 0000 0000 0PV9 6X5KP', ref)).toBe(true)
    expect(containsPaymentReference('RF13PV96X5KP', 'RF13PV96X5KP')).toBe(true)
    expect(containsPaymentReference('RF18SKE5TSYN', ref)).toBe(false)
    expect(containsPaymentReference('', ref)).toBe(false)
    expect(containsPaymentReference('RF13PV96X5KP', null)).toBe(false)
  })
})
