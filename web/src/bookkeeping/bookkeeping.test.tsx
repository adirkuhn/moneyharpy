import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { BookkeepingSettings, LedgerAccount, TaxRate, Template } from './queries'
import { tree } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const settings = (over: Partial<BookkeepingSettings> = {}): BookkeepingSettings => ({
  vat_documents_period: 'quarter',
  intra_community_transactions_declaration_period: 'quarter',
  chart_scheme: 'nl-rgs-3.8',
  period_locked_until: null,
  chart_schemes: [{ id: 'nl-rgs-3.8', country: 'NL', name: 'Referentie Grootboekschema (RGS)', version: '3.8', source: 'https://example.com' }],
  tax_report_boxes: [{ code: 'NL/1a', name: 'Supplies taxed at the standard rate' }, { code: 'NL/5b', name: 'Input tax' }],
  ...over,
})

const rate = (over: Partial<TaxRate> = {}): TaxRate => ({
  id: '901', administration_id: '11', name: 'VAT 21%', percentage: '21', tax_rate_type: 'sales_invoice', show_tax: true, active: true,
  country: null, created_at: stamp, updated_at: stamp, uri: '/11/tax_rates/901', tax_report_boxes: ['NL/1a'], position: 1, ...over,
})

const account = (over: Partial<LedgerAccount> = {}): LedgerAccount => ({
  id: '1001', administration_id: '11', name: 'Office costs', account_type: 'expenses', account_id: '4400', parent_id: null, active: true,
  allowed_document_types: ['purchase_invoice'], financial_account_id: null, created_at: stamp, updated_at: stamp,
  taxonomy_item: { taxonomy_version: '3.8', code: 'WBedKan', name: 'Kantoorkosten', name_english: null, reference: null },
  uri: '/11/ledger_accounts/1001', description: null, system_role: null, template: null, ...over,
})

const office = account()
const software = account({ id: '1002', name: 'Software', account_id: '4430', parent_id: '1001', uri: '/11/ledger_accounts/1002', taxonomy_item: null })
const payable = account({ id: '1003', name: 'Accounts payable', account_type: 'current_liabilities', account_id: '1600', system_role: 'accounts_payable', uri: '/11/ledger_accounts/1003', taxonomy_item: null })
const old = account({ id: '1004', name: 'Old costs', account_id: '4999', active: false, uri: '/11/ledger_accounts/1004', taxonomy_item: null })

const loaded = (s = settings()) => [
  signedIn(),
  listing([harpy]),
  http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
  http.get('*/api/v2/11/bookkeeping_settings', () => HttpResponse.json(s)),
  http.get('*/api/v2/11/tax_rates', () =>
    HttpResponse.json([rate(), rate({ id: '902', name: 'Reverse charge', show_tax: false, tax_rate_type: 'purchase_invoice', tax_report_boxes: ['NL/5b'], uri: '/11/tax_rates/902' })]),
  ),
  http.get('*/api/v2/11/tax_rates/901', () => HttpResponse.json(rate())),
  http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([payable, office, software, old])),
  http.get('*/api/v2/11/taxonomy_items', () => HttpResponse.json([])),
]

describe('bookkeeping settings', () => {
  it('shows every part and the bookkeeping menu', async () => {
    server.use(...loaded())
    renderAt('/11/settings/bookkeeping')
    expect(await screen.findByRole('heading', { name: 'Bookkeeping', level: 1 })).toBeInTheDocument()
    for (const name of ['Declaration period', 'VAT rates', 'Categories', 'Chart scheme', 'Locked period', 'Export']) {
      expect(screen.getByRole('region', { name })).toBeInTheDocument()
    }
    const nav = screen.getByRole('navigation', { name: 'Settings' })
    expect(within(nav).getByRole('link', { name: 'VAT rates' })).toHaveAttribute('href', '/11/settings/bookkeeping#vat-rates')
    expect(screen.getByRole('link', { name: 'Read more at the Belastingdienst' })).toBeInTheDocument()
  })

  it('asks before changing the declaration period', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/bookkeeping_settings', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(settings({ vat_documents_period: 'month' }))
    }))
    renderAt('/11/settings/bookkeeping')
    const vat = await screen.findByRole('group', { name: 'VAT return' })
    expect(within(vat).getByRole('radio', { name: 'Quarter' })).toBeChecked()
    await userEvent.click(within(vat).getByRole('radio', { name: 'Month' }))
    expect(sent).toBeUndefined()
    expect(screen.getByRole('alert')).toHaveTextContent('covers a Month')
    await userEvent.click(screen.getByRole('button', { name: 'Change' }))
    await waitFor(() => expect(sent).toEqual({ bookkeeping_settings: { vat_documents_period: 'month' } }))
  })

  it('lists VAT rates per kind with their boxes', async () => {
    server.use(...loaded())
    renderAt('/11/settings/bookkeeping')
    const sales = await screen.findByRole('table', { name: 'Sales' })
    expect(within(sales).getByRole('link', { name: 'VAT 21%' })).toHaveAttribute('href', '/11/tax_rates/901')
    expect(within(sales).getByText('1a')).toBeInTheDocument()
    const purchases = screen.getByRole('table', { name: 'Purchases' })
    expect(within(purchases).getByText('No VAT on invoice and VAT 21%')).toBeInTheDocument()
    expect(screen.getByText('No VAT rates of this kind.')).toBeInTheDocument()
  })

  it('lists categories as a tree and keeps system ones', async () => {
    let deactivated = false
    server.use(...loaded(), http.delete('*/api/v2/11/ledger_accounts/1002', () => {
      deactivated = true
      return HttpResponse.json({ ...software, active: false })
    }))
    renderAt('/11/settings/bookkeeping')
    const pl = await screen.findByRole('table', { name: 'Profit and loss' })
    expect(await within(pl).findByRole('rowheader', { name: 'Expenses' })).toBeInTheDocument()
    expect(within(pl).getByText('WBedKan')).toBeInTheDocument()
    expect(within(pl).getByRole('button', { name: 'Reactivate Old costs' })).toBeInTheDocument()
    await userEvent.click(within(pl).getByRole('button', { name: 'Deactivate Software' }))
    await waitFor(() => expect(deactivated).toBe(true))
    const balance = screen.getByRole('table', { name: 'Balance sheet' })
    expect(within(balance).getByRole('link', { name: 'Accounts payable' })).toBeInTheDocument()
    expect(within(balance).queryByRole('button', { name: 'Deactivate Accounts payable' })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Edit chart scheme codes' })).toHaveAttribute('href', '/11/ledger_accounts/scheme')
  })

  it('locks the books up to a date', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/bookkeeping_settings', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(settings({ period_locked_until: '2026-06-30' }))
    }))
    renderAt('/11/settings/bookkeeping')
    const block = await screen.findByRole('region', { name: 'Locked period' })
    expect(within(block).getByText('The books are not locked.')).toBeInTheDocument()
    await userEvent.type(within(block).getByLabelText('Locked until'), '2026-06-30')
    await userEvent.click(within(block).getByRole('button', { name: 'Lock' }))
    await waitFor(() => expect(sent).toEqual({ bookkeeping_settings: { period_locked_until: '2026-06-30' } }))
    expect(await within(screen.getByRole('region', { name: 'Locked period' })).findByText(/locked up to and including/)).toBeInTheDocument()
  })
})

