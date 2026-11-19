import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { Asset } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

const asset = (over: Partial<Asset> = {}): Asset => ({
  id: '701', administration_id: '11', ledger_account_id: '600', name: 'Laptop', purchase_date: '2026-06-01', purchase_value: '1200.0',
  calculated_purchase_value: '1200.0', current_value: '1100.0', uri: '/11/assets/701',
  value_change_plan: { lifespan_in_years: 1, residual_value: '0.0', active: true, monthly_depreciation: '100.0' },
  disposal: null, sources: [], reinvestment_reserve_purchase: null, notes: [], events: [],
  value_changes: [{ type: 'linear', date: '2026-06-30', amount: '-100.0', description: 'Linear depreciation', externally_booked: false, asset_id: '701' }],
  ...over,
})

const sold = asset({
  id: '702', name: 'Old van', current_value: '0.0', uri: '/11/assets/702',
  disposal: { date: '2026-10-01', reason: 'sold', externally_booked: false, asset_id: '702', reinvestment_reserve_sale: null },
})

const accounts = [
  { id: '600', name: 'Company car', account_type: 'non_current_assets', active: true },
  { id: '601', name: 'Inventory', account_type: 'non_current_assets', active: false },
  { id: '800', name: 'Revenue', account_type: 'revenue', active: true },
]

function loaded(seen: URL[] = [], all = [asset(), sold]) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json(accounts)),
    http.get('*/api/v2/11/users', () => HttpResponse.json([{ id: '1', name: 'Ann de Vries' }])),
    http.get('*/api/v2/11/assets', ({ request }) => {
      const url = new URL(request.url)
      seen.push(url)
      const list = url.searchParams.get('active') === 'false' ? all : all.filter((a) => !a.disposal)
      return HttpResponse.json(list.slice(0, Number(url.searchParams.get('per_page') ?? 25)), { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/assets/701', () => HttpResponse.json(asset())),
  ]
}

describe('the asset list', () => {
  it('lists the active assets as Moneybird does, and the deactivated ones on request', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/assets')
    const table = await screen.findByRole('table', { name: 'Assets' })
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Name', 'Category', 'Commissioning date', 'Current value', 'Status'])
    const row = within(table).getAllByRole('row')[1] as HTMLElement
    expect(within(row).getByRole('link', { name: 'Laptop' })).toHaveAttribute('href', '/11/assets/701')
    expect(row).toHaveTextContent('Company car')
    expect(row).toHaveTextContent('2026-06-01')
    expect(row).toHaveTextContent('Active')
    expect(screen.getByRole('link', { name: 'Add' })).toHaveAttribute('href', '/11/assets/new')
    expect(screen.queryByRole('region', { name: 'Getting started with assets' })).toBeNull()
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Assets' })).toHaveAttribute('aria-current', 'page')

    expect(await within(table).findByText('There is 1 deactivated item.')).toBeInTheDocument()
    await userEvent.click(within(table).getByRole('link', { name: 'Show deactivated items.' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('active')).toBe('false'))
    expect(await within(screen.getByRole('table', { name: 'Assets' })).findByText('Sold')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Hide deactivated items.' })).toHaveAttribute('href', '/11/assets')
  })

  it('introduces assets until the first one is added', async () => {
    server.use(...loaded([], []))
    renderAt('/11/assets')
    expect(await screen.findByRole('region', { name: 'Getting started with assets' })).toBeInTheDocument()
    const table = screen.getByRole('table', { name: 'Assets' })
    expect(within(table).getByRole('link', { name: 'Add asset' })).toHaveAttribute('href', '/11/assets/new')
  })
})

describe('the asset form', () => {
  it('adds an asset with its plan', async () => {
    let sent: unknown
    server.use(
      ...loaded(),
      http.post('*/api/v2/11/assets', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(asset({ id: '705' }), { status: 201 })
      }),
      http.get('*/api/v2/11/assets/705', () => HttpResponse.json(asset({ id: '705' }))),
    )
    renderAt('/11/assets/new')
    const form = await screen.findByRole('form', { name: 'Asset' })
    expect(screen.getByRole('heading', { name: 'Asset' })).toBeInTheDocument()
    const category = within(form).getByRole('combobox', { name: 'Ledger account' })
    await within(category).findByRole('option', { name: 'Company car' })
    expect(within(category).getAllByRole('option').map((o) => o.textContent)).toEqual(['Select an option…', 'Company car'])
    await userEvent.type(within(form).getByRole('textbox', { name: 'Name' }), 'Car')
    await userEvent.selectOptions(category, '600')
    await userEvent.type(within(form).getByRole('textbox', { name: 'Purchase value' }), '20003.60')
    const date = within(form).getByRole('textbox', { name: 'Commissioning date' })
    await userEvent.clear(date)
    await userEvent.type(date, '2025-01-14')
    const lifespan = within(form).getByRole('combobox', { name: 'Lifespan' })
    expect(lifespan).toHaveValue('5')
    await userEvent.selectOptions(lifespan, 'other')
    const years = within(form).getByRole('spinbutton', { name: 'Lifespan in years' })
    await userEvent.clear(years)
    await userEvent.type(years, '6')
    await userEvent.type(within(form).getByRole('textbox', { name: 'Residual value' }), '5000')
    expect(within(form).getByRole('region', { name: 'Calculation example' })).toHaveTextContent(
      'Starting in January 2025, we will depreciate €15,003.60 over 6 years, with a monthly depreciation of €208.38. In January 2031, the value is €5,000.00. '
      + 'The depreciation in the first and last months is calculated pro rata by the number of days. In the first month it is €120.99, and in the last month it is €87.63.',
    )
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/assets/705'))
    expect(sent).toEqual({ asset: {
      name: 'Car', ledger_account_id: '600', purchase_date: '2025-01-14', purchase_value: '20003.60',
      value_change_plan_attributes: { lifespan_in_years: 6, residual_value: '5000' },
    } })
  })

  it('changes only the name once the asset has depreciations', async () => {
    let sent: unknown
    server.use(
      ...loaded(),
      http.patch('*/api/v2/11/assets/701', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(asset({ name: 'Laptop 2' }))
      }),
    )
    renderAt('/11/assets/701/edit')
    const form = await screen.findByRole('form', { name: 'Asset' })
    expect(screen.getByRole('heading', { name: 'Edit asset' })).toBeInTheDocument()
    expect(within(form).getByRole('note')).toHaveTextContent('Once an asset has depreciations, only its name can be changed.')
    expect(within(form).getByRole('combobox', { name: 'Ledger account' })).toBeDisabled()
    expect(within(form).getByRole('textbox', { name: 'Purchase value' })).toBeDisabled()
    expect(within(form).getByRole('combobox', { name: 'Lifespan' })).toHaveValue('other')
    const name = within(form).getByRole('textbox', { name: 'Name' })
    await userEvent.clear(name)
    await userEvent.type(name, 'Laptop 2')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ asset: { name: 'Laptop 2' } }))
  })
})

