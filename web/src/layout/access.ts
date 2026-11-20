import type { components } from '../api/schema'

type Permission = components['schemas']['Permission']

/**
 * What a page asks of a member: one of `any`, and every one of `all`. The
 * permissions are those the API asks for the page's data, so a member who
 * may open a page can also load it.
 */
export type Access = { any?: Permission[]; all?: Permission[] }

const everything: Permission[] = ['sales_invoices', 'documents', 'estimates', 'bank', 'settings', 'time_entries']

/**
 * The pages of an administration, by path below it. `*` stands for one part
 * of the path, such as an id. The most specific rule of a page applies.
 */
export const pageAccess: Record<string, Access> = {
  sales_invoices: { any: ['sales_invoices'] },
  external_sales_invoices: { any: ['sales_invoices'] },
  recurring_sales_invoices: { any: ['sales_invoices'] },
  documents: { any: ['documents'] },
  estimates: { any: ['estimates'] },
  financial_accounts: { any: ['bank'] },
  financial_mutations: { any: ['bank'] },
  purchase_transaction_batches: { any: ['bank'] },
  time_entries: { any: ['time_entries'] },
  time_entries_report: { any: ['time_entries'] },
  // Billing time makes a sales invoice.
  time_entries_wizard: { all: ['time_entries', 'sales_invoices'] },
  contacts: { any: ['sales_invoices', 'documents', 'estimates'] },
  declarations: { any: ['documents'] },
  reports: { any: ['documents', 'bank'] },
  // The events sit with the reports.
  events: { any: ['documents', 'bank'] },
  'report/profit_loss': { any: ['documents'] },
  'report/balance_sheet': { any: ['bank'] },
  'report/assets_report': { any: ['documents'] },
  'report/tax': { any: ['documents'] },
  'report/cash_flow': { any: ['bank'] },
  'report/debtors': { any: ['sales_invoices'] },
  'report/debtors_aging': { any: ['sales_invoices'] },
  'report/revenue_by_contact': { any: ['sales_invoices'] },
  'report/revenue_by_project': { any: ['sales_invoices'] },
  'report/creditors': { any: ['documents'] },
  'report/creditors_aging': { any: ['documents'] },
  'report/expenses_by_contact': { any: ['documents'] },
  'report/expenses_by_project': { any: ['documents'] },
  'report/general_ledger': { any: ['documents', 'bank'] },
  'report/audit_report': { any: ['documents', 'bank'] },
  'report/journal_entries': { any: ['documents', 'bank'] },
  products: { any: ['settings', 'sales_invoices', 'estimates'] },
  payment_links: { any: ['settings', 'sales_invoices', 'estimates'] },
  subscription_templates: { any: ['settings'] },
  assets: { any: ['documents'] },
  projects: { any: ['settings', 'sales_invoices', 'documents', 'estimates', 'time_entries'] },
  'projects/new': { any: ['settings'] },
  'projects/*/edit': { any: ['settings'] },
  // As on Moneybird, task lists ask for every permission but ownership.
  task_lists: { all: everything },
  users: { any: ['ownership'] },
  user_invitations: { any: ['ownership'] },
  support_access: { any: ['ownership'] },
  delete: { any: ['ownership'] },
  empty: { any: ['ownership'] },
  webhooks: { any: ['settings'] },
}

/** Whether `rule` (parts of a path, `*` for any one part) is `path` or a page below it. */
function covers(rule: string[], path: string[]): boolean {
  return rule.length <= path.length && rule.every((part, i) => part === '*' || part === path[i])
}

/** The rule of the page at `path`, relative to the administration: the most specific one. */
export function accessFor(path: string): Access {
  const parts = path.split('/').filter(Boolean)
  let best: { length: number; access: Access } | undefined
  for (const [rule, access] of Object.entries(pageAccess)) {
    const ruleParts = rule.split('/')
    if (covers(ruleParts, parts) && (!best || ruleParts.length > best.length)) {
      best = { length: ruleParts.length, access }
    }
  }
  return best?.access ?? {}
}

type UserType = components['schemas']['Administration']['user_type']

/** Parts of a path that make a page one that changes data, such as a form. */
const changeParts = new Set(['new', 'edit', 'edit_general', 'edit_default', 'value_changes', 'payments', 'templates_selection', 'blueprints', 'scheme'])

/** Pages, below the administration, that are forms as a whole. */
const formPages = [
  'time_entries_wizard', 'time_entries/*', 'identities', 'document_styles', 'workflows', 'custom_fields',
  'financial_accounts/*', 'booking_rules', 'webhooks', 'connections', 'users', 'user_invitations', 'support_access', 'delete', 'empty',
  'settings/company', 'settings/invoices_and_estimates', 'settings/bank', 'settings/connections',
]

/** The bookkeeping, which accountants may change, with the task lists they keep for it. */
const bookkeepingPages = ['ledger_accounts', 'ledger_account_templates', 'tax_rates', 'declarations', 'documents/general_journal_documents', 'settings/bookkeeping', 'task_lists']

const under = (rules: string[], parts: string[]) => rules.some((rule) => covers(rule.split('/'), parts))

/** Whether the page at `path` is there to change data rather than to see it. */
export function changesData(path: string): boolean {
  const parts = path.split('/').filter(Boolean)
  return parts.some((part) => changeParts.has(part)) || under(formPages, parts)
}

/** Whether the page at `path` belongs to the bookkeeping. */
export function isBookkeeping(path: string): boolean {
  return under(bookkeepingPages, path.split('/').filter(Boolean))
}

/**
 * Whether a member with these permissions may open the page at `path`. An
 * accountant sees what their permissions allow but only changes the
 * bookkeeping, so other pages that change data are closed to them.
 */
export function mayOpen(permissions: readonly Permission[], path: string, userType?: UserType): boolean {
  const { any = [], all = [] } = accessFor(path)
  const permitted = (any.length === 0 || any.some((p) => permissions.includes(p))) && all.every((p) => permissions.includes(p))
  return permitted && (userType !== 'accountant' || !changesData(path) || isBookkeeping(path))
}
