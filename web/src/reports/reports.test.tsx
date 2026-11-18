import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

const pl = (revenue: string, rent: string) => ({
  revenue_by_ledger_account: values([['8000', revenue]]), direct_costs_by_ledger_account: values([]),
  expenses_by_ledger_account: values([['1201', rent], ['1202', '10.0']]), other_income_expenses_by_ledger_account: values([]),
  gross_profit: revenue, operating_profit: String(Number(revenue) - Number(rent) - 10), net_profit: String(Number(revenue) - Number(rent) - 10),
  total_expenses: String(Number(rent) + 10), total_revenue: revenue,
})

function loaded(seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([
      account({ id: '1200', name: 'Housing costs', account_id: '4100' }),
      account({ parent_id: '1200' }), account({ id: '1202', name: 'Energy', account_id: '4120', parent_id: '1200' }),
      account({ id: '8000', name: 'Revenue', account_type: 'revenue', account_id: '8000' }),
      account({ id: '1010', name: 'Main bank', account_type: 'current_assets', account_id: '1010' }),
    ])),
    http.get('*/api/v2/11/reports/profit_loss', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json({ ...pl('300.0', '50.0'), periods: [{ period: '202610..202610', ...pl('200.0', '50.0') }, { period: '202611..202611', ...pl('100.0', '0.0') }] })
    }),
    http.get('*/api/v2/11/reports/balance_sheet', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json({
        debit: { total: '150.0', current_assets: [{ ledger_account_id: '1010', value: '150.0', children: [] }], non_current_assets: [] },
        credit: { total: '150.0', equity: [], provisions: [], non_current_liabilities: [], current_liabilities: [], open_years: [{ year: '2026', value: '150.0' }] },
      })
    }),
    http.get('*/api/v2/11/reports/debtors', () => HttpResponse.json({ debtors: [{ contact_id: '301', amount: '242.0' }, { contact_id: null, amount: '8.0' }] })),
    http.get('*/api/v2/11/contacts/301', () => HttpResponse.json({ id: '301', company_name: 'Example Shop B.V.', firstname: null, lastname: null })),
    http.get('*/api/v2/11/reports/creditors_aging', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json({ creditors: [{ contact_id: '301', amount: '121.0', aging_buckets: [
        { name: '0 - 30 days', amount: '121.0' }, { name: '30 - 60 days', amount: '0' }, { name: 'Older', amount: '0' }, { name: 'Total', amount: '121.0' },
      ] }] })
    }),
  ]
}