const car = asset({
  name: 'Car', purchase_date: '2025-01-14', purchase_value: '20003.6', calculated_purchase_value: '20003.6', current_value: '19465.85',
  value_change_plan: { lifespan_in_years: 6, residual_value: '5000.0', active: true, monthly_depreciation: '208.38' },
  value_changes: [
    { type: 'linear', date: '2025-01-31', amount: '-120.99', description: null, externally_booked: false, asset_id: '701' },
    { type: 'linear', date: '2025-02-28', amount: '-208.38', description: null, externally_booked: false, asset_id: '701' },
    { type: 'linear', date: `${new Date().getFullYear()}-01-31`, amount: '-208.38', description: null, externally_booked: false, asset_id: '701' },
  ],
  sources: [{ id: '81', asset_id: '701', detail_id: '91', booking: {
    detail_id: '91', document_type: 'PurchaseInvoice', document_id: '61', reference: 'INV-1', date: '2025-01-14', description: 'Car', amount: '20003.6',
  } }] as Asset['sources'],
  notes: [{ id: '5', note: 'Keys in the drawer', todo: false, user_id: '1', created_at: '2026-04-02T10:00:00Z', entity_type: 'Asset', entity_id: '701' }],
  events: [
    { action: 'asset_created', created_at: '2026-04-02T09:00:00Z', user_id: '1', data: {}, entity_type: 'Asset', entity_id: '701' },
    { action: 'asset_source_created', created_at: '2026-04-02T09:01:00Z', user_id: '1', data: { kind: 'detail' }, entity_type: 'Asset', entity_id: '701' },
  ],
})

