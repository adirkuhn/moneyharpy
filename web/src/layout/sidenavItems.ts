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
    kind: 'invoices', key: 'invoices', to: 'sales_invoices', ready: false, permissions: ['sales_invoices'],
    children: [
      link('salesInvoices', 'sales_invoices'),
      link('externalInvoices', 'external_sales_invoices'),
      link('recurringInvoices', 'recurring_sales_invoices'),
    ],
  },
  { kind: 'incoming', key: 'incoming', to: 'documents', ready: false, permissions: ['documents'] },
  { kind: 'quotes', key: 'quotes', to: 'estimates', ready: false, permissions: ['estimates'] },
  {
    kind: 'bank', key: 'bank', to: 'financial_mutations', ready: true, permissions: ['bank'],
    children: [
      link('financialAccounts', 'settings/bank', true),
      link('transactions', 'financial_mutations', true),
      link('payments', 'purchase_transaction_batches'),
    ],
  },
  {
    kind: 'timesheets', key: 'timesheets', to: 'time_entries', ready: false, permissions: ['time_entries'],
    children: [
      link('register', 'time_entries'),
      link('hoursReport', 'time_entries_report'),
      link('projectsReport', 'time_entries_report/projects'),
    ],
  },
  { kind: 'contacts', key: 'contacts', to: 'contacts', ready: true, permissions: ['sales_invoices', 'documents', 'estimates'] },
  { kind: 'vat', key: 'vat', to: 'declarations', ready: false, permissions: bookkeeping },
  {
    kind: 'reports', key: 'reports', to: 'reports', ready: false, permissions: bookkeeping,
    children: [
      link('reportsOverview', 'reports'),
      link('profitLoss', 'report/profit_loss'),
      link('balanceSheet', 'report/balance_sheet'),
      link('assetsReport', 'report/assets_report'),
      link('vatSummary', 'report/tax'),
      link('cashFlow', 'report/cash_flow'),
      link('debtors', 'report/debtors'),
      link('creditors', 'report/creditors'),
      link('debtorsAging', 'report/debtors_aging'),
      link('creditorsAging', 'report/creditors_aging'),
      link('revenueByContact', 'report/revenue_by_contact'),
      link('expensesByContact', 'report/expenses_by_contact'),
      link('revenueByProject', 'report/revenue_by_project'),
      link('expensesByProject', 'report/expenses_by_project'),
      link('trialBalance', 'report/general_ledger'),
      link('auditReport', 'report/audit_report'),
      link('events', 'events'),
    ],
  },
  {
    kind: 'products', key: 'products', to: 'products', ready: false, permissions: ['sales_invoices', 'estimates'],
    children: [link('productsOverview', 'products'), link('onlineSales', 'payment_links')],
  },
  { kind: 'assets', key: 'assets', to: 'assets', ready: false, permissions: ['documents'] },
  { kind: 'projects', key: 'projects', to: 'projects', ready: false, permissions: ['time_entries', 'sales_invoices'] },
  {
    kind: 'tasks', key: 'tasks', to: 'task_lists/lists', ready: false, permissions: [],
    children: [link('taskLists', 'task_lists/lists'), link('taskTemplates', 'task_lists/templates')],
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
