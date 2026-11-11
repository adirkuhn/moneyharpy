import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { Product } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const vat21: TaxRate = {
  id: '2101', administration_id: '11', name: '21% btw', percentage: '21', tax_rate_type: 'sales_invoice', show_tax: true, active: true,
  country: null, created_at: stamp, updated_at: stamp, uri: '/11/tax_rates/2101', tax_report_boxes: ['NL/1a'], position: 1,
}
const vat9: TaxRate = { ...vat21, id: '2102', name: '9% btw', percentage: '9', uri: '/11/tax_rates/2102', position: 2 }
const purchase21: TaxRate = { ...vat21, id: '2201', name: '21% btw inkoop', tax_rate_type: 'purchase_invoice', uri: '/11/tax_rates/2201' }

const sales: LedgerAccount = {
  id: '8001', administration_id: '11', name: 'Sales', account_type: 'revenue', account_id: '8000', parent_id: null, active: true,
  allowed_document_types: ['sales_invoice'], financial_account_id: null, created_at: stamp, updated_at: stamp,
  taxonomy_item: null, uri: '/11/ledger_accounts/8001', description: null, system_role: null, template: null,
}

const product = (over: Partial<Product> = {}): Product => ({
  id: '501', administration_id: '11', title: 'Website hosting', description: 'Hosting for one website', identifier: 'HOST-1', price: '50.5',
  currency: 'EUR', frequency: 1, frequency_type: 'month', tax_rate_id: '2101', ledger_account_id: '8001', active: true,
  uri: '/11/products/501', created_at: stamp, updated_at: stamp,
  ...over,
})

const consult = product({ id: '502', title: null, description: 'Consultancy hour', identifier: null, price: '95.0', frequency: null, frequency_type: null, uri: '/11/products/502' })

function loaded(list: Product[] = [product(), consult], seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/products', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json(list, { headers: { 'X-Total-Count': String(list.length) } })
    }),
    http.get('*/api/v2/11/products/501', () => HttpResponse.json(product())),
    http.get('*/api/v2/11/tax_rates', () => HttpResponse.json([vat21, vat9, purchase21])),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales])),
  ]
}

describe('the product list', () => {
  it('lists products with their price and frequency, searches and shows deactivated ones on request', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/products')
    const table = await screen.findByRole('table', { name: 'Products' })
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('Website hosting')
    expect(rows[1]).toHaveTextContent('HOST-1')
    expect(rows[1]).toHaveTextContent('Every month')
    expect(rows[1]).toHaveTextContent('€50.50')
    expect(rows[1]).toHaveTextContent('Active')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Website hosting' })).toHaveAttribute('href', '/11/products/501')
    expect(rows[2]).toHaveTextContent('Consultancy hour')
    expect(rows[2]).toHaveTextContent('€95.00')
    expect(seen.at(-1)?.searchParams.get('active')).toBeNull()
    expect(seen.at(-1)?.searchParams.get('per_page')).toBe('25')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Overview', current: 'page' })).toHaveAttribute('href', '/11/products')

    await userEvent.type(screen.getByRole('searchbox', { name: 'Search products' }), 'host')
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('query')).toBe('host'))

    await userEvent.click(screen.getByRole('button', { name: 'Active products' }))
    await userEvent.click(within(screen.getByRole('search', { name: 'Product filters' })).getByRole('link', { name: 'All products' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('active')).toBe('false'))
    expect(screen.getByRole('button', { name: 'All products' })).toBeInTheDocument()
  })

  it('says when there are none yet', async () => {
    server.use(...loaded([]))
    renderAt('/11/products')
    expect(await screen.findByText(/No products yet/)).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Sell the same things faster' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add product' })).toHaveAttribute('href', '/11/products/new')
  })
})

