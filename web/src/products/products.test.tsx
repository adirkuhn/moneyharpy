import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { Product } from './queries'

const written: unknown[][] = []
vi.mock('write-excel-file/browser', () => ({
  default: (rows: unknown, options: object) => ({
    toFile: (fileName: string) => {
      written.push([rows, { ...options, fileName }])
      return Promise.resolve()
    },
  }),
}))

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

const uncategorized: LedgerAccount = { ...sales, id: '8999', name: 'Uncategorized income', account_id: '8999', system_role: 'uncategorized_income', uri: '/11/ledger_accounts/8999' }

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
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([sales, uncategorized])),
  ]
}

describe('the product list', () => {
  it('lists products with their amount, VAT rate and category, as Moneybird, and searches', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/products')
    const table = await screen.findByRole('table', { name: 'Products' })
    expect(screen.getByRole('heading', { name: 'Products', level: 1 })).toBeInTheDocument()
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Title', 'Amount', 'VAT rate', 'Category'])
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent(/^Website hosting€50\.5021% btwSales$/)
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'Website hosting' })).toHaveAttribute('href', '/11/products/501')
    expect(rows[2]).toHaveTextContent('Consultancy hour')
    expect(seen.at(-1)?.searchParams.get('per_page')).toBe('25')
    // With products the introduction waits behind its button.
    expect(screen.queryByRole('region', { name: 'Getting started with products' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'More information' }))
    expect(screen.getByRole('region', { name: 'Getting started with products' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add' })).toHaveAttribute('href', '/11/products/new')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Overview', current: 'page' })).toHaveAttribute('href', '/11/products')

    await userEvent.type(screen.getByRole('searchbox', { name: 'Search for products' }), 'host')
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('query')).toBe('host'))
  })

  it('sorts by amount', async () => {
    server.use(...loaded())
    renderAt('/11/products')
    const table = await screen.findByRole('table', { name: 'Products' })
    await userEvent.click(within(table).getByRole('button', { name: 'Amount' }))
    expect(within(table).getAllByRole('row')[1]).toHaveTextContent('Website hosting')
    expect(within(table).getByRole('columnheader', { name: /Amount/ })).toHaveAttribute('aria-sort', 'ascending')
    await userEvent.click(within(table).getByRole('button', { name: 'Amount' }))
    expect(within(table).getAllByRole('row')[1]).toHaveTextContent('Consultancy hour')
    expect(within(table).getByRole('columnheader', { name: /Amount/ })).toHaveAttribute('aria-sort', 'descending')
  })

  it('starts with the introduction and an empty table to add the first product', async () => {
    server.use(...loaded([]))
    renderAt('/11/products')
    expect(await screen.findByRole('region', { name: 'Getting started with products' })).toBeInTheDocument()
    const table = screen.getByRole('table', { name: 'Products' })
    expect(within(table).getByRole('link', { name: 'Add product' })).toHaveAttribute('href', '/11/products/new')
    expect(screen.queryByRole('searchbox', { name: 'Search for products' })).toBeNull()
  })

  it('exports all products to Excel', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/products')
    await screen.findByRole('table', { name: 'Products' })
    await userEvent.click(screen.getByRole('button', { name: 'Export products' }))
    await waitFor(() => expect(written).toHaveLength(1))
    expect(seen.at(-1)?.searchParams.get('per_page')).toBe('100')
    const [rows, options] = (written[0] ?? []) as [unknown[][] | undefined, object | undefined]
    expect(rows?.[0]).toEqual(['Title', 'Reference', 'Description', 'Amount', 'Currency', 'VAT rate', 'Category', 'Frequency'].map((value) => ({ value, fontWeight: 'bold' })))
    expect(rows?.[1]).toEqual([
      { value: 'Website hosting' }, { value: 'HOST-1' }, { value: 'Hosting for one website' }, { value: 50.5, type: Number, format: '#,##0.00' },
      { value: 'EUR' }, { value: '21% btw' }, { value: 'Sales' }, { value: 'Every month' },
    ])
    expect(options).toMatchObject({ fileName: 'Products.xlsx' })
  })
})

