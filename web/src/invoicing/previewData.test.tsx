import type { TFunction } from 'i18next'
import { describe, expect, it } from 'vitest'
import { workflow } from '../test/server'
import { expandEmail } from './ChannelPreviews'
import { buildPreviewData, computeTotals, defaultWorkflow, standardRate } from './previewData'
import { salesRate, sentInvoice } from './testkit'

const dt = ((key: string) => key) as unknown as TFunction

describe('preview data', () => {
  it('prices the example at the standard sales rate in the administration currency', () => {
    const low = { ...salesRate, id: '2109', percentage: '9' }
    const purchase = { ...salesRate, id: '2201', percentage: '25', tax_rate_type: 'purchase_invoice' as const }
    expect(standardRate([low, salesRate, purchase])?.id).toBe('2101')
    const data = buildPreviewData({ dt, currency: 'USD', invoice: null, workflows: [workflow()], rates: [low, salesRate], products: [], invoiceNumber: '2026-0007', estimateNumber: 'Q-1' })
    expect(data.source).toBe('example')
    expect(data.currency).toBe('USD')
    expect(data.lines.every((l) => l.percentage === 21)).toBe(true)
    expect(data.totals).toEqual({ subtotal: 2201, vat: [{ percentage: 21, amount: 462.21 }], total: 2663.21 })
    expect(data.invoiceNumber).toBe('2026-0007')
  })

  it('takes the latest invoice with its customer, lines and totals', () => {
    const data = buildPreviewData({ dt, currency: 'EUR', invoice: sentInvoice, workflows: [], rates: [salesRate], products: [], invoiceNumber: 'x', estimateNumber: 'y' })
    expect(data.source).toBe('invoice')
    expect(data.customer.name).toBe('Example Shop B.V.')
    expect(data.customer.values.customer_tax_number).toBe('NL000099998B57')
    expect(data.lines).toEqual([{ quantity: 2, description: 'Consulting', price: 100, percentage: 21 }])
    expect(data.totals).toEqual({ subtotal: 200, vat: [{ percentage: 21, amount: 42 }], total: 242 })
    expect(data.invoiceNumber).toBe('2026-0042')
  })

  it('uses the default active workflow and fills in email placeholders', () => {
    const flows = [workflow({ id: '1', default: false, invoice_email_body: 'Old' }), workflow({ id: '2', invoice_email_body: 'Hi {contact_name}, pay {total}.' })]
    expect(defaultWorkflow(flows, 'InvoiceWorkflow')?.id).toBe('2')
    expect(expandEmail('Hi {contact_name}, pay {total} {unknown}.', { contact_name: 'Ann', total: '€1' })).toBe('Hi Ann, pay €1 {unknown}.')
    expect(computeTotals([{ quantity: 1, description: '', price: 10, percentage: 0 }])).toEqual({ subtotal: 10, vat: [], total: 10 })
  })
})
