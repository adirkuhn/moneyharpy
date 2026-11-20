import { describe, expect, it } from 'vitest'
import { mayOpen } from './access'

describe('page access', () => {
  it('lets every member open the overview and pages without a rule', () => {
    expect(mayOpen([], 'feed')).toBe(true)
    expect(mayOpen([], 'onboarding/first_action/new')).toBe(true)
  })

  it('asks for one of the permissions of the page and its pages below', () => {
    expect(mayOpen(['time_entries'], 'sales_invoices')).toBe(false)
    expect(mayOpen(['time_entries'], 'sales_invoices/12/payments/bank')).toBe(false)
    expect(mayOpen(['sales_invoices'], 'sales_invoices/12')).toBe(true)
    expect(mayOpen(['estimates'], 'contacts/new')).toBe(true)
  })

  it('takes the most specific rule', () => {
    expect(mayOpen(['time_entries'], 'projects')).toBe(true)
    expect(mayOpen(['time_entries'], 'projects/new')).toBe(false)
    expect(mayOpen(['time_entries'], 'projects/5/edit')).toBe(false)
    expect(mayOpen(['settings'], 'projects/5/edit')).toBe(true)
    expect(mayOpen(['sales_invoices'], 'report/debtors')).toBe(true)
    expect(mayOpen(['sales_invoices'], 'report/creditors')).toBe(false)
  })

  it('asks for every permission where the page needs them all', () => {
    expect(mayOpen(['time_entries'], 'time_entries_wizard')).toBe(false)
    expect(mayOpen(['time_entries', 'sales_invoices'], 'time_entries_wizard')).toBe(true)
    expect(mayOpen(['sales_invoices', 'documents', 'estimates', 'bank', 'time_entries'], 'task_lists/lists')).toBe(false)
    expect(mayOpen(['sales_invoices', 'documents', 'estimates', 'bank', 'settings', 'time_entries'], 'task_lists/lists')).toBe(true)
  })

  it('keeps managing users to owners', () => {
    expect(mayOpen(['settings'], 'users/3')).toBe(false)
    expect(mayOpen(['ownership'], 'user_invitations/new')).toBe(true)
  })
})