describe('an asset', () => {
  it('shows its values, its depreciations by year and its side panel as Moneybird does', async () => {
    server.use(http.get('*/api/v2/11/assets/701', () => HttpResponse.json(car)), ...loaded())
    renderAt('/11/assets/701')
    expect(await screen.findByRole('heading', { name: 'Car' })).toBeInTheDocument()
    const values = screen.getByRole('region', { name: 'Values' })
    expect(values).toHaveTextContent('Purchase value€20,003.60')
    expect(values).toHaveTextContent('Current value€19,465.85')
    expect(values).toHaveTextContent('Residual value€5,000.00')
    expect(values).toHaveTextContent('Monthly depreciation€208.38')
    expect(screen.getByRole('link', { name: 'Edit' })).toHaveAttribute('href', '/11/assets/701/edit')
    expect(screen.getByRole('link', { name: 'Journal entries' })).toHaveAttribute('href', '/11/report/journal_entries?ledger_account_id=600')

    const table = screen.getByRole('table', { name: 'Value changes' })
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Date', 'Depreciation type', 'Depreciation', 'Book value'])
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent(`${new Date().getFullYear()}-01-31Monthly€208.38€19,465.85`)
    expect(rows[2]).toHaveTextContent('2025-12-31€329.37€19,674.23')
    expect(within(table).queryByText('2025-01-31')).toBeNull()
    await userEvent.click(within(table).getByRole('button', { name: 'Show the depreciations of 2025' }))
    expect(within(table).getByText('2025-01-31').closest('tr')).toHaveTextContent('€120.99€19,882.61')

    const side = screen.getByRole('complementary', { name: 'Asset details' })
    expect(side).toHaveTextContent('Commissioning date2025-01-14')
    expect(side).toHaveTextContent('In use6 years')
    expect(within(side).getByText('Company car')).toBeInTheDocument()
    expect(within(side).getByText('Invoice detail linked to asset')).toBeInTheDocument()
    expect(within(side).getByText('Keys in the drawer')).toBeInTheDocument()
  })

  it('links and unlinks the documents it was bought with', async () => {
    let linked: unknown
    let unlinked = false
    server.use(
      http.get('*/api/v2/11/assets/701', () => HttpResponse.json(car)),
      ...loaded(),
      http.get('*/api/v2/11/assets/701/linkable_bookings', () => HttpResponse.json([
        { general_journal_document_entry_id: '95', document_type: 'GeneralJournalDocument', document_id: '62', date: '2025-02-01', description: 'Winter tyres', amount: '800.0' },
      ])),
      http.post('*/api/v2/11/assets/701/sources', async ({ request }) => {
        linked = await request.json()
        return HttpResponse.json({ id: '82', asset_id: '701', general_journal_document_entry_id: '95' }, { status: 201 })
      }),
      http.delete('*/api/v2/11/assets/701/sources/81', () => {
        unlinked = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/assets/701')
    const side = await screen.findByRole('complementary', { name: 'Asset details' })
    await userEvent.click(within(side).getByRole('button', { name: 'Linked documents' }))
    const list = within(side).getByRole('list', { name: 'Linked documents' })
    expect(within(list).getByRole('link', { name: 'Car' })).toHaveAttribute('href', '/11/documents/61')
    await userEvent.click(within(list).getByRole('button', { name: 'Unlink' }))
    await waitFor(() => expect(unlinked).toBe(true))

    await userEvent.click(within(side).getByRole('button', { name: 'Link document' }))
    await userEvent.click(await within(side).findByRole('button', { name: /Winter tyres/ }))
    await waitFor(() => expect(linked).toEqual({ general_journal_document_entry_id: '95' }))
  })

  it('leads to its actions and its edit form', async () => {
    server.use(...loaded())
    renderAt('/11/assets/701')
    expect(await screen.findByRole('link', { name: 'Actions' })).toHaveAttribute('href', '/11/assets/701/value_changes/new')
    expect(screen.getByRole('link', { name: 'Edit' })).toHaveAttribute('href', '/11/assets/701/edit')
  })
})

describe('the actions of an asset', () => {
  it('lists Moneybird\'s actions and deletes after asking', async () => {
    let deleted = false
    server.use(...loaded(), http.delete('*/api/v2/11/assets/701', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/assets/701/value_changes/new')
    const list = await screen.findByRole('list', { name: 'Actions' })
    const links = within(list).getAllByRole('link')
    expect(links.map((l) => l.getAttribute('href'))).toEqual([
      '/11/assets/701/value_changes/arbitrary/new', '/11/assets/701/value_changes/full_depreciation/new', '/11/assets/701/value_changes/divestment/new',
    ])
    expect(links[1]).toHaveTextContent('Take out of service')
    await userEvent.click(within(list).getByRole('button', { name: /Delete/ }))
    const dialog = screen.getByRole('dialog', { name: 'Are you sure you want to delete this asset? This cannot be undone.' })
    await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(deleted).toBe(true))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/assets'))
  })

  it('books an arbitrary depreciation as a lower value', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/11/assets/701/value_changes/arbitrary', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ type: 'arbitrary' }, { status: 201 })
    }))
    renderAt('/11/assets/701/value_changes/arbitrary/new')
    const form = await screen.findByRole('form', { name: 'Arbitrary depreciation' })
    expect(within(form).getByRole('link', { name: 'Back' })).toHaveAttribute('href', '/11/assets/701/value_changes/new')
    await userEvent.type(within(form).getByRole('textbox', { name: 'Description' }), 'VAMIL')
    await userEvent.type(within(form).getByRole('textbox', { name: 'Depreciation' }), '250')
    const date = within(form).getByRole('textbox', { name: 'Date' })
    await userEvent.clear(date)
    await userEvent.type(date, '2026-07-15')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ date: '2026-07-15', description: 'VAMIL', amount: '-250' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/assets/701'))
  })

  it('takes an asset out of service with a reason', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/11/assets/701/value_changes/full_depreciation', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ type: 'full_depreciation' }, { status: 201 })
    }))
    renderAt('/11/assets/701/value_changes/full_depreciation/new')
    const form = await screen.findByRole('form', { name: 'Take out of service' })
    await userEvent.type(within(form).getByRole('textbox', { name: 'Description' }), 'Stolen')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toMatchObject({ description: 'Stolen' }))
  })

  it('divests and makes the draft sales invoice of the sale', async () => {
    const calls: string[] = []
    let invoice: { sales_invoice?: { contact_id?: string; details_attributes?: { ledger_account_id?: string; price?: string; tax_rate_id?: string }[] } } = {}
    let category: unknown
    server.use(
      ...loaded(),
      http.get('*/api/v2/11/contacts', () => HttpResponse.json([{ id: '31', company_name: 'Beispiel GmbH' }])),
      http.get('*/api/v2/11/tax_rates', () => HttpResponse.json([
        { id: '2101', name: '21% VAT', percentage: '21.0', tax_rate_type: 'sales_invoice', active: true },
        { id: '901', name: '9% VAT', percentage: '9.0', tax_rate_type: 'sales_invoice', active: true },
      ])),
      http.post('*/api/v2/11/assets/701/value_changes/divestment', () => {
        calls.push('divest')
        return HttpResponse.json({ type: 'divestment' }, { status: 201 })
      }),
      http.post('*/api/v2/11/ledger_accounts', async ({ request }) => {
        category = await request.json()
        return HttpResponse.json({ id: '650', name: 'Book result company car' }, { status: 201 })
      }),
      http.post('*/api/v2/11/sales_invoices', async ({ request }) => {
        calls.push('invoice')
        invoice = await request.json() as typeof invoice
        return HttpResponse.json({ id: '77' }, { status: 201 })
      }),
    )
    renderAt('/11/assets/701/value_changes/divestment/new')
    const form = await screen.findByRole('form', { name: 'Divestment' })
    expect(within(form).getByRole('switch', { name: 'Create sales invoice' })).toBeChecked()
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    expect(await within(form).findByText("Contact can't be blank")).toBeInTheDocument()
    expect(calls).toEqual([])

    await userEvent.click(within(form).getByRole('combobox', { name: 'Contact' }))
    await userEvent.click(await screen.findByRole('option', { name: /Beispiel GmbH/ }))
    const amount = within(form).getByRole('textbox', { name: 'Amount' })
    await userEvent.clear(amount)
    await userEvent.type(amount, '900')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/sales_invoices/77'))
    expect(calls).toEqual(['divest', 'invoice'])
    expect(category).toEqual({ ledger_account: { name: 'Book result company car', account_type: 'other_income_expenses', allowed_document_types: ['sales_invoice'] } })
    expect(invoice.sales_invoice?.contact_id).toBe('31')
    expect(invoice.sales_invoice?.details_attributes?.[0]).toMatchObject({ ledger_account_id: '650', price: '900', tax_rate_id: '2101' })
  })
})