describe('reports', () => {
  it('shows the profit and loss statement per month, categories under their top category', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/report/profit_loss')
    const table = await screen.findByRole('table', { name: 'Profit and loss statement' })
    expect(seen.at(-1)?.searchParams.get('period')).toBe('this_quarter')
    expect(seen.at(-1)?.searchParams.get('granularity')).toBe('month')
    expect(screen.getByRole('button', { name: /^Profit and loss statement for quarter \d \d{4} per month$/ })).toBeInTheDocument()
    expect(within(table).getByRole('columnheader', { name: 'Oct 2026' })).toBeInTheDocument()
    expect(within(table).getByRole('row', { name: /^Revenue 8000 200\.00 100\.00$/ })).toBeInTheDocument()
    expect(within(table).getByRole('row', { name: /Housing costs 4100 60\.00 10\.00/ })).toBeInTheDocument()
    expect(within(table).getByRole('row', { name: /^Rent 4110 50\.00 0\.00$/ })).toBeInTheDocument()
    expect(within(table).getByRole('row', { name: /Net profit 140\.00 90\.00/ })).toBeInTheDocument()
    expect(within(table).getByRole('link', { name: 'Rent' })).toHaveAttribute('href', expect.stringContaining('/11/report/journal_entries?ledger_account_id=1201'))
    expect(screen.getByText('All amounts are in EUR and excluding VAT.')).toBeInTheDocument()

    await userEvent.click(within(table).getByRole('button', { name: 'Collapse Housing costs' }))
    expect(within(table).queryByRole('row', { name: /^Rent/ })).toBeNull()
    await userEvent.click(within(table).getByRole('button', { name: 'Expand all' }))
    expect(within(table).getByRole('row', { name: /^Rent/ })).toBeInTheDocument()
    await userEvent.click(within(table).getByRole('button', { name: 'Collapse all' }))
    expect(within(table).queryByRole('row', { name: /^Energy/ })).toBeNull()

    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Profit and loss statement' })).toHaveAttribute('aria-current', 'page')
    expect(within(nav).queryByRole('link', { name: 'Journal entries' })).toBeNull()
  })

  it('steps to the period before and shows it per quarter', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/report/profit_loss?period=202501..202503')
    await screen.findByRole('table', { name: 'Profit and loss statement' })
    await userEvent.click(screen.getByRole('link', { name: 'Go to quarter 4 2024' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('period')).toBe('202410..202412'))
    await userEvent.click(screen.getByRole('button', { name: 'Profit and loss statement for quarter 4 2024 per month' }))
    await userEvent.click(screen.getByRole('radio', { name: 'Quarter' }))
    await userEvent.click(screen.getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('granularity')).toBe('quarter'))
    expect(screen.getByRole('button', { name: 'Profit and loss statement for quarter 4 2024 per quarter' })).toBeInTheDocument()
  })

  it('shows the balance sheet debit beside credit, from the start of the period', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/report/balance_sheet?period=202610..202612&granularity=quarter')
    const debit = await screen.findByRole('table', { name: 'Debit' })
    expect(seen.map((u) => u.searchParams.get('period'))).toContain('202609..202609')
    expect(within(debit).getByRole('columnheader', { name: '2026-09-30' })).toBeInTheDocument()
    expect(within(debit).getByRole('row', { name: /Main bank/ })).toHaveTextContent('150.00')
    const credit = screen.getByRole('table', { name: 'Credit' })
    expect(within(credit).getByRole('row', { name: /^2026 / })).toHaveTextContent('150.00')
    expect(within(credit).getByRole('row', { name: /Current year result/ })).toBeInTheDocument()
  })

  it('lists the debtors by name with the total of the page', async () => {
    server.use(...loaded())
    renderAt('/11/report/debtors')
    const table = await screen.findByRole('table', { name: 'Receivable overview' })
    expect(screen.getByRole('button', { name: /^Aged receivables report for quarter \d \d{4}$/ })).toBeInTheDocument()
    expect(await within(table).findByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/contacts/301')
    expect(within(table).getByRole('row', { name: /No contact/ })).toHaveTextContent('8.00')
    expect(within(table).getByRole('row', { name: /Total this page/ })).toHaveTextContent('250.00')
  })

  it('shows what creditors have open by age up to a month', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/report/creditors_aging?period=202503..202503')
    const table = await screen.findByRole('table', { name: 'Payables aging' })
    expect(seen.at(-1)?.searchParams.get('period_until')).toBe('202503..202503')
    expect(screen.getByRole('button', { name: 'Accounts payable aging up to and including March 2025' })).toBeInTheDocument()
    expect(within(table).getByRole('columnheader', { name: '0 - 30 days' })).toBeInTheDocument()
    expect(within(table).getByRole('row', { name: /Total this page 121\.00 0\.00 0\.00 121\.00/ })).toBeInTheDocument()
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
    expect(within(table).getByRole('row', { name: /No project/ })).toHaveTextContent('20.00')
    expect(within(table).getByRole('row', { name: /Total/ })).toHaveTextContent('29.99')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Revenue by project' })).toHaveAttribute('href', '/11/report/revenue_by_project')
  })

  it('shows the assets of a year under their category', async () => {
    server.use(
      ...loaded(),
      http.get('*/api/v2/11/reports/assets', () => HttpResponse.json([{
        asset_id: '601', ledger_account_id: '1010', name: 'Laptop', purchase_date: '2026-01-15', lifespan: String(5 * 365.2425 * 86400), purchase_value: '1200.0',
        calculated_purchase_value: '1200.0', residual_value: '0.0', total_value_changes_since_purchase: '-200.0', value_at_begin: '0.0',
        investment: '1200.0', depreciation: '-200.0', divestment: '0.0', value_changes: '-200.0', value_at_end: '1000.0', value_not_in_use: '0.0',
      }])),
    )
    renderAt('/11/report/assets_report?period=2026')
    const table = await screen.findByRole('table', { name: 'Assets report' })
    expect(screen.getByText('Assets in 2026')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Go to 2025' })).toBeInTheDocument()
    expect(within(table).getByRole('link', { name: 'Laptop' })).toHaveAttribute('href', '/11/assets/601')
    expect(within(table).getByRole('row', { name: /Laptop/ })).toHaveTextContent(/5 years.*1,000\.00/)
  })
})
