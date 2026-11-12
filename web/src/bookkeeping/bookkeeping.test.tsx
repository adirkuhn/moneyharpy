import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, identity, listing, server, signedIn } from '../test/server'
import type { BookkeepingSettings, LedgerAccount, TaxRate, Template, TemplateAccount } from './queries'
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

  it('goes back to the invoice that asked, with the new rate on its row, as Moneybird does', async () => {
    server.use(...loaded(), http.post('*/api/v2/11/tax_rates', () => HttpResponse.json(rate({ id: '903' }), { status: 201 })))
    const field = 'sales_invoice[details_attributes][1][tax_rate_id]'
    renderAt(`/11/tax_rates/new?return_to=${encodeURIComponent('/11/sales_invoices/5')}&new_entity_field=${encodeURIComponent(field)}`)
    await userEvent.type(await screen.findByLabelText('Name'), 'Reduced')
    await userEvent.type(screen.getByLabelText('Percentage'), '9')
    expect(screen.getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/11/sales_invoices/5')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/5$/))
    expect(JSON.parse(screen.getByTestId('state').textContent || 'null')).toEqual({ details: true, newEntity: { field, id: '903' } })
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

const fromInvoice = `return_to=${encodeURIComponent('/11/sales_invoices/5/edit')}&source=sales_invoice`
/** Moneybird's new_entity_field of the invoice's first row, whose category is being added. */
const rowField = 'sales_invoice[details_attributes][0][ledger_account_id]'
const fromInvoiceRow = `${fromInvoice}&new_entity_field=${encodeURIComponent(rowField)}`
/** What the invoice gets back: its details open, with the new category on the row that asked. */
const backWith = (id: string) => ({ details: true, newEntity: { field: rowField, id } })
const passedState = () => JSON.parse(screen.getByTestId('state').textContent || 'null')

const entry = (over: Partial<TemplateAccount> & Pick<TemplateAccount, 'key' | 'name'>): TemplateAccount => ({
  parent: null, account_type: 'expenses', account_id: '', rgs_code: '', description: '', exists: false, selected: false, choices: [], ...over,
})
const theme = (over: Partial<Template> & Pick<Template, 'key' | 'name'>): Template => ({
  description: '', intro: '', warning: '', name_label: '', choice: null, single: null, applied: false, ledger_accounts: [], ...over,
})
const leased = ['financial_lease', 'asset']
const car = theme({
  key: 'car_set', name: 'Company car', description: 'Ledger accounts related to the company car',
  intro: 'When you want to breakdown the car costs, you can use the following categories.', warning: 'with private use, you also have tax for aggregation.',
  choice: {
    label: 'Financing of the car', prompt: 'Based on the financing we will show possible categories.',
    options: [{ key: 'operational_lease', name: 'Operational lease', rgs_code: '' }, { key: 'financial_lease', name: 'Financial lease', rgs_code: '' }, { key: 'asset', name: 'Paid with own money', rgs_code: '' }],
  },
  ledger_accounts: [
    entry({ key: 'company_car', name: 'Company car', account_type: 'non_current_assets', selected: true, choices: leased }),
    entry({ key: 'depreciation', name: 'Depreciation costs', exists: true, selected: true, choices: leased }),
    entry({ key: 'company_car_depreciation', parent: 'depreciation', name: 'Company car depreciation', description: 'The total of depreciation for the car.', selected: true, choices: leased }),
    entry({ key: 'company_car_book_result', parent: 'depreciation', name: 'Book result company car' }),
    entry({ key: 'company_car_costs', name: 'Company car costs', exists: true, choices: ['operational_lease', ...leased] }),
    entry({ key: 'company_car_lease_costs', parent: 'company_car_costs', name: 'Company car lease costs', selected: true, choices: ['operational_lease'] }),
  ],
})
const staff = theme({
  key: 'staff_set', name: 'Staff', description: 'Personnel costs and debts', intro: 'These categories are used for entering staff costs.',
  ledger_accounts: [
    entry({ key: 'staff', name: 'Staff', selected: true }),
    entry({ key: 'wages', parent: 'staff', name: 'Gross wages', selected: true }),
    entry({ key: 'hiring', parent: 'staff', name: 'Hiring' }),
  ],
})
const entrepreneur = theme({
  key: 'entrepreneur', name: 'Entrepreneur', description: 'The entrepreneur and the company', intro: 'Privately paid invoices.', name_label: 'Name entrepreneur',
  ledger_accounts: [
    entry({ key: 'entrepreneur_capital', name: 'Private {name} (capital)', account_type: 'equity', selected: true }),
    entry({ key: 'entrepreneur_withdrawals', parent: 'entrepreneur_capital', name: 'Private withdrawals {name}', account_type: 'equity', selected: true }),
  ],
})
const revenue = theme({
  key: 'revenue', name: 'Revenue', description: 'Income generated from the sale of your products or services.', intro: 'By breaking down your revenue by origin, you gain more insight.',
  single: {
    account_type: 'revenue', name_label: 'Name', options_label: '', options_help: '', parent: true,
    options: [{ key: 'net_sales_of_trade_goods', name: 'Revenue from trade goods', rgs_code: 'WOmzNoh' }, { key: 'revenue_from_provision_of_services', name: 'Revenue from services', rgs_code: 'WOmzNod' }],
  },
})
const fixedAssets = theme({
  key: 'fixed_assets', name: 'Fixed assets', description: 'Ledger accounts for the processing of your investments.', intro: 'Large purchases that will last for a long time.',
  single: {
    account_type: 'non_current_assets', name_label: 'Category name', options_label: 'Group', options_help: 'Select the group to which the category belongs', parent: false,
    options: [{ key: 'fixed_asset', name: 'Fixed assets', rgs_code: 'BMvaObe' }, { key: 'inventory', name: 'Inventory', rgs_code: 'BMvaBei' }],
  },
})
const housing = theme({
  key: 'housing', name: 'Housing', description: 'Housing costs', intro: 'If you want to breakdown housing costs.', applied: true,
  ledger_accounts: [entry({ key: 'housing', name: 'Housing costs', exists: true, selected: true })],
})
const sales = account({ id: '1010', name: 'Revenue', account_type: 'revenue', account_id: '8000', uri: '/11/ledger_accounts/1010', taxonomy_item: null })

const withTemplates = (applied: { key: string; body: unknown }[] = [], legalForm: string | null = null) => [
  http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([payable, office, software, old, sales])),
  ...loaded(),
  http.get('*/api/v2/11/identities', () => HttpResponse.json([identity({ default: true, legal_entity_type: legalForm })])),
  http.get('*/api/v2/11/ledger_account_templates', () => HttpResponse.json([revenue, car, housing, staff, entrepreneur, fixedAssets])),
  http.post('*/api/v2/11/ledger_account_templates/:key', async ({ params, request }) => {
    applied.push({ key: String(params.key), body: await request.json() })
    return HttpResponse.json([account({ id: '1020', name: 'Workshops' })], { status: 201 })
  }),
]

const creating = (sink: { body?: unknown }) =>
  http.post('*/api/v2/11/ledger_accounts', async ({ request }) => {
    sink.body = await request.json()
    return HttpResponse.json(account({ id: '1005' }), { status: 201 })
  })

describe('add category', () => {
  it('is reached from the categories of the bookkeeping settings', async () => {
    server.use(...loaded())
    renderAt('/11/settings/bookkeeping')
    const categories = await screen.findByRole('region', { name: 'Categories' })
    expect(within(categories).getByRole('link', { name: 'Add category' })).toHaveAttribute('href', '/11/ledger_accounts/blueprints')
  })

  it('offers the themes and a single category, after Moneybird', async () => {
    server.use(...withTemplates())
    renderAt(`/11/ledger_accounts/blueprints?${fromInvoice}`)
    expect(await screen.findByRole('heading', { name: 'Add category for Invoice', level: 1 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'To invoice' })).toHaveAttribute('href', '/11/sales_invoices/5/edit')
    expect(screen.getByText(/determined by the country of your administration/)).toBeInTheDocument()
    const themes = screen.getByRole('list', { name: 'Themes' })
    expect(within(themes).getAllByRole('link').map((l) => l.getAttribute('aria-label'))).toEqual(['Revenue', 'Company car', 'Housing', 'Staff', 'Entrepreneur', 'Fixed assets'])
    const carLink = within(themes).getByRole('link', { name: 'Company car' })
    expect(carLink).toHaveAttribute('href', `/11/ledger_accounts/blueprint_ledger_accounts/new?ledger_account_set=car_set&${fromInvoice}`)
    expect(carLink).toHaveTextContent('Ledger accounts related to the company car')
    expect(within(themes).getByRole('link', { name: 'Housing' })).toHaveTextContent('Added')
    expect(screen.getByText(/add it separately based on the Referentie Grootboekschema \(RGS\)/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add single category' })).toHaveAttribute('href', `/11/ledger_accounts/new?${fromInvoice}`)
  })

  it('names the legal form the categories are determined by, when the company has one', async () => {
    server.use(...withTemplates([], 'VOF - Vennootschap onder firma'))
    renderAt('/11/ledger_accounts/blueprints')
    expect(await screen.findByText(/Below categories are determined by your chosen legal form:/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'VOF - Vennootschap onder firma' })).toHaveAttribute('href', '/11/identities/edit_default')
    expect(screen.queryByText(/country of your administration/)).not.toBeInTheDocument()
  })

  it('goes back to the settings, and never to another site', async () => {
    server.use(...withTemplates())
    renderAt(`/11/ledger_accounts/blueprints?return_to=${encodeURIComponent('https://example.com/11/')}`)
    expect(await screen.findByRole('heading', { name: 'Add category', level: 1 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'To Bookkeeping' })).toHaveAttribute('href', '/11/settings/bookkeeping#categories')
    expect(screen.getByRole('link', { name: 'Add single category' })).toHaveAttribute('href', '/11/ledger_accounts/new')
  })

  it('still opens from the old templates address', async () => {
    server.use(...withTemplates())
    renderAt('/11/ledger_account_templates')
    expect(await screen.findByRole('heading', { name: 'Add category', level: 1 })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent('/11/ledger_accounts/blueprints')
  })
})

describe('add categories from a theme', () => {
  it('shows the categories for the financing chosen, ticked one by one, then goes back to the invoice', async () => {
    const applied: { key: string; body: unknown }[] = []
    server.use(...withTemplates(applied))
    renderAt(`/11/ledger_accounts/blueprint_ledger_accounts/new?ledger_account_set=car_set&${fromInvoice}`)
    expect(await screen.findByRole('heading', { name: 'Add category for Invoice from Company car', level: 1 })).toBeInTheDocument()
    expect(screen.getByText('When you want to breakdown the car costs, you can use the following categories.')).toBeInTheDocument()
    expect(screen.getByText('Warning:')).toBeInTheDocument()
    expect(screen.getByText('Based on the financing we will show possible categories.')).toBeInTheDocument()
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()

    await userEvent.selectOptions(screen.getByLabelText('Financing of the car'), 'Operational lease')
    expect(screen.queryByRole('checkbox', { name: 'Company car' })).not.toBeInTheDocument()
    const costs = screen.getByRole('checkbox', { name: 'Company car costs' })
    expect(costs).toBeChecked()
    expect(costs).toBeDisabled()
    expect(costs).toHaveAccessibleDescription('Category already exists')
    expect(screen.getByRole('checkbox', { name: 'Company car lease costs' })).toBeChecked()

    await userEvent.selectOptions(screen.getByLabelText('Financing of the car'), 'Financial lease')
    const balance = screen.getByRole('group', { name: 'Balance sheet' })
    expect(within(balance).getByRole('heading', { name: 'Non-current assets' })).toBeInTheDocument()
    expect(within(balance).getByRole('checkbox', { name: 'Company car' })).toBeChecked()
    const pl = screen.getByRole('group', { name: 'Profit and loss statement' })
    // Sub-categories sit in their parent's list.
    const depreciation = within(pl).getByRole('checkbox', { name: 'Depreciation costs' }).closest('li')!
    const bookResult = within(depreciation).getByRole('checkbox', { name: 'Book result company car' })
    expect(bookResult).not.toBeChecked()
    expect(within(depreciation).getByRole('button', { name: 'More information' })).toHaveAccessibleDescription('The total of depreciation for the car.')
    expect(screen.queryByRole('checkbox', { name: 'Company car lease costs' })).not.toBeInTheDocument()

    await userEvent.click(bookResult)
    await userEvent.click(within(balance).getByRole('checkbox', { name: 'Company car' }))
    expect(screen.getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/11/sales_invoices/5/edit')
    await userEvent.click(screen.getByRole('button', { name: 'Save and go back to the invoice' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/5\/edit$/))
    expect(applied).toEqual([{ key: 'car_set', body: { ledger_accounts: ['company_car_depreciation', 'company_car_book_result'] } }])
  })

  it('adds a parent along with its sub-categories, and keeps it ticked', async () => {
    const applied: { key: string; body: unknown }[] = []
    server.use(...withTemplates(applied))
    renderAt('/11/ledger_accounts/blueprint_ledger_accounts/new?ledger_account_set=staff_set')
    expect(await screen.findByRole('heading', { name: 'Add category from Staff', level: 1 })).toBeInTheDocument()
    const parent = screen.getByRole('checkbox', { name: 'Staff' })
    expect(parent).toBeChecked()
    expect(parent).toBeDisabled()
    await userEvent.click(screen.getByRole('checkbox', { name: 'Gross wages' }))
    await userEvent.click(screen.getByRole('checkbox', { name: 'Hiring' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(applied).toEqual([{ key: 'staff_set', body: { ledger_accounts: ['staff', 'hiring'] } }]))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/settings\/bookkeeping$/))
  })

  it('completes the names with the entrepreneur', async () => {
    const applied: { key: string; body: unknown }[] = []
    server.use(...withTemplates(applied))
    renderAt('/11/ledger_accounts/blueprint_ledger_accounts/new?ledger_account_set=entrepreneur')
    expect(await screen.findByRole('checkbox', { name: 'Private {name} (capital)' })).toBeChecked()
    await userEvent.type(screen.getByLabelText('Name entrepreneur'), 'Ann')
    expect(screen.getByRole('checkbox', { name: 'Private Ann (capital)' })).toBeDisabled()
    expect(screen.getByRole('checkbox', { name: 'Private withdrawals Ann' })).toBeChecked()
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(applied).toEqual([{ key: 'entrepreneur', body: { ledger_accounts: ['entrepreneur_capital', 'entrepreneur_withdrawals'], name: 'Ann' } }]))
  })

  it('adds nothing when every category is there already', async () => {
    const applied: { key: string; body: unknown }[] = []
    server.use(...withTemplates(applied))
    renderAt('/11/ledger_accounts/blueprint_ledger_accounts/new?ledger_account_set=housing')
    expect(await screen.findByRole('heading', { name: 'Add category from Housing', level: 1 })).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Housing costs' })).toHaveAccessibleDescription('Category already exists')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/settings\/bookkeeping$/))
    expect(applied).toEqual([])
  })

  it('adds one revenue category of the kind chosen, under the parent chosen', async () => {
    const applied: { key: string; body: unknown }[] = []
    server.use(...withTemplates(applied))
    renderAt(`/11/ledger_accounts/blueprint_ledger_accounts/new?ledger_account_set=revenue&${fromInvoiceRow}`)
    expect(await screen.findByRole('heading', { name: 'Add category for Invoice from Revenue', level: 1 })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Revenue from trade goods' })).toBeChecked()
    const parent = screen.getByLabelText('Parent category')
    expect(within(parent).getAllByRole('option').map((o) => o.textContent)).toEqual(['None', 'Revenue'])
    await userEvent.type(screen.getByLabelText('Name'), 'Workshops')
    await userEvent.selectOptions(parent, 'Revenue')
    await userEvent.click(screen.getByRole('radio', { name: 'Revenue from services' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save and go back to the invoice' }))
    await waitFor(() => expect(applied).toEqual([{ key: 'revenue', body: { name: 'Workshops', option: 'revenue_from_provision_of_services', parent_id: '1010' } }]))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/5\/edit$/))
    expect(passedState()).toEqual(backWith('1020'))
  })

  it('adds a fixed asset category to the group chosen', async () => {
    const applied: { key: string; body: unknown }[] = []
    server.use(...withTemplates(applied))
    renderAt('/11/ledger_accounts/blueprint_ledger_accounts/new?ledger_account_set=fixed_assets')
    await userEvent.type(await screen.findByLabelText('Category name'), 'Laptops')
    expect(screen.queryByLabelText('Parent category')).not.toBeInTheDocument()
    await userEvent.selectOptions(screen.getByLabelText('Group'), 'Inventory')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(applied).toEqual([{ key: 'fixed_assets', body: { name: 'Laptops', option: 'inventory' } }]))
  })

  it('does not know other themes', async () => {
    server.use(...withTemplates())
    renderAt('/11/ledger_accounts/blueprint_ledger_accounts/new?ledger_account_set=spaceship')
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  })
})

describe('add single category', () => {
  it('suggests chart scheme codes for the name and goes back to the invoice, after Moneybird', async () => {
    const sent: { body?: unknown } = {}
    server.use(
      http.get('*/api/v2/11/taxonomy_items', ({ request }) => {
        const url = new URL(request.url)
        const found = url.searchParams.get('query') === 'Consulting' && url.searchParams.get('account_type') === 'revenue'
        return HttpResponse.json(found ? [{ taxonomy_version: '3.8', code: 'WOmzNodOdh', name: 'Netto-omzet diensten', name_english: null, reference: null, level: 4, side: 'profit_and_loss' }] : [])
      }),
      ...loaded(), creating(sent),
    )
    renderAt(`/11/ledger_accounts/new?${fromInvoiceRow}`)
    expect(await screen.findByRole('heading', { name: 'Add single category', level: 1 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'To Add category' })).toHaveAttribute('href', `/11/ledger_accounts/blueprints?${fromInvoiceRow}`)
    const form = screen.getByRole('form', { name: 'Add single category' })
    expect(within(form).getByText('Profit and loss statement')).toBeInTheDocument()
    expect(within(form).getByText('Revenue')).toBeInTheDocument()
    expect(screen.getByText('Enter a name on the left, we will suggest the most appropriate category.')).toBeInTheDocument()
    await userEvent.type(within(form).getByLabelText('Name'), 'Consulting')
    const suggestions = await screen.findByRole('radiogroup', { name: 'Suggested categories' })
    await userEvent.click(await within(suggestions).findByRole('radio', { name: /WOmzNodOdh/ }))
    expect(within(form).getByText(/Netto-omzet diensten/)).toBeInTheDocument()
    expect(within(form).getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/11/sales_invoices/5/edit')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(sent.body).toEqual({
        ledger_account: { name: 'Consulting', account_type: 'revenue', account_id: '', parent_id: '', description: '' },
        rgs_code: 'WOmzNodOdh',
      }),
    )
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/sales_invoices\/5\/edit$/))
    expect(passedState()).toEqual(backWith('1005'))
  })

  it('takes a type, parent, number and code chosen by hand', async () => {
    const sent: { body?: unknown } = {}
    server.use(...loaded(), creating(sent))
    renderAt('/11/ledger_accounts/new')
    const form = await screen.findByRole('form', { name: 'Add single category' })
    expect(within(form).getByText('Expenses')).toBeInTheDocument()
    await userEvent.type(within(form).getByLabelText('Name'), 'Hosting')
    await userEvent.click(screen.getByRole('button', { name: 'Select a category manually' }))
    expect(screen.getByRole('heading', { name: 'Select a category' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('radio', { name: 'Balance sheet' }))
    await userEvent.click(screen.getByRole('radio', { name: 'Current liabilities' }))
    expect(within(form).getByText('Current liabilities')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('radio', { name: 'Profit and loss statement' }))
    await userEvent.click(screen.getByRole('radio', { name: 'Expenses' }))
    await userEvent.selectOptions(within(form).getByLabelText('Parent category'), '1001')
    await userEvent.type(within(form).getByLabelText('Account number'), '4460')
    await userEvent.type(within(form).getByLabelText('Description'), 'Servers')
    await userEvent.type(screen.getByLabelText('Chart scheme code'), 'WBedKanSof')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(sent.body).toEqual({
        ledger_account: { name: 'Hosting', account_type: 'expenses', account_id: '4460', parent_id: '1001', description: 'Servers' },
        rgs_code: 'WBedKanSof',
      }),
    )
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/settings\/bookkeeping$/))
  })

  it('only asks for the type without a chart scheme', async () => {
    const sent: { body?: unknown } = {}
    server.use(...loaded(settings({ chart_scheme: '' })), creating(sent))
    renderAt('/11/ledger_accounts/new')
    const form = await screen.findByRole('form', { name: 'Add single category' })
    expect(screen.queryByText(/we will suggest/)).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Chart scheme code')).not.toBeInTheDocument()
    await userEvent.type(within(form).getByLabelText('Name'), 'Rent')
    await userEvent.click(screen.getByRole('radio', { name: 'Expenses' }))
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.body).toEqual({ ledger_account: { name: 'Rent', account_type: 'expenses', account_id: '', parent_id: '', description: '' } }))
  })
})

describe('category form', () => {
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
    const parent = await screen.findByLabelText('Parent category')
    const options = within(parent).getAllByRole('option').map((o) => o.textContent)
    expect(options.some((o) => o?.includes('Software'))).toBe(false)
    expect(options.some((o) => o?.includes('Office costs'))).toBe(false)
  })
})

describe('tree', () => {
  it('puts children after their parent, and orphans at the top', () => {
    const orphan = account({ id: '9', parent_id: '404', name: 'Orphan' })
    expect(tree([software, orphan, office]).map((r) => [r.account.name, r.depth])).toEqual([['Orphan', 0], ['Office costs', 0], ['Software', 1]])
  })
})
