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

describe('accountants', () => {
  const all = ['sales_invoices', 'documents', 'estimates', 'bank', 'settings', 'time_entries'] as const
  const accountant = (path: string) => mayOpen([...all], path, 'accountant')

  it('see the pages their permissions allow', () => {
    expect(accountant('sales_invoices')).toBe(true)
    expect(accountant('sales_invoices/12')).toBe(true)
    expect(accountant('contacts/4')).toBe(true)
    expect(accountant('report/profit_loss')).toBe(true)
    expect(accountant('settings/administration')).toBe(true)
  })

  it('do not open pages that change anything but the bookkeeping', () => {
    expect(accountant('sales_invoices/new')).toBe(false)
    expect(accountant('contacts/4/edit')).toBe(false)
    expect(accountant('sales_invoices/12/payments/bank')).toBe(false)
    expect(accountant('assets/3/value_changes/new')).toBe(false)
    expect(accountant('time_entries/7')).toBe(false)
    expect(accountant('settings/company')).toBe(false)
    expect(accountant('identities/new')).toBe(false)
    expect(mayOpen([...all], 'sales_invoices/new', 'employee')).toBe(true)
  })

  it('open the pages of the bookkeeping', () => {
    expect(accountant('documents/general_journal_documents/new')).toBe(true)
    expect(accountant('declarations/new')).toBe(true)
    expect(accountant('ledger_accounts/new')).toBe(true)
    expect(accountant('tax_rates/9')).toBe(true)
    expect(accountant('settings/bookkeeping')).toBe(true)
  })
})
