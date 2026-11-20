import type { components } from '../api/schema'
import { mayOpen } from './access'

type Permission = components['schemas']['Permission']

export type NavKind =
  | 'overview' | 'invoices' | 'incoming' | 'quotes' | 'bank' | 'timesheets' | 'contacts' | 'vat'
  | 'reports' | 'products' | 'assets' | 'projects' | 'tasks' | 'settings'

/** A page in the side menu. `ready` is false while MoneyHarpy has no such page yet. */
export type NavLink = { key: string; to: string; ready: boolean }

export type NavItem = NavLink & {
  kind: NavKind
  children?: NavLink[]
}

const link = (key: string, to: string, ready = false): NavLink => ({ key, to, ready })

/** The parts of an administration, in Moneybird's order. */
export const navItems: NavItem[] = [
  { kind: 'overview', key: 'overview', to: 'feed', ready: true },
  {
    kind: 'invoices', key: 'invoices', to: 'sales_invoices', ready: true,
    children: [
      link('salesInvoices', 'sales_invoices', true),
      link('externalInvoices', 'external_sales_invoices', true),
      link('recurringInvoices', 'recurring_sales_invoices', true),
    ],
  },
  { kind: 'incoming', key: 'incoming', to: 'documents', ready: true },
  { kind: 'quotes', key: 'quotes', to: 'estimates', ready: true },
  {
    kind: 'bank', key: 'bank', to: 'financial_mutations', ready: true,
    children: [
      link('financialAccounts', 'financial_accounts', true),
      link('transactions', 'financial_mutations', true),
      link('payments', 'purchase_transaction_batches', true),
    ],
  },
  {
    kind: 'timesheets', key: 'timesheets', to: 'time_entries', ready: true,
    children: [
      link('register', 'time_entries', true),
      link('hoursReport', 'time_entries_report', true),
      link('projectsReport', 'time_entries_report/projects', true),
    ],
  },
  { kind: 'contacts', key: 'contacts', to: 'contacts', ready: true },
  { kind: 'vat', key: 'vat', to: 'declarations', ready: true },
  {
    kind: 'reports', key: 'reports', to: 'reports', ready: true,
    children: [
      link('reportsOverview', 'reports', true),
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
      link('auditReport', 'report/audit_report', true),
      link('events', 'events', true),
    ],
  },
  {
    kind: 'products', key: 'products', to: 'products', ready: true,
    children: [link('productsOverview', 'products', true), link('onlineSales', 'payment_links', true)],
  },
  { kind: 'assets', key: 'assets', to: 'assets', ready: true },
  { kind: 'projects', key: 'projects', to: 'projects', ready: true },
  {
    kind: 'tasks', key: 'tasks', to: 'task_lists/lists', ready: true,
    children: [link('taskLists', 'task_lists/lists', true), link('taskTemplates', 'task_lists/templates', true)],
  },
  { kind: 'settings', key: 'settings', to: 'settings', ready: true },
]

/** The items a member with these permissions may see, with only the pages they may open. */
export function visibleItems(permissions: readonly Permission[]): NavItem[] {
  return navItems.flatMap((item) => {
    if (!item.children) {
      return mayOpen(permissions, item.to) ? [item] : []
    }
    const children = item.children.filter((l) => mayOpen(permissions, l.to))
    if (children.length === 0) {
      return []
    }
    return [{ ...item, to: children.some((l) => l.to === item.to) ? item.to : (children[0]?.to ?? item.to), children }]
  })
}

/** Whether `to` is the page at `path` (relative to the administration) or a page below it. */
export function isAt(path: string, to: string): boolean {
  return path === to || path.startsWith(`${to}/`)
}

/** The link of the page at `path`: the most specific one, so a report's sub-report does not also mark the report. */
export function currentLink(items: NavItem[], path: string): string | undefined {
  return items
    .flatMap((i) => i.children ?? [i])
    .filter((l) => l.ready && isAt(path, l.to))
    .reduce<string | undefined>((best, l) => (best === undefined || l.to.length > best.length ? l.to : best), undefined)
}

/** Whether the page at `path` belongs to the item. */
export function contains(item: NavItem, path: string): boolean {
  return (item.children ?? [item]).some((l) => l.ready && isAt(path, l.to))
}
