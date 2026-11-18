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

  it('opens a row of the aging report up into its invoices', async () => {
    const seen: URL[] = []
    server.use(
      ...loaded(seen),
      http.get('*/api/v2/11/reports/creditors_aging/documents', ({ request }) => {
        seen.push(new URL(request.url))
        return HttpResponse.json({ documents: [{ document_type: 'PurchaseInvoice', document_id: '701', date: '2025-03-10', amount: '121.0', aging_buckets: [
          { name: '0 - 30 days', amount: '121.0' }, { name: '30 - 60 days', amount: '0' }, { name: 'Older', amount: '0' }, { name: 'Total', amount: '121.0' },
        ] }] })
      }),
      http.get('*/api/v2/11/documents/purchase_invoices/701', () => HttpResponse.json({ id: '701', reference: 'INV-2025-7', entry_number: 7 })),
    )
    renderAt('/11/report/creditors_aging?period=202503..202503')
    const table = await screen.findByRole('table', { name: 'Payables aging' })
    const expand = await within(table).findByRole('button', { name: 'Expand Example Shop B.V.' })
    expect(expand).toHaveAttribute('aria-expanded', 'false')
    expect(seen.some((u) => u.pathname.endsWith('/documents'))).toBe(false)
    await userEvent.click(expand)
    const link = await within(table).findByRole('link', { name: 'INV-2025-7' })
    expect(link).toHaveAttribute('href', '/11/documents/701')
    expect(within(table).getByRole('row', { name: /^INV-2025-7 121\.00 0\.00 0\.00 121\.00$/ })).toBeInTheDocument()
    const asked = seen.find((u) => u.pathname.endsWith('/creditors_aging/documents'))
    expect(asked?.searchParams.get('contact_id')).toBe('301')
    expect(asked?.searchParams.get('period_until')).toBe('202503..202503')
    await userEvent.click(within(table).getByRole('button', { name: 'Collapse Example Shop B.V.' }))
    expect(within(table).queryByRole('link', { name: 'INV-2025-7' })).toBeNull()
  })

  it('pages the reports per contact by 50, as Moneybird', async () => {
    const seen: URL[] = []
    server.use(http.get('*/api/v2/11/reports/debtors', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json({ debtors: [] })
    }), ...loaded(seen))
    renderAt('/11/report/debtors')
    await waitFor(() => expect(seen.find((u) => u.pathname.endsWith('/debtors'))?.searchParams.get('per_page')).toBe('50'))
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

  it('shows the amounts as percentages of revenue on request', async () => {
    server.use(...loaded())
    renderAt('/11/report/profit_loss?period=202610..202611')
    const table = await screen.findByRole('table', { name: 'Profit and loss statement' })
    await userEvent.click(within(table).getByRole('button', { name: 'Show percentages of revenue' }))
    expect(within(table).getByRole('row', { name: /^Rent 4110 50\.00 25% 0\.00 0%$/ })).toBeInTheDocument()
    expect(within(table).getByRole('row', { name: /Net profit 140\.00 70% 90\.00 90%/ })).toBeInTheDocument()
  })

  it('shows the balance sheet at the end of the period without comparison', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/report/balance_sheet?period=202610..202612')
    await screen.findByRole('table', { name: 'Debit' })
    await userEvent.click(screen.getByRole('button', { name: /^Balance sheet for quarter 4 2026 per month$/ }))
    await userEvent.click(screen.getByRole('radio', { name: 'No comparison' }))
    await userEvent.click(screen.getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('granularity')).toBeNull())
    expect(screen.getByRole('button', { name: 'Balance sheet for quarter 4 2026' })).toBeInTheDocument()
    const debit = screen.getByRole('table', { name: 'Debit' })
    expect(within(debit).getAllByRole('columnheader').map((c) => c.textContent)).toEqual(['Collapse all', '2026-12-31'])
  })

  it('shows the VAT report as the Dutch return, the ICP declaration or per rate', async () => {
    const seen: URL[] = []
    const figures = (kind: string) => ({
      kind, period_from: '2026-10-01', period_to: '2026-12-31', granularity: 'quarter', deadline: '2027-01-31', corrections: [],
      boxes: kind === 'vat_return' ? [{ box: '1a', base: '716.0', tax: '150.0', period_base: '716.0', period_tax: '150.0', correction_base: '0', correction_tax: '0' }] : [],
      icp_lines: kind === 'icp_declaration' ? [{ contact_id: '301', name: 'Beispiel GmbH', country: 'DE', vat_number: 'DE123456789', kind: 'services', amount: '500.0' }] : [],
      owed: '150.0', input_tax: '0.0', total: '150.0',
    })
    server.use(
      ...loaded(seen),
      http.get('*/api/v2/11/declarations/overview', ({ request }) => {
        const url = new URL(request.url)
        seen.push(url)
        return HttpResponse.json(figures(url.searchParams.get('kind') ?? ''))
      }),
      http.get('*/api/v2/11/reports/tax', () => HttpResponse.json({ tax_rates: [{ tax_rate_id: '9', name: '21% VAT', type: 'sales_invoice', report_reference: 'NL/1a', amount: '716.0', tax: '150.0' }] })),
    )
    renderAt('/11/report/tax?period=202610..202612')
    const boxes = await screen.findByRole('table', { name: 'VAT report Netherlands' })
    expect(seen.at(-1)?.searchParams.get('period')).toBe('20261001..20261231')
    expect(within(boxes).getByRole('row', { name: /1a.*€716.*€150/ })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'VAT report Netherlands for quarter 4 2026' }))
    await userEvent.click(screen.getByRole('radio', { name: 'Dutch ICP overview' }))
    await userEvent.click(screen.getByRole('button', { name: 'Filter' }))
    const icp = await screen.findByRole('table', { name: 'ICP report Netherlands' })
    expect(within(icp).getByRole('row', { name: /Beispiel GmbH/ })).toHaveTextContent('€500')
    await userEvent.click(screen.getByRole('button', { name: 'ICP report Netherlands for quarter 4 2026' }))
    await userEvent.click(screen.getByRole('radio', { name: 'VAT summary' }))
    await userEvent.click(screen.getByRole('button', { name: 'Filter' }))
    const rates = await screen.findByRole('table', { name: 'VAT summary' })
    expect(within(rates).getByRole('row', { name: /21% VAT/ })).toHaveTextContent(/716\.00\s*150\.00/)
  })

  it('shows the graphs above the profit and loss statement until hidden', async () => {
    server.use(...loaded())
    renderAt('/11/report/profit_loss?period=202610..202611')
    await screen.findByRole('table', { name: 'Profit and loss statement' })
    expect(screen.getByRole('img', { name: 'Net result' })).toBeInTheDocument()
    const costs = screen.getByRole('img', { name: 'Cost and Revenue' }).closest('section')!
    expect(within(costs).getByText('Revenue').closest('li')).toHaveTextContent('300.00')
    expect(within(costs).getByText('Costs').closest('li')).toHaveTextContent('70.00')
    await userEvent.click(screen.getByRole('button', { name: 'Show graphs' }))
    expect(screen.queryByRole('img', { name: 'Net result' })).toBeNull()
  })

  it('gives an overview of the period in six cards', async () => {
    server.use(
      ...loaded(),
      http.get('*/api/v2/11/reports/working_capital', () => HttpResponse.json({ working_capital: '900.0', bank: '1000.0', to_receive: '250.0', to_pay: '350.0', periods: [] })),
      http.get('*/api/v2/11/declarations/overview', () => HttpResponse.json({
        kind: 'vat_return', period_from: '2026-10-01', period_to: '2026-12-31', granularity: 'quarter', deadline: '2027-01-31',
        boxes: [], corrections: [], icp_lines: [], owed: '63.0', input_tax: '12.0', total: '51.0',
      })),
      http.get('*/api/v2/11/reports/tax', () => HttpResponse.json({ tax_rates: [], periods: [] })),
      http.get('*/api/v2/11/reports/bank_balance', () => HttpResponse.json({ total: '1000.0', financial_accounts: [], periods: [] })),
      http.get('*/api/v2/11/reports/cash_flow', () => HttpResponse.json({ opening_balance: '400.0', closing_balance: '1000.0', cash_received_by_ledger_account: values([['1010', '700.0']]), cash_paid_by_ledger_account: values([['1201', '-100.0']]) })),
      http.get('*/api/v2/11/reports/creditors', () => HttpResponse.json({ creditors: [{ contact_id: '301', amount: '-75.0' }] })),
    )
    renderAt('/11/reports?period=202610..202612')
    expect(await screen.findByRole('button', { name: 'Report for quarter 4 2026 per month' })).toBeInTheDocument()
    const card = (name: string) => screen.getByRole('heading', { name: new RegExp(`^${name}`) }).closest('section')!
    await waitFor(() => expect(card('Profit')).toHaveTextContent('€240.00'))
    expect(within(card('Profit')).getByRole('link', { name: 'Profit and loss statement' })).toHaveAttribute('href', expect.stringContaining('/11/report/profit_loss'))
    await waitFor(() => expect(card('Working capital')).toHaveTextContent('€900.00'))
    expect(screen.getByRole('tooltip', { hidden: true })).toHaveTextContent(/current assets minus the current liabilities/)
    await waitFor(() => expect(card('VAT')).toHaveTextContent(/€51\.00.*Payable VAT€63\.00.*Receivable VAT€12\.00/))
    await waitFor(() => expect(card('Bank balance')).toHaveTextContent(/€1,000\.00.*Credited€700\.00.*Debited€100\.00/))
    await waitFor(() => expect(card('Outstanding from customers')).toHaveTextContent(/€250\.00.*Example Shop B\.V\.€242\.00.*No contact€8\.00/))
    await waitFor(() => expect(card('Owed to suppliers')).toHaveTextContent('€75.00'))
    expect(screen.queryByRole('button', { name: 'Export' })).toBeNull()
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Overview' , current: 'page' })).toHaveAttribute('href', '/11/reports')
  })

  it('marks categories as audited in the audit report', async () => {
    const seen: { method: string; url: URL }[] = []
    let audited = false
    server.use(
      ...loaded(),
      http.get('*/api/v2/11/reports/general_ledger', () => HttpResponse.json({
        start_balance: values([['1010', '0.0'], ['1201', '0.0']]), debit_sums: values([['1201', '50.0']]),
        credit_sums: values([['1010', '50.0']]), final_balance: values([['1010', '-50.0'], ['1201', '50.0']]),
      })),
      http.get('*/api/v2/11/tax_rates', () => HttpResponse.json([{ id: '21', name: '21% VAT' }])),
      http.get('*/api/v2/11/users', () => HttpResponse.json([{ id: '7', name: 'Jan Voorbeeld', user_type: 'owner' }])),
      http.get('*/api/v2/11/reports/audit', () => HttpResponse.json({
        period_from: '2026-10-01', period_to: '2026-12-31', ledger_accounts: [
          { ledger_account_id: '1201', debit: '50.0', credit: '0.0', tax_rate_ids: ['21'], changed: false,
            audit: audited ? { user_id: '7', audited_at: '2026-11-18T10:00:00Z', debit: '50.0', credit: '0.0' } : null },
        ],
      })),
      http.post('*/api/v2/11/reports/audit/:id', ({ request }) => {
        seen.push({ method: 'POST', url: new URL(request.url) })
        audited = true
        return HttpResponse.json({ ledger_account_id: '1201', debit: '50.0', credit: '0.0', tax_rate_ids: [], changed: false, audit: null })
      }),
    )
    renderAt('/11/report/audit_report?period=202501..202503')
    const table = await screen.findByRole('table', { name: 'Audit report' })
    expect(screen.getByText('0 of 2 audited')).toBeInTheDocument()
    const rent = within(table).getByRole('row', { name: /^Rent 4110/ })
    expect(rent).toHaveTextContent(/0\.00\s*50\.00\s*0\.00\s*50\.00\s*21% VAT/)
    await userEvent.click(within(rent).getByRole('button', { name: 'Mark Rent as audited' }))
    expect(seen[0]?.url.pathname).toBe('/api/v2/11/reports/audit/1201')
    expect(seen[0]?.url.searchParams.get('period')).toBe('202501..202503')
    expect(await within(table).findByRole('button', { name: /Rent audited by Jan Voorbeeld/ })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText('1 of 2 audited')).toBeInTheDocument()
  })
})
