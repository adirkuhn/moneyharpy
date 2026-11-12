import { describe, expect, it } from 'vitest'
import { byDay, eventText, historyOf } from './history'
import type { SalesInvoice } from './queries'

const base = { id: '901', created_at: '2026-11-10T10:15:00Z', payments: [], notes: [], events: [] } as unknown as SalesInvoice

const event = (action: string, at: string) => ({ action, created_at: at, updated_at: at, user_id: '1', administration_id: '11', link_entity_id: '901', link_entity_type: 'SalesInvoice', data: {} })

describe('the history of an invoice', () => {
  it("lists the invoice's events, payments and notes, newest first, leaving out what the payments already show", () => {
    const inv = {
      ...base,
      events: [
        event('sales_invoice_created', '2026-11-10T10:15:00Z'),
        event('sales_invoice_send_manually', '2026-11-10T10:15:02Z'),
        event('payment_registered', '2026-11-10T10:15:03Z'),
        event('sales_invoice_state_changed_to_paid', '2026-11-10T10:15:04Z'),
      ],
      payments: [{ id: '981', price: '33.98', created_at: '2026-11-10T10:15:03Z', user_id: '1' }],
      notes: [{ id: '971', note: 'Called', created_at: '2026-11-11T09:00:00Z' }],
    } as unknown as SalesInvoice
    expect(historyOf(inv).map((h) => (h.kind === 'event' ? h.action : h.kind))).toEqual([
      'note', 'sales_invoice_state_changed_to_paid', 'payment', 'sales_invoice_send_manually', 'sales_invoice_created',
    ])
  })

  it('tells what happened from the dates the invoice keeps when it has no events', () => {
    const inv = {
      ...base, state: 'paid', delivery_method: 'Manual', sent_at: '2026-11-10', paid_at: '2026-11-10',
      payments: [{ id: '981', price: '33.98', created_at: '2026-11-10T10:16:00Z' }],
    } as unknown as SalesInvoice
    const items = historyOf(inv)
    expect(items.map((h) => (h.kind === 'event' ? h.action : h.kind))).toEqual([
      'sales_invoice_state_changed_to_paid', 'payment', 'sales_invoice_send_manually', 'sales_invoice_created',
    ])
    // Only the creation has a time of day of its own.
    expect(items.map((h) => h.timed)).toEqual([false, true, false, true])
  })

  it('tells who wrote a note', () => {
    const inv = { ...base, notes: [{ id: '971', note: 'Called', user_id: '7', created_at: '2026-11-11T09:00:00Z' }] } as unknown as SalesInvoice
    expect(historyOf(inv)[0]).toMatchObject({ kind: 'note', userId: '7' })
  })

  it('keeps to the real events once there are any, without guessing from dates', () => {
    const inv = {
      ...base, state: 'paid', delivery_method: 'Manual', sent_at: '2026-11-10', paid_at: '2026-11-10',
      events: [event('sales_invoice_created', '2026-11-10T10:15:00Z')],
    } as unknown as SalesInvoice
    expect(historyOf(inv).map((h) => (h.kind === 'event' ? h.action : h.kind))).toEqual(['sales_invoice_created'])
  })

  it('groups by day, as Today, Yesterday and the date', () => {
    const now = new Date('2026-11-11T11:00:00Z')
    const days = byDay([{ at: '2026-11-11T08:00:00Z' }, { at: '2026-11-10T22:30:00Z' }, { at: '2026-11-09T08:00:00Z' }], now)
    expect(days.map((d) => [d.label, d.items.length])).toEqual([['today', 1], ['yesterday', 1], ['2026-11-09', 1]])
  })

  it('says what an event means, with the state it changed to', () => {
    expect(eventText('sales_invoice_created')).toEqual({ key: 'created' })
    expect(eventText('sales_invoice_state_changed_to_open')).toEqual({ key: 'stateChanged', state: 'open' })
    expect(eventText('sales_invoice_state_changed_to_paid')).toEqual({ key: 'paid' })
    expect(eventText('sales_invoice_send_manually')).toEqual({ key: 'sentManually' })
    expect(eventText('credit_invoice_created_from_original')).toEqual({ key: 'createdFromOriginal' })
    expect(eventText('credit_invoice_created')).toEqual({ key: 'creditCreated' })
    expect(eventText('sales_invoice_state_changed_to_late')).toEqual({ key: 'stateChanged', state: 'late' })
    expect(eventText('something_new')).toEqual({ key: 'other', action: 'something new' })
  })
})
