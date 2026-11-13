import type { components } from '../api/schema'

type Permission = components['schemas']['Permission']

export type NavKind =
  | 'overview' | 'invoices' | 'incoming' | 'quotes' | 'bank' | 'timesheets' | 'contacts' | 'vat'
  | 'reports' | 'products' | 'assets' | 'projects' | 'tasks' | 'settings'

/** A page in the side menu. `ready` is false while MoneyHarpy has no such page yet. */
export type NavLink = { key: string; to: string; ready: boolean }

export type NavItem = NavLink & {
  kind: NavKind
  /** The member needs one of these; none means every member. */
  permissions: Permission[]
  children?: NavLink[]
}

const link = (key: string, to: string, ready = false): NavLink => ({ key, to, ready })

const bookkeeping: Permission[] = ['bank', 'documents', 'sales_invoices']

/** The parts of an administration, in Moneybird's order. */
export const navItems: NavItem[] = [
  { kind: 'overview', key: 'overview', to: 'feed', ready: true, permissions: [] },
  {
    kind: 'invoices', key: 'invoices', to: 'sales_invoices', ready: true, permissions: ['sales_invoices'],
    children: [
      link('salesInvoices', 'sales_invoices', true),
      link('externalInvoices', 'external_sales_invoices', true),
      link('recurringInvoices', 'recurring_sales_invoices', true),
    ],
  },
  { kind: 'incoming', key: 'incoming', to: 'documents', ready: true, permissions: ['documents'] },
  { kind: 'quotes', key: 'quotes', to: 'estimates', ready: true, permissions: ['estimates'] },
  {
    kind: 'bank', key: 'bank', to: 'financial_mutations', ready: true, permissions: ['bank'],
    children: [
      link('financialAccounts', 'settings/bank', true),
      link('transactions', 'financial_mutations', true),
      link('payments', 'purchase_transaction_batches'),
    ],
  },
  {
    kind: 'timesheets', key: 'timesheets', to: 'time_entries', ready: true, permissions: ['time_entries'],
    children: [
      link('register', 'time_entries', true),
      link('hoursReport', 'time_entries_report'),
      link('projectsReport', 'time_entries_report/projects'),
    ],
  },
  { kind: 'contacts', key: 'contacts', to: 'contacts', ready: true, permissions: ['sales_invoices', 'documents', 'estimates'] },
  { kind: 'vat', key: 'vat', to: 'declarations', ready: false, permissions: bookkeeping },
  {
    kind: 'reports', key: 'reports', to: 'reports', ready: false, permissions: ['bank', 'documents'],
    children: [
      link('reportsOverview', 'reports'),
      link('profitLoss', 'report/profit_loss', true),
      link('balanceSheet', 'report/balance_sheet', true),
      link('assetsReport', 'report/assets_report', true),
      link('vatSummary', 'report/tax', true),
      link('cashFlow', 'report/cash_flow', true),
      link('debtors', 'report/debtors', true),
      link('creditors', 'report/creditors', true),
      link('debtorsAging', 'report/debtors_aging', true),
      link('creditorsAging', 'report/creditors_aging', true),
      link('revenueByContact', 'report/revenue_by_contact', true),
      link('expensesByContact', 'report/expenses_by_contact', true),
      link('revenueByProject', 'report/revenue_by_project', true),
      link('expensesByProject', 'report/expenses_by_project', true),
      link('trialBalance', 'report/general_ledger', true),
      link('journalEntries', 'documents/general_journal_documents', true),
      link('auditReport', 'report/audit_report'),
      link('events', 'events'),
    ],
  },
  {
    kind: 'products', key: 'products', to: 'products', ready: true, permissions: ['settings', 'sales_invoices', 'estimates'],
    children: [link('productsOverview', 'products', true), link('onlineSales', 'payment_links')],
  },
  { kind: 'assets', key: 'assets', to: 'assets', ready: true, permissions: ['documents'] },
  { kind: 'projects', key: 'projects', to: 'projects', ready: true, permissions: ['settings', 'sales_invoices', 'documents', 'estimates', 'time_entries'] },
  {
    kind: 'tasks', key: 'tasks', to: 'task_lists/lists', ready: true, permissions: [],
    children: [link('taskLists', 'task_lists/lists', true), link('taskTemplates', 'task_lists/templates')],
  },
  { kind: 'settings', key: 'settings', to: 'settings', ready: true, permissions: [] },
]

/** The items a member with these permissions may see. */
export function visibleItems(permissions: readonly Permission[]): NavItem[] {
  return navItems.filter((item) => item.permissions.length === 0 || item.permissions.some((p) => permissions.includes(p)))
}

/** Whether `to` is the page at `path` (relative to the administration) or a page below it. */
export function isAt(path: string, to: string): boolean {
  return path === to || path.startsWith(`${to}/`)
}

/** Whether the page at `path` belongs to the item. */
export function contains(item: NavItem, path: string): boolean {
  return (item.children ?? [item]).some((l) => l.ready && isAt(path, l.to))
}
