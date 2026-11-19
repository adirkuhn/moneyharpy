import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { chooseMore } from '../test/menu'
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

describe('an asset', () => {
  it('shows its plan and value changes, and is deleted', async () => {
    let deleted = false
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/assets/701', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/assets/701')
    expect(await screen.findByRole('heading', { name: 'Laptop' })).toBeInTheDocument()
    expect(screen.getByText('Monthly depreciation')).toBeInTheDocument()
    const changes = screen.getByRole('table', { name: 'Value changes' })
    expect(within(changes).getAllByRole('row')[1]).toHaveTextContent('Depreciation')

    await chooseMore('Delete')
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(deleted).toBe(true))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/assets'))
  })
})
