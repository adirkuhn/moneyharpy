import { describe, expect, it } from 'vitest'
import { renderTags } from '../forms/textile'
import { conditionTagValues, tagMoney } from './conditions'

describe('payment condition tags', () => {
  it('writes amounts as the server does', () => {
    expect(tagMoney(123450, 'EUR', 'nl')).toBe('€ 1.234,50')
    expect(tagMoney(123450, 'EUR', 'en')).toBe('€1,234.50')
    expect(tagMoney(-500, 'USD', 'en')).toBe('-$5.00')
  })

  it('fills in the document, recipient and sender', () => {
    const values = conditionTagValues({
      language: 'nl', currency: 'EUR', total: 24200, unpaid: 4200, invoiceDate: '2026-11-01', dueDate: '2026-11-15', invoiceId: '2026-0001',
      paymentReference: 'RF130000000000000PV96X5KP', reference: 'Order 12',
      contact: { company_name: 'Example Shop B.V.', firstname: 'Ann', lastname: 'de Vries', send_invoices_to_email: 'billing@example.com' },
      sender: { company_name: 'Harpy B.V.', bank_account_number: 'NL91ABNA0417164300', custom_fields: [{ id: '5', value: 'Fast' }] },
    })
    const text = '{document.total_price} {document.total_unpaid_raw} {document.due_date} {document.payment_reference} {recipient.name} {recipient.contact_name} {recipient.email} {company.bank_account_number} {company.custom_field_5}'
    expect(renderTags(text, values)).toBe('€ 242,00 42,00 15-11-2026 RF13PV96X5KP Example Shop B.V. Ann de Vries billing@example.com NL91ABNA0417164300 Fast')
  })
})