describe('a product', () => {
  it('is added with Moneybird\'s form: reference, amount, VAT rate, category, period and image', async () => {
    let sent: unknown
    const uploads: string[] = []
    server.use(...loaded())
    server.use(
      http.post('*/api/v2/11/products', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(product(), { status: 201 })
      }),
      http.put('*/api/v2/11/products/501/image', ({ request }) => {
        uploads.push(request.headers.get('content-type')?.split(';')[0] ?? '')
        return HttpResponse.json(product({ image: { filename: 'mug.png', content_type: 'image/png', size: 10 } }))
      }),
    )
    renderAt('/11/products/new')
    const form = await screen.findByRole('form', { name: 'Product' })
    expect(screen.getByRole('heading', { name: 'New product' })).toBeInTheDocument()
    expect(within(form).getByRole('checkbox', { name: 'Use above description on the checkout page' })).toBeChecked()
    expect(within(form).getByRole('checkbox', { name: 'Use smart tax selection' })).not.toBeChecked()
    const rate = within(form).getByRole('combobox', { name: 'VAT rate' })
    expect(within(rate).queryByRole('option', { name: '21% btw inkoop' })).toBeNull()
    expect(rate).toHaveValue('2101')
    expect(within(form).getByRole('combobox', { name: 'Category' })).toHaveValue('8999')
    expect(within(form).getByRole('combobox', { name: 'Frequency' })).toHaveValue('')
    expect(within(form).getByText('EUR')).toBeInTheDocument()
    expect(within(form).queryByRole('combobox', { name: 'Currency' })).toBeNull()

    await userEvent.upload(within(form).getByLabelText('Product image'), new File(['png'], 'mug.png', { type: 'image/png' }))
    expect(within(form).getByText('mug.png')).toBeInTheDocument()
    await userEvent.type(within(form).getByLabelText('Title'), 'Website hosting')
    await userEvent.type(within(form).getByLabelText('Reference'), 'HOST-1')
    await userEvent.type(within(form).getByLabelText('Amount'), '50.5')
    await userEvent.click(within(form).getByRole('button', { name: 'Change currency' }))
    await userEvent.selectOptions(within(form).getByRole('combobox', { name: 'Currency' }), 'USD')
    await userEvent.selectOptions(rate, '9% btw')
    await userEvent.selectOptions(within(form).getByRole('combobox', { name: 'Category' }), 'Sales')
    await userEvent.selectOptions(within(form).getByRole('combobox', { name: 'Frequency' }), 'One quarter')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({
      product: {
        title: 'Website hosting', identifier: 'HOST-1', description: '', checkout_description: '', price: '50.5', currency: 'USD',
        tax_rate_id: '2102', product_type: '', vat_rate_type: '', ledger_account_id: '8001', frequency_type: 'quarter', frequency: 1,
      },
    }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/products/501'))
    expect(uploads).toEqual(['multipart/form-data'])
  })

  it('chooses its VAT by smart tax selection and has its own checkout text', async () => {
    let sent: { product: Record<string, unknown> } | undefined
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/products', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(product(), { status: 201 })
    }))
    renderAt('/11/products/new')
    const form = await screen.findByRole('form', { name: 'Product' })
    await userEvent.type(within(form).getByLabelText('Title'), 'E-book')
    await userEvent.type(within(form).getByLabelText('Amount'), '9')
    await userEvent.click(within(form).getByRole('checkbox', { name: 'Use above description on the checkout page' }))
    await userEvent.type(within(form).getByLabelText('Checkout description'), 'A long read')
    await userEvent.click(within(form).getByRole('checkbox', { name: 'Use smart tax selection' }))
    expect(within(form).queryByRole('combobox', { name: 'VAT rate' })).toBeNull()
    await userEvent.click(within(within(form).getByRole('radiogroup', { name: 'Product type' })).getByRole('radio', { name: 'Digital service' }))
    await userEvent.selectOptions(within(form).getByRole('combobox', { name: 'VAT rate type' }), 'Reduced rate (9% in NL)')
    await userEvent.click(within(form).getByRole('button', { name: 'Show smart tax rate preview' }))
    expect(within(form).getByRole('table', { name: 'Smart tax rate preview' })).toBeInTheDocument()
    await userEvent.selectOptions(within(form).getByRole('combobox', { name: 'Frequency' }), 'Other')
    await userEvent.clear(within(form).getByLabelText('Every'))
    await userEvent.type(within(form).getByLabelText('Every'), '2')
    await userEvent.selectOptions(within(form).getByRole('combobox', { name: 'Period' }), 'Weeks')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent?.product).toMatchObject({
      checkout_description: 'A long read', tax_rate_id: '', product_type: 'digital_service', vat_rate_type: 'reduced', frequency_type: 'week', frequency: 2,
    }))
  })

  it('shows the problems the server finds beside their fields', async () => {
    server.use(...loaded())
    server.use(http.post('*/api/v2/11/products', () => HttpResponse.json({ error: { price: ['is required'] } }, { status: 422 })))
    renderAt('/11/products/new')
    const form = await screen.findByRole('form', { name: 'Product' })
    await userEvent.type(within(form).getByLabelText('Title'), 'Website hosting')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    expect(await within(form).findByText('Amount is required')).toBeInTheDocument()
  })

  it('shows its details as Moneybird, and is edited on its own page', async () => {
    let sent: unknown
    server.use(...loaded())
    server.use(http.patch('*/api/v2/11/products/501', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(product({ price: '60.0', updated_at: '2026-11-11T09:00:00Z' }))
    }))
    renderAt('/11/products/501')
    expect(await screen.findByRole('heading', { name: 'Website hosting', level: 1 })).toBeInTheDocument()
    const details = screen.getByRole('region', { name: 'Product details' })
    const value = (term: string) => within(details).getByText(term, { selector: 'dt' }).nextSibling
    expect(value('Title')).toHaveTextContent('Website hosting')
    expect(value('Description')).toHaveTextContent('Hosting for one website')
    expect(value('Amount')).toHaveTextContent('€50.50')
    expect(value('VAT rate')).toHaveTextContent('21% btw')
    expect(value('Category')).toHaveTextContent('Sales')
    expect(value('Frequency')).toHaveTextContent('Every month')
    expect(value('Image')).toHaveTextContent('-')
    expect(screen.getByRole('link', { name: 'To Products' })).toHaveAttribute('href', '/11/products')
    expect(screen.getByRole('link', { name: 'Create payment link' })).toHaveAttribute('href', '/11/payment_links/501/edit?return_to=%2F11%2Fproducts%2F501')

    await userEvent.click(screen.getByRole('link', { name: 'Edit' }))
    const form = await screen.findByRole('form', { name: 'Product' })
    expect(screen.getByRole('heading', { name: 'Edit product' })).toBeInTheDocument()
    expect(within(form).getByLabelText('Amount')).toHaveValue('50.5')
    expect(within(form).getByRole('combobox', { name: 'Frequency' })).toHaveValue('1:month')
    await userEvent.clear(within(form).getByLabelText('Amount'))
    await userEvent.type(within(form).getByLabelText('Amount'), '60')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toMatchObject({ product: { price: '60', identifier: 'HOST-1', frequency_type: 'month', frequency: 1 } }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/products\/501$/))
  })

  it('shows the smart tax choice in its details', async () => {
    server.use(http.get('*/api/v2/11/products/501', () => HttpResponse.json(product({ tax_rate_id: null, product_type: 'service', vat_rate_type: 'reduced', frequency: null, frequency_type: null }))), ...loaded())
    renderAt('/11/products/501')
    const details = await screen.findByRole('region', { name: 'Product details' })
    const value = (term: string) => within(details).getByText(term, { selector: 'dt' }).nextSibling
    expect(value('Product type')).toHaveTextContent('Service')
    expect(value('Tax rate type')).toHaveTextContent('Reduced rate (9% in NL)')
    expect(value('Frequency')).toHaveTextContent('No period selected')
  })

  it('is removed and the list shows again', async () => {
    server.use(...loaded())
    server.use(http.delete('*/api/v2/11/products/501', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/products/501')
    await chooseMore('Remove product')
    await userEvent.click(screen.getByRole('button', { name: 'Remove product' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/products$/))
  })

  it('is kept, deactivated, when invoices use it', async () => {
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
    await chooseMore('Remove product')
    await userEvent.click(screen.getByRole('button', { name: 'Remove product' }))
    expect(await screen.findByText('Invoices use this product, so it was deactivated instead of removed.')).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent('/11/products/501')
  })
})