describe('a product', () => {
  it('is added with a sales VAT rate and a category', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/products', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(product(), { status: 201 })
    }))
    renderAt('/11/products/new')
    const form = await screen.findByRole('form', { name: 'Product' })
    expect(screen.getByRole('heading', { name: 'New product' })).toBeInTheDocument()
    const rate = within(form).getByLabelText('VAT rate')
    expect(within(rate).queryByRole('option', { name: '21% btw inkoop' })).toBeNull()
    expect(rate).toHaveValue('2101')
    await userEvent.type(within(form).getByLabelText('Title'), 'Website hosting')
    await userEvent.type(within(form).getByLabelText('Price'), '50.5')
    await userEvent.selectOptions(rate, '9% btw')
    await userEvent.selectOptions(within(form).getByLabelText('Category'), '8000 Sales')
    await userEvent.selectOptions(within(form).getByLabelText('Frequency'), 'Monthly')
    await userEvent.type(within(form).getByLabelText('Every'), '3')
    await userEvent.click(within(form).getByRole('button', { name: 'Save product' }))
    await waitFor(() => expect(sent).toEqual({
      product: {
        title: 'Website hosting', description: '', identifier: '', price: '50.5', currency: 'EUR', tax_rate_id: '2102', ledger_account_id: '8001',
        frequency_type: 'month', frequency: 3,
      },
    }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/products/501'))
  })

  it('shows the problems the server finds beside their fields', async () => {
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/products', () => HttpResponse.json({ error: { price: ['is required'], ledger_account_id: ['is required'] } }, { status: 422 })))
    renderAt('/11/products/new')
    const form = await screen.findByRole('form', { name: 'Product' })
    await userEvent.type(within(form).getByLabelText('Title'), 'Website hosting')
    await userEvent.click(within(form).getByRole('button', { name: 'Save product' }))
    expect(await within(form).findByText('Price is required')).toBeInTheDocument()
    expect(within(form).getByText('Category is required')).toBeInTheDocument()
  })

  it('shows its details and changes its price', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/products/501', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(product({ price: '60.0', updated_at: '2026-11-11T09:00:00Z' }))
    }))
    renderAt('/11/products/501')
    expect(await screen.findByRole('heading', { name: 'Website hosting' })).toBeInTheDocument()
    const details = screen.getByRole('region', { name: 'Product details' })
    expect(within(details).getByText('Price', { selector: 'dt' }).nextSibling).toHaveTextContent('€50.50')
    expect(within(details).getByText('VAT rate', { selector: 'dt' }).nextSibling).toHaveTextContent('21% btw')
    expect(within(details).getByText('Category', { selector: 'dt' }).nextSibling).toHaveTextContent('Sales')

    const form = screen.getByRole('form', { name: 'Product' })
    expect(within(form).getByLabelText('Price')).toHaveValue('50.5')
    await userEvent.clear(within(form).getByLabelText('Price'))
    await userEvent.type(within(form).getByLabelText('Price'), '60')
    await userEvent.click(within(form).getByRole('button', { name: 'Save product' }))
    await waitFor(() => expect(sent).toEqual({
      product: {
        title: 'Website hosting', description: 'Hosting for one website', identifier: 'HOST-1', price: '60', currency: 'EUR',
        tax_rate_id: '2101', ledger_account_id: '8001', frequency_type: 'month', frequency: 1,
      },
    }))
    await waitFor(() => expect(within(details).getByText('Price', { selector: 'dt' }).nextSibling).toHaveTextContent('€60.00'))
  })

  it('is deleted and the list shows again', async () => {
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/products/501', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/products/501')
    await chooseMore('Delete')
    await userEvent.click(screen.getByRole('button', { name: 'Delete product' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/products$/))
  })

  it('is deactivated instead when invoices use it', async () => {
    let current = product()
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/products/501', () => HttpResponse.json(current)),
      http.delete('*/api/v2/11/products/501', () => {
        current = product({ active: false, updated_at: '2026-11-11T09:00:00Z' })
        return new HttpResponse(null, { status: 204, headers: { 'X-Kept-As': 'deactivated' } })
      }),
    )
    renderAt('/11/products/501')
    await chooseMore('Delete')
    await userEvent.click(screen.getByRole('button', { name: 'Delete product' }))
    expect(await screen.findByText('Invoices use this product, so it was deactivated instead of deleted.')).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent('/11/products/501')
    await waitFor(() => expect(within(screen.getByRole('region', { name: 'Product details' })).getByText('Deactivated')).toBeInTheDocument())
  })

  it('is deactivated and reactivated from the menu', async () => {
    const sent: unknown[] = []
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/products/501', async ({ request }) => {
      const body = (await request.json()) as { product: { active: boolean } }
      sent.push(body)
      return HttpResponse.json(product({ active: body.product.active, updated_at: `2026-11-11T09:00:0${sent.length}Z` }))
    }))
    renderAt('/11/products/501')
    await chooseMore('Deactivate')
    expect(await screen.findByText('This product is deactivated and is not offered for new invoices.')).toBeInTheDocument()
    await chooseMore('Reactivate')
    await waitFor(() => expect(screen.queryByText('This product is deactivated and is not offered for new invoices.')).toBeNull())
    expect(sent).toEqual([{ product: { active: false } }, { product: { active: true } }])
  })
})