describe('VAT rate form', () => {
  it('creates a rate with its boxes', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/11/tax_rates', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(rate({ id: '903' }), { status: 201 })
    }))
    renderAt('/11/tax_rates/new')
    await userEvent.type(await screen.findByLabelText('Name'), 'Reduced')
    await userEvent.type(screen.getByLabelText('Percentage'), '9')
    await userEvent.click(screen.getByRole('checkbox', { name: '1a Supplies taxed at the standard rate' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(sent).toEqual({ tax_rate: { name: 'Reduced', percentage: '9', tax_rate_type: 'sales_invoice', show_tax: true, active: true, country: '', tax_report_boxes: ['NL/1a'] } }),
    )
  })

  it('shows problems beside the fields', async () => {
    server.use(...loaded(), http.post('*/api/v2/11/tax_rates', () =>
      HttpResponse.json({ error: { percentage: ['must be a number from 0 to 100 with at most 2 decimals'] } }, { status: 422 }),
    ))
    renderAt('/11/tax_rates/new')
    await userEvent.type(await screen.findByLabelText('Name'), 'Bad')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText(/Percentage must be a number/)).toBeVisible()
  })
})

describe('category form', () => {
  it('creates a category under a parent with a chart scheme code', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/11/ledger_accounts', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(account({ id: '1005' }), { status: 201 })
    }))
    renderAt('/11/ledger_accounts/new')
    await userEvent.type(await screen.findByLabelText('Name'), 'Hosting')
    await userEvent.selectOptions(screen.getByLabelText('Part of'), '1001')
    await userEvent.type(screen.getByLabelText('Chart scheme code'), 'WBedKanSof')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(sent).toEqual({
        ledger_account: { name: 'Hosting', account_type: 'expenses', account_id: '', parent_id: '1001', description: '', active: true },
        rgs_code: 'WBedKanSof',
      }),
    )
  })

  it('locks the type of a system category and offers no parent loops', async () => {
    server.use(...loaded())
    renderAt('/11/ledger_accounts/1003')
    expect(await screen.findByText(/Your books rely on this category/)).toBeInTheDocument()
    expect(screen.getByLabelText('Type')).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Deactivate' })).not.toBeInTheDocument()
  })

  it('does not offer a category or its children as its parent', async () => {
    server.use(...loaded())
    renderAt('/11/ledger_accounts/1001')
    const parent = await screen.findByLabelText('Part of')
    const options = within(parent).getAllByRole('option').map((o) => o.textContent)
    expect(options.some((o) => o?.includes('Software'))).toBe(false)
    expect(options.some((o) => o?.includes('Office costs'))).toBe(false)
  })
})

describe('templates', () => {
  it('adds a template once', async () => {
    let applied = ''
    const car: Template = { key: 'company_car', name: 'Company car', description: 'Costs of a company car', applied: false, ledger_accounts: [{ name: 'Fuel', account_type: 'expenses', account_id: '4251' }] }
    server.use(...loaded(),
      http.get('*/api/v2/11/ledger_account_templates', () => HttpResponse.json([car, { ...car, key: 'loan', name: 'Loan', applied: true }])),
      http.post('*/api/v2/11/ledger_account_templates/:key', ({ params }) => {
        applied = String(params.key)
        return HttpResponse.json([], { status: 201 })
      }))
    renderAt('/11/ledger_account_templates')
    const loan = await screen.findByRole('region', { name: 'Loan' })
    expect(within(loan).getByText('Added')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Add Company car' }))
    await waitFor(() => expect(applied).toBe('company_car'))
  })
})

describe('tree', () => {
  it('puts children after their parent, and orphans at the top', () => {
    const orphan = account({ id: '9', parent_id: '404', name: 'Orphan' })
    expect(tree([software, orphan, office]).map((r) => [r.account.name, r.depth])).toEqual([['Orphan', 0], ['Office costs', 0], ['Software', 1]])
  })
})
