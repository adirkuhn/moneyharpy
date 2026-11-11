import { screen, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { LedgerAccount } from '../bookkeeping/queries'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const account = (over: Partial<LedgerAccount>): LedgerAccount => ({
  id: '1201', administration_id: '11', name: 'Rent', account_type: 'expenses', account_id: '4110', parent_id: null, active: true,
  allowed_document_types: ['purchase_invoice'], financial_account_id: null, created_at: stamp, updated_at: stamp, taxonomy_item: null,
  uri: '/11/ledger_accounts/1201', description: null, system_role: null, template: null, ...over,
})
const values = (pairs: [string, string][]) => ({ ledger_accounts: pairs.map(([ledger_account_id, value]) => ({ ledger_account_id, value })) })

function loaded() {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([
      account({}), account({ id: '8000', name: 'Revenue', account_type: 'revenue', account_id: '8000' }),
      account({ id: '1010', name: 'Main bank', account_type: 'current_assets', account_id: '1010' }),
    ])),
    http.get('*/api/v2/11/reports/profit_loss', () => HttpResponse.json({
      revenue_by_ledger_account: values([['8000', '200.0']]), direct_costs_by_ledger_account: values([]),
      expenses_by_ledger_account: values([['1201', '50.0']]), other_income_expenses_by_ledger_account: values([]),
      gross_profit: '200.0', operating_profit: '150.0', net_profit: '150.0', total_expenses: '50.0', total_revenue: '200.0',
    })),
    http.get('*/api/v2/11/reports/balance_sheet', () => HttpResponse.json({
      debit: { total: '150.0', current_assets: [{ ledger_account_id: '1010', value: '150.0', children: [] }], non_current_assets: [] },
      credit: { total: '150.0', equity: [], provisions: [], non_current_liabilities: [], current_liabilities: [], open_years: [{ year: '2026', value: '150.0' }] },
    })),
    http.get('*/api/v2/11/reports/debtors', () => HttpResponse.json({ debtors: [{ contact_id: '301', amount: '242.0' }, { contact_id: null, amount: '8.0' }] })),
    http.get('*/api/v2/11/contacts/301', () => HttpResponse.json({ id: '301', company_name: 'Example Shop B.V.', firstname: null, lastname: null })),
    http.get('*/api/v2/11/reports/creditors_aging', () => HttpResponse.json({ creditors: [{ contact_id: '301', amount: '121.0', aging_buckets: [
      { name: '0 - 30 days', amount: '121.0' }, { name: '30 - 60 days', amount: '0' }, { name: 'Older', amount: '0' }, { name: 'Total', amount: '121.0' },
    ] }] })),
  ]
}

describe('reports', () => {
  it('shows the profit and loss account', async () => {
    server.use(...loaded())
    renderAt('/11/report/profit_loss')
    const table = await screen.findByRole('table', { name: 'Profit and loss statement' })
    expect(within(table).getByRole('row', { name: /Revenue.*€200.00/ })).toBeInTheDocument()
    expect(within(table).getByRole('row', { name: /Net profit/ })).toHaveTextContent('€150.00')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Profit and loss statement' })).toHaveAttribute('aria-current', 'page')
  })

  it('shows the balance sheet with open years', async () => {
    server.use(...loaded())
    renderAt('/11/report/balance_sheet')
    const assets = await screen.findByRole('table', { name: 'Assets' })
    expect(within(assets).getByRole('row', { name: /Main bank/ })).toHaveTextContent('€150.00')
    const liabilities = screen.getByRole('table', { name: 'Equity and liabilities' })
    expect(within(liabilities).getByRole('row', { name: /2026/ })).toHaveTextContent('€150.00')
  })

  it('lists the debtors by name', async () => {
    server.use(...loaded())
    renderAt('/11/report/debtors')
    const table = await screen.findByRole('table', { name: 'Receivable overview' })
    expect(await within(table).findByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/contacts/301')
    expect(within(table).getByRole('row', { name: /Without a contact/ })).toHaveTextContent('€8.00')
    expect(within(table).getByRole('row', { name: /Total/ })).toHaveTextContent('€250.00')
  })

  it('shows what creditors have open by age', async () => {
    server.use(...loaded())
    renderAt('/11/report/creditors_aging')
    const table = await screen.findByRole('table', { name: 'Payables aging' })
    expect(within(table).getByRole('columnheader', { name: '0 - 30 days' })).toBeInTheDocument()
    expect(within(table).getAllByText('€121.00')).toHaveLength(2)
  })

  it('lists the expenses by project', async () => {
    server.use(
      ...loaded(),
      http.get('*/api/v2/11/reports/expenses_by_project', () => HttpResponse.json({ expenses_by_project: [{ project_id: null, amount: '20.0' }, { project_id: '501', amount: '9.99' }] })),
      http.get('*/api/v2/11/projects/501', () => HttpResponse.json({ id: '501', name: 'Falcon', state: 'active', budget: null })),
    )
    renderAt('/11/report/expenses_by_project')
    const table = await screen.findByRole('table', { name: 'Expenses by project' })
    expect(await within(table).findByRole('link', { name: 'Falcon' })).toHaveAttribute('href', '/11/projects/501')
    expect(within(table).getByRole('row', { name: /Without a project/ })).toHaveTextContent('€20.00')
    expect(within(table).getByRole('row', { name: /Total/ })).toHaveTextContent('€29.99')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Revenue by project' })).toHaveAttribute('href', '/11/report/revenue_by_project')
  })

  it('shows the assets report', async () => {
    server.use(
      ...loaded(),
      http.get('*/api/v2/11/reports/assets', () => HttpResponse.json([{
        asset_id: '601', ledger_account_id: '1010', name: 'Laptop', purchase_date: '2026-01-15', lifespan: null, purchase_value: '1200.0',
        calculated_purchase_value: '1200.0', residual_value: '0.0', total_value_changes_since_purchase: '-200.0', value_at_begin: '0.0',
        investment: '1200.0', depreciation: '-200.0', divestment: '0.0', value_changes: '-200.0', value_at_end: '1000.0', value_not_in_use: '0.0',
      }])),
    )
    renderAt('/11/report/assets_report')
    const table = await screen.findByRole('table', { name: 'Assets report' })
    expect(within(table).getByRole('link', { name: 'Laptop' })).toHaveAttribute('href', '/11/assets/601')
    expect(within(table).getByRole('row', { name: /Laptop/ })).toHaveTextContent('€1,000.00')
  })
})
