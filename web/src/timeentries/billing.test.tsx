import { describe, expect, it } from 'vitest'
import { billDetails, hoursOf } from './billing'
import type { TimeEntry } from './queries'

const entry = (over: Partial<TimeEntry>): TimeEntry => ({
  id: '1', administration_id: '11', user_id: '1', description: 'Design', started_at: '2026-11-16T09:00:00Z', ended_at: '2026-11-16T11:00:00Z',
  paused_duration: 0, billable: true, project_id: null, project: null, user: { id: '1', name: 'Ann de Vries' }, uri: '',
  ...over,
} as TimeEntry)

const entries = [
  entry({ id: '1' }),
  entry({ id: '2', description: 'Support', started_at: '2026-11-17T09:00:00Z', ended_at: '2026-11-17T09:35:00Z', project_id: '601', project: { id: '601', name: 'Website' } }),
  entry({ id: '3', started_at: '2026-11-18T13:00:00Z', ended_at: '2026-11-18T14:00:00Z', project_id: '601', project: { id: '601', name: 'Website' } }),
]
const line = { price: '80', taxRateId: '9', ledgerAccountId: '' }

describe('billing time', () => {
  it('writes hours as a quantity', () => {
    expect(hoursOf(120)).toBe('2')
    expect(hoursOf(35)).toBe('0.58')
    expect(hoursOf(90)).toBe('1.5')
  })

  it('merges all time into one line over its days', () => {
    expect(billDetails(entries, 'combine_all', [], line)).toEqual([
      { description: 'Design, Support', amount: '3.58', price: '80', tax_rate_id: '9', period: '20261116..20261118' },
    ])
  })

  it('groups the time by the info chosen', () => {
    expect(billDetails(entries, 'group_by', ['description', 'project'], { ...line, ledgerAccountId: '7' })).toEqual([
      { description: 'Design', amount: '2', price: '80', tax_rate_id: '9', ledger_account_id: '7', period: '20261116..20261116' },
      { description: 'Support · Website', amount: '0.58', price: '80', tax_rate_id: '9', ledger_account_id: '7', period: '20261117..20261117' },
      { description: 'Design · Website', amount: '1', price: '80', tax_rate_id: '9', ledger_account_id: '7', period: '20261118..20261118' },
    ])
    expect(billDetails(entries, 'group_by', ['user'], line).map((d) => [d.description, d.amount])).toEqual([['Ann de Vries', '3.58']])
  })

  it('splits the time into a line each', () => {
    expect(billDetails(entries, 'split_all', [], line).map((d) => [d.description, d.amount, d.period])).toEqual([
      ['Design', '2', '20261116..20261116'], ['Support', '0.58', '20261117..20261117'], ['Design', '1', '20261118..20261118'],
    ])
  })
})
