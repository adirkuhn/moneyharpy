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
  disposal: null, sources: [], reinvestment_reserve_purchase: null,
  value_changes: [{ type: 'linear', date: '2026-06-30', amount: '-100.0', description: 'Linear depreciation', externally_booked: false, asset_id: '701' }],
  ...over,
})

const sold = asset({
  id: '702', name: 'Old van', current_value: '0.0', uri: '/11/assets/702',
  disposal: { date: '2026-10-01', reason: 'sold', externally_booked: false, asset_id: '702', reinvestment_reserve_sale: null },
})

function loaded(seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/assets', ({ request }) => {
      const url = new URL(request.url)
      seen.push(url)
      const list = url.searchParams.get('active') === 'false' ? [asset(), sold] : [asset()]
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/assets/701', () => HttpResponse.json(asset())),
  ]
}

describe('the asset list', () => {
  it('lists active assets with their values, and all of them on request', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/assets')
    const table = await screen.findByRole('table', { name: 'Assets' })
    const rows = within(table).getAllByRole('row')
    expect(rows).toHaveLength(2)
    expect(rows[1]).toHaveTextContent('Laptop')
    expect(rows[1]).toHaveTextContent('Depreciating')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Laptop' })).toHaveAttribute('href', '/11/assets/701')
    expect(seen.at(-1)?.searchParams.get('active')).toBe('true')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Assets' })).toHaveAttribute('aria-current', 'page')

    await userEvent.click(screen.getByRole('button', { name: 'Active assets' }))
    await userEvent.click(within(screen.getByRole('search', { name: 'Filters' })).getByRole('link', { name: 'All assets' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('active')).toBe('false'))
    expect(await screen.findByText('Disposed of')).toBeInTheDocument()
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
